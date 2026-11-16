package githubclient

import (
	"context"
	"errors"
	"fmt"
	gh "github.com/google/go-github/v84/github"
	"github.com/grafana/github-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
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

// Exercise real REST failures at every workflow endpoint before the SDK classifies them.
func TestWorkflowResultsSanitizeRESTErrors(t *testing.T) {
	endpoints := []struct {
		name   string
		path   string
		prefix string
		query  func(*testing.T, *Client) error
	}{
		{
			name: "repository runs", path: "/repos/acme/demo/actions/runs", prefix: "listing workflow runs:",
			query: func(t *testing.T, client *Client) error {
				rows, limited, err := client.GetWorkflowRunResults(context.Background(), models.WorkflowRunsOptions{Owner: "acme", Repository: "demo"}, backend.TimeRange{})
				assert.Nil(t, rows)
				assert.False(t, limited)
				return err
			},
		},
		{
			name: "workflow ID runs", path: "/repos/acme/demo/actions/workflows/7/runs", prefix: "listing workflow runs:",
			query: func(t *testing.T, client *Client) error {
				rows, limited, err := client.GetWorkflowRunResults(context.Background(), models.WorkflowRunsOptions{Owner: "acme", Repository: "demo", Workflow: "7"}, backend.TimeRange{})
				assert.Nil(t, rows)
				assert.False(t, limited)
				return err
			},
		},
		{
			name: "workflow file runs", path: "/repos/acme/demo/actions/workflows/build.yml/runs", prefix: "listing workflow runs:",
			query: func(t *testing.T, client *Client) error {
				rows, limited, err := client.GetWorkflowRunResults(context.Background(), models.WorkflowRunsOptions{Owner: "acme", Repository: "demo", Workflow: "build.yml"}, backend.TimeRange{})
				assert.Nil(t, rows)
				assert.False(t, limited)
				return err
			},
		},
		{
			name: "single job", path: "/repos/acme/demo/actions/jobs/42", prefix: "getting workflow job:",
			query: func(t *testing.T, client *Client) error {
				job, err := client.GetWorkflowJobResult(context.Background(), "acme", "demo", 42)
				assert.Nil(t, job)
				return err
			},
		},
		{
			name: "latest jobs", path: "/repos/acme/demo/actions/runs/42/jobs", prefix: "listing workflow jobs:",
			query: func(t *testing.T, client *Client) error {
				rows, limited, err := client.GetWorkflowJobResults(context.Background(), "acme", "demo", 42, 0)
				assert.Nil(t, rows)
				assert.False(t, limited)
				return err
			},
		},
		{
			name: "attempt jobs", path: "/repos/acme/demo/actions/runs/42/attempts/2/jobs", prefix: "listing workflow jobs:",
			query: func(t *testing.T, client *Client) error {
				rows, limited, err := client.GetWorkflowJobResults(context.Background(), "acme", "demo", 42, 2)
				assert.Nil(t, rows)
				assert.False(t, limited)
				return err
			},
		},
	}
	failures := []struct {
		name    string
		status  int
		message string
		headers map[string]string
	}{
		{name: "missing resource", status: http.StatusNotFound, message: "Not Found"},
		{name: "expired token", status: http.StatusUnauthorized, message: "Bad credentials"},
		{name: "denied access", status: http.StatusForbidden, message: "Resource not accessible by integration"},
		{name: "primary rate limit", status: http.StatusForbidden, message: "API rate limit exceeded", headers: map[string]string{
			"X-RateLimit-Limit": "60", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
		}},
		{name: "secondary rate limit", status: http.StatusTooManyRequests, message: "You have exceeded a secondary rate limit", headers: map[string]string{"Retry-After": "60"}},
	}
	for _, endpoint := range endpoints {
		for _, failure := range failures {
			t.Run(endpoint.name+"/"+failure.name, func(t *testing.T) {
				t.Parallel()
				client := workflowHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, endpoint.path, r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					for name, value := range failure.headers {
						w.Header().Set(name, value)
					}
					w.WriteHeader(failure.status)
					fmt.Fprintf(w, `{"message":%q}`, failure.message)
				})
				err := endpoint.query(t, client)
				require.Error(t, err)
				assert.Contains(t, err.Error(), endpoint.prefix)
				assert.Contains(t, err.Error(), failure.message)
				assert.True(t, backend.IsDownstreamError(err), "GitHub failures must retain their downstream source")

				// These typed-nil targets reproduce the SDK's guessErrorStatus calls.
				var connErr *url.Error
				var netErr *net.OpError
				require.NotPanics(t, func() {
					errors.Is(err, connErr)
					errors.Is(err, netErr)
				}, "REST errors must be sanitized before reaching SDK classification")

				var responseErr *gh.ErrorResponse
				var rateErr *gh.RateLimitError
				var abuseErr *gh.AbuseRateLimitError
				assert.False(t, errors.As(err, &responseErr))
				assert.False(t, errors.As(err, &rateErr))
				assert.False(t, errors.As(err, &abuseErr))
			})
		}
	}
}
