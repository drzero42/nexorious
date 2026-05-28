export function formatTtb(hours: number | null | undefined): string {
  return hours != null ? `${hours}h` : '—';
}

export function formatHoursPlayed(hours: number | null | undefined): string {
  const h = hours ?? 0;
  // Half-hour buckets below 10h, whole hours from 10h up.
  const rounded = h < 10 ? Math.round(h * 2) / 2 : Math.round(h);
  return `${rounded}h`;
}

export function formatIgdbRating(value: number | null | undefined): string {
  if (value == null) return '—';
  return (value / 10).toFixed(1);
}
