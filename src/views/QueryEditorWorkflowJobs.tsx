import React from 'react';
import { Switch } from '@grafana/ui';
import { EditorField, EditorRow } from '@grafana/plugin-ui';
import { QueryEditorTextOption } from './QueryEditorAction';
import type { WorkflowJobsOptions } from 'types/query';

interface Props {
  options: WorkflowJobsOptions;
  stepsOnly?: boolean;
  onChange: (value: WorkflowJobsOptions) => void;
}

export const QueryEditorWorkflowJobs = ({ options, stepsOnly, onChange }: Props) => (
  <EditorRow>
    <QueryEditorTextOption
      label="Run ID"
      tooltip="Exact GitHub workflow run ID. Jobs default to the latest attempt; for steps this guards the selected job."
      value={options.runId}
      onChange={(runId) => onChange({ ...options, runId })}
    />
    <QueryEditorTextOption
      label="Attempt"
      tooltip="Optional exact attempt number, or latest."
      value={options.attempt}
      onChange={(attempt) => onChange({ ...options, attempt })}
    />
    <QueryEditorTextOption
      label="Job ID"
      tooltip="Exact GitHub job ID. Required for step results."
      value={options.jobId}
      onChange={(jobId) => onChange({ ...options, jobId })}
    />
    {!stepsOnly && (
      <EditorField
        label="Failures first"
        tooltip="Prefer failed and timed-out jobs when building a job catalogue; explicit job IDs remain exact."
      >
        <Switch
          value={options.failureFirst || false}
          onChange={(e) => onChange({ ...options, failureFirst: e.currentTarget.checked })}
        />
      </EditorField>
    )}
  </EditorRow>
);
