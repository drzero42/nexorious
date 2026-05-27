import { describe, it, expect } from 'vitest';
import { SyncStorefront, SUPPORTED_SYNC_STOREFRONTS, getStorefrontDisplayInfo } from './sync';

describe('SyncStorefront', () => {
  it('should include PSN in enum', () => {
    expect(SyncStorefront.PSN).toBe('psn');
  });

  it('should include PSN in SUPPORTED_SYNC_STOREFRONTS', () => {
    expect(SUPPORTED_SYNC_STOREFRONTS).toContain(SyncStorefront.PSN);
  });

  it('should have all supported storefronts defined', () => {
    expect(SUPPORTED_SYNC_STOREFRONTS).toEqual(
      expect.arrayContaining([SyncStorefront.STEAM, SyncStorefront.EPIC, SyncStorefront.PSN]),
    );
  });
});

describe('getStorefrontDisplayInfo', () => {
  it('should return correct info for Steam', () => {
    const info = getStorefrontDisplayInfo(SyncStorefront.STEAM);
    expect(info.name).toBe('Steam');
    expect(info.color).toBe('text-[#1b2838]');
    expect(info.bgColor).toBe('bg-[#1b2838]/10 dark:bg-[#1b2838]/30');
  });

  it('should return correct info for Epic', () => {
    const info = getStorefrontDisplayInfo(SyncStorefront.EPIC);
    expect(info.name).toBe('Epic Games');
    expect(info.color).toBe('text-gray-800 dark:text-gray-200');
    expect(info.bgColor).toBe('bg-gray-100 dark:bg-gray-700');
  });

  it('should return correct info for PSN', () => {
    const info = getStorefrontDisplayInfo(SyncStorefront.PSN);
    expect(info.name).toBe('PlayStation Network');
    expect(info.color).toBe('text-[#003087]');
    expect(info.bgColor).toBe('bg-[#003087]/10 dark:bg-[#003087]/30');
  });
});
