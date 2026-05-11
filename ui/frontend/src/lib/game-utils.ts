export function formatTtb(hours: number | null | undefined): string {
  return hours != null ? `${hours}h` : '—';
}

export function formatIgdbRating(value: number | null | undefined): string {
  if (value == null) return '—';
  return (value / 10).toFixed(1);
}
