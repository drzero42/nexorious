import {
  createContext,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useRouterState } from '@tanstack/react-router';
import { nextTitleWrite, initialTitleWriteState, type TitleWriteState } from '@/lib/title-write';

interface TitleOverrideValue {
  override: string | undefined;
  setOverride: (title: string | undefined) => void;
}

const TitleOverrideContext = createContext<TitleOverrideValue>({
  override: undefined,
  setOverride: () => {},
});

export function DocumentTitleProvider({ children }: { children: ReactNode }) {
  const [override, setOverride] = useState<string | undefined>(undefined);
  return (
    <TitleOverrideContext.Provider value={{ override, setOverride }}>
      {children}
    </TitleOverrideContext.Provider>
  );
}

/**
 * Let the active route supply a dynamic document title (e.g. one that reflects
 * the current library filters). The override is cleared when the route unmounts,
 * falling back to the route's static `head()` title.
 */
// eslint-disable-next-line react-refresh/only-export-components
export function useDocumentTitleOverride(title: string) {
  const { setOverride } = useContext(TitleOverrideContext);
  useEffect(() => {
    setOverride(title);
    return () => setOverride(undefined);
  }, [title, setOverride]);
}

/**
 * Single owner of `document.title`. Reads the active route's static title from
 * router `meta` (populated by each route's `head()`), honours a dynamic
 * {@link useDocumentTitleOverride}, and re-asserts the title on every navigation
 * so Firefox Android keeps showing it (see {@link nextTitleWrite}).
 *
 * This replaces TanStack Router's declarative `<HeadContent>` for the title:
 * `<HeadContent>` only re-writes the title when its value changes, which is the
 * root cause of the tab reverting to the URL on Firefox Android after a
 * search-param navigation.
 */
export function DocumentTitle() {
  const metaTitle = useRouterState({
    select: (s) => {
      for (let i = s.matches.length - 1; i >= 0; i--) {
        const t = s.matches[i].meta?.find((m) => m?.title)?.title;
        if (t) return t;
      }
      return 'Nexorious';
    },
  });
  // Re-run on every navigation, not only when the title string changes.
  const href = useRouterState({ select: (s) => s.location.href });
  const { override } = useContext(TitleOverrideContext);
  const title = override ?? metaTitle;

  const stateRef = useRef<TitleWriteState>(initialTitleWriteState);
  useLayoutEffect(() => {
    const { value, state } = nextTitleWrite(title, stateRef.current);
    stateRef.current = state;
    document.title = value;
  }, [title, href]);

  return null;
}
