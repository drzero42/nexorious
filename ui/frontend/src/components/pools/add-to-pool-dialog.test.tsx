import { describe, it, expect } from 'vitest';
import { mergeMembership } from './add-to-pool-dialog';
import type { PoolListItem, PoolMembership } from '@/types';

function pool(id: string, name: string): PoolListItem {
  return {
    id,
    name,
    color: null,
    position: 0,
    has_filter: false,
    queue_count: 0,
    candidate_count: 0,
  };
}

describe('mergeMembership', () => {
  const pools: PoolListItem[] = [pool('p1', 'A'), pool('p2', 'B'), pool('p3', 'C')];

  it('marks pools the game belongs to as checked', () => {
    const memberships: PoolMembership[] = [
      { pool_id: 'p1', position: 0 },
      { pool_id: 'p3', position: null },
    ];
    const rows = mergeMembership(pools, memberships);
    expect(rows.find((r) => r.pool.id === 'p1')?.member).toBe(true);
    expect(rows.find((r) => r.pool.id === 'p2')?.member).toBe(false);
    expect(rows.find((r) => r.pool.id === 'p3')?.member).toBe(true);
  });

  it('treats all pools as not-member when memberships is empty', () => {
    const rows = mergeMembership(pools, []);
    expect(rows.every((r) => !r.member)).toBe(true);
  });

  it('falls back to not-member when memberships is undefined (no #971 data yet)', () => {
    const rows = mergeMembership(pools, undefined);
    expect(rows.every((r) => !r.member)).toBe(true);
  });
});
