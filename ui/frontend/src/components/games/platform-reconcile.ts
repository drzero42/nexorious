import type { UserGamePlatform } from '@/types';
import { OwnershipStatus } from '@/types';
import type { PlatformSelection } from '@/components/ui/platform-selector';

/** Per-row ownership/playtime state as edited in the form, keyed by row id. */
export interface PlatformDetailState {
  hoursPlayed: number;
  ownershipStatus: OwnershipStatus;
  acquiredDate: string; // '' when none
}

interface PlatformChangeSet {
  adds: {
    platform: string;
    storefront?: string;
    hoursPlayed: number;
    ownershipStatus: OwnershipStatus;
    acquiredDate?: string;
  }[];
  removes: { id: string }[];
  updates: {
    id: string;
    platform: string;
    storefront?: string;
    hoursPlayed: number;
    ownershipStatus: OwnershipStatus;
    acquiredDate?: string;
  }[];
}

/**
 * Diffs the persisted platform rows against the edited selections by row id.
 * - selections without an `id` are adds, carrying their edited detail state
 * - original rows whose id is no longer selected are removes
 * - selected rows whose storefront (from the selection) or ownership/date/hours
 *   (from `details`) changed are updates
 *
 * `details` is keyed by the row's client `key` (present on every selection,
 * persisted or not), so both adds and updates resolve their detail state the
 * same way.
 */
export function planPlatformChanges(
  original: UserGamePlatform[],
  selections: PlatformSelection[],
  details: Record<string, PlatformDetailState>,
): PlatformChangeSet {
  const currentIds = new Set(selections.map((s) => s.id).filter((id): id is string => !!id));

  const adds = selections
    .filter((s) => !s.id)
    .map((s) => {
      const d = details[s.key] ?? {
        hoursPlayed: 0,
        ownershipStatus: OwnershipStatus.OWNED,
        acquiredDate: '',
      };
      return {
        platform: s.platform,
        storefront: s.storefront,
        hoursPlayed: d.hoursPlayed,
        ownershipStatus: d.ownershipStatus,
        acquiredDate: d.acquiredDate || undefined,
      };
    });

  const removes = original.filter((o) => !currentIds.has(o.id)).map((o) => ({ id: o.id }));

  const updates: PlatformChangeSet['updates'] = [];
  // Caller guarantees at most one selection per persisted id (one row -> one selection).
  for (const s of selections) {
    if (!s.id) continue;
    const o = original.find((p) => p.id === s.id);
    if (!o) continue;

    const d = details[s.key] ?? {
      hoursPlayed: o.hours_played,
      ownershipStatus: o.ownership_status,
      acquiredDate: o.acquired_date ?? '',
    };

    const changed =
      (o.storefront ?? '') !== (s.storefront ?? '') ||
      o.hours_played !== d.hoursPlayed ||
      o.ownership_status !== d.ownershipStatus ||
      (o.acquired_date ?? '') !== d.acquiredDate;

    if (changed) {
      updates.push({
        id: s.id,
        platform: o.platform ?? '',
        storefront: s.storefront,
        hoursPlayed: d.hoursPlayed,
        ownershipStatus: d.ownershipStatus,
        // Carry the value as-is: '' is the explicit "clear to NULL" signal the
        // backend needs; collapsing it to undefined would drop the field and
        // leave a previously-set date in place (#849).
        acquiredDate: d.acquiredDate,
      });
    }
  }

  return { adds, removes, updates };
}
