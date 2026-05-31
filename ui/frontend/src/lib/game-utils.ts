// Half-hour buckets below 10h, whole hours from 10h up. The display rule for
// every hours value in the app — recorded playtime and HLTB times alike.
function bucketHours(hours: number): number {
  return hours < 10 ? Math.round(hours * 2) / 2 : Math.round(hours);
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
