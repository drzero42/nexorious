import { useEffect, useRef } from 'react';

/**
 * Calls `onComplete` when `activeJobId` transitions from a non-null value to
 * null/undefined — the signal that a tracked job has finished. Does not fire on
 * mount or on the null → non-null transition.
 *
 * Callers should memoise `onComplete` (useCallback) so the effect deps stay
 * stable.
 */
export function useJobCompletionEffect(
  activeJobId: string | null | undefined,
  onComplete: () => void,
) {
  const prevRef = useRef<string | null>(null);
  useEffect(() => {
    if (prevRef.current && !activeJobId) {
      onComplete();
    }
    prevRef.current = activeJobId ?? null;
  }, [activeJobId, onComplete]);
}
