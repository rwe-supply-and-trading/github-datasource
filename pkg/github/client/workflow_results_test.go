package githubclient

import (
	"context"
	"fmt"
	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/github-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func workflowHTTPClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	rest := gh.NewClient(server.Client())
	base, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	rest.BaseURL = base
	return &Client{restClient: rest}
}

func TestWorkflowRunResultsUseRepositoryEndpointAndReportTruncation(t *testing.T) {
	t.Parallel()
	requests := 0
	client := workflowHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, "/repos/acme/demo/actions/runs", r.URL.Path)
		assert.Empty(t, r.URL.Query().Get("status"), "All must not be sent as a GitHub status")
		assert.Equal(t, "main", r.URL.Query().Get("branch"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"total_count":3,"workflow_runs":[{"id":3},{"id":2},{"id":1}]}`)
	})
	rows, limited, err := client.GetWorkflowRunResults(context.Background(), models.WorkflowRunsOptions{Owner: "acme", Repository: "demo", Status: "all", Branch: "main", MaxRuns: 2}, backend.TimeRange{})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, int64(3), rows[0].GetID())
	assert.True(t, limited)
	assert.Equal(t, 1, requests)
}

func TestWorkflowJobResultsSelectExactAttemptAndPaginateLatest(t *testing.T) {
	for _, attempt := range []int64{0, 2} {
		t.Run(fmt.Sprint(attempt), func(t *testing.T) {
			t.Parallel()
			requests := 0
			client := workflowHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if attempt == 0 {
					assert.Equal(t, "/repos/acme/demo/actions/runs/42/jobs", r.URL.Path)
					assert.Equal(t, "latest", r.URL.Query().Get("filter"))
				} else {
					assert.Equal(t, "/repos/acme/demo/actions/runs/42/attempts/2/jobs", r.URL.Path)
					assert.Empty(t, r.URL.Query().Get("filter"))
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("page") == "1" {
					w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
					fmt.Fprint(w, `{"total_count":2,"jobs":[{"id":10}]}`)
				} else {
					assert.Equal(t, "2", r.URL.Query().Get("page"))
					fmt.Fprint(w, `{"total_count":2,"jobs":[{"id":11}]}`)
				}
			})
			rows, limited, err := client.GetWorkflowJobResults(context.Background(), "acme", "demo", 42, attempt)
			require.NoError(t, err)
			require.Len(t, rows, 2)
			assert.Equal(t, int64(11), rows[1].GetID())
			assert.False(t, limited)
			assert.Equal(t, 2, requests)
		})
	}
}

func TestWorkflowResultsSurfacePermissionErrors(t *testing.T) {
	t.Parallel()
	client := workflowHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
	})
	rows, limited, err := client.GetWorkflowRunResults(context.Background(), models.WorkflowRunsOptions{Owner: "acme", Repository: "demo"}, backend.TimeRange{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.Nil(t, rows)
	assert.False(t, limited)
	job, err := client.GetWorkflowJobResult(context.Background(), "acme", "demo", 42)
	require.Error(t, err)
	assert.Nil(t, job)
}
