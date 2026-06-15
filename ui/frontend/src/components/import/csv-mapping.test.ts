import { describe, it, expect } from 'vitest';
import { emptyCsvMapping, initStatusValueMap } from './csv-mapping';
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
});
