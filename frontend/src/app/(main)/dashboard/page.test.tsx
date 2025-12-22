import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@/test/test-utils';
import DashboardPage from './page';

// Mock next/link
vi.mock('next/link', () => ({
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

// Mock the useCollectionStats hook to have full control over test data
vi.mock('@/hooks', async () => {
  const actual = await vi.importActual('@/hooks');
  return {
    ...actual,
    useCollectionStats: vi.fn(),
  };
});

import { useCollectionStats } from '@/hooks';
import { PlayStatus, OwnershipStatus } from '@/types';

const mockedUseCollectionStats = vi.mocked(useCollectionStats);

const mockStats = {
  totalGames: 50,
  completionStats: {
    [PlayStatus.NOT_STARTED]: 15,
    [PlayStatus.IN_PROGRESS]: 8,
    [PlayStatus.COMPLETED]: 12,
    [PlayStatus.MASTERED]: 5,
    [PlayStatus.DOMINATED]: 3,
    [PlayStatus.SHELVED]: 4,
    [PlayStatus.DROPPED]: 2,
    [PlayStatus.REPLAY]: 1,
  } as Record<PlayStatus, number>,
  ownershipStats: {
    [OwnershipStatus.OWNED]: 45,
    [OwnershipStatus.SUBSCRIPTION]: 5,
  } as Record<OwnershipStatus, number>,
  platformStats: {
    PC: 30,
    PlayStation: 15,
    Nintendo: 5,
  },
  genreStats: {
    RPG: 20,
    Action: 15,
    Adventure: 10,
    Puzzle: 5,
  },
  pileOfShame: 15,
  completionRate: 40,
  averageRating: 4.0,
  totalHoursPlayed: 800,
};

describe('DashboardPage', () => {
  it('renders page header', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(
      screen.getByText('Your gaming statistics and insights')
    ).toBeInTheDocument();
  });

  it('displays loading skeleton when loading', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    // Should show skeleton loaders
    const skeletons = document.querySelectorAll('[class*="animate-pulse"]');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it('displays statistics when loaded', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: mockStats,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('50')).toBeInTheDocument(); // Total Games
    // Completion rate appears multiple times on the page (overview and progress breakdown)
    expect(screen.getAllByText(/40\.0.*%/).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('Progress Breakdown')).toBeInTheDocument();
  });

  it('displays top genres section', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: mockStats,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('Top Genres')).toBeInTheDocument();
    expect(screen.getByText('RPG')).toBeInTheDocument();
    expect(screen.getByText('Action')).toBeInTheDocument();
  });

  it('displays personal stats section', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: mockStats,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('Personal Stats')).toBeInTheDocument();
    expect(screen.getByText('Average Rating')).toBeInTheDocument();
    expect(screen.getByText('4.0/5')).toBeInTheDocument();
  });

  it('displays game insights section', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: mockStats,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('Game Insights')).toBeInTheDocument();
    expect(screen.getByText('Pile of Shame')).toBeInTheDocument();
    expect(screen.getByText('15 games')).toBeInTheDocument();
  });

  it('shows empty state when no games', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: { ...mockStats, totalGames: 0 },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('No games in your collection')).toBeInTheDocument();
    expect(screen.getByText('Add Your First Game')).toBeInTheDocument();
  });

  it('shows error state on API failure', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed to fetch'),
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(
      screen.getByText('Failed to load statistics. Please try again later.')
    ).toBeInTheDocument();
  });
});

describe('DashboardPage - TopGenres', () => {
  it('displays genres sorted by count descending', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: mockStats,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    const genreSection = screen.getByText('Top Genres').closest('div')?.parentElement;
    const genreLabels = genreSection?.querySelectorAll('.text-sm.font-medium');

    // RPG (20) should come before Action (15)
    if (genreLabels && genreLabels.length >= 2) {
      expect(genreLabels[0]).toHaveTextContent('RPG');
      expect(genreLabels[1]).toHaveTextContent('Action');
    }
  });

  it('limits to top 5 genres', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: {
        ...mockStats,
        genreStats: {
          RPG: 20,
          Action: 18,
          Adventure: 15,
          Puzzle: 12,
          Strategy: 10,
          Sports: 8,
          Racing: 5,
        },
      },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('RPG')).toBeInTheDocument();
    // Should not show the 6th and 7th genres
    expect(screen.queryByText('Sports')).not.toBeInTheDocument();
    expect(screen.queryByText('Racing')).not.toBeInTheDocument();
  });
});

describe('DashboardPage - PersonalStats', () => {
  it('displays N/A when average rating is null', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: { ...mockStats, averageRating: null },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('N/A')).toBeInTheDocument();
  });

  it('displays N/A when average rating is 0', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: { ...mockStats, averageRating: 0 },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    expect(screen.getByText('N/A')).toBeInTheDocument();
  });

  it('calculates average hours per game', () => {
    mockedUseCollectionStats.mockReturnValue({
      data: mockStats,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useCollectionStats>);

    render(<DashboardPage />);

    // Average Hours per Game appears in both Personal Stats and Time Investment sections
    expect(screen.getAllByText('Average Hours per Game').length).toBeGreaterThanOrEqual(1);
    // 800 hours / 50 games = 16.0h
    expect(screen.getByText('16.0h')).toBeInTheDocument();
  });
});
