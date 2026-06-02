export type ExpiryPreset = '30' | '90' | '365' | 'never';

export const EXPIRY_PRESETS: { value: ExpiryPreset; label: string }[] = [
  { value: '30', label: '30 days' },
  { value: '90', label: '90 days' },
  { value: '365', label: '365 days' },
  { value: 'never', label: 'Never' },
];

/**
 * Convert an expiry preset into the RFC3339 string the API expects, or null for
 * "never". `now` is injectable so callers/tests can be deterministic.
 */
export function expiryPresetToRFC3339(preset: ExpiryPreset, now: Date = new Date()): string | null {
  if (preset === 'never') return null;
  const days = Number(preset);
  const expiry = new Date(now.getTime() + days * 24 * 60 * 60 * 1000);
  return expiry.toISOString();
}
