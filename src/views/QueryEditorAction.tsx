import React from 'react';
import { Combobox, Input, Switch } from '@grafana/ui';
import { EditorField, EditorRow } from '@grafana/plugin-ui';
import type { ActionInventoryOptions, WorkflowFailuresOptions, WorkflowRunOptions } from 'types/query';

export function QueryEditorTextOption(props: {
  label: string;
  tooltip: string;
  value?: string;
  onChange: (value: string) => void;
}) {
  // Keep draft edits in the input until blur. A changed query value remounts the
  // input, so restored queries cannot display a stale draft from another query.
  return (
    <EditorField label={props.label} tooltip={props.tooltip}>
      <Input
        key={props.value ?? ''}
        defaultValue={props.value ?? ''}
        onBlur={(e) => props.onChange(e.currentTarget.value)}
      />
    </EditorField>
  );
}

export function QueryEditorWorkflowRun(props: {
  options: WorkflowRunOptions;
  attemptsOnly?: boolean;
  onChange: (value: WorkflowRunOptions) => void;
}) {
  const { options, onChange } = props;
  return (
    <EditorRow>
      <QueryEditorTextOption
        label="Run ID"
        tooltip="Exact GitHub workflow run ID."
        value={options.runId}
        onChange={(runId) => onChange({ ...options, runId })}
      />
      {!props.attemptsOnly && (
        <QueryEditorTextOption
          label="Attempt"
          tooltip="Exact attempt number. Empty selects the latest attempt."
          value={options.attempt}
          onChange={(attempt) => onChange({ ...options, attempt })}
        />
      )}
    </EditorRow>
  );
}

export function QueryEditorWorkflowFailures(props: {
  options: WorkflowFailuresOptions;
  onChange: (value: WorkflowFailuresOptions) => void;
}) {
  const { options, onChange } = props;
  return (
    <EditorRow>
      <QueryEditorTextOption
        label="Repositories"
        tooltip="Comma-separated repository names or a multi-value variable; at most 10 within one organization."
        value={options.repositories?.join(',')}
        onChange={(v) =>
          onChange({
            ...options,
            repositories: v
              .split(',')
              .map((x) => x.trim())
              .filter(Boolean),
          })
        }
      />
      <QueryEditorTextOption
        label="Branch"
        tooltip="Optional branch filter."
        value={options.branch}
        onChange={(branch) => onChange({ ...options, branch })}
      />
      <EditorField
        label="Maximum failures per repository"
        tooltip="1–200 retrieved failed or timed-out runs, default 100. Coverage identifies incomplete results."
      >
        <Input
          key={options.maxRuns ?? 100}
          type="number"
          min={1}
          max={200}
          defaultValue={options.maxRuns ?? 100}
          onBlur={(e) => {
            if (e.currentTarget.checkValidity()) {
              onChange({
                ...options,
                maxRuns: e.currentTarget.value === '' ? undefined : e.currentTarget.valueAsNumber,
              });
            } else {
              e.currentTarget.reportValidity();
            }
          }}
        />
      </EditorField>
      <EditorField label="View">
        <Combobox
          value={options.view || 'failures'}
          options={[
            { label: 'Failures', value: 'failures' },
            { label: 'Repository coverage', value: 'coverage' },
          ]}
          onChange={(v) => onChange({ ...options, view: v.value })}
        />
      </EditorField>
    </EditorRow>
  );
}

export function QueryEditorActionInventory(props: {
  options: ActionInventoryOptions;
  onChange: (value: ActionInventoryOptions) => void;
}) {
  const { options, onChange } = props;
  return (
    <EditorRow>
      <QueryEditorTextOption
        label="Repositories"
        tooltip="Comma-separated repository names or a multi-value variable; at most five per scan."
        value={options.repositories?.join(',')}
        onChange={(v) =>
          onChange({
            ...options,
            repositories: v
              .split(',')
              .map((x) => x.trim())
              .filter(Boolean),
          })
        }
      />
      <QueryEditorTextOption
        label="Source ref"
        tooltip="Workflow source branch or commit; empty uses each repository's default branch. Action identities retain their full pinned SHA."
        value={options.ref}
        onChange={(ref) => onChange({ ...options, ref })}
      />
      <EditorField
        label="Resolve dependencies"
        tooltip="Read reachable reusable-workflow and composite definitions with explicit depth and request bounds. Missing access remains visible."
      >
        <Switch
          value={options.resolveDependencies || false}
          onChange={(e) => onChange({ ...options, resolveDependencies: e.currentTarget.checked })}
        />
      </EditorField>
      <EditorField label="View">
        <Combobox
          value={options.view || 'references'}
          options={[
            { label: 'Reference locations', value: 'references' },
            { label: 'Action commits', value: 'commits' },
            { label: 'Scan coverage', value: 'coverage' },
          ]}
          onChange={(v) => onChange({ ...options, view: v.value })}
        />
      </EditorField>
    </EditorRow>
  );
}
