import { describe, it, expect } from 'vitest';
import { planPlatformChanges, type PlatformDetailState } from './platform-reconcile';
import { OwnershipStatus } from '@/types';
import type { UserGamePlatform } from '@/types';

const orig = (
  id: string,
  platform: string,
  storefront: string | undefined,
  extra: Partial<UserGamePlatform> = {},
): UserGamePlatform => ({
  id,
  platform,
  storefront,
  is_available: true,
  hours_played: 0,
  ownership_status: OwnershipStatus.OWNED,
  created_at: '2024-01-01T00:00:00Z',
  ...extra,
});

const detail = (
  hoursPlayed = 0,
  ownershipStatus = OwnershipStatus.OWNED,
  acquiredDate = '',
): PlatformDetailState => ({ hoursPlayed, ownershipStatus, acquiredDate });

describe('planPlatformChanges', () => {
  it('emits an update carrying the new storefront when it changes (#847)', () => {
    const original = [
      orig('ugp-1', 'pc', 'steam', { hours_played: 10, acquired_date: '2024-01-15' }),
    ];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'epic' }];
    const details = { 'ugp-1': detail(10, OwnershipStatus.OWNED, '2024-01-15') };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs.adds).toEqual([]);
    expect(cs.removes).toEqual([]);
    expect(cs.updates).toEqual([
      {
        id: 'ugp-1',
        platform: 'pc',
        storefront: 'epic',
        hoursPlayed: 10,
        ownershipStatus: OwnershipStatus.OWNED,
        acquiredDate: '2024-01-15',
      },
    ]);
  });

  it('removes the correct row id and leaves the sibling when a duplicate is deleted (#846)', () => {
    const original = [orig('ugp-1', 'pc', 'steam'), orig('ugp-2', 'pc', undefined)];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'steam' }];
    const details = { 'ugp-1': detail(0), 'ugp-2': detail(0) };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs.removes).toEqual([{ id: 'ugp-2' }]);
    expect(cs.adds).toEqual([]);
    expect(cs.updates).toEqual([]);
  });

  it('treats a selection without an id as an add', () => {
    const selections = [{ key: 'new-1', platform: 'ps5', storefront: 'psn' }];

    const cs = planPlatformChanges([], selections, {});

    expect(cs.adds).toEqual([{ platform: 'ps5', storefront: 'psn' }]);
    expect(cs.removes).toEqual([]);
    expect(cs.updates).toEqual([]);
  });

  it('emits nothing when there are no changes', () => {
    const original = [
      orig('ugp-1', 'pc', 'steam', { hours_played: 10, acquired_date: '2024-01-15' }),
    ];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'steam' }];
    const details = { 'ugp-1': detail(10, OwnershipStatus.OWNED, '2024-01-15') };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs).toEqual({ adds: [], removes: [], updates: [] });
  });

  it('emits an update when an existing storefront is cleared to none (#847)', () => {
    const original = [orig('ugp-1', 'pc', 'steam', { hours_played: 4 })];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: undefined }];
    const details = { 'ugp-1': detail(4) };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs.updates).toEqual([
      {
        id: 'ugp-1',
        platform: 'pc',
        storefront: undefined,
        hoursPlayed: 4,
        ownershipStatus: OwnershipStatus.OWNED,
      },
    ]);
  });

  it('emits an update when ownership or hours change', () => {
    const original = [orig('ugp-1', 'pc', 'steam', { hours_played: 5 })];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'steam' }];
    const details = { 'ugp-1': detail(12, OwnershipStatus.BORROWED, '') };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs.updates).toEqual([
      {
        id: 'ugp-1',
        platform: 'pc',
        storefront: 'steam',
        hoursPlayed: 12,
        ownershipStatus: OwnershipStatus.BORROWED,
      },
    ]);
  });
});
