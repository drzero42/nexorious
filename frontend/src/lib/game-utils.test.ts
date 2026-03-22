import { describe, it, expect } from 'vitest';
import { formatTtb } from './game-utils';

describe('formatTtb', () => {
  it('formats a whole number of hours', () => {
    expect(formatTtb(10)).toBe('10h');
  });

  it('formats 0 hours', () => {
    expect(formatTtb(0)).toBe('0h');
  });

  it('formats decimal hours', () => {
    expect(formatTtb(12.5)).toBe('12.5h');
  });

  it('returns em-dash for null', () => {
    expect(formatTtb(null)).toBe('—');
  });

  it('returns em-dash for undefined', () => {
    expect(formatTtb(undefined)).toBe('—');
  });
});
