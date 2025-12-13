import { describe, it, expect } from 'vitest';

describe('Stores Index Exports', () => {
  it('should import the stores index module successfully', async () => {
    // Import the stores index module to ensure it's covered
    const module = await import('./index');

    // The module should be importable and defined
    expect(module).toBeDefined();

    // Should have some exports
    const exportKeys = Object.keys(module);
    expect(exportKeys.length).toBeGreaterThan(0);
  });

  it('should export auth store', async () => {
    const { auth } = await import('./auth.svelte');
    expect(auth).toBeDefined();
    expect(typeof auth.login).toBe('function');
    expect(typeof auth.logout).toBe('function');
  });

  it('should export games store', async () => {
    const { games } = await import('./games.svelte');
    expect(games).toBeDefined();
    expect(typeof games.loadGames).toBe('function');
  });

  it('should export platforms store', async () => {
    const { platforms } = await import('./platforms.svelte');
    expect(platforms).toBeDefined();
    expect(platforms.value).toBeDefined();
  });

  it('should export userGames store', async () => {
    const { userGames } = await import('./user-games.svelte');
    expect(userGames).toBeDefined();
    expect(userGames.value).toBeDefined();
  });

  it('should export ui store', async () => {
    const { ui } = await import('./ui.svelte');
    expect(ui).toBeDefined();
    expect(ui.value).toBeDefined();
  });

  it('should export search store', async () => {
    const { search } = await import('./search.svelte');
    expect(search).toBeDefined();
    expect(search.value).toBeDefined();
  });

  it('should export admin store', async () => {
    const { admin } = await import('./admin.svelte');
    expect(admin).toBeDefined();
    // Admin store uses classic Svelte writable store pattern with subscribe
    expect(typeof admin.subscribe).toBe('function');
  });
});
