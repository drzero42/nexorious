import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import {
  setupFetchMock,
  resetFetchMock,
  mockConfig
} from '../../../test-utils/api-mocks';
import { resetStoresMocks } from '../../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../../test-utils/auth-mocks';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock the auth module
vi.mock('$lib/stores/auth.svelte', () => ({
  auth: {
    value: {
      accessToken: 'test-token',
      user: { id: '1', username: 'testuser' }
    }
  }
}));

// Mock navigation - define inside factory to avoid hoisting issues
vi.mock('$app/navigation', () => ({
  goto: vi.fn()
}));

// Mock RouteGuard
vi.mock('$lib/components/RouteGuard.svelte', () => {
  return import('../../../test-utils/MockRouteGuard.svelte');
});

// Mock the components index file
vi.mock('$lib/components', async () => {
  const MockRouteGuard = await import('../../../test-utils/MockRouteGuard.svelte');
  return {
    RouteGuard: MockRouteGuard.default
  };
});

// Mock ui store - define inside factory to avoid hoisting issues
vi.mock('$lib/stores', () => ({
  ui: {
    showSuccess: vi.fn(),
    showError: vi.fn()
  }
}));

// Import component after mocks
import NexoriousImportPage from './+page.svelte';
import { goto } from '$app/navigation';
import { ui } from '$lib/stores';

// Type for mutable mocks
const mockGoto = goto as ReturnType<typeof vi.fn>;
const mockShowSuccess = ui.showSuccess as ReturnType<typeof vi.fn>;
const mockShowError = ui.showError as ReturnType<typeof vi.fn>;

// Helper to create a mock file
function createMockFile(name: string, content: string, type: string): File {
  const blob = new Blob([content], { type });
  return new File([blob], name, { type });
}

describe('Nexorious Import Page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetAuthMocks();
    setupFetchMock();
    setAuthenticatedState();
    mockGoto.mockClear();
    mockShowSuccess.mockClear();
    mockShowError.mockClear();
  });

  describe('Core Rendering', () => {
    it('should render the page title', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('Nexorious JSON Import')).toBeInTheDocument();
    });

    it('should render the page description', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('Restore your game collection from a Nexorious export file')).toBeInTheDocument();
    });

    it('should set document title correctly', () => {
      render(NexoriousImportPage);

      const titleElement = document.querySelector('title');
      expect(titleElement?.textContent).toBe('Nexorious Import - Nexorious');
    });

    it('should have breadcrumb navigation', () => {
      render(NexoriousImportPage);

      expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('href', '/dashboard');
      expect(screen.getByRole('link', { name: 'Import' })).toHaveAttribute('href', '/import');
    });
  });

  describe('File Upload Zone', () => {
    it('should render file upload section', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('Upload Export File')).toBeInTheDocument();
      expect(screen.getByText('Drop your export file here')).toBeInTheDocument();
      expect(screen.getByText('or click to browse')).toBeInTheDocument();
    });

    it('should have file input accepting JSON', () => {
      render(NexoriousImportPage);

      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
      expect(fileInput).toBeInTheDocument();
      expect(fileInput.accept).toBe('.json,application/json');
    });

    it('should have disabled start import button when no file selected', () => {
      render(NexoriousImportPage);

      const startButton = screen.getByRole('button', { name: /Start Import/i });
      expect(startButton).toBeDisabled();
    });
  });

  describe('File Selection', () => {
    it('should display selected file name', async () => {
      render(NexoriousImportPage);

      const file = createMockFile('test-export.json', '{"games": []}', 'application/json');
      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

      await fireEvent.change(fileInput, { target: { files: [file] } });

      expect(screen.getByText('test-export.json')).toBeInTheDocument();
    });

    it('should enable upload button when valid file is selected', async () => {
      render(NexoriousImportPage);

      const file = createMockFile('test-export.json', '{"games": []}', 'application/json');
      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

      await fireEvent.change(fileInput, { target: { files: [file] } });

      const startButton = screen.getByRole('button', { name: /Start Import/i });
      expect(startButton).not.toBeDisabled();
    });

    it('should show error for non-JSON file', async () => {
      render(NexoriousImportPage);

      const file = createMockFile('test.txt', 'not json', 'text/plain');
      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

      await fireEvent.change(fileInput, { target: { files: [file] } });

      expect(screen.getByText('Please select a JSON file')).toBeInTheDocument();
    });

    it('should allow clearing file selection', async () => {
      render(NexoriousImportPage);

      const file = createMockFile('test-export.json', '{"games": []}', 'application/json');
      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

      await fireEvent.change(fileInput, { target: { files: [file] } });
      expect(screen.getByText('test-export.json')).toBeInTheDocument();

      const removeButton = screen.getByText('Remove file');
      await fireEvent.click(removeButton);

      expect(screen.queryByText('test-export.json')).not.toBeInTheDocument();
      expect(screen.getByText('Drop your export file here')).toBeInTheDocument();
    });
  });

  describe('File Upload', () => {
    it('should upload file and navigate to job page on success', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({
          job_id: 'test-job-123',
          total_items: 50
        })
      });

      render(NexoriousImportPage);

      const file = createMockFile('test-export.json', '{"games": []}', 'application/json');
      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

      await fireEvent.change(fileInput, { target: { files: [file] } });

      const startButton = screen.getByRole('button', { name: /Start Import/i });
      await fireEvent.click(startButton);

      await waitFor(() => {
        expect(mockShowSuccess).toHaveBeenCalledWith('Import started! Processing 50 games.');
        expect(mockGoto).toHaveBeenCalledWith('/jobs/test-job-123');
      });
    });

    it('should show error on upload failure', async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        statusText: 'Bad Request',
        json: () => Promise.resolve({ detail: 'Invalid JSON format' })
      });

      render(NexoriousImportPage);

      const file = createMockFile('test-export.json', '{"games": []}', 'application/json');
      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

      await fireEvent.change(fileInput, { target: { files: [file] } });

      const startButton = screen.getByRole('button', { name: /Start Import/i });
      await fireEvent.click(startButton);

      await waitFor(() => {
        expect(mockShowError).toHaveBeenCalledWith('Invalid JSON format');
      });
    });

    it('should show error on network failure', async () => {
      global.fetch = vi.fn().mockRejectedValue(new Error('Network error'));

      render(NexoriousImportPage);

      const file = createMockFile('test-export.json', '{"games": []}', 'application/json');
      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

      await fireEvent.change(fileInput, { target: { files: [file] } });

      const startButton = screen.getByRole('button', { name: /Start Import/i });
      await fireEvent.click(startButton);

      await waitFor(() => {
        expect(mockShowError).toHaveBeenCalledWith('Network error');
      });
    });
  });

  describe('Instructions Section', () => {
    it('should display how it works section', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('How It Works')).toBeInTheDocument();
    });

    it('should display step 1: Export from Nexorious', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('Export from Nexorious')).toBeInTheDocument();
      expect(screen.getByText(/Go to Settings/)).toBeInTheDocument();
    });

    it('should display step 2: Upload Your Export', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('Upload Your Export')).toBeInTheDocument();
      expect(screen.getByText(/Drag and drop your JSON file/)).toBeInTheDocument();
    });

    it('should display step 3: Automatic Restoration', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('Automatic Restoration')).toBeInTheDocument();
      expect(screen.getByText(/Your games will be restored/)).toBeInTheDocument();
    });

    it('should display non-interactive import info box', () => {
      render(NexoriousImportPage);

      expect(screen.getByText('Non-Interactive Import')).toBeInTheDocument();
      expect(screen.getByText(/Nexorious exports include trusted IGDB IDs/)).toBeInTheDocument();
    });
  });

  describe('Drag and Drop', () => {
    it('should handle drag enter event', async () => {
      render(NexoriousImportPage);

      const dropZone = screen.getByText('Drop your export file here').closest('[role="button"]');
      expect(dropZone).toBeInTheDocument();

      // Trigger drag enter
      await fireEvent.dragEnter(dropZone!, {
        dataTransfer: { files: [] }
      });

      // Check for visual feedback (indigo border class)
      expect(dropZone).toHaveClass('border-indigo-500');
    });

    it('should handle drag leave event', async () => {
      render(NexoriousImportPage);

      const dropZone = screen.getByText('Drop your export file here').closest('[role="button"]');
      expect(dropZone).toBeInTheDocument();

      await fireEvent.dragEnter(dropZone!, {
        dataTransfer: { files: [] }
      });
      await fireEvent.dragLeave(dropZone!, {
        dataTransfer: { files: [] }
      });

      expect(dropZone).not.toHaveClass('border-indigo-500');
    });
  });
});
