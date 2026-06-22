import { describe, expect, it } from 'vitest';
import { isValidRelease, isNewer } from './version-compare';

describe('version-compare', () => {
  it('validates release versions', () => {
    expect(isValidRelease('0.90.0')).toBe(true);
    expect(isValidRelease('v0.90.0')).toBe(true);
    expect(isValidRelease('dev')).toBe(false);
    expect(isValidRelease('main-20260621-abc1234')).toBe(false);
    expect(isValidRelease('')).toBe(false);
  });

  it('compares newer-than', () => {
    expect(isNewer('0.90.0', '0.17.1')).toBe(true);
    expect(isNewer('0.17.1', '0.90.0')).toBe(false);
    expect(isNewer('0.90.0', '0.90.0')).toBe(false);
    expect(isNewer('0.90.0', 'dev')).toBe(false); // invalid baseline -> not newer
  });
});
