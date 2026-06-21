// Library view preferences (filters, sort, view mode, per-page, search, page)
// mirrored to localStorage so the library view is restored across browser
// sessions. See docs/superpowers/specs/2026-06-21-remember-library-view-design.md.

const KEY = 'nexorious:library-view:v1';

export type LibrarySearch = Record<string, string | string[]>;

/** Write the current library search params to localStorage. Never throws. */
export function saveLibraryPrefs(search: LibrarySearch): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(search));
  } catch {
    // Quota exceeded or serialization failure — persistence is best-effort.
  }
}

/** Read saved library search params, or null if absent/corrupt. Never throws. */
export function loadLibraryPrefs(): LibrarySearch | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as LibrarySearch;
    }
    return null;
  } catch {
    return null;
  }
}

/** True when the search params object carries no keys. */
export function isEmptySearch(search: LibrarySearch): boolean {
  return Object.keys(search).length === 0;
}
