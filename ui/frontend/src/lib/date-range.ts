/**
 * Convert the `YYYY-MM-DD` values from a pair of `<input type="date">` controls
 * into the half-open UTC instant range `[since, until)` that the events API's
 * `since`/`until` query params expect.
 *
 * The dates are interpreted in the admin's **local** timezone: `since` becomes
 * local start-of-day and `until` becomes the start of the *next* local day. The
 * backend applies an exclusive `<` upper bound, so the range covers the entire
 * picked local day with no sub-second truncation, and a non-UTC admin sees their
 * own calendar day rather than the UTC one.
 *
 * Either bound is omitted (left `undefined`) when its input is empty.
 */
export function dayRangeToUTC(since: string, until: string): { since?: string; until?: string } {
  return {
    since: since ? localDayStart(since, 0).toISOString() : undefined,
    until: until ? localDayStart(until, 1).toISOString() : undefined,
  };
}

/**
 * Build a local-timezone midnight `Date` for the given `YYYY-MM-DD`, offset by
 * `dayOffset` days. Using the numeric `Date` constructor (rather than parsing an
 * ISO string) guarantees local-timezone interpretation, and day arithmetic on
 * the day component correctly rolls over month/year boundaries.
 */
function localDayStart(date: string, dayOffset: number): Date {
  const [year, month, day] = date.split('-').map(Number);
  return new Date(year, month - 1, day + dayOffset, 0, 0, 0, 0);
}
