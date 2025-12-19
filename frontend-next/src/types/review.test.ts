import { describe, it, expect } from 'vitest';
import { formatReleaseYear } from './review';

describe('formatReleaseYear', () => {
  it('returns empty string for null', () => {
    expect(formatReleaseYear(null)).toBe('');
  });

  it('returns empty string for 0', () => {
    expect(formatReleaseYear(0)).toBe('');
  });

  it('returns formatted year for valid Unix timestamp', () => {
    // 1012867200 = 2002-02-05 00:00:00 UTC
    expect(formatReleaseYear(1012867200)).toBe('(2002)');
  });

  it('returns empty string for string value passed as any', () => {
    // This simulates legacy data where first_release_date was stored as a string
    // The function should handle this gracefully instead of returning (NaN)
    expect(formatReleaseYear('2002-02-06' as unknown as number)).toBe('');
  });

  it('returns empty string for undefined passed as any', () => {
    expect(formatReleaseYear(undefined as unknown as number)).toBe('');
  });

  it('returns empty string for NaN', () => {
    expect(formatReleaseYear(NaN)).toBe('');
  });
});
