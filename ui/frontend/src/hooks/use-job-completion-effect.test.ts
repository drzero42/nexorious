import { describe, it, expect, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useJobCompletionEffect } from './use-job-completion-effect';

describe('useJobCompletionEffect', () => {
  it('does not fire on mount', () => {
    const onComplete = vi.fn();
    renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: 'job-1' as string | null },
    });
    expect(onComplete).not.toHaveBeenCalled();
  });

  it('does not fire on null -> non-null', () => {
    const onComplete = vi.fn();
    const { rerender } = renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: null as string | null },
    });
    rerender({ id: 'job-1' });
    expect(onComplete).not.toHaveBeenCalled();
  });

  it('fires once on non-null -> null', () => {
    const onComplete = vi.fn();
    const { rerender } = renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: 'job-1' as string | null },
    });
    rerender({ id: null });
    expect(onComplete).toHaveBeenCalledTimes(1);
  });

  it('does not fire when the id stays constant across rerenders', () => {
    const onComplete = vi.fn();
    const { rerender } = renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: 'job-1' as string | null },
    });
    rerender({ id: 'job-1' });
    rerender({ id: 'job-1' });
    expect(onComplete).not.toHaveBeenCalled();
  });

  it('re-arms: fires again for a new job after the first completion', () => {
    const onComplete = vi.fn();
    const { rerender } = renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: 'job-1' as string | null },
    });
    rerender({ id: null });
    expect(onComplete).toHaveBeenCalledTimes(1);
    rerender({ id: 'job-2' });
    expect(onComplete).toHaveBeenCalledTimes(1);
    rerender({ id: null });
    expect(onComplete).toHaveBeenCalledTimes(2);
  });
});
