import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CsvMappingDialog } from './csv-mapping-dialog';
import type { CsvInspectResponse } from '@/types';

const inspect: CsvInspectResponse = {
  headers: ['Name', 'Status'],
  row_count: 3,
  columns: [
    { name: 'Name', distinct_values: ['Celeste', 'Hades', 'Tunic'], distinct_truncated: false },
    { name: 'Status', distinct_values: ['Beaten', 'Playing'], distinct_truncated: false },
  ],
};

function renderDialog(onImport = vi.fn()) {
  render(
    <CsvMappingDialog
      open
      onOpenChange={vi.fn()}
      inspect={inspect}
      isImporting={false}
      onImport={onImport}
    />,
  );
  return onImport;
}

describe('CsvMappingDialog', () => {
  it('disables Import until a title column is chosen', () => {
    renderDialog();
    expect(screen.getByRole('button', { name: /import 3 games/i })).toBeDisabled();
  });

  it('shows status-value rows only after a status column is chosen', async () => {
    const user = userEvent.setup();
    renderDialog();

    expect(screen.queryByText('2 · Map status values')).not.toBeInTheDocument();

    await user.click(screen.getByRole('combobox', { name: 'Play status column' }));
    await user.click(screen.getByRole('option', { name: 'Status' }));

    await waitFor(() => {
      expect(screen.getByText('2 · Map status values')).toBeInTheDocument();
    });
    expect(screen.getByRole('combobox', { name: 'Status for Beaten' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Status for Playing' })).toBeInTheDocument();
  });

  it('imports with the assembled mapping (status values default to not_started)', async () => {
    const user = userEvent.setup();
    const onImport = renderDialog();

    await user.click(screen.getByRole('combobox', { name: 'Title column' }));
    await user.click(screen.getByRole('option', { name: 'Name' }));

    await user.click(screen.getByRole('combobox', { name: 'Play status column' }));
    await user.click(screen.getByRole('option', { name: 'Status' }));
    await waitFor(() => screen.getByRole('combobox', { name: 'Status for Beaten' }));

    const importBtn = screen.getByRole('button', { name: /import 3 games/i });
    expect(importBtn).toBeEnabled();
    await user.click(importBtn);

    expect(onImport).toHaveBeenCalledTimes(1);
    expect(onImport).toHaveBeenCalledWith({
      columns: {
        title: 'Name',
        igdb_id: '',
        platform: '',
        storefront: '',
        rating: '',
        notes: '',
        acquired_date: '',
        hours_played: '',
        tags: '',
        loved: '',
      },
      status: { column: 'Status', value_map: { Beaten: 'not_started', Playing: 'not_started' } },
      rating_scale: 5,
      merge_by_title: true,
    });
  });

  it('pre-fills the form from inspect.suggested_mapping', () => {
    const seeded: CsvInspectResponse = {
      ...inspect,
      suggested_mapping: {
        columns: {
          title: 'Name',
          igdb_id: '',
          platform: '',
          storefront: '',
          rating: '',
          notes: '',
          acquired_date: '',
          hours_played: '',
          tags: '',
          loved: '',
        },
        status: { column: 'Status', value_map: { Beaten: 'completed', Playing: 'in_progress' } },
        rating_scale: 5,
        merge_by_title: true,
      },
    };
    render(
      <CsvMappingDialog
        open
        onOpenChange={vi.fn()}
        inspect={seeded}
        isImporting={false}
        onImport={vi.fn()}
      />,
    );
    // Title pre-filled -> Import button is enabled.
    expect(screen.getByRole('button', { name: /import 3 games/i })).toBeEnabled();
    // Title select shows the guessed header.
    expect(screen.getByRole('combobox', { name: 'Title column' })).toHaveTextContent('Name');
    // Status column guessed -> the status-value section is already shown.
    expect(screen.getByText('2 · Map status values')).toBeInTheDocument();
    // Seeded status value_map is reflected (not reset to Not Started on mount).
    expect(screen.getByRole('combobox', { name: 'Status for Beaten' })).toHaveTextContent(
      'Completed',
    );
    expect(screen.getByRole('combobox', { name: 'Status for Playing' })).toHaveTextContent(
      'In Progress',
    );
  });
});
