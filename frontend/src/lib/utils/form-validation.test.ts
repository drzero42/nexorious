import { describe, it, expect } from 'vitest';
import {
  validatePersonalRating,
  validateHoursPlayed,
  validateStoreUrl,
  validatePlatformSelection,
  validatePersonalNotes,
  validateField,
  validateForm,
  getFieldError,
  hasFieldError
} from './form-validation';

describe('form-validation', () => {
  describe('validatePersonalRating', () => {
    it('should return null for valid ratings', () => {
      expect(validatePersonalRating(1)).toBeNull();
      expect(validatePersonalRating(3)).toBeNull();
      expect(validatePersonalRating(5)).toBeNull();
    });

    it('should return null for null/undefined (optional field)', () => {
      expect(validatePersonalRating(null)).toBeNull();
      expect(validatePersonalRating(undefined)).toBeNull();
    });

    it('should return error for ratings out of range', () => {
      expect(validatePersonalRating(0)).toBe('Rating must be between 1 and 5 stars');
      expect(validatePersonalRating(6)).toBe('Rating must be between 1 and 5 stars');
      expect(validatePersonalRating(-1)).toBe('Rating must be between 1 and 5 stars');
    });

    it('should return error for non-integer ratings', () => {
      expect(validatePersonalRating(3.5)).toBe('Rating must be a whole number');
      expect(validatePersonalRating(2.1)).toBe('Rating must be a whole number');
    });
  });

  describe('validateHoursPlayed', () => {
    it('should return null for valid hours', () => {
      expect(validateHoursPlayed(0)).toBeNull();
      expect(validateHoursPlayed(10.5)).toBeNull();
      expect(validateHoursPlayed(100)).toBeNull();
      expect(validateHoursPlayed(999.99)).toBeNull();
    });

    it('should return error for negative hours', () => {
      expect(validateHoursPlayed(-1)).toBe('Hours played cannot be negative');
      expect(validateHoursPlayed(-0.1)).toBe('Hours played cannot be negative');
    });

    it('should return error for invalid numbers', () => {
      expect(validateHoursPlayed(Infinity)).toBe('Hours played must be a valid number');
      expect(validateHoursPlayed(NaN)).toBe('Hours played must be a valid number');
    });

    it('should return error for unrealistic hours', () => {
      expect(validateHoursPlayed(10001)).toBe('Hours played seems unrealistic (max 10,000 hours)');
    });
  });

  describe('validateStoreUrl', () => {
    it('should return null for empty/undefined URLs (optional field)', () => {
      expect(validateStoreUrl('')).toBeNull();
      expect(validateStoreUrl('   ')).toBeNull();
      expect(validateStoreUrl(undefined)).toBeNull();
    });

    it('should return null for valid HTTPS URLs', () => {
      expect(validateStoreUrl('https://store.steampowered.com/app/12345')).toBeNull();
      expect(validateStoreUrl('https://www.epicgames.com/store/p/game')).toBeNull();
      expect(validateStoreUrl('https://gog.com/game/test')).toBeNull();
    });

    it('should return error for invalid URL format', () => {
      expect(validateStoreUrl('not-a-url')).toBe('Please enter a valid URL (e.g., https://store.steampowered.com/app/12345)');
      expect(validateStoreUrl('invalid url with spaces')).toBe('Please enter a valid URL (e.g., https://store.steampowered.com/app/12345)');
    });

    it('should return error for valid URLs with non-HTTPS protocols', () => {
      expect(validateStoreUrl('steam://app/12345')).toBe('Store URLs should use HTTPS for security');
    });

    it('should return error for non-HTTPS URLs', () => {
      expect(validateStoreUrl('http://store.steampowered.com/app/12345')).toBe('Store URLs should use HTTPS for security');
      expect(validateStoreUrl('ftp://example.com/game')).toBe('Store URLs should use HTTPS for security');
    });
  });

  describe('validatePlatformSelection', () => {
    it('should return null when ownership status is not "owned"', () => {
      expect(validatePlatformSelection('borrowed', 0)).toBeNull();
      expect(validatePlatformSelection('no_longer_owned', 0)).toBeNull();
      expect(validatePlatformSelection('subscription', 0)).toBeNull();
    });

    it('should return null when ownership status is "owned" and platforms exist', () => {
      expect(validatePlatformSelection('owned', 1)).toBeNull();
      expect(validatePlatformSelection('owned', 3)).toBeNull();
    });

    it('should return error when ownership status is "owned" but no platforms', () => {
      const error = validatePlatformSelection('owned', 0);
      expect(error).toBe('At least one platform is required when ownership status is "Owned"');
    });
  });

  describe('validatePersonalNotes', () => {
    it('should return null for empty/undefined notes (optional field)', () => {
      expect(validatePersonalNotes('')).toBeNull();
      expect(validatePersonalNotes('   ')).toBeNull();
      expect(validatePersonalNotes(undefined)).toBeNull();
    });

    it('should return null for normal length notes', () => {
      expect(validatePersonalNotes('Short note')).toBeNull();
      expect(validatePersonalNotes('A'.repeat(1000))).toBeNull();
      expect(validatePersonalNotes('A'.repeat(9999))).toBeNull();
    });

    it('should return error for notes that are too long', () => {
      const longNotes = 'A'.repeat(10001);
      expect(validatePersonalNotes(longNotes)).toBe('Personal notes must be no more than 10,000 characters');
    });
  });

  describe('validateField', () => {
    it('should validate required fields', () => {
      const result = validateField('test_field', null, { required: true });
      expect(result).toEqual({
        field: 'test_field',
        message: 'test_field is required'
      });
    });

    it('should skip validation for optional empty fields', () => {
      const result = validateField('test_field', null, { required: false });
      expect(result).toBeNull();
    });

    it('should validate numeric ranges', () => {
      const result = validateField('test_field', 5, { min: 10 });
      expect(result).toEqual({
        field: 'test_field',
        message: 'test_field must be at least 10'
      });
    });

    it('should validate string lengths', () => {
      const result = validateField('test_field', 'ab', { min: 5 });
      expect(result).toEqual({
        field: 'test_field',
        message: 'test_field must be at least 5 characters'
      });
    });

    it('should use custom validator', () => {
      const customValidator = (value: any) => value === 'invalid' ? 'Custom error' : null;
      const result = validateField('test_field', 'invalid', { customValidator });
      expect(result).toEqual({
        field: 'test_field',
        message: 'Custom error'
      });
    });

    it('should validate patterns', () => {
      const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
      const result = validateField('email', 'invalid-email', { pattern: emailPattern });
      expect(result).toEqual({
        field: 'email',
        message: 'email format is invalid'
      });
    });
  });

  describe('validateForm', () => {
    it('should validate multiple fields and return all errors', () => {
      const fields = {
        name: { value: '', options: { required: true } },
        age: { value: -1, options: { min: 0 } },
        email: { value: 'valid@email.com', options: { pattern: /^[^\s@]+@[^\s@]+\.[^\s@]+$/ } }
      };

      const result = validateForm(fields);
      expect(result.isValid).toBe(false);
      expect(result.errors).toHaveLength(2);
      expect(result.errors).toContainEqual({
        field: 'name',
        message: 'name is required'
      });
      expect(result.errors).toContainEqual({
        field: 'age',
        message: 'age must be at least 0'
      });
    });

    it('should return valid result when all fields pass', () => {
      const fields = {
        name: { value: 'John', options: { required: true } },
        age: { value: 25, options: { min: 0 } },
        email: { value: 'john@example.com', options: { pattern: /^[^\s@]+@[^\s@]+\.[^\s@]+$/ } }
      };

      const result = validateForm(fields);
      expect(result.isValid).toBe(true);
      expect(result.errors).toHaveLength(0);
    });
  });

  describe('getFieldError', () => {
    const errors = [
      { field: 'field1', message: 'Error 1' },
      { field: 'field2', message: 'Error 2' }
    ];

    it('should return error message for existing field', () => {
      expect(getFieldError(errors, 'field1')).toBe('Error 1');
      expect(getFieldError(errors, 'field2')).toBe('Error 2');
    });

    it('should return null for non-existing field', () => {
      expect(getFieldError(errors, 'field3')).toBeNull();
    });
  });

  describe('hasFieldError', () => {
    const errors = [
      { field: 'field1', message: 'Error 1' },
      { field: 'field2', message: 'Error 2' }
    ];

    it('should return true for fields with errors', () => {
      expect(hasFieldError(errors, 'field1')).toBe(true);
      expect(hasFieldError(errors, 'field2')).toBe(true);
    });

    it('should return false for fields without errors', () => {
      expect(hasFieldError(errors, 'field3')).toBe(false);
    });
  });
});