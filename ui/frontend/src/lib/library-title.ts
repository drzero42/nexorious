import { sortOptions, type SortField, type SortOrder } from '@/lib/sort-options';
import { playStatusLabels, ownershipStatusLabels } from '@/lib/filter-labels';

const sortLabels: Record<string, string> = Object.fromEntries(
  sortOptions.map((o) => [o.value, o.label]),
);

/** Default library sort — kept out of the title so an unfiltered view stays "Library". */
const DEFAULT_SORT_BY: SortField = 'title';
const DEFAULT_SORT_ORDER: SortOrder = 'asc';

/** Cap the number of filter labels shown so the title stays readable. */
const MAX_LABELS = 4;

/** Fields read from the library route's filter state. */
export interface LibraryTitleFilters {
  status?: string[];
  ownershipStatus?: string;
  isLoved?: boolean;
  platforms?: string[];
  storefronts?: string[];
  genres?: string[];
  gameModes?: string[];
  themes?: string[];
  playerPerspectives?: string[];
  tags?: string[];
}

export interface LibraryTitleLookups {
  /** platform `name` → display name */
  platformLabels?: Record<string, string>;
  /** storefront `name` → display name */
  storefrontLabels?: Record<string, string>;
}

function humanize(value: string): string {
  return value
    .split('-')
    .map((part) => (part ? part.charAt(0).toUpperCase() + part.slice(1) : part))
    .join(' ');
}

/**
 * Build the browser-tab title for the library view, summarising the active
 * filters and (non-default) sort. Reflecting the filter/sort state in the title
 * is both informative and the mechanism that keeps Firefox Android showing a
 * title (see `nextTitleWrite` in `document-title.tsx`).
 *
 * Examples:
 *   - no filters, default sort  → "Library | Nexorious"
 *   - filters, default sort     → "Not Started, PC — Library | Nexorious"
 *   - filters + sort            → "Not Started · by IGDB Rating — Library | Nexorious"
 */
export function buildLibraryTitle(
  filters: LibraryTitleFilters,
  sortBy: SortField,
  sortOrder: SortOrder,
  lookups: LibraryTitleLookups = {},
): string {
  const labels: string[] = [];

  for (const s of filters.status ?? []) labels.push(playStatusLabels[s] ?? humanize(s));
  if (filters.ownershipStatus) {
    labels.push(
      ownershipStatusLabels[filters.ownershipStatus] ?? humanize(filters.ownershipStatus),
    );
  }
  if (filters.isLoved === true) labels.push('Loved');
  else if (filters.isLoved === false) labels.push('Not loved');
  for (const p of filters.platforms ?? []) labels.push(lookups.platformLabels?.[p] ?? humanize(p));
  for (const sf of filters.storefronts ?? [])
    labels.push(lookups.storefrontLabels?.[sf] ?? humanize(sf));
  for (const g of filters.genres ?? []) labels.push(g);
  for (const gm of filters.gameModes ?? []) labels.push(gm);
  for (const t of filters.themes ?? []) labels.push(t);
  for (const pp of filters.playerPerspectives ?? []) labels.push(pp);
  for (const tag of filters.tags ?? []) labels.push(tag);

  let filterSummary = '';
  if (labels.length > 0) {
    const shown = labels.slice(0, MAX_LABELS);
    const extra = labels.length - shown.length;
    filterSummary = shown.join(', ') + (extra > 0 ? `, +${extra} more` : '');
  }

  const sortIsDefault = sortBy === DEFAULT_SORT_BY && sortOrder === DEFAULT_SORT_ORDER;
  const sortSummary = sortIsDefault ? '' : `by ${sortLabels[sortBy] ?? sortBy}`;

  const lead = [filterSummary, sortSummary].filter(Boolean).join(' · ');
  return lead ? `${lead} — Library | Nexorious` : 'Library | Nexorious';
}
