import { describe, it, expect } from 'vitest';
import {
  SyncPlatform,
  SUPPORTED_SYNC_PLATFORMS,
  getPlatformDisplayInfo,
} from './sync';

describe('SyncPlatform', () => {
  it('should include PSN in enum', () => {
    expect(SyncPlatform.PSN).toBe('psn');
  });

  it('should include PSN in SUPPORTED_SYNC_PLATFORMS', () => {
    expect(SUPPORTED_SYNC_PLATFORMS).toContain(SyncPlatform.PSN);
  });

  it('should have all supported platforms defined', () => {
    expect(SUPPORTED_SYNC_PLATFORMS).toEqual(
      expect.arrayContaining([
        SyncPlatform.STEAM,
        SyncPlatform.EPIC,
        SyncPlatform.PSN,
      ])
    );
  });
});

describe('getPlatformDisplayInfo', () => {
  it('should return correct info for Steam', () => {
    const info = getPlatformDisplayInfo(SyncPlatform.STEAM);
    expect(info.name).toBe('Steam');
    expect(info.color).toBe('text-[#1b2838]');
    expect(info.bgColor).toBe('bg-[#1b2838]/10 dark:bg-[#1b2838]/30');
  });

  it('should return correct info for Epic', () => {
    const info = getPlatformDisplayInfo(SyncPlatform.EPIC);
    expect(info.name).toBe('Epic Games');
    expect(info.color).toBe('text-gray-800 dark:text-gray-200');
    expect(info.bgColor).toBe('bg-gray-100 dark:bg-gray-700');
  });

  it('should return correct info for PSN', () => {
    const info = getPlatformDisplayInfo(SyncPlatform.PSN);
    expect(info.name).toBe('PlayStation Network');
    expect(info.color).toBe('text-[#003087]');
    expect(info.bgColor).toBe('bg-[#003087]/10 dark:bg-[#003087]/30');
  });
});
