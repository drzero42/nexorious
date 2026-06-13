import type { FilterCard, PoolFilter } from '@/types';

/** True if the card constrains at least one facet (mirrors FilterCard.HasFacets in Go). */
export function cardHasFacets(c: FilterCard): boolean {
  return (
    (c.play_status?.length ?? 0) > 0 ||
    (c.genre?.length ?? 0) > 0 ||
    (c.theme?.length ?? 0) > 0 ||
    (c.tag?.length ?? 0) > 0 ||
    (c.platform?.length ?? 0) > 0 ||
    (c.storefront?.length ?? 0) > 0 ||
    c.rating_min != null ||
    c.rating_max != null ||
    c.is_loved != null ||
    (c.game_mode?.length ?? 0) > 0 ||
    (c.player_perspective?.length ?? 0) > 0 ||
    (c.q != null && c.q !== '') ||
    c.time_to_beat_min != null ||
    c.time_to_beat_max != null
  );
}

/** Drop empty arrays / blank scalars from a card, returning a minimal card. */
function cleanCard(c: FilterCard): FilterCard {
  const out: FilterCard = {};
  if (c.play_status?.length) out.play_status = c.play_status;
  if (c.genre?.length) out.genre = c.genre;
  if (c.theme?.length) out.theme = c.theme;
  if (c.tag?.length) out.tag = c.tag;
  if (c.platform?.length) out.platform = c.platform;
  if (c.storefront?.length) out.storefront = c.storefront;
  if (c.rating_min != null) out.rating_min = c.rating_min;
  if (c.rating_max != null) out.rating_max = c.rating_max;
  if (c.is_loved != null) out.is_loved = c.is_loved;
  if (c.game_mode?.length) out.game_mode = c.game_mode;
  if (c.player_perspective?.length) out.player_perspective = c.player_perspective;
  if (c.q) out.q = c.q;
  if (c.time_to_beat_min != null) out.time_to_beat_min = c.time_to_beat_min;
  if (c.time_to_beat_max != null) out.time_to_beat_max = c.time_to_beat_max;
  return out;
}

/** Clean each card and drop those left with no facets. */
export function sanitizeFilter(f: PoolFilter): PoolFilter {
  return { filters: f.filters.map(cleanCard).filter(cardHasFacets) };
}

/** A filter is valid to save iff at least one card survives sanitization. */
export function isValidFilter(f: PoolFilter): boolean {
  return sanitizeFilter(f).filters.length > 0;
}
