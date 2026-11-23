import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryEditorWorkflowRuns } from './QueryEditorWorkflowRuns';

describe('QueryEditorWorkflowRuns', () => {
  it('restores all text options and the limit without committing stale drafts', () => {
    const onChange = jest.fn();
    const { rerender } = render(
      <QueryEditorWorkflowRuns
        workflow="old.yml"
        branch="old-branch"
        status="failure"
        maxRuns={10}
        onChange={onChange}
      />
    );
    for (const value of ['old.yml', 'old-branch', 'failure', '10']) {
      fireEvent.change(screen.getByDisplayValue(value), { target: { value: value === '10' ? '99' : 'stale-draft' } });
    }
    rerender(
      <QueryEditorWorkflowRuns
        workflow="new.yml"
        branch="new-branch"
        status="completed"
        maxRuns={20}
        onChange={onChange}
      />
    );
    for (const value of ['new.yml', 'new-branch', 'completed', '20']) {
      fireEvent.blur(screen.getByDisplayValue(value));
      expect(onChange).toHaveBeenLastCalledWith(
        expect.objectContaining({ workflow: 'new.yml', branch: 'new-branch', status: 'completed', maxRuns: 20 })
      );
    }
  });

  it.each(['0', '1001', '1.5'])('rejects invalid limit %s before running a query', (value) => {
    const onChange = jest.fn();
    render(<QueryEditorWorkflowRuns onChange={onChange} />);
    const input = screen.getByRole('spinbutton') as HTMLInputElement;
    const report = jest.spyOn(input, 'reportValidity');
    fireEvent.change(input, { target: { value } });
    fireEvent.blur(input);
    expect(onChange).not.toHaveBeenCalled();
    expect(report).toHaveBeenCalledTimes(1);
  });

  it.each([1, 1000])('accepts valid limit %s', (value) => {
    const onChange = jest.fn();
    render(<QueryEditorWorkflowRuns onChange={onChange} />);
    const input = screen.getByRole('spinbutton');
    fireEvent.change(input, { target: { value: String(value) } });
    fireEvent.blur(input);
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ maxRuns: value }));
  });

  it('restores the backend default when the limit is cleared', () => {
    const onChange = jest.fn();
    render(<QueryEditorWorkflowRuns maxRuns={20} onChange={onChange} />);
    const input = screen.getByRole('spinbutton');
    fireEvent.change(input, { target: { value: '' } });
    fireEvent.blur(input);
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ maxRuns: undefined }));
  });
});
