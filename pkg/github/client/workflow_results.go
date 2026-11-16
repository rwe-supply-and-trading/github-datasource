package githubclient

import (
	"context"
	"fmt"
	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/github-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"strconv"
	"time"
)

// GetWorkflowRunResults bounds interactive work to one repository and at most 1000 runs.
func (c *Client) GetWorkflowRunResults(ctx context.Context, opt models.WorkflowRunsOptions, tr backend.TimeRange) ([]*gh.WorkflowRun, bool, error) {
	if opt.Owner == "" || opt.Repository == "" {
		return nil, false, nil
	}
	limit := opt.MaxRuns
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 1000 {
		return nil, false, fmt.Errorf("max runs must be between 1 and 1000")
	}
	out := []*gh.WorkflowRun{}
	for page := 1; page <= 10; page++ {
		if opt.Status == "all" {
			opt.Status = ""
		}
		opts := &gh.ListWorkflowRunsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: 100}, Branch: opt.Branch, Status: opt.Status}
		if !tr.From.IsZero() && !tr.To.IsZero() {
			opts.Created = tr.From.UTC().Format(time.RFC3339) + ".." + tr.To.UTC().Format(time.RFC3339)
		}
		var rows *gh.WorkflowRuns
		var resp *gh.Response
		var err error
		if opt.Workflow == "" {
			rows, resp, err = c.restClient.Actions.ListRepositoryWorkflowRuns(ctx, opt.Owner, opt.Repository, opts)
		} else if id, e := strconv.ParseInt(opt.Workflow, 10, 64); e == nil && id > 0 {
			rows, resp, err = c.restClient.Actions.ListWorkflowRunsByID(ctx, opt.Owner, opt.Repository, id, opts)
		} else {
			rows, resp, err = c.restClient.Actions.ListWorkflowRunsByFileName(ctx, opt.Owner, opt.Repository, opt.Workflow, opts)
		}
		if err != nil {
			return nil, false, addErrorSourceToError(fmt.Errorf("listing workflow runs: %w", err), resp)
		}
		if rows == nil {
			return nil, false, fmt.Errorf("GitHub returned no workflow-run response")
		}
		out = append(out, rows.WorkflowRuns...)
		if len(out) >= limit {
			return out[:limit], len(out) > limit || (resp != nil && resp.NextPage != 0) || rows.GetTotalCount() > len(out), nil
		}
		if resp == nil || resp.NextPage == 0 {
			return out, false, nil
		}
	}
	return out, true, nil
}
func (c *Client) GetWorkflowJobResult(ctx context.Context, owner, repo string, id int64) (*gh.WorkflowJob, error) {
	j, resp, err := c.restClient.Actions.GetWorkflowJobByID(ctx, owner, repo, id)
	if err != nil {
		return nil, addErrorSourceToError(fmt.Errorf("getting workflow job: %w", err), resp)
	}
	if j == nil {
		return nil, fmt.Errorf("GitHub returned no job response")
	}
	return j, nil
}

// Jobs are fetched only after an exact run is selected; no run-to-job fan-out.
func (c *Client) GetWorkflowJobResults(ctx context.Context, owner, repo string, run, attempt int64) ([]*gh.WorkflowJob, bool, error) {
	out := []*gh.WorkflowJob{}
	for page := 1; page <= 10; page++ {
		var rows *gh.Jobs
		var resp *gh.Response
		var err error
		opts := &gh.ListOptions{Page: page, PerPage: 100}
		if attempt > 0 {
			rows, resp, err = c.restClient.Actions.ListWorkflowJobsAttempt(ctx, owner, repo, run, attempt, opts)
		} else {
			rows, resp, err = c.restClient.Actions.ListWorkflowJobs(ctx, owner, repo, run, &gh.ListWorkflowJobsOptions{ListOptions: *opts, Filter: "latest"})
		}
		if err != nil {
			return nil, false, addErrorSourceToError(fmt.Errorf("listing workflow jobs: %w", err), resp)
		}
		if rows == nil {
			return nil, false, fmt.Errorf("GitHub returned no workflow-job response")
		}
		out = append(out, rows.Jobs...)
		if resp == nil || resp.NextPage == 0 {
			return out, false, nil
		}
	}
	return out, true, nil
}
