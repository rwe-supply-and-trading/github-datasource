package github

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/github-datasource/pkg/dfutil"
	githubclient "github.com/grafana/github-datasource/pkg/github/client"
	"github.com/grafana/github-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type workflowRunResult struct {
	run      *gh.WorkflowRun
	attempts bool
}

func (r workflowRunResult) Frames() data.Frames {
	if !r.attempts {
		return WorkflowRunsWrapper{r.run}.Frames()
	}
	f := data.NewFrame(
		"workflow_attempts",
		data.NewField("attempt", nil, []string{}),
		data.NewField("display_name", nil, []string{}),
	)
	n := int64(r.run.GetRunAttempt())
	lower := int64(1)
	if n > 1000 {
		lower = n - 999
	}
	for i := n; i >= lower; i-- {
		label := strconv.FormatInt(i, 10)
		if i == n {
			label += " (latest)"
		}
		f.AppendRow(strconv.FormatInt(i, 10), label)
	}
	f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	if lower > 1 {
		f.Meta.Notices = []data.Notice{{Severity: data.NoticeSeverityWarning, Text: "Only the latest 1000 attempt choices are listed."}}
	}
	return data.Frames{f}
}

func (d *Datasource) HandleWorkflowRunQuery(ctx context.Context, q *models.WorkflowRunQuery, attempts bool) (dfutil.Framer, error) {
	if q.Owner == "" || q.Repository == "" || q.Options.RunID == "" {
		return actionFrames{}, nil
	}
	id, err := strconv.ParseInt(q.Options.RunID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("run ID must be a positive GitHub run ID")
	}
	attempt := q.Options.Attempt
	if attempts {
		attempt = ""
	}
	if attempt == "latest" {
		attempt = ""
	}
	cached, err := d.cachedActionQuery(ctx, []any{"run", q.Owner, q.Repository, id, attempt}, func() (dfutil.Framer, error) {
		r, err := d.client.GetWorkflowRunResult(ctx, q.Owner, q.Repository, id, attempt)
		if err != nil {
			return nil, err
		}
		return workflowRunResult{run: r}, nil
	})
	if err != nil {
		return nil, err
	}
	result := cached.(workflowRunResult)
	result.attempts = attempts
	return result, nil
}

func (s *QueryHandler) actionRunQuery(ctx context.Context, req *backend.QueryDataRequest, attempts bool) (*backend.QueryDataResponse, error) {
	return &backend.QueryDataResponse{Responses: processQueries(ctx, req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
		model := &models.WorkflowRunQuery{}
		if err := UnmarshalQuery(q.JSON, model); err != nil {
			return *err
		}
		return dfutil.FrameResponseWithError(s.Datasource.HandleWorkflowRunQuery(ctx, model, attempts))
	})}, nil
}

func (s *QueryHandler) HandleWorkflowRun(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return s.actionRunQuery(ctx, req, false)
}

func (s *QueryHandler) HandleWorkflowAttempts(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return s.actionRunQuery(ctx, req, true)
}

func actionRepositories(input []string, fallback string, limit int) ([]string, error) {
	if len(input) == 0 && fallback != "" {
		input = []string{fallback}
	}
	seen := map[string]bool{}
	result := []string{}
	for _, repo := range input {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		if strings.ContainsAny(repo, "/\\\x00") || repo == "." || repo == ".." {
			return nil, fmt.Errorf("select repository names within one organization")
		}
		key := strings.ToLower(repo)
		if !seen[key] {
			seen[key] = true
			result = append(result, repo)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("select at least one repository")
	}
	if len(result) > limit {
		return nil, fmt.Errorf("select at most %d repositories for this live query", limit)
	}
	sort.Strings(result)
	return result, nil
}

func actionRateLimited(err error) bool {
	return githubclient.IsRateLimitError(err)
}

type workflowFailureRepository struct {
	repository string
	runs       []*gh.WorkflowRun
	limited    bool
	err        string
	checked    time.Time
}

type workflowFailuresResult struct {
	repos []workflowFailureRepository
	view  string
}

func (r workflowFailuresResult) Frames() data.Frames {
	if r.view == "coverage" {
		f := data.NewFrame(
			"workflow_failure_coverage",
			data.NewField("repository", nil, []string{}),
			data.NewField("coverage", nil, []string{}),
			data.NewField("returned_failures", nil, []int64{}),
			data.NewField("returned_timeouts", nil, []int64{}),
			data.NewField("checked_at", nil, []time.Time{}),
			data.NewField("detail", nil, []string{}),
		)
		for _, repo := range r.repos {
			status := "Complete"
			if repo.limited {
				status = "Limited"
			}
			if repo.err != "" {
				status = "Unavailable"
			}
			var failures, timeouts int64
			for _, run := range repo.runs {
				if run.GetConclusion() == "timed_out" {
					timeouts++
				} else {
					failures++
				}
			}
			f.AppendRow(repo.repository, status, failures, timeouts, repo.checked, repo.err)
		}
		f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
		return data.Frames{f}
	}
	f := data.NewFrame(
		"workflow_failures",
		data.NewField("repository", nil, []string{}),
		data.NewField("workflow", nil, []string{}),
		data.NewField("display_name", nil, []string{}),
		data.NewField("run_id", nil, []string{}),
		data.NewField("run_number", nil, []int64{}),
		data.NewField("attempt", nil, []int64{}),
		data.NewField("branch", nil, []string{}),
		data.NewField("state", nil, []string{}),
		data.NewField("created_at", nil, []*time.Time{}),
		data.NewField("html_url", nil, []string{}),
	)
	var notices []data.Notice
	for _, repo := range r.repos {
		if repo.err != "" {
			notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: repo.repository + ": " + repo.err})
		}
		if repo.limited {
			notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: repo.repository + ": run limit reached; the list is partial."})
		}
		for _, run := range repo.runs {
			f.AppendRow(
				repo.repository,
				run.GetName(),
				fmt.Sprintf("%s #%d · attempt %d", run.GetName(), run.GetRunNumber(), run.GetRunAttempt()),
				strconv.FormatInt(run.GetID(), 10),
				int64(run.GetRunNumber()),
				int64(run.GetRunAttempt()),
				run.GetHeadBranch(),
				workflowResultState(run.GetStatus(), run.GetConclusion()),
				run.CreatedAt.GetTime(),
				run.GetHTMLURL(),
			)
		}
	}
	f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable, Notices: notices}
	return data.Frames{f}
}

func (d *Datasource) HandleWorkflowFailuresQuery(ctx context.Context, q *models.WorkflowFailuresQuery, req backend.DataQuery) (dfutil.Framer, error) {
	if q.Owner == "" {
		return nil, fmt.Errorf("organization is required")
	}
	repos, err := actionRepositories(q.Options.Repositories, q.Repository, 10)
	if err != nil {
		return nil, err
	}
	limit := q.Options.MaxRuns
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("maximum failures per repository must be between 1 and 200")
	}
	if q.Options.View != "" && q.Options.View != "failures" && q.Options.View != "coverage" {
		return nil, fmt.Errorf("unknown failure view")
	}
	value, err := d.cachedActionQuery(ctx, []any{"failures", q.Owner, repos, limit, q.Options.Branch, req.TimeRange.From.Unix(), req.TimeRange.To.Unix()}, func() (dfutil.Framer, error) {
		result := workflowFailuresResult{}
		stop := false
		for _, repo := range repos {
			row := workflowFailureRepository{repository: repo, checked: time.Now().UTC()}
			if stop {
				row.err = "Not queried after a rate limit"
				result.repos = append(result.repos, row)
				continue
			}
			for _, state := range []string{"failure", "timed_out"} {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				runs, limited, err := d.client.GetWorkflowRunResults(ctx, models.WorkflowRunsOptions{Owner: q.Owner, Repository: repo, Status: state, Branch: q.Options.Branch, MaxRuns: limit}, req.TimeRange)
				if err != nil {
					row.err = err.Error()
					stop = actionRateLimited(err)
					break
				}
				row.limited = row.limited || limited
				row.runs = append(row.runs, runs...)
			}
			sort.SliceStable(row.runs, func(i, j int) bool { return row.runs[i].GetCreatedAt().After(row.runs[j].GetCreatedAt().Time) })
			if len(row.runs) > limit {
				row.runs = row.runs[:limit]
				row.limited = true
			}
			result.repos = append(result.repos, row)
		}
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	result := value.(workflowFailuresResult)
	result.view = q.Options.View
	return result, nil
}

func (s *QueryHandler) HandleWorkflowFailures(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return &backend.QueryDataResponse{Responses: processQueries(ctx, req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
		model := &models.WorkflowFailuresQuery{}
		if err := UnmarshalQuery(q.JSON, model); err != nil {
			return *err
		}
		return dfutil.FrameResponseWithError(s.Datasource.HandleWorkflowFailuresQuery(ctx, model, q))
	})}, nil
}
