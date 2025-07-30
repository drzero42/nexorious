import { describe, it, expect } from 'vitest';
import { formatOwnershipStatus } from './format-utils';

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