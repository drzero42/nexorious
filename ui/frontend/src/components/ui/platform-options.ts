import type { Platform, Storefront } from '@/types';

/**
 * Slot arithmetic for platform/storefront selection.
 *
 * A platform has N+1 storefront "slots": its N storefronts plus the `undefined`
 * ("No storefront") slot. The DB unique key `(user_game_id, platform, storefront)`
 * (NULLS NOT DISTINCT) means each slot holds at most one association. These
 * helpers let the selectors offer only free slots, making duplicate rows —
 * which the backend rejects with a 409 — structurally impossible.
 */

/** Minimal shape of a selected platform row needed for slot math. */
export interface PlatformRow {
  key: string;
  platform: string;
  storefront?: string;
}

/**
 * Storefront values (including `undefined` for "No storefront") occupied by rows
 * of `platformName`. Pass `exceptKey` to ignore one row (e.g. the row being edited).
 */
export function usedStorefronts(
  rows: PlatformRow[],
  platformName: string,
  exceptKey?: string,
): Set<string | undefined> {
  const used = new Set<string | undefined>();
  for (const r of rows) {
    if (r.platform !== platformName) continue;
    if (exceptKey !== undefined && r.key === exceptKey) continue;
    used.add(r.storefront);
  }
  return used;
}

/**
 * The platform's storefronts that are still free for the row identified by
 * `currentRowKey` — i.e. not taken by a sibling row. The row's own current
 * storefront is always retained (it excludes itself via `currentRowKey`).
 */
export function availableStorefronts(
  platform: Platform,
  rows: PlatformRow[],
  currentRowKey: string,
): Storefront[] {
  const used = usedStorefronts(rows, platform.name, currentRowKey);
  return (platform.storefronts ?? []).filter((s) => !used.has(s.name));
}

/** True when every slot (storefronts + "No storefront") of `platform` is taken. */
export function isPlatformExhausted(platform: Platform, rows: PlatformRow[]): boolean {
  const totalSlots = (platform.storefronts ?? []).length + 1; // +1 for "No storefront"
  return usedStorefronts(rows, platform.name).size >= totalSlots;
}

/**
 * The storefront slot to assign when a platform is added to a new row: the
 * platform's default if free, else the first free storefront, else `undefined`
 * ("No storefront"). Callers guard against exhausted platforms.
 */
export function firstFreeStorefront(platform: Platform, rows: PlatformRow[]): string | undefined {
  const used = usedStorefronts(rows, platform.name);
  const storefronts = platform.storefronts ?? [];
  const def = platform.default_storefront;
  if (def && storefronts.some((s) => s.name === def) && !used.has(def)) {
    return def;
  }
  for (const s of storefronts) {
    if (!used.has(s.name)) return s.name;
  }
  return undefined;
}
