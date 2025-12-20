// frontend/src/app/(main)/import/mapping/page.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useRouter, useSearchParams } from 'next/navigation';
import MappingPage from './page';
import { usePlatformSummary, useAllPlatforms, useAllStorefronts } from '@/hooks';
import { ImportMappingProvider } from '@/contexts/import-mapping-context';

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
  });

  const renderWithProvider = () => {
    return render(
      <ImportMappingProvider>
        <MappingPage />
      </ImportMappingProvider>
    );
  };

  it('should redirect to review when all resolved', async () => {
    (usePlatformSummary as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { ...mockPlatformSummary, allResolved: true },
      isLoading: false,
      error: null,
    });

    renderWithProvider();

    await waitFor(() => {
      expect(mockRouter.replace).toHaveBeenCalledWith('/review?job_id=test-job-123');
    });
  });

  it('should display page title and description', () => {
    renderWithProvider();

    expect(screen.getByText('Platform & Storefront Mapping')).toBeInTheDocument();
    expect(
      screen.getByText(/Some values from your CSV need to be mapped/)
    ).toBeInTheDocument();
  });

  it('should display unresolved platform section', () => {
    renderWithProvider();

    expect(screen.getByText('Platforms')).toBeInTheDocument();
    expect(screen.getByText('"PS4"')).toBeInTheDocument();
  });

  it('should display unresolved storefront section', () => {
    renderWithProvider();

    expect(screen.getByText('Storefronts')).toBeInTheDocument();
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

    // Select platform mapping
    const platformSelect = screen.getAllByRole('combobox')[0];
    await user.click(platformSelect);
    await user.click(screen.getByText('PlayStation 4'));

    // Select storefront mapping
    const storefrontSelect = screen.getAllByRole('combobox')[1];
    await user.click(storefrontSelect);
    await user.click(screen.getByText('Epic Games Store'));

    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    expect(continueButton).toBeEnabled();
  });

  it('should navigate to review on continue', async () => {
    const user = userEvent.setup();
    renderWithProvider();

    // Select platform mapping
    const platformSelect = screen.getAllByRole('combobox')[0];
    await user.click(platformSelect);
    await user.click(screen.getByText('PlayStation 4'));

    // Select storefront mapping
    const storefrontSelect = screen.getAllByRole('combobox')[1];
    await user.click(storefrontSelect);
    await user.click(screen.getByText('Epic Games Store'));

    // Click continue
    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    await user.click(continueButton);

    expect(mockRouter.push).toHaveBeenCalledWith('/review?job_id=test-job-123');
  });

  it('should show error when no job_id provided', () => {
    (useSearchParams as ReturnType<typeof vi.fn>).mockReturnValue(
      new URLSearchParams('')
    );

    renderWithProvider();

    expect(screen.getByText(/no job id provided/i)).toBeInTheDocument();
  });
});
