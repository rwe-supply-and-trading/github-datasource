package githubclient

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	gh "github.com/google/go-github/v84/github"
)

// GetWorkflowRunResult addresses an exact attempt when supplied. All REST errors
// retain their response for the SDK's error-source classification.
func (c *Client) GetWorkflowRunResult(ctx context.Context, owner, repo string, run int64, attempt string) (*gh.WorkflowRun, error) {
	var result *gh.WorkflowRun
	var resp *gh.Response
	var err error
	if attempt == "" || attempt == "latest" {
		result, resp, err = c.restClient.Actions.GetWorkflowRunByID(ctx, owner, repo, run)
	} else {
		n, e := strconv.Atoi(attempt)
		if e != nil || n < 1 {
			return nil, fmt.Errorf("attempt must be a positive integer")
		}
		result, resp, err = c.restClient.Actions.GetWorkflowRunAttempt(ctx, owner, repo, run, n, nil)
	}
	if err != nil {
		return nil, addErrorSourceToError(fmt.Errorf("getting workflow run: %w", err), resp)
	}
	if result == nil {
		return nil, fmt.Errorf("GitHub returned no workflow-run response")
	}
	return result, nil
}

func (c *Client) ResolveActionInventoryRevision(ctx context.Context, owner, repo, ref string) (string, error) {
	if ref == "" {
		r, resp, err := c.restClient.Repositories.Get(ctx, owner, repo)
		if err != nil {
			return "", addErrorSourceToError(fmt.Errorf("reading inventory repository: %w", err), resp)
		}
		if r == nil || r.GetDefaultBranch() == "" {
			return "", fmt.Errorf("repository has no default branch")
		}
		ref = r.GetDefaultBranch()
	}
	sha, resp, err := c.restClient.Repositories.GetCommitSHA1(ctx, owner, repo, ref, "")
	if err != nil {
		return "", addErrorSourceToError(fmt.Errorf("resolving inventory revision: %w", err), resp)
	}
	if sha == "" {
		return "", fmt.Errorf("repository revision has no commit SHA")
	}
	return sha, nil
}

func (c *Client) ActionInventoryFiles(ctx context.Context, owner, repo, revision string) ([]*gh.RepositoryContent, error) {
	_, files, resp, err := c.restClient.Repositories.GetContents(ctx, owner, repo, ".github/workflows", &gh.RepositoryContentGetOptions{Ref: revision})
	if err != nil {
		// A 404 can mean either a missing directory or unavailable Contents
		// access. Neither establishes a complete scan with zero workflows.
		return nil, addErrorSourceToError(fmt.Errorf("workflow directory unavailable; verify path and Contents read access: %w", err), resp)
	}
	return files, nil
}

// ActionInventoryFile reads only through the configured GitHub API. Download
// URLs from repository contents are deliberately not followed to another host.
func (c *Client) ActionInventoryFile(ctx context.Context, owner, repo, path, revision string) (string, bool, error) {
	file, _, resp, err := c.restClient.Repositories.GetContents(ctx, owner, repo, path, &gh.RepositoryContentGetOptions{Ref: revision})
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == 404 {
			return "", false, nil
		}
		return "", false, addErrorSourceToError(fmt.Errorf("reading Action definition %s: %w", path, err), resp)
	}
	if file == nil {
		return "", false, fmt.Errorf("Action definition is not a file")
	}
	if file.GetSize() > 1024*1024 {
		return "", false, fmt.Errorf("Action definition exceeds the 1 MiB content limit")
	}
	content, err := file.GetContent()
	if err != nil {
		return "", false, fmt.Errorf("decoding Action definition: %w", err)
	}
	if file.GetSize() > 0 && content == "" {
		return "", false, fmt.Errorf("Action definition content is unavailable")
	}
	return strings.TrimPrefix(content, "\ufeff"), true, nil // cspell:ignore ufeff
}

func (c *Client) ActionRepositoryURL(owner, repo, revision, path string) string {
	// Enterprise API URLs are not necessarily the website origin (GHE.com).
	base := strings.TrimSuffix(c.restClient.BaseURL.String(), "/")
	base = strings.TrimSuffix(base, "/api/v3")
	if strings.HasPrefix(base, "https://api.") {
		base = "https://" + strings.TrimPrefix(base, "https://api.")
	}
	if base == "https://api.github.com" || base == "https://github.com" {
		base = "https://github.com"
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	u.Path = "/" + owner + "/" + repo + "/blob/" + revision + "/" + path
	return u.String()
}
