import { describe, it, expect } from 'vitest';
import {
  formatTtb,
  formatIgdbRating,
  formatHoursPlayed,
  formatPlatformLabel,
  resolveImageUrl,
  getCoverUrl,
  toDateInputValue,
  bestAchievementProgress,
} from './game-utils';
import type { UserGame, UserGamePlatform } from '@/types';

const plat = (over: Partial<UserGamePlatform>): UserGamePlatform =>
  ({
    id: 'x',
    is_available: true,
    hours_played: 0,
    ownership_status: 'owned',
    created_at: '',
    ...over,
  }) as UserGamePlatform;

describe('bestAchievementProgress', () => {
  it('returns null when undefined or empty', () => {
    expect(bestAchievementProgress(undefined)).toBeNull();
    expect(bestAchievementProgress([])).toBeNull();
  });
  it('ignores rows with null or zero total', () => {
    expect(
      bestAchievementProgress([
        plat({}),
        plat({ achievements_total: 0, achievements_unlocked: 0 }),
      ]),
    ).toBeNull();
  });
  it('returns the single qualifying row', () => {
    expect(
      bestAchievementProgress([plat({ achievements_unlocked: 3, achievements_total: 10 })]),
    ).toEqual({ unlocked: 3, total: 10 });
  });
  it('picks the highest unlocked/total ratio', () => {
    const result = bestAchievementProgress([
      plat({ achievements_unlocked: 5, achievements_total: 10 }), // 0.5
      plat({ achievements_unlocked: 9, achievements_total: 10 }), // 0.9
    ]);
    expect(result).toEqual({ unlocked: 9, total: 10 });
  });
});

// config.staticUrl is '' in the test env (VITE_STATIC_URL unset), so the
// origin prefix is empty and relative paths reduce to a leading-slash path.

describe('resolveImageUrl', () => {
  it('returns "" for null/undefined/empty', () => {
    expect(resolveImageUrl(null)).toBe('');
    expect(resolveImageUrl(undefined)).toBe('');
    expect(resolveImageUrl('')).toBe('');
  });

  it('passes absolute http(s) URLs through untouched', () => {
    expect(resolveImageUrl('http://example.com/a.jpg')).toBe('http://example.com/a.jpg');
    expect(resolveImageUrl('https://cdn.example.com/b.png')).toBe('https://cdn.example.com/b.png');
  });

  it('keeps a leading slash on already-rooted relative paths', () => {
    expect(resolveImageUrl('/covers/c.jpg')).toBe('/covers/c.jpg');
  });

  it('adds a leading slash to bare relative paths', () => {
    expect(resolveImageUrl('covers/d.jpg')).toBe('/covers/d.jpg');
  });
});

describe('getCoverUrl', () => {
  it('returns null when the game has no cover art', () => {
    expect(getCoverUrl({ game: {} } as UserGame)).toBeNull();
    expect(getCoverUrl({} as UserGame)).toBeNull();
  });

  it('resolves a present cover_art_url via resolveImageUrl', () => {
    expect(getCoverUrl({ game: { cover_art_url: '/x.jpg' } } as UserGame)).toBe('/x.jpg');
    expect(getCoverUrl({ game: { cover_art_url: 'https://e.com/x.jpg' } } as UserGame)).toBe(
      'https://e.com/x.jpg',
    );
  });
});

describe('formatHoursPlayed', () => {
  it.each([
    [0, '0h'],
    [null, '0h'],
    [undefined, '0h'],
    [1.2, '1h'],
    [1.3, '1.5h'],
    [7.4, '7.5h'],
    [9.74, '9.5h'],
    [9.75, '10h'],
    [9.8, '10h'],
    [10, '10h'],
    [30.299999999999997, '30h'],
    [134, '134h'],
  ])('formats %s as %s', (input, expected) => {
    expect(formatHoursPlayed(input as number | null | undefined)).toBe(expected);
  });
});

describe('formatTtb', () => {
  // Same half-hour-bucket rule as formatHoursPlayed; only null handling differs
  // (TTB uses an em-dash placeholder because "no HLTB data" is meaningfully
  // distinct from "0 hours").
  it.each([
    [null, '—'],
    [undefined, '—'],
    [0, '0h'],
    [1.2, '1h'],
    [1.3, '1.5h'],
    [7.4, '7.5h'],
    [9.75, '10h'], // boundary: half-hour rule rounds to exactly 10
    [10, '10h'],
    [12.5, '13h'], // ≥10, integer rule (Math.round half-up)
    [13.14, '13h'], // canonical case from HLTB display in issue #641 follow-up
    [134, '134h'],
  ])('formats %s as %s', (input, expected) => {
    expect(formatTtb(input as number | null | undefined)).toBe(expected);
  });
});

describe('formatIgdbRating', () => {
  it('converts 85.42 to "8.5"', () => {
    expect(formatIgdbRating(85.42)).toBe('8.5');
  });
  it('converts 72.10 to "7.2"', () => {
    expect(formatIgdbRating(72.1)).toBe('7.2');
  });
  it('converts 100 to "10.0"', () => {
    expect(formatIgdbRating(100)).toBe('10.0');
  });
  it('converts 0 to "0.0"', () => {
    expect(formatIgdbRating(0)).toBe('0.0');
  });
  it('returns "—" for null', () => {
    expect(formatIgdbRating(null)).toBe('—');
  });
  it('returns "—" for undefined', () => {
    expect(formatIgdbRating(undefined)).toBe('—');
  });
});

describe('formatPlatformLabel', () => {
  it('returns "Platform (Storefront)" when both details are present', () => {
    expect(
      formatPlatformLabel({
        platform: 'windows',
        storefront: 'gog',
        platform_details: { display_name: 'Windows' },
        storefront_details: { display_name: 'GOG' },
      }),
    ).toBe('Windows (GOG)');
  });

  it('falls back to raw names when details are absent', () => {
    expect(
      formatPlatformLabel({
        platform: 'windows',
        storefront: 'gog',
        platform_details: null,
        storefront_details: null,
      }),
    ).toBe('windows (gog)');
  });

  it('shows only storefront when platform is missing', () => {
    expect(
      formatPlatformLabel({
        platform: null,
        storefront: null,
        platform_details: null,
        storefront_details: { display_name: 'Steam' },
      }),
    ).toBe('Steam');
  });

  it('shows only platform when storefront is missing', () => {
    expect(
      formatPlatformLabel({
        platform: 'linux',
        storefront: null,
        platform_details: { display_name: 'Linux PC' },
        storefront_details: null,
      }),
    ).toBe('Linux PC');
  });

  it('returns "Unknown" when everything is absent', () => {
    expect(
      formatPlatformLabel({
        platform: null,
        storefront: null,
        platform_details: null,
        storefront_details: null,
      }),
    ).toBe('Unknown');
  });
});

describe('toDateInputValue', () => {
  it('extracts the date from an RFC3339 timestamp (the API format)', () => {
    expect(toDateInputValue('2024-06-01T00:00:00Z')).toBe('2024-06-01');
  });

  it('passes a bare YYYY-MM-DD through unchanged', () => {
    expect(toDateInputValue('2024-06-01')).toBe('2024-06-01');
  });

  it('returns "" for null/undefined/empty', () => {
    expect(toDateInputValue(null)).toBe('');
    expect(toDateInputValue(undefined)).toBe('');
    expect(toDateInputValue('')).toBe('');
  });

  it('returns "" for an unparseable value', () => {
    expect(toDateInputValue('not-a-date')).toBe('');
  });
});
