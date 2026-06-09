import { describe, it, expect } from 'vitest';
import { mostRecentCompletedJobId } from './maintenance';
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

describe('mostRecentCompletedJobId', () => {
  it('returns the more recently completed of the two job types', () => {
    const refresh = status({
      lastCompletedJobId: 'meta-done',
      lastCompletedAt: '2026-01-01T00:00:00Z',
    });
    const store = status({
      lastCompletedJobId: 'store-done',
      lastCompletedAt: '2026-02-01T00:00:00Z',
    });
    expect(mostRecentCompletedJobId(refresh, store)).toBe('store-done');
  });

  it('returns whichever side has a completed job when only one does', () => {
    const refresh = status({ lastCompletedJobId: 'meta-done' });
    expect(mostRecentCompletedJobId(refresh, status({}))).toBe('meta-done');
    expect(mostRecentCompletedJobId(status({}), refresh)).toBe('meta-done');
  });

  it('returns undefined when neither type has a completed job', () => {
    expect(mostRecentCompletedJobId(undefined, undefined)).toBeUndefined();
    expect(mostRecentCompletedJobId(status({}), status({}))).toBeUndefined();
  });
});
