import { describe, it, expect } from 'vitest';
import { formatDate, formatDateTime } from './format-date';

// Construct dates from LOCAL components so assertions are timezone-independent.
const d = new Date(2026, 5, 23, 14, 30, 0); // 2026-06-23 14:30 local
const single = new Date(2026, 0, 5, 9, 7, 0); // 2026-01-05 09:07 local

describe('formatDate', () => {
  it('iso → YYYY-MM-DD', () => {
    expect(formatDate(d, 'iso')).toBe('2026-06-23');
    expect(formatDate(single, 'iso')).toBe('2026-01-05');
  });
  it('dmy → DD-MM-YYYY', () => {
    expect(formatDate(d, 'dmy')).toBe('23-06-2026');
  });
  it('mdy → MM-DD-YYYY', () => {
    expect(formatDate(d, 'mdy')).toBe('06-23-2026');
  });
  it('auto returns a non-empty locale string', () => {
    expect(formatDate(d, 'auto')).not.toBe('');
    expect(formatDate(d, 'auto')).not.toBe('-');
  });
  it('defaults to auto when no pref given', () => {
    expect(formatDate(d)).toBe(formatDate(d, 'auto'));
  });
  it('returns the null label for missing/invalid input', () => {
    expect(formatDate(null, 'iso')).toBe('-');
    expect(formatDate(undefined, 'iso', 'Never')).toBe('Never');
    expect(formatDate('not-a-date', 'iso')).toBe('-');
  });
});

describe('formatDateTime', () => {
  it('appends fixed 24-hour time to the formatted date', () => {
    expect(formatDateTime(d, 'iso')).toBe('2026-06-23 14:30');
    expect(formatDateTime(single, 'iso')).toBe('2026-01-05 09:07');
  });
  it('honours pref for the date portion', () => {
    expect(formatDateTime(d, 'dmy')).toBe('23-06-2026 14:30');
  });
  it('returns the null label for missing input', () => {
    expect(formatDateTime(null, 'iso', 'Never')).toBe('Never');
  });
});
