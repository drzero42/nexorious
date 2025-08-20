import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import DarkadiaFileUpload from './DarkadiaFileUpload.svelte';

// Mock the darkadia store
vi.mock('$lib/stores/darkadia.svelte', () => ({
  darkadia: {
    value: {
      uploadState: {
        isDragging: false,
        isUploading: false,
        isImporting: false,
        uploadProgress: 0,
        importProgress: 0,
        uploadedFile: null,
        uploadResult: null,
        error: null
      }
    },
    uploadCSV: vi.fn().mockResolvedValue({
      message: 'Success',
      file_id: 'test-id',
      total_games: 100,
      file_path: '/tmp/test.csv',
      file_size: 1024,
      preview_games: []
    })
  }
}));

describe('DarkadiaFileUpload', () => {
  describe('Initial State', () => {
    it('renders default upload area', () => {
      render(DarkadiaFileUpload);
      
      expect(screen.getByText('Upload Darkadia CSV')).toBeInTheDocument();
      expect(screen.getByText('Click here or drag and drop your Darkadia export file')).toBeInTheDocument();
      expect(screen.getByText(/Only CSV files are accepted/)).toBeInTheDocument();
      expect(screen.getByText(/Maximum file size: 10MB/)).toBeInTheDocument();
      expect(screen.getByText(/File will be automatically imported after upload/)).toBeInTheDocument();
    });

    it('has proper accessibility attributes', () => {
      render(DarkadiaFileUpload);
      
      const uploadArea = screen.getByRole('button', { name: 'Upload CSV file' });
      expect(uploadArea).toHaveAttribute('tabindex', '0');
      expect(uploadArea).toHaveAttribute('aria-label', 'Upload CSV file');
    });

    it('renders with custom class name', () => {
      const { container } = render(DarkadiaFileUpload, {
        props: { class: 'custom-class' }
      });
      
      expect(container.querySelector('.custom-class')).toBeInTheDocument();
    });

    it('applies disabled styling when disabled', () => {
      render(DarkadiaFileUpload, {
        props: { disabled: true }
      });
      
      const uploadArea = screen.getByRole('button', { name: 'Upload CSV file' });
      expect(uploadArea).toHaveClass('opacity-50');
      expect(uploadArea).toHaveClass('cursor-not-allowed');
    });

    it('applies normal styling when not disabled', () => {
      render(DarkadiaFileUpload);
      
      const uploadArea = screen.getByRole('button', { name: 'Upload CSV file' });
      expect(uploadArea).toHaveClass('cursor-pointer');
      expect(uploadArea).not.toHaveClass('opacity-50');
    });
  });

  describe('File Input', () => {
    it('renders hidden file input with correct attributes', () => {
      const { container } = render(DarkadiaFileUpload);
      
      const fileInput = container.querySelector('input[type="file"]');
      expect(fileInput).toBeInTheDocument();
      expect(fileInput).toHaveAttribute('accept', '.csv,text/csv,application/csv');
      expect(fileInput).toHaveAttribute('aria-hidden', 'true');
      expect(fileInput).toHaveClass('hidden');
    });

    it('disables file input when component is disabled', () => {
      const { container } = render(DarkadiaFileUpload, {
        props: { disabled: true }
      });
      
      const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
      expect(fileInput).toBeDisabled();
    });

    it('enables file input when component is not disabled', () => {
      const { container } = render(DarkadiaFileUpload);
      
      const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
      expect(fileInput).not.toBeDisabled();
    });
  });

  describe('Props', () => {
    it('accepts onUploadComplete callback prop', () => {
      const onUploadComplete = vi.fn();
      
      expect(() => {
        render(DarkadiaFileUpload, {
          props: { onUploadComplete }
        });
      }).not.toThrow();
    });

    it('accepts onUploadStart callback prop', () => {
      const onUploadStart = vi.fn();
      
      expect(() => {
        render(DarkadiaFileUpload, {
          props: { onUploadStart }
        });
      }).not.toThrow();
    });

    it('accepts onUploadError callback prop', () => {
      const onUploadError = vi.fn();
      
      expect(() => {
        render(DarkadiaFileUpload, {
          props: { onUploadError }
        });
      }).not.toThrow();
    });

    it('accepts disabled prop', () => {
      expect(() => {
        render(DarkadiaFileUpload, {
          props: { disabled: true }
        });
      }).not.toThrow();
    });

    it('accepts class prop', () => {
      expect(() => {
        render(DarkadiaFileUpload, {
          props: { class: 'custom-styles' }
        });
      }).not.toThrow();
    });
  });

  describe('Visual Elements', () => {
    it('displays upload icon in default state', () => {
      render(DarkadiaFileUpload);
      
      const iconContainer = screen.getByText('Upload Darkadia CSV').parentElement?.parentElement;
      expect(iconContainer?.querySelector('svg')).toBeInTheDocument();
    });

    it('displays informational text about file requirements', () => {
      render(DarkadiaFileUpload);
      
      // Check for file requirement information
      expect(screen.getByText(/Only CSV files are accepted/)).toBeInTheDocument();
      expect(screen.getByText(/Maximum file size: 10MB/)).toBeInTheDocument();
      expect(screen.getByText(/File will be automatically imported after upload/)).toBeInTheDocument();
    });

    it('uses proper responsive container classes', () => {
      const { container } = render(DarkadiaFileUpload);
      
      const mainContainer = container.firstChild as HTMLElement;
      expect(mainContainer).toHaveClass('w-full');
      expect(mainContainer).toHaveClass('max-w-2xl');
      expect(mainContainer).toHaveClass('mx-auto');
    });

    it('applies proper drag and drop styling classes', () => {
      render(DarkadiaFileUpload);
      
      const uploadArea = screen.getByRole('button', { name: 'Upload CSV file' });
      expect(uploadArea).toHaveClass('border-2');
      expect(uploadArea).toHaveClass('border-dashed');
      expect(uploadArea).toHaveClass('rounded-xl');
      expect(uploadArea).toHaveClass('transition-all');
      expect(uploadArea).toHaveClass('duration-200');
    });
  });

  describe('Component Structure', () => {
    it('maintains proper component hierarchy', () => {
      const { container } = render(DarkadiaFileUpload);
      
      // Check main container exists
      expect(container.firstChild).toHaveClass('w-full');
      
      // Check upload area exists within container
      const uploadArea = container.querySelector('[role="button"]');
      expect(uploadArea).toBeInTheDocument();
      
      // Check file input is present
      const fileInput = container.querySelector('input[type="file"]');
      expect(fileInput).toBeInTheDocument();
    });

    it('has correct ARIA structure for accessibility', () => {
      const { container } = render(DarkadiaFileUpload);
      
      const uploadButton = screen.getByRole('button', { name: 'Upload CSV file' });
      expect(uploadButton).toBeInTheDocument();
      
      const fileInput = container.querySelector('input[type="file"]');
      expect(fileInput).toHaveAttribute('aria-hidden', 'true');
    });
  });

  describe('Content Display', () => {
    it('shows correct title text', () => {
      render(DarkadiaFileUpload);
      
      expect(screen.getByRole('heading', { level: 3 })).toHaveTextContent('Upload Darkadia CSV');
    });

    it('shows correct description text', () => {
      render(DarkadiaFileUpload);
      
      expect(screen.getByText('Click here or drag and drop your Darkadia export file')).toBeInTheDocument();
    });

    it('displays file requirements clearly', () => {
      render(DarkadiaFileUpload);
      
      const requirements = [
        /Only CSV files are accepted/,
        /Maximum file size: 10MB/,
        /File will be automatically imported after upload/
      ];

      requirements.forEach(requirement => {
        expect(screen.getByText(requirement)).toBeInTheDocument();
      });
    });
  });
});