package github

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/github-datasource/pkg/dfutil"
	"github.com/grafana/github-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const governanceSettingsPath = ".github/settings.yaml"
const governanceUnassigned = "__unassigned__"

var governanceOwner = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

type governanceRepository struct {
	repository    *gh.Repository
	ownership     string
	ownershipKey  string
	finding       string
	configuration string
	sourceURL     string
	hasFinding    bool
	unchecked     bool
}

type repositoryGovernanceResult struct {
	repositories []governanceRepository
	ownerships   []string
	notices      []data.Notice
	checked      time.Time
	view         string
	filter       string
}

func governanceOwnership(value any) (string, string) {
	v, ok := value.(string)
	if !ok || strings.TrimSpace(v) == "" || strings.EqualFold(strings.TrimSpace(v), "Unknown") {
		return "Unassigned", governanceUnassigned
	}
	return v, v
}

func (d *Datasource) repositoryGovernanceSnapshot(ctx context.Context, owner string) (repositoryGovernanceResult, error) {
	value, err := d.cachedActionQuery(ctx, []any{"repository-governance", owner}, func() (dfutil.Framer, error) {
		result := repositoryGovernanceResult{checked: time.Now().UTC()}
		warn := func(message string) {
			result.notices = append(result.notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: message})
		}
		var repositories []*gh.Repository
		for page, pages := 1, 0; page != 0 && pages < 20; pages++ {
			rows, response, err := d.client.ListRepositoriesByOrg(ctx, owner, &gh.RepositoryListByOrgOptions{ListOptions: gh.ListOptions{Page: page, PerPage: 100}})
			if err != nil {
				return nil, fmt.Errorf("listing governance repositories: %w", err)
			}
			repositories = append(repositories, rows...)
			page = 0
			if response != nil {
				page = response.NextPage
			}
			if pages == 19 && page != 0 {
				warn("Repository retrieval limited to 2000 entries; counts cover retrieved repositories only.")
			}
		}
		properties, schemaErr := getCustomProperties(ctx, d.client, owner, "ownership")
		if schemaErr != nil {
			warn("Ownership schema unavailable: " + schemaErr.Error())
		}
		for _, property := range properties {
			result.ownerships = append(result.ownerships, property.AllowedValues...)
		}
		search := CodeSearch{}
		var searchErr error
		if actionRateLimited(schemaErr) {
			searchErr = fmt.Errorf("configuration search skipped after a rate limit")
		} else {
			search, searchErr = getCodeSearch(ctx, d.client, "org:"+owner+" filename:settings.yaml path:.github", governanceSettingsPath, false, false)
		}
		if searchErr != nil {
			warn("Configuration search unavailable: " + searchErr.Error())
		} else if !search.Complete {
			warn("Configuration search is incomplete; absence from these results does not establish a missing configuration.")
		}
		found := map[string]string{}
		for _, item := range search.Items {
			if item != nil && item.Repository != nil {
				found[strings.ToLower(item.Repository.GetFullName())] = item.GetHTMLURL()
			}
		}
		for _, repo := range repositories {
			if repo == nil {
				continue
			}
			ownership, key := governanceOwnership(repo.CustomProperties["ownership"])
			row := governanceRepository{repository: repo, ownership: ownership, ownershipKey: key}
			findings := []string{}
			if key == governanceUnassigned {
				findings = append(findings, "Assign ownership")
				row.hasFinding = true
			}
			source, matched := found[strings.ToLower(repo.GetFullName())]
			switch {
			case matched:
				row.configuration = "Indexed file found"
				row.sourceURL = source
			case searchErr != nil:
				row.configuration = "Search unavailable"
				row.unchecked = true
			case !search.Complete:
				row.configuration = "Search incomplete"
				row.unchecked = true
			default:
				row.configuration = "No indexed match"
				row.hasFinding = true
				findings = append(findings, "Verify settings file")
			}
			if row.unchecked {
				findings = append(findings, "Configuration check unavailable")
			}
			if len(findings) == 0 {
				row.finding = "No finding in observed checks"
			} else {
				row.finding = strings.Join(findings, "; ")
			}
			result.repositories = append(result.repositories, row)
		}
		sort.SliceStable(result.repositories, func(i, j int) bool {
			a, b := result.repositories[i], result.repositories[j]
			if (a.ownershipKey == governanceUnassigned) != (b.ownershipKey == governanceUnassigned) {
				return a.ownershipKey == governanceUnassigned
			}
			if a.hasFinding != b.hasFinding {
				return a.hasFinding
			}
			return strings.ToLower(a.repository.GetName()) < strings.ToLower(b.repository.GetName())
		})
		return result, nil
	})
	if err != nil {
		return repositoryGovernanceResult{}, err
	}
	return value.(repositoryGovernanceResult), nil
}

func (r repositoryGovernanceResult) Frames() data.Frames {
	if r.view == "ownership" {
		type count struct {
			name                              string
			repositories, findings, unchecked int64
		}
		groups := map[string]*count{}
		for _, value := range r.ownerships {
			name, key := governanceOwnership(value)
			groups[key] = &count{name: name}
		}
		for _, row := range r.repositories {
			c := groups[row.ownershipKey]
			if c == nil {
				c = &count{name: row.ownership}
				groups[row.ownershipKey] = c
			}
			c.repositories++
			if row.hasFinding {
				c.findings++
			}
			if row.unchecked {
				c.unchecked++
			}
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i] == governanceUnassigned && keys[j] != governanceUnassigned {
				return true
			}
			if keys[j] == governanceUnassigned {
				return false
			}
			return keys[i] < keys[j]
		})
		f := data.NewFrame("repository_ownership",
			data.NewField("ownership", nil, []string{}),
			data.NewField("repositories", nil, []int64{}),
			data.NewField("with_findings", nil, []int64{}),
			data.NewField("unchecked_configuration", nil, []int64{}),
			data.NewField("ownership_key", nil, []string{}),
		)
		for _, key := range keys {
			c := groups[key]
			f.AppendRow(c.name, c.repositories, c.findings, c.unchecked, key)
		}
		f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable, Notices: r.notices}
		return data.Frames{f}
	}
	f := data.NewFrame("repository_governance",
		data.NewField("repository", nil, []string{}),
		data.NewField("ownership", nil, []string{}),
		data.NewField("finding", nil, []string{}),
		data.NewField("configuration", nil, []string{}),
		data.NewField("details", nil, []string{}),
		data.NewField("full_name", nil, []string{}),
		data.NewField("repository_url", nil, []string{}),
		data.NewField("source_url", nil, []string{}),
		data.NewField("ownership_key", nil, []string{}),
		data.NewField("checked_at", nil, []time.Time{}),
	)
	for _, row := range r.repositories {
		if r.filter != "" && r.filter != "*" && row.ownershipKey != r.filter {
			continue
		}
		f.AppendRow(
			row.repository.GetName(),
			row.ownership,
			row.finding,
			row.configuration,
			"Inspect",
			row.repository.GetFullName(),
			row.repository.GetHTMLURL(),
			row.sourceURL,
			row.ownershipKey,
			r.checked,
		)
	}
	f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable, Notices: r.notices}
	return data.Frames{f}
}

func (d *Datasource) HandleRepositoryGovernanceQuery(ctx context.Context, q *models.RepositoryGovernanceQuery) (dfutil.Framer, error) {
	if !governanceOwner.MatchString(q.Owner) {
		return nil, fmt.Errorf("a GitHub organization name is required")
	}
	if q.Options.View != "" && q.Options.View != "ownership" && q.Options.View != "repositories" && q.Options.View != "details" {
		return nil, fmt.Errorf("unknown repository governance view")
	}
	snapshot, err := d.repositoryGovernanceSnapshot(ctx, q.Owner)
	if err != nil {
		return nil, err
	}
	if q.Options.View == "details" {
		if q.Repository == "" {
			return governanceDetailResult{rows: []governanceDetailRow{{check: "Repository", status: "Not selected", evidence: "Choose Inspect in the repository table."}}}, nil
		}
		if _, err := actionRepositories([]string{q.Repository}, "", 1); err != nil {
			return nil, err
		}
		var selected *gh.Repository
		for _, row := range snapshot.repositories {
			if strings.EqualFold(row.repository.GetName(), q.Repository) {
				selected = row.repository
				break
			}
		}
		if selected == nil {
			return nil, fmt.Errorf("selected repository is outside the retrieved organization scope")
		}
		_, selectedOwnership := governanceOwnership(selected.CustomProperties["ownership"])
		if q.Options.Ownership != "" && q.Options.Ownership != "*" && q.Options.Ownership != selectedOwnership {
			return governanceDetailResult{rows: []governanceDetailRow{{check: "Repository", status: "Outside selected ownership", evidence: "Choose Inspect in the currently filtered repository table."}}}, nil
		}
		value, err := d.cachedActionQuery(ctx, []any{"repository-governance-detail", q.Owner, selected.GetName(), selected.CustomProperties}, func() (dfutil.Framer, error) { return d.repositoryGovernanceDetail(ctx, q.Owner, selected) })
		return value, err
	}
	snapshot.view = q.Options.View
	snapshot.filter = q.Options.Ownership
	return snapshot, nil
}

func (s *QueryHandler) HandleRepositoryGovernance(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return &backend.QueryDataResponse{Responses: processQueries(ctx, req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
		model := &models.RepositoryGovernanceQuery{}
		if err := UnmarshalQuery(q.JSON, model); err != nil {
			return *err
		}
		return dfutil.FrameResponseWithError(s.Datasource.HandleRepositoryGovernanceQuery(ctx, model))
	})}, nil
}
