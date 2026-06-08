import { describe, it, expect } from 'vitest';
import { SyncStorefront, SUPPORTED_SYNC_STOREFRONTS } from './sync';

describe('SyncStorefront', () => {
  it('uses the canonical storefronts.name slug as each value', () => {
    expect(SyncStorefront.PLAYSTATION_STORE).toBe('playstation-store');
    expect(SyncStorefront.EPIC_GAMES_STORE).toBe('epic-games-store');
  });

  it('should include PlayStation Store in SUPPORTED_SYNC_STOREFRONTS', () => {
    expect(SUPPORTED_SYNC_STOREFRONTS).toContain(SyncStorefront.PLAYSTATION_STORE);
  });

  it('should have all supported storefronts defined', () => {
    expect(SUPPORTED_SYNC_STOREFRONTS).toEqual(
      expect.arrayContaining([
        SyncStorefront.STEAM,
        SyncStorefront.EPIC_GAMES_STORE,
        SyncStorefront.PLAYSTATION_STORE,
      ]),
    );
  });
});
