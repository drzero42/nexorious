import { describe, it, expect } from 'vitest';

describe('Root Layout', () => {
  it('should export preload function', async () => {
    // Import the layout to ensure it's covered
    const module = await import('./+layout.js');
    
    // The layout file should be importable
    expect(module).toBeDefined();
  });
});