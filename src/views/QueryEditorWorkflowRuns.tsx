import React from 'react';
import { Input } from '@grafana/ui';
import { EditorField, EditorRow } from '@grafana/plugin-ui';
import { RightColumnWidth } from './QueryEditor';
import type { WorkflowRunsOptions } from 'types/query';

interface Props extends WorkflowRunsOptions {
  onChange: (value: WorkflowRunsOptions) => void;
}

export const QueryEditorWorkflowRuns = (props: Props) => (
  <EditorRow>
    <EditorField label="Workflow" tooltip="The workflow id number or file name (e.g my-workflow.yml)">
      <Input
        key={props.workflow ?? ''}
        defaultValue={props.workflow ?? ''}
        width={RightColumnWidth}
        onBlur={(el) => props.onChange({ ...props, workflow: el.currentTarget.value })}
      />
    </EditorField>
    <EditorField label="Branch" tooltip="The branch to filter on (can be left empty)">
      <Input
        key={props.branch ?? ''}
        defaultValue={props.branch ?? ''}
        width={RightColumnWidth}
        onBlur={(el) => props.onChange({ ...props, branch: el.currentTarget.value })}
      />
    </EditorField>
    <EditorField
      label="Result / status"
      tooltip="GitHub status or conclusion, for example failure, cancelled or in_progress. Empty includes all."
    >
      <Input
        key={props.status ?? ''}
        defaultValue={props.status ?? ''}
        onBlur={(el) => props.onChange({ ...props, status: el.currentTarget.value })}
      />
    </EditorField>
    <EditorField
      label="Maximum runs"
      tooltip="Bounded to 1–1000 runs; defaults to 100. A warning identifies truncated lists."
    >
      <Input
        key={props.maxRuns ?? 100}
        type="number"
        min={1}
        max={1000}
        step={1}
        defaultValue={props.maxRuns ?? 100}
        onBlur={(el) => {
          if (el.currentTarget.checkValidity()) {
            props.onChange({
              ...props,
              maxRuns: el.currentTarget.value === '' ? undefined : el.currentTarget.valueAsNumber,
            });
          } else {
            el.currentTarget.reportValidity();
          }
        }}
      />
    </EditorField>
  </EditorRow>
);
