import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import type { Snippet } from 'svelte';
import FormField from './FormField.svelte';

// Mock snippet for tests - use type assertion to match Snippet<[]> type
const mockChildren = (() => {}) as Snippet<[]>;

describe('FormField', () => {
  it('should render form field container', () => {
    const { container } = render(FormField, {
      props: {
        label: 'Test Label',
        id: 'test-field',
        children: mockChildren
      }
    });

    expect(container.querySelector('.form-field')).toBeInTheDocument();
  });

  it('should render label with correct text and for attribute', () => {
    render(FormField, {
      props: {
        label: 'Test Label',
        id: 'test-field',
        children: mockChildren
      }
    });

    const label = screen.getByText('Test Label');
    expect(label).toBeInTheDocument();
    expect(label.getAttribute('for')).toBe('test-field');
  });

  it('should show required indicator when required is true', () => {
    render(FormField, {
      props: {
        label: 'Required Field',
        id: 'required-field',
        required: true,
        children: mockChildren
      }
    });

    expect(screen.getByText('*')).toBeInTheDocument();
  });

  it('should show error message when error is provided', () => {
    render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        error: 'This field has an error',
        children: mockChildren
      }
    });

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText('This field has an error')).toBeInTheDocument();
  });

  it('should show help text when provided and no error', () => {
    render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        helpText: 'This is helpful information',
        children: mockChildren
      }
    });

    expect(screen.getByText('This is helpful information')).toBeInTheDocument();
  });

  it('should show dirty indicator when isDirty is true', () => {
    render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        isDirty: true,
        children: mockChildren
      }
    });

    expect(screen.getByText('(modified)')).toBeInTheDocument();
  });

  it('should not show help text when error is present', () => {
    render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        error: 'This is an error',
        helpText: 'This is help text',
        children: mockChildren
      }
    });

    expect(screen.getByText('This is an error')).toBeInTheDocument();
    expect(screen.queryByText('This is help text')).not.toBeInTheDocument();
  });

  it('should apply error styling class when error is present', () => {
    const { container } = render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        error: 'Error message',
        children: mockChildren
      }
    });

    const fieldInput = container.querySelector('.form-field-input');
    expect(fieldInput).toHaveClass('form-field-error');
  });

  it('should not show dirty indicator when error is present', () => {
    render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        error: 'This is an error',
        isDirty: true,
        children: mockChildren
      }
    });

    expect(screen.getByText('This is an error')).toBeInTheDocument();
    expect(screen.queryByText('(modified)')).not.toBeInTheDocument();
  });

  it('should apply custom class when provided', () => {
    const { container } = render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        class: 'custom-class',
        children: mockChildren
      }
    });

    const formField = container.querySelector('.form-field');
    expect(formField).toHaveClass('custom-class');
  });

  it('should render form-field-input container', () => {
    const { container } = render(FormField, {
      props: {
        label: 'Test Field',
        id: 'test-field',
        children: mockChildren
      }
    });

    expect(container.querySelector('.form-field-input')).toBeInTheDocument();
  });
});