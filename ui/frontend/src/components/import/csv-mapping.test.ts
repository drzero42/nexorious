import { describe, it, expect } from 'vitest';
import { emptyCsvMapping, initStatusValueMap, usedColumns, availableHeaders } from './csv-mapping';
import { PlayStatus } from '@/types';

describe('csv-mapping helpers', () => {
  it('emptyCsvMapping defaults merge on, scale 5, all columns blank', () => {
    const m = emptyCsvMapping();
    expect(m.columns).toEqual({
      title: '',
      platform: '',
      storefront: '',
      rating: '',
      notes: '',
      acquired_date: '',
      hours_played: '',
      tags: '',
      loved: '',
    });
    expect(m.rating_scale).toBe(5);
    expect(m.merge_by_title).toBe(true);
    expect(m.status.column).toBe('');
    expect(m.status.value_map).toEqual({});
  });

  it('initStatusValueMap maps every distinct value to Not Started', () => {
    expect(initStatusValueMap(['Beaten', 'Playing'])).toEqual({
      Beaten: PlayStatus.NOT_STARTED,
      Playing: PlayStatus.NOT_STARTED,
    });
  });

  it('initStatusValueMap returns an empty map for no values', () => {
    expect(initStatusValueMap([])).toEqual({});
  });

  it('usedColumns collects every non-empty column + status column', () => {
    const m = emptyCsvMapping();
    m.columns.title = 'Name';
    m.columns.platform = 'System';
    m.status.column = 'Status';
    expect(usedColumns(m)).toEqual(new Set(['Name', 'System', 'Status']));
  });

  it('availableHeaders hides headers used by other fields but keeps own value', () => {
    const headers = ['Name', 'System', 'Status'];
    const m = emptyCsvMapping();
    m.columns.title = 'Name';
    m.columns.platform = 'System';
    // For the title field (own value 'Name'): 'System' is used elsewhere and hidden.
    expect(availableHeaders(headers, m, 'Name')).toEqual(['Name', 'Status']);
    // For an unset field: both used headers hidden.
    expect(availableHeaders(headers, m, '')).toEqual(['Status']);
  });

  it('availableHeaders frees a column once its field is cleared', () => {
    const headers = ['Name', 'System'];
    const m = emptyCsvMapping();
    m.columns.title = 'Name';
    m.columns.platform = 'System';
    // Platform still set -> hidden for the unset rating field.
    expect(availableHeaders(headers, m, '')).toEqual([]);
    // Clear platform -> 'System' returns to the pool immediately.
    m.columns.platform = '';
    expect(availableHeaders(headers, m, '')).toEqual(['System']);
  });
});
