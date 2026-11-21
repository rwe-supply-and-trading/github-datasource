package github

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/grafana/github-datasource/pkg/dfutil"
	"github.com/grafana/github-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"go.yaml.in/yaml/v3"
)

const inventoryMaxRequests = 160
const inventoryMaxReferences = 1000
const inventoryMaxDepth = 4

var fullActionSHA = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

type inventoryStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
}

type inventoryJob struct {
	Uses  string          `yaml:"uses"`
	Steps []inventoryStep `yaml:"steps"`
}

type inventoryDefinition struct {
	Jobs map[string]inventoryJob `yaml:"jobs"`
	Runs struct {
		Using string          `yaml:"using"`
		Steps []inventoryStep `yaml:"steps"`
	} `yaml:"runs"`
}

type actionReference struct {
	repository     string
	workflow       string
	sourceRevision string
	job            string
	step           string
	action         string
	sha            string
	kind           string
	parent         string
	site           string
	definitionURL  string
	resolution     string
	declaredRef    string
	depth          int64
}

type actionInventoryCoverage struct {
	repository     string
	revision       string
	status         string
	detail         string
	workflows      int64
	scanned        int64
	unresolved     int64
	checked        time.Time
	unresolvedKeys map[string]bool
}

type actionInventoryResult struct {
	references []actionReference
	coverage   []actionInventoryCoverage
	view       string
}

type inventoryFile struct {
	content string
	found   bool
	err     error
}

type inventoryScanner struct {
	sourceOwner string
	d           *Datasource
	ctx         context.Context
	requests    int
	files       map[string]inventoryFile
	revisions   map[string]inventoryFile
	references  []actionReference
	resolve     bool
	stop        bool
	coverage    *actionInventoryCoverage
}

func validActionPath(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && p != ".." && !strings.HasPrefix(p, "../") && !strings.Contains(p, "\\") && path.Clean(p) == p
}

func reusableWorkflowPath(p string) bool {
	return strings.HasPrefix(p, ".github/workflows/") && (strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml"))
}

func (s *inventoryScanner) read(owner, repo, file, revision string) inventoryFile {
	key := owner + "/" + repo + "@" + revision + ":" + file
	if r, ok := s.files[key]; ok {
		return r
	}
	if s.stop {
		return inventoryFile{err: fmt.Errorf("remaining definitions not queried after a rate limit")}
	}
	if s.requests >= inventoryMaxRequests {
		return inventoryFile{err: fmt.Errorf("definition request limit reached")}
	}
	s.requests++
	content, found, err := s.d.client.ActionInventoryFile(s.ctx, owner, repo, file, revision)
	if actionRateLimited(err) {
		s.stop = true
	}
	result := inventoryFile{content: content, found: found, err: err}
	s.files[key] = result
	return result
}

func (s *inventoryScanner) dependencyProblem(row int, status string) {
	s.references[row].resolution = status
	s.markUnresolved(s.references[row])
}

func (s *inventoryScanner) reference(owner, repo, file, revision, root, job, step, uses, parent, chain string, depth int, active map[string]bool) {
	if len(s.references) >= inventoryMaxReferences {
		s.coverage.status = "Limited"
		s.coverage.detail = "Reference limit reached"
		return
	}
	row := actionReference{repository: s.coverage.repository, workflow: root, sourceRevision: s.coverage.revision, job: job, step: step, parent: parent, site: chain, depth: int64(depth), definitionURL: s.d.client.ActionRepositoryURL(owner, repo, revision, file)}
	childOwner, childRepo, childPath, childRevision := owner, repo, "", revision
	if strings.HasPrefix(uses, "./") {
		childPath = strings.TrimPrefix(uses, "./")
		// Relative workflow imports use the declaring workflow's commit, but
		// relative Actions use the runner's checkout. A remote definition does
		// not establish which repository/ref was checked out in the workspace.
		if !reusableWorkflowPath(childPath) && (!strings.EqualFold(owner, s.sourceOwner) || !strings.EqualFold(repo, s.coverage.repository) || revision != s.coverage.revision) {
			row.action = uses
			row.kind = "Local action"
			row.resolution = "Checkout context not resolved"
			s.references = append(s.references, row)
			s.markUnresolved(row)
			return
		}
		row.action = owner + "/" + repo + "/" + childPath
		row.sha = revision
		row.declaredRef = revision
		row.kind = "Local action"
		row.resolution = "Source commit"
	} else if strings.HasPrefix(uses, "docker://") {
		row.action = uses
		row.kind = "Container action"
		row.resolution = "Container reference"
		s.references = append(s.references, row)
		return
	} else {
		parts := strings.SplitN(uses, "@", 2)
		if len(parts) != 2 {
			row.action = uses
			row.kind = "Action"
			row.resolution = "Missing commit SHA"
			s.references = append(s.references, row)
			s.markUnresolved(row)
			return
		}
		loc := strings.Split(parts[0], "/")
		row.action = parts[0]
		row.sha = parts[1]
		row.kind = "Action"
		row.resolution = "SHA pinned"
		if len(loc) < 2 || loc[0] == "" || loc[1] == "" {
			row.resolution = "Invalid Action reference"
			s.references = append(s.references, row)
			s.markUnresolved(row)
			return
		}
		childOwner, childRepo, childRevision = loc[0], loc[1], parts[1]
		if len(loc) > 2 {
			childPath = strings.Join(loc[2:], "/")
		}
		row.declaredRef = childRevision
		isWorkflow := reusableWorkflowPath(childPath)
		if isWorkflow {
			row.kind = "Reusable workflow"
		}
		if !fullActionSHA.MatchString(childRevision) {
			row.sha = ""
			if !isWorkflow {
				row.resolution = "Not pinned to a full commit SHA"
				s.references = append(s.references, row)
				s.markUnresolved(row)
				return
			}
			if !s.resolve {
				row.resolution = "Workflow ref not expanded"
				s.references = append(s.references, row)
				return
			}
			key := childOwner + "/" + childRepo + "@" + childRevision
			resolved, ok := s.revisions[key]
			if !ok {
				if s.requests >= inventoryMaxRequests || s.stop {
					resolved.err = fmt.Errorf("revision request budget unavailable")
				} else {
					s.requests++
					resolved.content, resolved.err = s.d.client.ResolveActionInventoryRevision(s.ctx, childOwner, childRepo, childRevision)
					if actionRateLimited(resolved.err) {
						s.stop = true
					}
				}
				s.revisions[key] = resolved
			}
			if resolved.err != nil {
				row.resolution = resolved.err.Error()
				s.references = append(s.references, row)
				s.markUnresolved(row)
				return
			}
			childRevision = resolved.content
			row.resolution = "Workflow ref resolved at scan time"
		}
		childRevision = strings.ToLower(childRevision)
		row.sha = childRevision
	}
	reusable := reusableWorkflowPath(childPath)
	if reusable {
		row.kind = "Reusable workflow"
	}
	if childPath != "" && !validActionPath(childPath) {
		row.resolution = "Invalid definition path"
		s.references = append(s.references, row)
		s.markUnresolved(row)
		return
	}
	index := len(s.references)
	s.references = append(s.references, row)
	if !s.resolve {
		s.references[index].resolution = "Not expanded"
		return
	}
	if depth >= inventoryMaxDepth {
		s.dependencyProblem(index, "Dependency depth limit")
		return
	}
	definitionPath := childPath
	if !reusable {
		definitionPath = path.Join(childPath, "action.yml")
	}
	key := childOwner + "/" + childRepo + "@" + childRevision + ":" + definitionPath
	if active[key] {
		s.dependencyProblem(index, "Dependency cycle")
		return
	}
	content := s.read(childOwner, childRepo, definitionPath, childRevision)
	if !reusable && content.err == nil && !content.found {
		definitionPath = path.Join(childPath, "action.yaml")
		content = s.read(childOwner, childRepo, definitionPath, childRevision)
	}
	if content.err != nil {
		s.dependencyProblem(index, content.err.Error())
		return
	}
	if !content.found {
		s.dependencyProblem(index, "Definition unavailable on this GitHub host")
		return
	}
	var definition inventoryDefinition
	if err := yaml.Unmarshal([]byte(content.content), &definition); err != nil {
		s.dependencyProblem(index, "Invalid definition YAML")
		return
	}
	if !reusable && definition.Runs.Using == "" {
		s.dependencyProblem(index, "Action definition has no runner type")
		return
	}
	if !reusable && definition.Runs.Using != "composite" {
		s.references[index].resolution = "Resolved"
		return
	}
	s.references[index].resolution = "Resolved"
	if row.declaredRef != "" && !fullActionSHA.MatchString(row.declaredRef) {
		s.references[index].resolution = "Resolved workflow ref " + row.declaredRef + " at scan time"
	}
	active[key] = true
	defer delete(active, key)
	s.walk(childOwner, childRepo, definitionPath, childRevision, root, job, uses, chain, depth+1, definition, active)
}

func (s *inventoryScanner) walk(owner, repo, file, revision, root, originJob, parent, chain string, depth int, definition inventoryDefinition, active map[string]bool) {
	if s.ctx.Err() != nil {
		return
	}
	jobs := make([]string, 0, len(definition.Jobs))
	for k := range definition.Jobs {
		jobs = append(jobs, k)
	}
	sort.Strings(jobs)
	for _, key := range jobs {
		job := definition.Jobs[key]
		label := key
		if originJob != "" {
			label = originJob + " / " + key
		}
		if job.Uses != "" {
			s.reference(owner, repo, file, revision, root, label, "", job.Uses, parent, chain+"/job:"+key, depth, active)
		}
		for i, step := range job.Steps {
			if step.Uses != "" {
				s.reference(owner, repo, file, revision, root, label, step.Name, step.Uses, parent, fmt.Sprintf("%s/job:%s/step:%d", chain, key, i+1), depth, active)
			}
		}
	}
	for i, step := range definition.Runs.Steps {
		if step.Uses != "" {
			s.reference(owner, repo, file, revision, root, originJob, step.Name, step.Uses, parent, fmt.Sprintf("%s/composite-step:%d", chain, i+1), depth, active)
		}
	}
}

func (d *Datasource) scanActionInventory(ctx context.Context, owner string, repos []string, ref string, resolve bool) (actionInventoryResult, error) {
	scanner := inventoryScanner{sourceOwner: owner, d: d, ctx: ctx, files: map[string]inventoryFile{}, revisions: map[string]inventoryFile{}, resolve: resolve}
	result := actionInventoryResult{}
	for _, repo := range repos {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		coverage := actionInventoryCoverage{repository: repo, status: "Complete", checked: time.Now().UTC(), unresolvedKeys: map[string]bool{}}
		scanner.coverage = &coverage
		if scanner.stop || scanner.requests+3 > inventoryMaxRequests {
			coverage.status = "Unavailable"
			coverage.detail = "Remaining API budget unavailable"
			result.coverage = append(result.coverage, coverage)
			continue
		}
		scanner.requests += 3 // default-branch lookup, revision lookup and directory listing
		revision, err := d.client.ResolveActionInventoryRevision(ctx, owner, repo, ref)
		if err != nil {
			coverage.status = "Unavailable"
			coverage.detail = err.Error()
			scanner.stop = actionRateLimited(err)
			result.coverage = append(result.coverage, coverage)
			continue
		}
		coverage.revision = revision
		files, err := d.client.ActionInventoryFiles(ctx, owner, repo, revision)
		if err != nil {
			coverage.status = "Unavailable"
			coverage.detail = err.Error()
			scanner.stop = actionRateLimited(err)
			result.coverage = append(result.coverage, coverage)
			continue
		}
		sort.Slice(files, func(i, j int) bool { return files[i].GetPath() < files[j].GetPath() })
		workflows := []string{}
		for _, file := range files {
			if file.GetType() == "file" && (strings.HasSuffix(file.GetName(), ".yml") || strings.HasSuffix(file.GetName(), ".yaml")) {
				workflows = append(workflows, file.GetPath())
			}
		}
		coverage.workflows = int64(len(workflows))
		// Read the root workflow population before expanding dependencies so a
		// large shared workflow cannot consume the source-file scan budget first.
		type rootDefinition struct {
			file       string
			definition inventoryDefinition
		}
		roots := []rootDefinition{}
		for i, file := range workflows {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if i >= 40 || scanner.requests >= inventoryMaxRequests {
				coverage.status = "Limited"
				coverage.detail = "Workflow or definition limit reached"
				break
			}
			content := scanner.read(owner, repo, file, revision)
			if content.err != nil || !content.found {
				coverage.status = "Partial"
				coverage.detail = "One or more workflow files could not be read"
				continue
			}
			var definition inventoryDefinition
			if err := yaml.Unmarshal([]byte(content.content), &definition); err != nil {
				coverage.status = "Partial"
				coverage.detail = "One or more workflow files contain invalid YAML"
				continue
			}
			coverage.scanned++
			roots = append(roots, rootDefinition{file: file, definition: definition})
		}
		for _, root := range roots {
			scanner.walk(owner, repo, root.file, revision, root.file, "", root.file, root.file, 0, root.definition, map[string]bool{owner + "/" + repo + "@" + revision + ":" + root.file: true})
		}
		if coverage.unresolved > 0 && coverage.status == "Complete" {
			coverage.status = "Dependencies partial"
			coverage.detail = "Direct declarations read; some dependency definitions could not be resolved"
		}
		result.coverage = append(result.coverage, coverage)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.references = scanner.references
	return result, nil
}

func (r actionInventoryResult) notices() []data.Notice {
	notices := []data.Notice{}
	for _, c := range r.coverage {
		if c.status != "Complete" {
			notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: c.repository + ": " + c.status + ". " + c.detail})
		}
	}
	return notices
}

func (r actionInventoryResult) Frames() data.Frames {
	if r.view == "coverage" {
		f := data.NewFrame(
			"action_inventory_coverage",
			data.NewField("repository", nil, []string{}),
			data.NewField("source_revision", nil, []string{}),
			data.NewField("coverage", nil, []string{}),
			data.NewField("workflows_found", nil, []int64{}),
			data.NewField("workflows_read", nil, []int64{}),
			data.NewField("unresolved_dependencies", nil, []int64{}),
			data.NewField("checked_at", nil, []time.Time{}),
			data.NewField("detail", nil, []string{}),
		)
		for _, c := range r.coverage {
			f.AppendRow(c.repository, c.revision, c.status, c.workflows, c.scanned, c.unresolved, c.checked, c.detail)
		}
		f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
		return data.Frames{f}
	}
	if r.view == "commits" {
		type group struct {
			action, sha, kind string
			references        int64
			repos, workflows  map[string]bool
		}
		groups := map[string]*group{}
		variants := map[string]map[string]bool{}
		for _, row := range r.references {
			if row.kind == "Reusable workflow" || row.kind == "Container action" || !fullActionSHA.MatchString(row.sha) {
				continue
			}
			identity := actionInventoryIdentity(row.action)
			key := identity + "@" + row.sha
			g := groups[key]
			if g == nil {
				g = &group{action: row.action, sha: row.sha, kind: row.kind, repos: map[string]bool{}, workflows: map[string]bool{}}
				groups[key] = g
			}
			g.references++
			g.repos[row.repository] = true
			g.workflows[row.repository+":"+row.workflow] = true
			if variants[identity] == nil {
				variants[identity] = map[string]bool{}
			}
			variants[identity][row.sha] = true
		}
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		f := data.NewFrame(
			"action_commits",
			data.NewField("action", nil, []string{}),
			data.NewField("action_sha", nil, []string{}),
			data.NewField("kind", nil, []string{}),
			data.NewField("commit_variants", nil, []int64{}),
			data.NewField("reference_sites", nil, []int64{}),
			data.NewField("repositories", nil, []string{}),
			data.NewField("workflow_count", nil, []int64{}),
		)
		for _, key := range keys {
			g := groups[key]
			repos := []string{}
			for repo := range g.repos {
				repos = append(repos, repo)
			}
			sort.Strings(repos)
			f.AppendRow(
				g.action,
				g.sha,
				g.kind,
				int64(len(variants[actionInventoryIdentity(g.action)])),
				g.references,
				strings.Join(repos, ", "),
				int64(len(g.workflows)),
			)
		}
		f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable, Notices: r.notices()}
		return data.Frames{f}
	}
	f := data.NewFrame(
		"action_references",
		data.NewField("repository", nil, []string{}),
		data.NewField("workflow", nil, []string{}),
		data.NewField("job", nil, []string{}),
		data.NewField("step", nil, []string{}),
		data.NewField("action", nil, []string{}),
		data.NewField("action_sha", nil, []string{}),
		data.NewField("kind", nil, []string{}),
		data.NewField("depth", nil, []int64{}),
		data.NewField("used_by", nil, []string{}),
		data.NewField("resolution", nil, []string{}),
		data.NewField("source_revision", nil, []string{}),
		data.NewField("definition_url", nil, []string{}),
		data.NewField("location", nil, []string{}),
		data.NewField("action_sha_short", nil, []string{}),
		data.NewField("declared_ref", nil, []string{}),
		data.NewField("reference_label", nil, []string{}),
	)
	for _, row := range r.references {
		shortSHA := row.sha
		if shortSHA == "" {
			shortSHA = row.declaredRef
		}
		if len(shortSHA) > 12 {
			shortSHA = shortSHA[:12]
		}
		location := row.workflow + " · " + row.job
		if row.step != "" {
			location += " · " + row.step
		}
		label := row.action + "\n" + row.kind
		if row.resolution != "Resolved" {
			label += "\n" + row.resolution
		}
		f.AppendRow(
			row.repository,
			row.workflow,
			row.job,
			row.step,
			row.action,
			row.sha,
			row.kind,
			row.depth,
			row.parent,
			row.resolution,
			row.sourceRevision,
			row.definitionURL,
			location,
			shortSHA,
			row.declaredRef,
			label,
		)
	}
	f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable, Notices: r.notices()}
	return data.Frames{f}
}

func (d *Datasource) HandleActionInventoryQuery(ctx context.Context, q *models.ActionInventoryQuery) (dfutil.Framer, error) {
	if q.Owner == "" {
		return nil, fmt.Errorf("organization is required")
	}
	repos, err := actionRepositories(q.Options.Repositories, q.Repository, 5)
	if err != nil {
		return nil, err
	}
	if q.Options.View != "" && q.Options.View != "references" && q.Options.View != "coverage" && q.Options.View != "commits" {
		return nil, fmt.Errorf("unknown inventory view")
	}
	value, err := d.cachedActionQuery(ctx, []any{"inventory", q.Owner, repos, q.Options.Ref, q.Options.ResolveDependencies}, func() (dfutil.Framer, error) {
		return d.scanActionInventory(ctx, q.Owner, repos, q.Options.Ref, q.Options.ResolveDependencies)
	})
	if err != nil {
		return nil, err
	}
	result := value.(actionInventoryResult)
	result.view = q.Options.View
	return result, nil
}

func (s *QueryHandler) HandleActionInventory(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return &backend.QueryDataResponse{Responses: processQueries(ctx, req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
		model := &models.ActionInventoryQuery{}
		if err := UnmarshalQuery(q.JSON, model); err != nil {
			return *err
		}
		return dfutil.FrameResponseWithError(s.Datasource.HandleActionInventoryQuery(ctx, model))
	})}, nil
}

func (s *inventoryScanner) markUnresolved(row actionReference) {
	key := row.action + "@" + row.sha + "|" + row.declaredRef
	if row.resolution == "Checkout context not resolved" {
		key += "|" + row.definitionURL
	}
	if !s.coverage.unresolvedKeys[key] {
		s.coverage.unresolvedKeys[key] = true
		s.coverage.unresolved++
	}
}

func actionInventoryIdentity(value string) string {
	parts := strings.SplitN(value, "/", 3)
	if len(parts) < 2 {
		return value
	}
	id := strings.ToLower(parts[0] + "/" + parts[1])
	if len(parts) > 2 {
		id += "/" + parts[2]
	}
	return id
}
