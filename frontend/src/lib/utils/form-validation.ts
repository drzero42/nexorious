/**
 * Form validation utilities for game editing and other forms
 */

export interface ValidationError {
  field: string;
  message: string;
}

export interface ValidationResult {
  isValid: boolean;
  errors: ValidationError[];
}

export interface FormValidationOptions {
  required?: boolean;
  min?: number;
  max?: number;
  pattern?: RegExp;
  customValidator?: (value: any) => string | null;
}

/**
 * Validates a single field value
 */
export function validateField(
  fieldName: string, 
  value: any, 
  options: FormValidationOptions = {}
): ValidationError | null {
  const { required = false, min, max, pattern, customValidator } = options;

  // Required validation
  if (required && (value === null || value === undefined || value === '')) {
    return { field: fieldName, message: `${fieldName} is required` };
  }

  // Skip other validations if value is empty/null and not required
  if (!required && (value === null || value === undefined || value === '')) {
    return null;
  }

  // Custom validator takes precedence
  if (customValidator) {
    const error = customValidator(value);
    if (error) {
      return { field: fieldName, message: error };
    }
  }

  // Numeric range validation
  if (typeof value === 'number') {
    if (min !== undefined && value < min) {
      return { field: fieldName, message: `${fieldName} must be at least ${min}` };
    }
    if (max !== undefined && value > max) {
      return { field: fieldName, message: `${fieldName} must be no more than ${max}` };
    }
  }

  // String length validation (treat as character count)
  if (typeof value === 'string') {
    if (min !== undefined && value.length < min) {
      return { field: fieldName, message: `${fieldName} must be at least ${min} characters` };
    }
    if (max !== undefined && value.length > max) {
      return { field: fieldName, message: `${fieldName} must be no more than ${max} characters` };
    }
  }

  // Pattern validation
  if (pattern && typeof value === 'string' && !pattern.test(value)) {
    return { field: fieldName, message: `${fieldName} format is invalid` };
  }

  return null;
}

/**
 * Validates multiple fields
 */
export function validateForm(fields: Record<string, { value: any; options?: FormValidationOptions }>): ValidationResult {
  const errors: ValidationError[] = [];

  for (const [fieldName, { value, options }] of Object.entries(fields)) {
    const error = validateField(fieldName, value, options);
    if (error) {
      errors.push(error);
    }
  }

  return {
    isValid: errors.length === 0,
    errors
  };
}

// Specific validators for common game editing fields

/**
 * Validates personal rating (1-5 stars)
 */
export function validatePersonalRating(rating: number | null | undefined): string | null {
  if (rating === null || rating === undefined) {
    return null; // Optional field
  }
  
  if (!Number.isInteger(rating)) {
    return 'Rating must be a whole number';
  }
  
  if (rating < 1 || rating > 5) {
    return 'Rating must be between 1 and 5 stars';
  }
  
  return null;
}

/**
 * Validates hours played
 */
export function validateHoursPlayed(hours: number): string | null {
  if (hours < 0) {
    return 'Hours played cannot be negative';
  }
  
  if (!Number.isFinite(hours)) {
    return 'Hours played must be a valid number';
  }
  
  if (hours > 10000) {
    return 'Hours played seems unrealistic (max 10,000 hours)';
  }
  
  return null;
}

/**
 * Validates store URL
 */
export function validateStoreUrl(url: string | undefined): string | null {
  if (!url || url.trim() === '') {
    return null; // Optional field
  }
  
  const trimmedUrl = url.trim();
  
  // Basic URL format validation
  try {
    new URL(trimmedUrl);
  } catch {
    return 'Please enter a valid URL (e.g., https://store.steampowered.com/app/12345)';
  }
  
  // Should be HTTPS for security
  if (!trimmedUrl.startsWith('https://')) {
    return 'Store URLs should use HTTPS for security';
  }
  
  return null;
}

/**
 * Validates platform selection based on ownership status
 */
export function validatePlatformSelection(
  ownershipStatus: string, 
  platformCount: number
): string | null {
  if (ownershipStatus === 'owned' && platformCount === 0) {
    return 'At least one platform is required when ownership status is "Owned"';
  }
  
  return null;
}

/**
 * Validates personal notes length
 */
export function validatePersonalNotes(notes: string | undefined): string | null {
  if (!notes || notes.trim() === '') {
    return null; // Optional field
  }
  
  if (notes.length > 10000) {
    return 'Personal notes must be no more than 10,000 characters';
  }
  
  return null;
}

/**
 * Creates a validation function for form fields
 */
export function createFieldValidator(fieldName: string, options: FormValidationOptions = {}) {
  return (value: any) => validateField(fieldName, value, options);
}

/**
 * Helper to get validation error for a specific field
 */
export function getFieldError(errors: ValidationError[], fieldName: string): string | null {
  const error = errors.find(e => e.field === fieldName);
  return error ? error.message : null;
}

/**
 * Helper to check if a specific field has an error
 */
export function hasFieldError(errors: ValidationError[], fieldName: string): boolean {
  return errors.some(e => e.field === fieldName);
}