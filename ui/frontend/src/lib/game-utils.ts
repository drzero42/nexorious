import { config } from '@/lib/env';
import type { UserGame, UserGamePlatform } from '@/types';

// Half-hour buckets below 10h, whole hours from 10h up. The display rule for
// every hours value in the app — recorded playtime and HLTB times alike.
function bucketHours(hours: number): number {
  return hours < 10 ? Math.round(hours * 2) / 2 : Math.round(hours);
}

// Resolve a stored image URL to something the browser can load: absolute
// http(s) URLs pass through untouched, relative paths are prefixed with the
// static-asset origin. Returns '' for a missing URL.
export function resolveImageUrl(url: string | undefined | null): string {
  if (!url) return '';
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url;
  }
  return `${config.staticUrl}${url.startsWith('/') ? url : `/${url}`}`;
}

// Cover-art URL for a game, or null when it has none. Thin wrapper over
// resolveImageUrl that reads the nested cover_art_url off a UserGame.
export function getCoverUrl(game: UserGame): string | null {
  const url = game.game?.cover_art_url;
  return url ? resolveImageUrl(url) : null;
}

export function formatTtb(hours: number | null | undefined): string {
  if (hours == null) return '—';
  return `${bucketHours(hours)}h`;
}

export function formatHoursPlayed(hours: number | null | undefined): string {
  return `${bucketHours(hours ?? 0)}h`;
}

export function formatIgdbRating(value: number | null | undefined): string {
  if (value == null) return '—';
  return (value / 10).toFixed(1);
}

// Normalize a stored date — RFC3339 ("2024-06-01T00:00:00Z", the API format)
// or a bare "2024-06-01" — to the YYYY-MM-DD form an <input type="date">
// requires. The input silently renders blank for any other format, so the edit
// form must convert before binding. Returns '' for missing/unparseable values.
export function toDateInputValue(value: string | null | undefined): string {
  if (!value) return '';
  const match = /^\d{4}-\d{2}-\d{2}/.exec(value);
  return match ? match[0] : '';
}

export function bestAchievementProgress(
  platforms?: UserGamePlatform[],
): { unlocked: number; total: number } | null {
  if (!platforms) return null;
  let best: { unlocked: number; total: number } | null = null;
  for (const p of platforms) {
    if (p.achievements_total == null || p.achievements_total <= 0) continue;
    const unlocked = p.achievements_unlocked ?? 0;
    if (best === null || unlocked / p.achievements_total > best.unlocked / best.total) {
      best = { unlocked, total: p.achievements_total };
    }
  }
  return best;
}

export function formatPlatformLabel(p: {
  platform?: string | null;
  storefront?: string | null;
  platform_details?: { display_name: string } | null;
  storefront_details?: { display_name: string } | null;
}): string {
  const platform = p.platform_details?.display_name || p.platform;
  const storefront = p.storefront_details?.display_name || p.storefront;
  if (platform && storefront) return `${platform} (${storefront})`;
  return platform || storefront || 'Unknown';
}
