import { useCallback, useSyncExternalStore } from 'react';

/**
 * Subscribe to a CSS media query and return whether it currently matches.
 *
 * Pass a standard media-query string, e.g. `'(max-width: 1023px)'`. The hook
 * re-renders when the match state changes (viewport resize, orientation, etc.).
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (onChange: () => void) => {
      const mql = window.matchMedia(query);
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    },
    [query],
  );

  const getSnapshot = useCallback(() => window.matchMedia(query).matches, [query]);

  // No DOM on the server — media queries never match during SSR/prerender.
  const getServerSnapshot = () => false;

  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
