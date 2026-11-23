package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"go.yaml.in/yaml/v3"
)

type governanceDetailRow struct {
	check    string
	status   string
	evidence string
	source   string
}
type governanceDetailResult struct {
	rows []governanceDetailRow
}

func (r governanceDetailResult) Frames() data.Frames {
	f := data.NewFrame("repository_governance_details",
		data.NewField("check", nil, []string{}),
		data.NewField("status", nil, []string{}),
		data.NewField("evidence", nil, []string{}),
		data.NewField("source", nil, []string{}),
	)
	for _, row := range r.rows {
		f.AppendRow(row.check, row.status, row.evidence, row.source)
	}
	f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return data.Frames{f}
}

type governanceDeclaration struct {
	value  any
	source string
}
type governanceDetailReader struct {
	d         *Datasource
	ctx       context.Context
	calls     int
	stopped   bool
	revisions map[string]string
	rows      []governanceDetailRow
	active    map[string]bool
}

func (r *governanceDetailReader) problem(file, status string, err error, source string) {
	evidence := file
	if err != nil {
		evidence += " · " + err.Error()
	}
	r.rows = append(r.rows, governanceDetailRow{check: "Configuration source", status: status, evidence: evidence, source: source})
}
func (r *governanceDetailReader) read(owner, repo, file string, depth int) (map[string]governanceDeclaration, bool) {
	values := map[string]governanceDeclaration{}
	if r.ctx.Err() != nil {
		return values, false
	}
	if !validActionPath(file) || depth > 4 || r.calls >= 16 || r.stopped {
		r.problem(repo+":"+file, "Not resolved", fmt.Errorf("path, depth or API budget unavailable"), "")
		return values, false
	}
	key := strings.ToLower(owner+"/"+repo) + ":" + file
	if r.active[key] {
		r.problem(repo+":"+file, "Not resolved", fmt.Errorf("inheritance cycle"), "")
		return values, false
	}
	r.active[key] = true
	defer delete(r.active, key)
	revision := r.revisions[strings.ToLower(owner+"/"+repo)]
	if revision == "" {
		if r.calls+3 > 16 {
			r.problem(repo+":"+file, "Not resolved", fmt.Errorf("API request limit reached"), "")
			return values, false
		}
		r.calls += 2
		var err error
		revision, err = r.d.client.ResolveActionInventoryRevision(r.ctx, owner, repo, "")
		if err != nil {
			r.stopped = actionRateLimited(err)
			r.problem(repo+":"+file, "Unavailable", err, "")
			return values, false
		}
		r.revisions[strings.ToLower(owner+"/"+repo)] = revision
	}
	r.calls++
	source := r.d.client.ActionRepositoryURL(owner, repo, revision, file)
	content, found, err := r.d.client.ActionInventoryFile(r.ctx, owner, repo, file, revision)
	if err != nil {
		r.stopped = actionRateLimited(err)
		r.problem(repo+":"+file, "Unavailable", err, source)
		return values, false
	}
	if !found {
		r.problem(repo+":"+file, "Not returned", fmt.Errorf("file not returned by the API; verify path and Contents access"), source)
		return values, false
	}
	var document struct {
		Extends    any            `yaml:"_extends"`
		Properties map[string]any `yaml:"custom_properties"`
	}
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		r.problem(repo+":"+file, "Invalid YAML", err, source)
		return values, false
	}
	references := []string{}
	complete := true
	switch value := document.Extends.(type) {
	case nil:
	case string:
		if strings.TrimSpace(value) != "" {
			references = append(references, value)
		}
	case []any:
		for _, item := range value {
			if ref, ok := item.(string); ok && strings.TrimSpace(ref) != "" {
				references = append(references, ref)
			} else {
				complete = false
			}
		}
	default:
		complete = false
	}
	if !complete {
		r.problem(repo+":"+file, "Not resolved", fmt.Errorf("unsupported _extends declaration"), source)
	}
	status := "Read at source commit"
	if document.Extends == nil {
		status = "Self-contained configuration"
	}
	r.rows = append(r.rows, governanceDetailRow{check: "Configuration source", status: status, evidence: owner + "/" + repo + ":" + file + " @ " + revision, source: source})
	for _, ref := range references {
		childOwner, childRepo, childFile := owner, repo, ref
		if parts := strings.SplitN(ref, ":", 2); len(parts) == 2 {
			target := strings.Split(parts[0], "/")
			childFile = parts[1]
			if len(target) == 1 {
				childRepo = target[0]
			} else if len(target) == 2 {
				childOwner, childRepo = target[0], target[1]
			} else {
				complete = false
				r.problem(ref, "Not resolved", fmt.Errorf("unsupported inheritance reference"), source)
				continue
			}
		}
		if !governanceOwner.MatchString(childOwner) || (childRepo == "" || childRepo == "." || childRepo == "..") || strings.ContainsAny(childRepo, "/\\@ \t\r\n") {
			complete = false
			r.problem(ref, "Not resolved", fmt.Errorf("invalid inheritance repository"), source)
			continue
		}
		inherited, ok := r.read(childOwner, childRepo, childFile, depth+1)
		complete = complete && ok
		for name, value := range inherited {
			values[name] = value
		}
	}
	for name, value := range document.Properties {
		values[name] = governanceDeclaration{value: value, source: source}
	}
	return values, complete
}

// Canonical property values keep multi-select ordering out of comparisons.
// GitHub reports booleans as strings while YAML declarations can use booleans.
func governancePropertyValue(value any) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "null", true
	case string:
		return "scalar:" + v, true
	case bool:
		return "scalar:" + fmt.Sprint(v), true
	case []any:
		values := make([]string, len(v))
		for i, entry := range v {
			text, ok := entry.(string)
			if !ok {
				return "", false
			}
			values[i] = text
		}
		return governancePropertyValue(values)
	case []string:
		values := append([]string{}, v...)
		sort.Strings(values)
		encoded, err := json.Marshal(values)
		return "list:" + string(encoded), err == nil
	default:
		return "", false
	}
}

func (d *Datasource) repositoryGovernanceDetail(ctx context.Context, owner string, repo *gh.Repository) (governanceDetailResult, error) {
	reader := governanceDetailReader{d: d, ctx: ctx, revisions: map[string]string{}, active: map[string]bool{}}
	declared, complete := reader.read(owner, repo.GetName(), governanceSettingsPath, 0)
	if err := ctx.Err(); err != nil {
		return governanceDetailResult{}, err
	}
	ownership, key := governanceOwnership(repo.CustomProperties["ownership"])
	evidence := "Reported ownership: " + ownership
	if key == governanceUnassigned {
		evidence += " · Assign a responsible ownership"
	}
	result := governanceDetailResult{rows: []governanceDetailRow{{check: "Repository", status: "Selected", evidence: repo.GetFullName(), source: repo.GetHTMLURL()}, {check: "Responsibility", status: "Reported metadata", evidence: evidence, source: repo.GetHTMLURL()}}}
	result.rows = append(result.rows, reader.rows...)
	keys := make([]string, 0, len(declared))
	for name := range declared {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		target := declared[name]
		current, present := repo.CustomProperties[name]
		status := "Not reported"
		if !complete {
			status = "Inheritance incomplete"
		} else if present {
			observedValue, observedOK := governancePropertyValue(current)
			declaredValue, declaredOK := governancePropertyValue(target.value)
			switch {
			case !declaredOK:
				status = "Unsupported declaration"
			case !observedOK:
				status = "Not comparable"
			case observedValue == declaredValue:
				status = "Matches declaration"
			default:
				status = "Different from declaration"
			}
		}
		observed := "Not reported"
		if present {
			observed = fmt.Sprint(current)
		}
		result.rows = append(result.rows, governanceDetailRow{check: "Property · " + name, status: status, evidence: "Reported: " + observed + " · Declared: " + fmt.Sprint(target.value), source: target.source})
	}
	if len(declared) == 0 {
		result.rows = append(result.rows, governanceDetailRow{check: "Property comparison", status: "No comparison", evidence: "No resolved custom-property declarations are available. File presence does not establish enforcement."})
	}
	result.rows = append(result.rows, governanceDetailRow{check: "Enforcement", status: "Not checked", evidence: "Only reported repository metadata and settings declarations are compared. Rulesets, permissions and application reconciliation are not assessed."})
	return result, nil
}
