import { describe, it, expect } from 'vitest';
import { buildDealLinks } from './deal-links';

describe('buildDealLinks', () => {
  it('builds ITAD (region-agnostic) and psprices (region-scoped) URLs', () => {
    const { itad, psprices } = buildDealLinks('Hades', 'us');
    expect(itad).toBe('https://isthereanydeal.com/search/?q=Hades');
    expect(psprices).toBe('https://psprices.com/region-us/games/?q=Hades');
  });

  it('uses the supplied deal region for psprices', () => {
    expect(buildDealLinks('Hades', 'gb').psprices).toBe(
      'https://psprices.com/region-gb/games/?q=Hades',
    );
  });

  it('url-encodes the title', () => {
    const { itad, psprices } = buildDealLinks('Ori & the Blind Forest', 'jp');
    expect(itad).toContain('?q=Ori%20%26%20the%20Blind%20Forest');
    expect(psprices).toContain('region-jp/games/?q=Ori%20%26%20the%20Blind%20Forest');
  });

  it('falls back to the default region when none is given', () => {
    expect(buildDealLinks('Hades').psprices).toContain('region-us/');
  });
});
