import { describe, it, expect } from 'vitest';
import { SyncStorefront, SUPPORTED_SYNC_STOREFRONTS } from './sync';

describe('SyncStorefront', () => {
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
