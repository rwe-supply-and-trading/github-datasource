import React from 'react';
import { QueryEditorWebhookDeliveries } from './QueryEditorWebhookDeliveries';
import { render, screen, fireEvent } from '@testing-library/react';
import { components } from '../components/selectors';
import { WebhookDeliveryStatus } from '../constants';

describe('QueryEditorWebhookDeliveries', () => {
  it('renders the hook id, event and status fields', () => {
    render(<QueryEditorWebhookDeliveries onChange={jest.fn()} />);
    expect(screen.getByText('Hook ID')).toBeInTheDocument();
    expect(screen.getByText('Event')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
  });

  it('shows the existing options', () => {
    render(
      <QueryEditorWebhookDeliveries
        hookId="12345"
        event="pull_request"
        status={WebhookDeliveryStatus.Failure}
        onChange={jest.fn()}
      />
    );
    expect(screen.getByLabelText(components.QueryEditor.WebhookDeliveries.hookIdInput)).toHaveValue('12345');
    expect(screen.getByLabelText(components.QueryEditor.WebhookDeliveries.eventInput)).toHaveValue('pull_request');
  });

  it('reports the hook id when the field loses focus', () => {
    const onChange = jest.fn();
    render(<QueryEditorWebhookDeliveries onChange={onChange} />);

    const input = screen.getByLabelText(components.QueryEditor.WebhookDeliveries.hookIdInput);
    fireEvent.change(input, { target: { value: '12345' } });
    fireEvent.blur(input);

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ hookId: '12345' }));
  });

  it('restores changed hook and event options before either input is committed', () => {
    const onChange = jest.fn();
    const { rerender } = render(<QueryEditorWebhookDeliveries hookId="123" event="push" onChange={onChange} />);
    fireEvent.change(screen.getByDisplayValue('123'), { target: { value: '789' } });
    fireEvent.change(screen.getByDisplayValue('push'), { target: { value: 'stale-event' } });
    rerender(<QueryEditorWebhookDeliveries hookId="456" event="pull_request" onChange={onChange} />);
    expect(screen.getByDisplayValue('456')).toBeInTheDocument();
    expect(screen.getByDisplayValue('pull_request')).toBeInTheDocument();
    fireEvent.blur(screen.getByDisplayValue('456'));
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ hookId: '456', event: 'pull_request' }));
    fireEvent.blur(screen.getByDisplayValue('pull_request'));
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ hookId: '456', event: 'pull_request' }));
  });
});
