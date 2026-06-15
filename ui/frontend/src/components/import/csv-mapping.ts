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

/** The set of headers currently claimed by any field (columns + status). */
export function usedColumns(mapping: CsvMapping): Set<string> {
  const used = new Set<string>();
  for (const v of Object.values(mapping.columns)) {
    if (v) used.add(v);
  }
  if (mapping.status.column) used.add(mapping.status.column);
  return used;
}

/**
 * Headers selectable for one field: every header not claimed by another field,
 * plus this field's own current value. Derived purely from the current mapping,
 * so clearing or reassigning any field immediately frees its column for others.
 */
export function availableHeaders(
  allHeaders: string[],
  mapping: CsvMapping,
  currentValue: string,
): string[] {
  const used = usedColumns(mapping);
  return allHeaders.filter((h) => h === currentValue || !used.has(h));
}
