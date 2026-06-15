import { PlayStatus } from '@/types';
import type { CsvMapping } from '@/types';

/** A blank mapping: no columns chosen, merge-by-title on, rating scale 5. */
export function emptyCsvMapping(): CsvMapping {
  return {
    columns: {
      title: '',
      platform: '',
      storefront: '',
      rating: '',
      notes: '',
      acquired_date: '',
      hours_played: '',
      tags: '',
      loved: '',
    },
    status: { column: '', value_map: {} },
    rating_scale: 5,
    merge_by_title: true,
  };
}

/** Map every distinct source value to the Not Started default. */
export function initStatusValueMap(distinct: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const v of distinct) {
    out[v] = PlayStatus.NOT_STARTED;
  }
  return out;
}
