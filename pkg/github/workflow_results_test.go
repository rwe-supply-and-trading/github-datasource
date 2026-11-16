package github

import (
	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func workflowResultValue(t *testing.T, frame *data.Frame, name string, row int) any {
	t.Helper()
	for _, field := range frame.Fields {
		if field.Name == name {
			return field.At(row)
		}
	}
	t.Fatalf("missing workflow field %q", name)
	return nil
}

func TestWorkflowJobResultsKeepJobConclusionIndependentOfSteps(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.November, 16, 8, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Second)
	job := &gh.WorkflowJob{ID: ptr(int64(9007199254740993)), RunID: ptr(int64(42)), RunAttempt: ptr(int64(2)), Name: ptr("Build"), Status: ptr("completed"), Conclusion: ptr("success"), StartedAt: &gh.Timestamp{Time: start}, CompletedAt: &gh.Timestamp{Time: end}, Steps: []*gh.TaskStep{{Name: ptr("Optional check"), Conclusion: ptr("failure")}}}
	frame := (workflowJobResults{jobs: []*gh.WorkflowJob{job}}).Frames()[0]
	assert.Equal(t, "9007199254740993", workflowResultValue(t, frame, "job_id", 0), "job identity must not lose precision through a float")
	assert.Equal(t, int64(2), workflowResultValue(t, frame, "run_attempt", 0))
	assert.Equal(t, "success", workflowResultValue(t, frame, "state", 0), "a failed step must not overwrite the GitHub job conclusion")
	assert.Equal(t, "Optional check", workflowResultValue(t, frame, "failed_steps", 0))
	duration := workflowResultValue(t, frame, "duration_seconds", 0)
	require.IsType(t, (*float64)(nil), duration)
	assert.Equal(t, 10.0, *duration.(*float64))
}

func TestWorkflowJobResultsDoNotInventMissingObservations(t *testing.T) {
	t.Parallel()
	job := &gh.WorkflowJob{ID: ptr(int64(1)), Status: ptr("in_progress")}
	frame := (workflowJobResults{jobs: []*gh.WorkflowJob{nil, job}}).Frames()[0]
	assert.Equal(t, 1, frame.Rows())
	assert.Equal(t, "in_progress", workflowResultValue(t, frame, "state", 0))
	assert.Nil(t, workflowResultValue(t, frame, "conclusion", 0))
	assert.Nil(t, workflowResultValue(t, frame, "duration_seconds", 0))
	job.Status = ptr("completed")
	job.Conclusion = ptr("failure")
	frame = (workflowJobResults{jobs: []*gh.WorkflowJob{job}}).Frames()[0]
	assert.Equal(t, "failure", workflowResultValue(t, frame, "state", 0))
	assert.Equal(t, "No failed step reported", workflowResultValue(t, frame, "failed_steps", 0))
	assert.Zero(t, (workflowJobResults{jobs: []*gh.WorkflowJob{job}, steps: true}).Frames()[0].Rows())
}

func TestWorkflowStepResultsRetainReportedOrderAndUnknownDuration(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.November, 16, 8, 0, 0, 0, time.UTC)
	before := start.Add(-time.Second)
	job := &gh.WorkflowJob{ID: ptr(int64(42)), RunID: ptr(int64(21)), RunAttempt: ptr(int64(3)), Steps: []*gh.TaskStep{nil, {Number: ptr(int64(4)), Name: ptr("Compile"), Status: ptr("completed"), Conclusion: ptr("failure"), StartedAt: &gh.Timestamp{Time: start}, CompletedAt: &gh.Timestamp{Time: before}}, {Number: ptr(int64(5)), Name: ptr("Cleanup"), Status: ptr("in_progress")}}}
	frame := (workflowJobResults{jobs: []*gh.WorkflowJob{job}, steps: true}).Frames()[0]
	require.Equal(t, 2, frame.Rows())
	assert.Equal(t, int64(4), workflowResultValue(t, frame, "step_number", 0))
	assert.Equal(t, "failure", workflowResultValue(t, frame, "state", 0))
	assert.Nil(t, workflowResultValue(t, frame, "duration_seconds", 0), "invalid negative elapsed time must remain unknown")
	assert.Equal(t, "in_progress", workflowResultValue(t, frame, "state", 1))
	assert.Nil(t, workflowResultValue(t, frame, "conclusion", 1))
	assert.Nil(t, workflowResultValue(t, frame, "completed_at", 1))
}
