import React, { useState } from 'react';
import { Input } from '@grafana/ui';
import { EditorField, EditorRow } from '@grafana/plugin-ui';
import type { WorkflowJobsOptions } from 'types/query';
interface Props extends WorkflowJobsOptions { onChange: (value: WorkflowJobsOptions) => void; }
export const QueryEditorWorkflowJobs = (props: Props) => {
 const [runId, setRunId] = useState(props.runId || '');
 const [attempt, setAttempt] = useState(props.attempt || '');
 const [jobId, setJobId] = useState(props.jobId || '');
 return <EditorRow>
  <EditorField label="Run ID" tooltip="Exact GitHub workflow run ID. Jobs default to the latest attempt."><Input value={runId} onChange={e => setRunId(e.currentTarget.value)} onBlur={e => props.onChange({...props, runId:e.currentTarget.value})} /></EditorField>
  <EditorField label="Attempt" tooltip="Optional exact attempt number, or latest."><Input value={attempt} onChange={e => setAttempt(e.currentTarget.value)} onBlur={e => props.onChange({...props, attempt:e.currentTarget.value})} /></EditorField>
  <EditorField label="Job ID" tooltip="Exact GitHub job ID. Required for step results."><Input value={jobId} onChange={e => setJobId(e.currentTarget.value)} onBlur={e => props.onChange({...props, jobId:e.currentTarget.value})} /></EditorField>
 </EditorRow>;
};
