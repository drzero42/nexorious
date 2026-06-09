import { describe, it, expect } from 'vitest';
import { candidateDisplayJobId, resolveDisplayJobId } from './maintenance';
import type { JobTypeStatus } from '@/types';

function status(over: Partial<JobTypeStatus>): JobTypeStatus {
  return {
    isActive: false,
    activeJobId: null,
    lastCompletedJobId: null,
    lastCompletedAt: null,
    ...over,
  };
}

describe('candidateDisplayJobId', () => {
  it('prefers an active metadata job over everything else', () => {
    const refresh = status({ isActive: true, activeJobId: 'meta-active', lastCompletedJobId: 'x' });
    const store = status({ isActive: true, activeJobId: 'store-active' });
    expect(candidateDisplayJobId(refresh, store)).toBe('meta-active');
  });

  it('falls back to an active store-link job when no metadata job is active', () => {
    const refresh = status({ lastCompletedJobId: 'meta-done' });
    const store = status({ isActive: true, activeJobId: 'store-active' });
    expect(candidateDisplayJobId(refresh, store)).toBe('store-active');
  });

  it('falls back to the most recently completed job across both types', () => {
    const refresh = status({
      lastCompletedJobId: 'meta-done',
      lastCompletedAt: '2026-01-01T00:00:00Z',
    });
    const store = status({
      lastCompletedJobId: 'store-done',
      lastCompletedAt: '2026-02-01T00:00:00Z',
    });
    expect(candidateDisplayJobId(refresh, store)).toBe('store-done');
  });

  it('returns undefined when there is nothing to show', () => {
    expect(candidateDisplayJobId(undefined, undefined)).toBeUndefined();
    expect(candidateDisplayJobId(status({}), status({}))).toBeUndefined();
  });
});

describe('resolveDisplayJobId', () => {
  it('returns the candidate when it has not been dismissed', () => {
    const store = status({
      lastCompletedJobId: 'store-done',
      lastCompletedAt: '2026-02-01T00:00:00Z',
    });
    expect(resolveDisplayJobId(undefined, store, null)).toBe('store-done');
  });

  // Regression for #884: after the user dismisses a completed job via "Start
  // New" and starts another refresh, the most-recent-completed fallback must
  // NOT resurrect the dismissed job while the new job's row is still being
  // created server-side.
  it('suppresses the candidate when it is the dismissed job', () => {
    const store = status({
      lastCompletedJobId: 'store-done',
      lastCompletedAt: '2026-02-01T00:00:00Z',
    });
    expect(resolveDisplayJobId(undefined, store, 'store-done')).toBeUndefined();
  });

  it('still shows a newly-active job even if an old completed job was dismissed', () => {
    const refresh = status({ isActive: true, activeJobId: 'meta-active' });
    const store = status({
      lastCompletedJobId: 'store-done',
      lastCompletedAt: '2026-02-01T00:00:00Z',
    });
    expect(resolveDisplayJobId(refresh, store, 'store-done')).toBe('meta-active');
  });
});
