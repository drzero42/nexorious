import { describe, it, expect, vi } from 'vitest';
import { buildIconUrl, getPlatformFallbackIcon, getStorefrontFallbackIcon } from './icon-utils';

// Mock the config
vi.mock('$lib/env', () => ({
  config: {
    staticUrl: 'http://localhost:8000'
  }
}));

describe('Icon Utils', () => {
  describe('buildIconUrl', () => {
    it('should build correct URL with leading slash path', () => {
      const result = buildIconUrl('/static/logos/platforms/pc-windows/pc-windows-icon-light.svg');
      expect(result).toBe('http://localhost:8000/static/logos/platforms/pc-windows/pc-windows-icon-light.svg');
    });

    it('should build correct URL without leading slash path', () => {
      const result = buildIconUrl('static/logos/platforms/pc-windows/pc-windows-icon-light.svg');
      expect(result).toBe('http://localhost:8000/static/logos/platforms/pc-windows/pc-windows-icon-light.svg');
    });

    it('should return null for empty string', () => {
      const result = buildIconUrl('');
      expect(result).toBeNull();
    });

    it('should return null for null input', () => {
      const result = buildIconUrl(null);
      expect(result).toBeNull();
    });

    it('should return null for undefined input', () => {
      const result = buildIconUrl(undefined);
      expect(result).toBeNull();
    });

    it('should handle whitespace-only string', () => {
      const result = buildIconUrl('   ');
      expect(result).toBeNull();
    });
  });

  describe('fallback icons', () => {
    it('should return platform fallback icon', () => {
      const result = getPlatformFallbackIcon();
      expect(result).toBe('🎮');
    });

    it('should return storefront fallback icon', () => {
      const result = getStorefrontFallbackIcon();
      expect(result).toBe('🏪');
    });
  });
});