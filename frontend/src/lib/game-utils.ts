export function formatTtb(hours: number | null | undefined): string {
  return hours != null ? `${hours}h` : '—';
}
