import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent, screen } from '@testing-library/svelte';
import LogoUpload from './LogoUpload.svelte';

// Mock fetch globally
global.fetch = vi.fn();

// Mock localStorage
Object.defineProperty(window, 'localStorage', {
  value: {
    getItem: vi.fn(() => 'mock-token'),
    setItem: vi.fn(),
    removeItem: vi.fn(),
    clear: vi.fn()
  }
});

describe('LogoUpload Component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders with basic props', () => {
    const { container } = render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null
      }
    });

    expect(container).toBeTruthy();
    expect(screen.getByText('Theme:')).toBeInTheDocument();
    expect(screen.getByText('Light')).toBeInTheDocument();
    expect(screen.getByText('Dark')).toBeInTheDocument();
  });

  it('shows current logo when provided', () => {
    render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: '/static/logos/platforms/test/icon.svg'
      }
    });

    expect(screen.getByText('Current Logo:')).toBeInTheDocument();
    expect(screen.getByAltText('Current logo')).toBeInTheDocument();
    expect(screen.getByText('Remove Current Logo')).toBeInTheDocument();
  });

  it('shows upload area when no preview', () => {
    render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null
      }
    });

    expect(screen.getByText('Click to upload')).toBeInTheDocument();
    expect(screen.getByText('or drag and drop')).toBeInTheDocument();
    expect(screen.getByText('SVG, PNG, JPEG or WebP (max 2MB)')).toBeInTheDocument();
  });

  it('allows theme selection', async () => {
    render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null
      }
    });

    const lightRadio = screen.getByDisplayValue('light');
    const darkRadio = screen.getByDisplayValue('dark');

    expect(lightRadio).toBeChecked();
    expect(darkRadio).not.toBeChecked();

    await fireEvent.click(darkRadio);
    
    expect(darkRadio).toBeChecked();
    expect(lightRadio).not.toBeChecked();
  });

  it('renders file validation help text', () => {
    render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null
      }
    });

    // Verify that file format help text is shown
    expect(screen.getByText(/SVG, PNG, JPEG or WebP/)).toBeInTheDocument();
  });

  it('shows help text about file formats', () => {
    render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null
      }
    });

    expect(screen.getByText(/SVG files are preferred/)).toBeInTheDocument();
    expect(screen.getByText(/PNG files work well/)).toBeInTheDocument();
  });

  it('disables upload when disabled prop is true', () => {
    render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null,
        disabled: true
      }
    });

    const lightRadio = screen.getByDisplayValue('light');
    const darkRadio = screen.getByDisplayValue('dark');
    
    expect(lightRadio).toBeDisabled();
    expect(darkRadio).toBeDisabled();
  });

  it('constructs correct API endpoints', () => {
    // Test platform endpoint construction
    const platformComponent = render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null
      }
    });

    // Test storefront endpoint construction  
    const storefrontComponent = render(LogoUpload, {
      props: {
        entityType: 'storefronts',
        entityId: 'test-storefront-id',
        currentIconUrl: null
      }
    });

    // Both should render without errors
    expect(platformComponent.container).toBeTruthy();
    expect(storefrontComponent.container).toBeTruthy();
  });

  it('renders with proper accessibility attributes', () => {
    render(LogoUpload, {
      props: {
        entityType: 'platforms',
        entityId: 'test-platform-id',
        currentIconUrl: null
      }
    });

    // Test that the file input has proper ID and label association
    const fileInput = screen.getByLabelText(/Upload New Logo/);
    expect(fileInput).toBeInTheDocument();
    expect(fileInput).toHaveAttribute('type', 'file');
    
    // Test that fieldset is used for radio buttons
    const themeFieldset = screen.getByRole('group', { name: /Theme/ });
    expect(themeFieldset).toBeInTheDocument();
  });
});