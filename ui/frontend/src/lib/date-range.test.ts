import { describe, it, expect } from 'vitest';
import { dayRangeToUTC, isRangeInverted } from './date-range';

describe('dayRangeToUTC', () => {
  it('returns undefined bounds for empty inputs', () => {
    expect(dayRangeToUTC('', '')).toEqual({ since: undefined, until: undefined });
  });

  it('maps `since` to local start-of-day', () => {
    const { since } = dayRangeToUTC('2026-06-03', '');
    const d = new Date(since!);
    // Local wall-clock of the returned instant is midnight on the picked day,
    // regardless of the timezone the test runs in.
    expect(d.getFullYear()).toBe(2026);
    expect(d.getMonth()).toBe(5); // June (0-based)
    expect(d.getDate()).toBe(3);
    expect(d.getHours()).toBe(0);
    expect(d.getMinutes()).toBe(0);
    expect(d.getSeconds()).toBe(0);
    expect(d.getMilliseconds()).toBe(0);
  });

  it('maps `until` to the exclusive start of the next local day', () => {
    const { until } = dayRangeToUTC('', '2026-06-03');
    const d = new Date(until!);
    expect(d.getFullYear()).toBe(2026);
    expect(d.getMonth()).toBe(5);
    expect(d.getDate()).toBe(4); // start of the day *after* the picked one
    expect(d.getHours()).toBe(0);
    expect(d.getMinutes()).toBe(0);
    expect(d.getSeconds()).toBe(0);
    expect(d.getMilliseconds()).toBe(0);
  });

  it('rolls `until` across a month boundary', () => {
    const { until } = dayRangeToUTC('', '2026-06-30');
    const d = new Date(until!);
    expect(d.getFullYear()).toBe(2026);
    expect(d.getMonth()).toBe(6); // July
    expect(d.getDate()).toBe(1);
  });

  it('produces a half-open range spanning exactly one local day', () => {
    const { since, until } = dayRangeToUTC('2026-06-03', '2026-06-03');
    expect(new Date(until!).getTime()).toBeGreaterThan(new Date(since!).getTime());
    // Across a non-DST day the span is 24h; assert it covers a full day either way.
    const spanHours = (new Date(until!).getTime() - new Date(since!).getTime()) / 3_600_000;
    expect(spanHours).toBeGreaterThanOrEqual(23);
    expect(spanHours).toBeLessThanOrEqual(25);
  });

  it('emits RFC3339/ISO UTC strings', () => {
    const { since, until } = dayRangeToUTC('2026-06-03', '2026-06-03');
    expect(since).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/);
    expect(until).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/);
  });
});

describe('isRangeInverted', () => {
  it('flags a range whose `until` precedes `since`', () => {
    expect(isRangeInverted('2026-06-10', '2026-06-03')).toBe(true);
  });

  it('treats equal dates as a valid single-day range', () => {
    expect(isRangeInverted('2026-06-03', '2026-06-03')).toBe(false);
  });

  it('treats a forward range as valid', () => {
    expect(isRangeInverted('2026-06-03', '2026-06-10')).toBe(false);
  });

  it('is not inverted when `since` is empty', () => {
    expect(isRangeInverted('', '2026-06-03')).toBe(false);
  });

  it('is not inverted when `until` is empty', () => {
    expect(isRangeInverted('2026-06-03', '')).toBe(false);
  });

  it('is not inverted when both bounds are empty', () => {
    expect(isRangeInverted('', '')).toBe(false);
  });
});
