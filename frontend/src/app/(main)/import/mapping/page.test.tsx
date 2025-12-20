// frontend/src/app/(main)/import/mapping/page.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useRouter, useSearchParams } from 'next/navigation';
import MappingPage from './page';
import {
  usePlatformSummary,
  useAllPlatforms,
  useAllStorefronts,
  useJob,
  useBatchImportMappings,
} from '@/hooks';
import { ImportMappingProvider } from '@/contexts/import-mapping-context';
import { JobSource, JobStatus, JobType, JobPriority } from '@/types';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  useRouter: vi.fn(),
  useSearchParams: vi.fn(),
}));

// Mock hooks
vi.mock('@/hooks', async () => {
  const actual = await vi.importActual('@/hooks');
  return {
    ...actual,
    usePlatformSummary: vi.fn(),
    useAllPlatforms: vi.fn(),
    useAllStorefronts: vi.fn(),
    useJob: vi.fn(),
    useBatchImportMappings: vi.fn(),
  };
});

const mockRouter = {
  push: vi.fn(),
  replace: vi.fn(),
};

const mockPlatformSummary = {
  platforms: [
    { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
    { original: 'PS4', count: 8, suggestedId: null, suggestedName: null },
  ],
  storefronts: [
    { original: 'Steam', count: 10, suggestedId: 'steam', suggestedName: 'Steam' },
    { original: 'Epic', count: 5, suggestedId: null, suggestedName: null },
  ],
  allResolved: false,
};

const mockPlatforms = [
  { id: 'pc-windows', display_name: 'PC (Windows)' },
  { id: 'playstation-4', display_name: 'PlayStation 4' },
];

const mockStorefronts = [
  { id: 'steam', display_name: 'Steam' },
  { id: 'epic-games-store', display_name: 'Epic Games Store' },
];

const mockJob = {
  id: 'test-job-123',
  userId: 'user-1',
  jobType: JobType.IMPORT,
  source: JobSource.DARKADIA,
  status: JobStatus.AWAITING_REVIEW,
  priority: JobPriority.HIGH,
  progressCurrent: 100,
  progressTotal: 100,
  progressPercent: 100,
  resultSummary: {},
  errorMessage: null,
  filePath: null,
  taskiqTaskId: null,
  createdAt: '2024-01-01T00:00:00Z',
  startedAt: '2024-01-01T00:00:00Z',
  completedAt: null,
  isTerminal: false,
  durationSeconds: null,
  reviewItemCount: 10,
  pendingReviewCount: 5,
};

const mockBatchImportMappings = {
  mutateAsync: vi.fn().mockResolvedValue({ created: 2, updated: 2 }),
};

describe('MappingPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (useRouter as ReturnType<typeof vi.fn>).mockReturnValue(mockRouter);
    (useSearchParams as ReturnType<typeof vi.fn>).mockReturnValue(
      new URLSearchParams('job_id=test-job-123')
    );
    (usePlatformSummary as ReturnType<typeof vi.fn>).mockReturnValue({
      data: mockPlatformSummary,
      isLoading: false,
      error: null,
    });
    (useAllPlatforms as ReturnType<typeof vi.fn>).mockReturnValue({
      data: mockPlatforms,
      isLoading: false,
    });
    (useAllStorefronts as ReturnType<typeof vi.fn>).mockReturnValue({
      data: mockStorefronts,
      isLoading: false,
    });
    (useJob as ReturnType<typeof vi.fn>).mockReturnValue({
      data: mockJob,
      isLoading: false,
    });
    (useBatchImportMappings as ReturnType<typeof vi.fn>).mockReturnValue(mockBatchImportMappings);
  });

  const renderWithProvider = () => {
    return render(
      <ImportMappingProvider>
        <MappingPage />
      </ImportMappingProvider>
    );
  };

  it('should show all items including resolved ones when all are auto-matched', async () => {
    // When all items have suggestions, page should still show them (no auto-redirect)
    const allResolvedSummary = {
      platforms: [
        { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
      ],
      storefronts: [
        { original: 'Steam', count: 10, suggestedId: 'steam', suggestedName: 'Steam' },
      ],
      allResolved: true,
    };
    (usePlatformSummary as ReturnType<typeof vi.fn>).mockReturnValue({
      data: allResolvedSummary,
      isLoading: false,
      error: null,
    });

    renderWithProvider();

    // Should show the mapping page, not redirect
    expect(screen.getByText('Platform & Storefront Mapping')).toBeInTheDocument();
    // Should indicate all items matched
    expect(screen.getByText(/All items have been automatically matched/)).toBeInTheDocument();
  });

  it('should display page title and description with unresolved count', () => {
    renderWithProvider();

    expect(screen.getByText('Platform & Storefront Mapping')).toBeInTheDocument();
    expect(
      screen.getByText(/2 items need manual mapping/)
    ).toBeInTheDocument();
  });

  it('should display all platform items including resolved ones', () => {
    renderWithProvider();

    expect(screen.getByText('Platforms')).toBeInTheDocument();
    // Should show both resolved and unresolved platforms
    expect(screen.getByText('"PC"')).toBeInTheDocument();
    expect(screen.getByText('"PS4"')).toBeInTheDocument();
  });

  it('should display all storefront items including resolved ones', () => {
    renderWithProvider();

    expect(screen.getByText('Storefronts')).toBeInTheDocument();
    // Should show both resolved and unresolved storefronts
    expect(screen.getByText('"Steam"')).toBeInTheDocument();
    expect(screen.getByText('"Epic"')).toBeInTheDocument();
  });

  it('should disable continue button when not all mapped', () => {
    renderWithProvider();

    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    expect(continueButton).toBeDisabled();
  });

  it('should enable continue button when all mapped', async () => {
    const user = userEvent.setup();
    renderWithProvider();

    // Need to map PS4 and Epic (the unresolved ones)
    // There are 4 comboboxes: PC, PS4, Steam, Epic
    const comboboxes = screen.getAllByRole('combobox');

    // Find and click the PS4 dropdown (second platform, index 1)
    await user.click(comboboxes[1]);
    await user.click(screen.getByText('PlayStation 4'));

    // Find and click the Epic dropdown (second storefront, index 3)
    await user.click(comboboxes[3]);
    await user.click(screen.getByText('Epic Games Store'));

    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    expect(continueButton).toBeEnabled();
  });

  it('should navigate to review on continue', async () => {
    const user = userEvent.setup();
    renderWithProvider();

    // Map the unresolved items
    const comboboxes = screen.getAllByRole('combobox');

    // Map PS4
    await user.click(comboboxes[1]);
    await user.click(screen.getByText('PlayStation 4'));

    // Map Epic
    await user.click(comboboxes[3]);
    await user.click(screen.getByText('Epic Games Store'));

    // Click continue
    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    await user.click(continueButton);

    // Should save mappings to backend
    await waitFor(() => {
      expect(mockBatchImportMappings.mutateAsync).toHaveBeenCalled();
    });

    // Should navigate to review
    await waitFor(() => {
      expect(mockRouter.push).toHaveBeenCalledWith('/review?job_id=test-job-123');
    });
  });

  it('should show error when no job_id provided', () => {
    (useSearchParams as ReturnType<typeof vi.fn>).mockReturnValue(
      new URLSearchParams('')
    );

    renderWithProvider();

    expect(screen.getByText(/no job id provided/i)).toBeInTheDocument();
  });

  it('should pre-select auto-matched values in dropdowns', async () => {
    renderWithProvider();

    // Wait for the component to render
    await waitFor(() => {
      expect(screen.getByText('Platform & Storefront Mapping')).toBeInTheDocument();
    });

    // The dropdowns for auto-matched items should show their suggested values
    // PC should show "PC (Windows)" and Steam should show "Steam"
    expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
    expect(screen.getByText('Steam')).toBeInTheDocument();
  });
});
