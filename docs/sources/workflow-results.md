# Workflow, job and step results

Use the existing GitHub App connection. Its installation needs repository **Actions: read** for selected repositories. No webhook collector or workflow changes are required.

`Workflow_Runs` accepts an optional workflow (empty means all workflows in one repository), branch, GitHub status/conclusion and `maxRuns` (default 100, maximum 1000). The period filters run creation time. Truncated results carry a warning; bounded lists must not be described as complete totals. Existing `Workflow_Usage` is unchanged.

`Workflow_Jobs` requires an exact `runId`; optional `attempt` selects that attempt, otherwise GitHub's latest attempt is used. Pagination is bounded to 1000 jobs, with a visible truncation warning. A `jobId` addresses one exact job. Job frames include actual conclusion, runner name, run attempt and names of reported failed/timed-out steps. They do not infer a job result from the runner state or step result.

`Workflow_Steps` requires an exact `jobId`, with optional `runId` and `attempt` guards. It returns every reported step's number, name, status, conclusion, timestamps and completed duration. No selection yields an empty frame without API calls. Null conclusions and unfinished durations remain unknown. A failed job can have no failed step (for example failure during setup); no step is invented.

Jobs/steps are fetched only for an explicitly selected run/job. The plugin does not scan all repositories or fan out over every run. Existing App authentication, enterprise API hostname and GitHub rate-limit responses remain in use. No job log contents are ingested.
