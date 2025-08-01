import { describe, it, expect } from 'vitest';
import { formatOwnershipStatus, formatIgdbRating } from './format-utils';

describe('formatOwnershipStatus', () => {
  it('should format owned status correctly', () => {
    expect(formatOwnershipStatus('owned')).toBe('Owned');
  });

  it('should format borrowed status correctly', () => {
    expect(formatOwnershipStatus('borrowed')).toBe('Borrowed');
  });

  it('should format rented status correctly', () => {
    expect(formatOwnershipStatus('rented')).toBe('Rented');
  });

  it('should format subscription status correctly', () => {
    expect(formatOwnershipStatus('subscription')).toBe('Subscription');
  });

  it('should format no_longer_owned status correctly', () => {
    expect(formatOwnershipStatus('no_longer_owned')).toBe('No Longer Owned');
  });

  it('should handle multi-word underscored strings', () => {
    expect(formatOwnershipStatus('test_multiple_words')).toBe('Test Multiple Words');
  });

  it('should handle single words correctly', () => {
    expect(formatOwnershipStatus('single')).toBe('Single');
  });

  it('should handle empty string', () => {
    expect(formatOwnershipStatus('')).toBe('');
  });

  it('should handle mixed case correctly', () => {
    expect(formatOwnershipStatus('MiXeD_CaSe')).toBe('Mixed Case');
  });
});

describe('formatIgdbRating', () => {
  describe('valid inputs', () => {
    it('should convert integer ratings correctly', () => {
      expect(formatIgdbRating(85)).toBe(8.5);
      expect(formatIgdbRating(75)).toBe(7.5);
      expect(formatIgdbRating(90)).toBe(9.0);
    });

    it('should handle boundary values correctly', () => {
      expect(formatIgdbRating(0)).toBe(0.0);
      expect(formatIgdbRating(100)).toBe(10.0);
    });

    it('should handle decimal inputs by converting to valid range', () => {
      expect(formatIgdbRating(85.7)).toBe(8.57);
      expect(formatIgdbRating(92.3)).toBe(9.23);
    });

    it('should handle typical IGDB rating values', () => {
      expect(formatIgdbRating(73)).toBe(7.3); // Good game
      expect(formatIgdbRating(86)).toBe(8.6); // Great game
      expect(formatIgdbRating(91)).toBe(9.1); // Excellent game
      expect(formatIgdbRating(45)).toBe(4.5); // Average game
    });
  });

  describe('edge cases and validation', () => {
    it('should clamp out-of-range values', () => {
      expect(formatIgdbRating(150)).toBe(10.0); // Above max
      expect(formatIgdbRating(-20)).toBe(0.0);  // Below min
      expect(formatIgdbRating(101)).toBe(10.0); // Just above max
      expect(formatIgdbRating(-5)).toBe(0.0);   // Just below min
    });

    it('should handle null and undefined inputs', () => {
      expect(formatIgdbRating(null)).toBeNull();
      expect(formatIgdbRating(undefined)).toBeNull();
    });

    it('should handle invalid input types', () => {
      expect(formatIgdbRating('85' as any)).toBeNull();
      expect(formatIgdbRating(true as any)).toBeNull();
      expect(formatIgdbRating([] as any)).toBeNull();
      expect(formatIgdbRating({} as any)).toBeNull();
    });

    it('should handle special number values', () => {
      expect(formatIgdbRating(NaN)).toBeNull();
      expect(formatIgdbRating(Infinity)).toBe(10.0); // Clamped to max
      expect(formatIgdbRating(-Infinity)).toBe(0.0); // Clamped to min
    });
  });

  describe('precision and formatting', () => {
    it('should maintain proper decimal precision', () => {
      expect(formatIgdbRating(83.456)).toBe(8.3456);
      expect(formatIgdbRating(77.123)).toBe(7.7123);
    });

    it('should handle integer values correctly', () => {
      expect(formatIgdbRating(80)).toBe(8.0);
      expect(formatIgdbRating(50)).toBe(5.0);
    });
  });
});