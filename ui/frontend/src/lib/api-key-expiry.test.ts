import { describe, it, expect } from 'vitest';
import { expiryPresetToRFC3339, EXPIRY_PRESETS } from './api-key-expiry';

describe('expiryPresetToRFC3339', () => {
  // Fixed base time so the test is deterministic.
  const now = new Date('2026-06-02T12:00:00.000Z');

  it('returns null for "never"', () => {
    expect(expiryPresetToRFC3339('never', now)).toBeNull();
  });

  it('returns an RFC3339 string 30 days out', () => {
    expect(expiryPresetToRFC3339('30', now)).toBe('2026-07-02T12:00:00.000Z');
  });

  it('returns an RFC3339 string 90 days out', () => {
    expect(expiryPresetToRFC3339('90', now)).toBe('2026-08-31T12:00:00.000Z');
  });

  it('returns an RFC3339 string 365 days out', () => {
    expect(expiryPresetToRFC3339('365', now)).toBe('2027-06-02T12:00:00.000Z');
  });

  it('exposes preset options with "write"-friendly defaults order', () => {
    expect(EXPIRY_PRESETS.map((p) => p.value)).toEqual(['30', '90', '365', 'never']);
  });
});
