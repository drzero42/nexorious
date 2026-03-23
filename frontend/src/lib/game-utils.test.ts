import { describe, it, expect } from 'vitest';
import { formatTtb, formatIgdbRating } from './game-utils';

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
    expect(formatIgdbRating(72.10)).toBe('7.2');
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
