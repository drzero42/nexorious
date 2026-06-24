import type { useNavigate } from '@tanstack/react-router';

/**
 * Labeled referrer for game detail/edit "Back" navigation.
 *
 * Every place that opens a game detail page sets this referrer first, so the
 * detail page's back button can return to the actual origin (Library Health,
 * Wishlist, a Pool, the games list with its filters) rather than always
 * dumping the user back in `/games`.
 */
export interface GameReturn {
  /** Target route path, e.g. `/games`, `/library-health`, `/pools/$id`. */
  to: string;
  /** Route params for parameterized targets (e.g. `{ id }` for `/pools/$id`). */
  params?: Record<string, string>;
  /** Search params to restore (e.g. the games-list filter state). */
  search?: Record<string, string>;
  /** Human label rendered in the button: `← Back to {label}`. */
  label: string;
}

const STORAGE_KEY = 'game_return';

const DEFAULT_RETURN: GameReturn = { to: '/games', label: 'Games' };

/** Persist the referrer before navigating into a game detail/edit page. */
export function setGameReturn(target: GameReturn): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(target));
  } catch {
    // sessionStorage may be unavailable (private mode / quota); back nav then
    // falls back to the default, which is acceptable.
  }
}

/**
 * Read the stored referrer, defaulting to the games list when absent or
 * unparseable (direct-URL load, refresh, or a malformed value).
 */
export function getGameReturn(): GameReturn {
  const stored = sessionStorage.getItem(STORAGE_KEY);
  if (!stored) return DEFAULT_RETURN;
  try {
    const parsed = JSON.parse(stored) as Partial<GameReturn>;
    if (typeof parsed.to !== 'string' || typeof parsed.label !== 'string') {
      return DEFAULT_RETURN;
    }
    return {
      to: parsed.to,
      label: parsed.label,
      ...(parsed.params ? { params: parsed.params } : {}),
      ...(parsed.search ? { search: parsed.search } : {}),
    };
  } catch {
    return DEFAULT_RETURN;
  }
}

/** Navigate to the stored referrer (or the games list when none is set). */
export function navigateToGameReturn(navigate: ReturnType<typeof useNavigate>): void {
  const { to, params, search } = getGameReturn();
  const arg: Record<string, unknown> = { to };
  if (params) arg.params = params;
  if (search) arg.search = search;
  // TanStack's typed route union can't be expressed for a target read from
  // storage at runtime, so the stored navigation is replayed through one cast.
  navigate(arg as Parameters<typeof navigate>[0]);
}
