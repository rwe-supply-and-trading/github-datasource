package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/github-datasource/pkg/dfutil"
	"github.com/grafana/github-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type workflowRunResults struct {
	runs    WorkflowRunsWrapper
	limited bool
}

func (r workflowRunResults) Frames() data.Frames {
	frames := r.runs.Frames()
	if r.limited {
		frames[0].Meta.Notices = append(frames[0].Meta.Notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: "Run limit reached. Narrow the period, workflow or branch; this is a bounded list."})
	}
	return frames
}

type workflowJobResults struct {
	jobs    []*gh.WorkflowJob
	steps   bool
	limited bool
}

func resultDuration(start, end *gh.Timestamp) *float64 {
	if start == nil || end == nil || end.Before(start.Time) {
		return nil
	}
	v := end.Sub(start.Time).Seconds()
	return &v
}
func workflowResultState(status, conclusion string) string {
	if conclusion != "" {
		return conclusion
	}
	return status
}
func failedStepSummary(j *gh.WorkflowJob, names []string) string {
	if len(names) == 0 && (j.GetConclusion() == "failure" || j.GetConclusion() == "timed_out") {
		return "No failed step reported"
	}
	return strings.Join(names, "; ")
}
func (r workflowJobResults) Frames() data.Frames {
	var f *data.Frame
	if r.steps {
		f = data.NewFrame("workflow_steps", data.NewField("job_id", nil, []string{}), data.NewField("job_name", nil, []string{}), data.NewField("run_id", nil, []string{}), data.NewField("run_attempt", nil, []int64{}), data.NewField("step_number", nil, []int64{}), data.NewField("step_name", nil, []string{}), data.NewField("status", nil, []*string{}), data.NewField("conclusion", nil, []*string{}), data.NewField("state", nil, []string{}), data.NewField("started_at", nil, []*time.Time{}), data.NewField("completed_at", nil, []*time.Time{}), data.NewField("duration_seconds", nil, []*float64{}), data.NewField("html_url", nil, []string{}))
	} else {
		f = data.NewFrame("workflow_jobs", data.NewField("job_id", nil, []string{}), data.NewField("job_name", nil, []string{}), data.NewField("run_id", nil, []string{}), data.NewField("run_attempt", nil, []int64{}), data.NewField("workflow_name", nil, []string{}), data.NewField("status", nil, []*string{}), data.NewField("conclusion", nil, []*string{}), data.NewField("state", nil, []string{}), data.NewField("started_at", nil, []*time.Time{}), data.NewField("completed_at", nil, []*time.Time{}), data.NewField("duration_seconds", nil, []*float64{}), data.NewField("runner_name", nil, []string{}), data.NewField("runner_group", nil, []string{}), data.NewField("failed_steps", nil, []string{}), data.NewField("html_url", nil, []string{}))
	}
	for _, j := range r.jobs {
		if j == nil {
			continue
		}
		if r.steps {
			for _, s := range j.Steps {
				if s == nil {
					continue
				}
				f.AppendRow(strconv.FormatInt(j.GetID(), 10), j.GetName(), strconv.FormatInt(j.GetRunID(), 10), j.GetRunAttempt(), s.GetNumber(), s.GetName(), s.Status, s.Conclusion, workflowResultState(s.GetStatus(), s.GetConclusion()), s.StartedAt.GetTime(), s.CompletedAt.GetTime(), resultDuration(s.StartedAt, s.CompletedAt), j.GetHTMLURL())
			}
		} else {
			failed := []string{}
			for _, s := range j.Steps {
				if s != nil && (s.GetConclusion() == "failure" || s.GetConclusion() == "timed_out") {
					failed = append(failed, s.GetName())
				}
			}
			f.AppendRow(strconv.FormatInt(j.GetID(), 10), j.GetName(), strconv.FormatInt(j.GetRunID(), 10), j.GetRunAttempt(), j.GetWorkflowName(), j.Status, j.Conclusion, workflowResultState(j.GetStatus(), j.GetConclusion()), j.StartedAt.GetTime(), j.CompletedAt.GetTime(), resultDuration(j.StartedAt, j.CompletedAt), j.GetRunnerName(), j.GetRunnerGroupName(), failedStepSummary(j, failed), j.GetHTMLURL())
		}
	}
	f.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	if r.limited {
		f.Meta.Notices = append(f.Meta.Notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: "Job list limited to 1000 entries. Select an exact attempt or job; the list is incomplete."})
	}
	return data.Frames{f}
}

func (d *Datasource) HandleWorkflowJobsQuery(ctx context.Context, query *models.WorkflowJobsQuery, req backend.DataQuery, steps bool) (dfutil.Framer, error) {
	result := workflowJobResults{steps: steps}
	if query.Owner == "" || query.Repository == "" {
		return result, nil
	}
	opt := query.Options
	if opt.Attempt == "latest" {
		opt.Attempt = ""
	}
	if opt.JobID != "" {
		id, err := strconv.ParseInt(opt.JobID, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("job ID must be a positive GitHub job ID")
		}
		job, err := d.client.GetWorkflowJobResult(ctx, query.Owner, query.Repository, id)
		if err != nil {
			return nil, err
		}
		if opt.RunID != "" && strconv.FormatInt(job.GetRunID(), 10) != opt.RunID {
			return nil, fmt.Errorf("selected job does not belong to the selected workflow run")
		}
		if opt.Attempt != "" && strconv.FormatInt(job.GetRunAttempt(), 10) != opt.Attempt {
			return nil, fmt.Errorf("selected job does not belong to the selected attempt")
		}
		result.jobs = []*gh.WorkflowJob{job}
		return result, nil
	}
	if opt.RunID == "" || steps {
		return result, nil
	}
	id, err := strconv.ParseInt(opt.RunID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("run ID must be a positive GitHub run ID")
	}
	attempt := int64(0)
	if opt.Attempt != "" {
		attempt, err = strconv.ParseInt(opt.Attempt, 10, 64)
		if err != nil || attempt <= 0 {
			return nil, fmt.Errorf("attempt must be a positive integer")
		}
	}
	result.jobs, result.limited, err = d.client.GetWorkflowJobResults(ctx, query.Owner, query.Repository, id, attempt)
	return result, err
}
func (s *QueryHandler) workflowResults(ctx context.Context, req *backend.QueryDataRequest, steps bool) (*backend.QueryDataResponse, error) {
	return &backend.QueryDataResponse{Responses: processQueries(ctx, req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
		query := &models.WorkflowJobsQuery{}
		if err := UnmarshalQuery(q.JSON, query); err != nil {
			return *err
		}
		return dfutil.FrameResponseWithError(s.Datasource.HandleWorkflowJobsQuery(ctx, query, q, steps))
	})}, nil
}
func (s *QueryHandler) HandleWorkflowJobs(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return s.workflowResults(ctx, req, false)
}
func (s *QueryHandler) HandleWorkflowSteps(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return s.workflowResults(ctx, req, true)
}
