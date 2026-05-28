import { describe, it, expect } from 'vitest';
import { formatTtb, formatIgdbRating, formatHoursPlayed } from './game-utils';

describe('formatHoursPlayed', () => {
  it.each([
    [0, '0h'],
    [null, '0h'],
    [undefined, '0h'],
    [1.2, '1h'],
    [1.3, '1.5h'],
    [7.4, '7.5h'],
    [9.74, '9.5h'],
    [9.75, '10h'],
    [9.8, '10h'],
    [10, '10h'],
    [30.299999999999997, '30h'],
    [134, '134h'],
  ])('formats %s as %s', (input, expected) => {
    expect(formatHoursPlayed(input as number | null | undefined)).toBe(expected);
  });
});

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

describe('formatIgdbRating', () => {
  it('converts 85.42 to "8.5"', () => {
    expect(formatIgdbRating(85.42)).toBe('8.5');
  });
  it('converts 72.10 to "7.2"', () => {
    expect(formatIgdbRating(72.1)).toBe('7.2');
  });
  it('converts 100 to "10.0"', () => {
    expect(formatIgdbRating(100)).toBe('10.0');
  });
  it('converts 0 to "0.0"', () => {
    expect(formatIgdbRating(0)).toBe('0.0');
  });
  it('returns "—" for null', () => {
    expect(formatIgdbRating(null)).toBe('—');
  });
  it('returns "—" for undefined', () => {
    expect(formatIgdbRating(undefined)).toBe('—');
  });
});
