// Which Library Health checks have their flagged-item lists expanded, mirrored
// to sessionStorage so the expansion survives navigating away and back (e.g.
// editing a game) within the same browser session. Cleared when the browser
// closes. See docs/superpowers/specs — companion of lib/library-prefs.ts.

const KEY = 'nexorious:library-health-expanded:v1';

/** Write the set of expanded check IDs to sessionStorage. Never throws. */
export function saveExpandedChecks(ids: string[]): void {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(ids));
  } catch {
    // Quota exceeded or serialization failure — persistence is best-effort.
  }
}

/** Read the saved expanded check IDs, or an empty array if absent/corrupt. Never throws. */
export function loadExpandedChecks(): string[] {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed.filter((v): v is string => typeof v === 'string');
    }
    return [];
  } catch {
    return [];
  }
}
