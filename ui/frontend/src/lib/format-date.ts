export type DateFormatPref = 'auto' | 'iso' | 'dmy' | 'mdy';

export const DATE_FORMAT_OPTIONS: { value: DateFormatPref; label: string }[] = [
  { value: 'auto', label: 'Auto (use browser locale)' },
  { value: 'iso', label: 'YYYY-MM-DD' },
  { value: 'dmy', label: 'DD-MM-YYYY' },
  { value: 'mdy', label: 'MM-DD-YYYY' },
];

function toDate(value: string | number | Date | null | undefined): Date | null {
  if (value === null || value === undefined || value === '') return null;
  const d = value instanceof Date ? value : new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/**
 * Format the date portion of `value` per the user's preference.
 * `auto` follows the browser locale (numeric short); the explicit prefs build
 * a fixed numeric order from LOCAL date components.
 */
export function formatDate(
  value: string | number | Date | null | undefined,
  pref: DateFormatPref = 'auto',
  nullLabel = '-',
): string {
  const d = toDate(value);
  if (!d) return nullLabel;

  if (pref === 'auto') {
    return d.toLocaleDateString(undefined, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  }

  const y = String(d.getFullYear());
  const m = pad(d.getMonth() + 1);
  const day = pad(d.getDate());
  switch (pref) {
    case 'iso':
      return `${y}-${m}-${day}`;
    case 'dmy':
      return `${day}-${m}-${y}`;
    case 'mdy':
      return `${m}-${day}-${y}`;
  }
}

/**
 * Format `value` as the preference-aware date plus a fixed 24-hour HH:MM
 * (local time), separated by a space.
 */
export function formatDateTime(
  value: string | number | Date | null | undefined,
  pref: DateFormatPref = 'auto',
  nullLabel = '-',
): string {
  const d = toDate(value);
  if (!d) return nullLabel;
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  return `${formatDate(d, pref)} ${time}`;
}
