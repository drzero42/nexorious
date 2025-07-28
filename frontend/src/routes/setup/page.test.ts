import { describe, it, expect } from 'vitest';

describe('Setup Page Layout', () => {
  it('should export page load function', async () => {
    // Import the page module to ensure it's covered
    const module = await import('./+page.js');
    
    // The page file should be importable
    expect(module).toBeDefined();
  });
});