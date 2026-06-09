import { describe, it, expect } from 'vitest';
import { resolveDocHref } from './doc-links';

describe('resolveDocHref', () => {
  it('rewrites a bare sibling .md link to an internal /help slug', () => {
    expect(resolveDocHref('user-guide.md', 'admin-guide')).toEqual({
      type: 'internal',
      slug: 'user-guide',
      hash: undefined,
    });
  });

  it('handles a ./-prefixed sibling link', () => {
    expect(resolveDocHref('./sync.md', 'user-guide')).toEqual({
      type: 'internal',
      slug: 'sync',
      hash: undefined,
    });
  });

  it('preserves an anchor on a sibling .md link', () => {
    expect(resolveDocHref('sync.md#epic-games-store-sync', 'user-guide')).toEqual({
      type: 'internal',
      slug: 'sync',
      hash: '#epic-games-store-sync',
    });
  });

  it('maps a non-embedded ../ .md link to a GitHub source URL', () => {
    expect(resolveDocHref('../DEV.md', 'admin-guide')).toEqual({
      type: 'external',
      value: 'https://github.com/drzero42/nexorious/blob/main/DEV.md',
    });
  });

  it('treats a same-page anchor as an in-page scroll target', () => {
    expect(resolveDocHref('#configuration', 'admin-guide')).toEqual({
      type: 'anchor',
      value: '#configuration',
    });
  });

  it('passes absolute http(s) links through as external', () => {
    expect(resolveDocHref('https://twitch.tv/settings', 'admin-guide')).toEqual({
      type: 'external',
      value: 'https://twitch.tv/settings',
    });
  });

  it('passes mailto: links through as external', () => {
    expect(resolveDocHref('mailto:support@example.com', 'admin-guide')).toEqual({
      type: 'external',
      value: 'mailto:support@example.com',
    });
  });
});
