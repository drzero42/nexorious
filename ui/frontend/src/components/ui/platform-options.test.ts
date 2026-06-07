import { describe, it, expect } from 'vitest';
import {
  usedStorefronts,
  availableStorefronts,
  isPlatformExhausted,
  firstFreeStorefront,
  type PlatformRow,
} from './platform-options';
import type { Platform, Storefront } from '@/types';

const sf = (name: string, display = name): Storefront => ({
  name,
  display_name: display,
  is_active: true,
  source: 'official',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
});

const platform = (
  name: string,
  storefronts: Storefront[],
  default_storefront?: string,
): Platform => ({
  name,
  display_name: name.toUpperCase(),
  is_active: true,
  source: 'official',
  default_storefront,
  storefronts,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
});

const row = (key: string, plat: string, storefront?: string): PlatformRow => ({
  key,
  platform: plat,
  storefront,
});

const pc = platform('pc', [sf('steam', 'Steam'), sf('epic', 'Epic'), sf('gog', 'GOG')], 'steam');
const switchConsole = platform('switch', []); // no storefronts -> single "No storefront" slot

describe('usedStorefronts', () => {
  it('collects storefront values used by rows of the platform, including undefined', () => {
    const rows = [row('a', 'pc', 'steam'), row('b', 'pc', undefined), row('c', 'ps5', 'psn')];
    const used = usedStorefronts(rows, 'pc');
    expect(used.has('steam')).toBe(true);
    expect(used.has(undefined)).toBe(true);
    expect(used.has('psn')).toBe(false); // different platform
    expect(used.size).toBe(2);
  });

  it('excludes the row identified by exceptKey', () => {
    const rows = [row('a', 'pc', 'steam'), row('b', 'pc', 'epic')];
    const used = usedStorefronts(rows, 'pc', 'a');
    expect(used.has('steam')).toBe(false); // row "a" excluded
    expect(used.has('epic')).toBe(true);
  });
});

describe('availableStorefronts', () => {
  it('omits storefronts taken by sibling rows', () => {
    const rows = [row('a', 'pc', 'steam')];
    const names = availableStorefronts(pc, rows, 'b').map((s) => s.name);
    expect(names).toEqual(['epic', 'gog']); // steam taken by sibling "a"
  });

  it('keeps the current row’s own storefront selectable', () => {
    const rows = [row('a', 'pc', 'steam'), row('b', 'pc', 'epic')];
    const names = availableStorefronts(pc, rows, 'b').map((s) => s.name);
    expect(names).toContain('epic'); // own value retained
    expect(names).not.toContain('steam'); // sibling's value omitted
    expect(names).toContain('gog');
  });

  it('a sibling using the No-storefront slot does not remove any named storefront', () => {
    const rows = [row('a', 'pc', undefined)];
    const names = availableStorefronts(pc, rows, 'b').map((s) => s.name);
    expect(names).toEqual(['steam', 'epic', 'gog']);
  });
});

describe('isPlatformExhausted', () => {
  it('is false while a slot remains free', () => {
    const rows = [row('a', 'pc', 'steam'), row('b', 'pc', 'epic')];
    expect(isPlatformExhausted(pc, rows)).toBe(false); // gog + No-storefront still free
  });

  it('is true once every slot (storefronts + No-storefront) is taken', () => {
    const rows = [
      row('a', 'pc', 'steam'),
      row('b', 'pc', 'epic'),
      row('c', 'pc', 'gog'),
      row('d', 'pc', undefined),
    ];
    expect(isPlatformExhausted(pc, rows)).toBe(true);
  });

  it('a storefront-less platform has exactly one slot', () => {
    expect(isPlatformExhausted(switchConsole, [])).toBe(false);
    expect(isPlatformExhausted(switchConsole, [row('a', 'switch', undefined)])).toBe(true);
  });
});

describe('firstFreeStorefront', () => {
  it('prefers the default storefront when free', () => {
    expect(firstFreeStorefront(pc, [])).toBe('steam');
  });

  it('falls back to the first free storefront when the default is taken', () => {
    expect(firstFreeStorefront(pc, [row('a', 'pc', 'steam')])).toBe('epic');
  });

  it('returns undefined (No storefront) when every named storefront is taken', () => {
    const rows = [row('a', 'pc', 'steam'), row('b', 'pc', 'epic'), row('c', 'pc', 'gog')];
    expect(firstFreeStorefront(pc, rows)).toBeUndefined();
  });

  it('returns undefined for a storefront-less platform', () => {
    expect(firstFreeStorefront(switchConsole, [])).toBeUndefined();
  });
});
