import React from 'react';
import { Combobox } from '@grafana/ui';
import { EditorField, EditorRow } from '@grafana/plugin-ui';
import { QueryEditorTextOption } from './QueryEditorAction';
import type { RepositoryGovernanceOptions } from '../types/query';

export function QueryEditorRepositoryGovernance(props: {
  options: RepositoryGovernanceOptions;
  onChange: (value: RepositoryGovernanceOptions) => void;
}) {
  const { options, onChange } = props;
  return (
    <EditorRow>
      <EditorField
        label="View"
        tooltip="Ownership counts cover the retrieved organization scope. Repository rows may be filtered; details inspect one exact repository."
      >
        <Combobox
          value={options.view || 'repositories'}
          options={[
            { label: 'Ownership counts', value: 'ownership' },
            { label: 'Repositories and findings', value: 'repositories' },
            { label: 'Selected repository details', value: 'details' },
          ]}
          onChange={(value) => onChange({ ...options, view: value.value })}
        />
      </EditorField>
      {(!options.view || options.view === 'repositories') && (
        <QueryEditorTextOption
          label="Ownership"
          tooltip="Exact ownership value, __unassigned__ for unresolved responsibility, or * for all."
          value={options.ownership}
          onChange={(ownership) => onChange({ ...options, ownership })}
        />
      )}
    </EditorRow>
  );
}
