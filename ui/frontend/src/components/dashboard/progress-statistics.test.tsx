import { describe, it, expect } from 'vitest';
import { render, screen } from '@/test/test-utils';
import { PlayStatus, OwnershipStatus } from '@/types';
import { ProgressStatistics } from './progress-statistics';

const createMockStats = (overrides = {}) => ({
  totalGames: 100,
  completionStats: {
    [PlayStatus.NOT_STARTED]: 30,
    [PlayStatus.IN_PROGRESS]: 15,
    [PlayStatus.COMPLETED]: 25,
    [PlayStatus.MASTERED]: 10,
    [PlayStatus.DOMINATED]: 5,
    [PlayStatus.SHELVED]: 8,
    [PlayStatus.DROPPED]: 5,
    [PlayStatus.REPLAY]: 2,
  },
  ownershipStats: {
    [OwnershipStatus.OWNED]: 80,
    [OwnershipStatus.SUBSCRIPTION]: 20,
  },
  platformStats: {
    PC: 60,
    PlayStation: 30,
    Nintendo: 10,
  },
  genreStats: {
    RPG: 35,
    Action: 25,
    Adventure: 20,
    Puzzle: 10,
    Strategy: 10,
  },
  pileOfShame: 30,
  completionRate: 40,
  averageRating: 4.2,
  totalHoursPlayed: 1500,
  ...overrides,
});

describe('ProgressStatistics', () => {
  describe('Overview Stats', () => {
    it('displays total games count', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('Total Games')).toBeInTheDocument();
      expect(screen.getByText('100')).toBeInTheDocument();
    });

    it('displays completion rate', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('Completion Rate')).toBeInTheDocument();
      expect(screen.getByText('40.0%')).toBeInTheDocument();
    });

    it('displays total hours', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('Total Hours')).toBeInTheDocument();
      // toLocaleString may format differently in test environment, and value appears in two places
      expect(screen.getAllByText(/1[,.]?500/).length).toBeGreaterThanOrEqual(1);
    });

    it('displays active games count (in_progress + replay)', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('Active Games')).toBeInTheDocument();
      // 15 in_progress + 2 replay = 17
      expect(screen.getByText('17')).toBeInTheDocument();
    });
  });

  describe('Progress Breakdown', () => {
    it('displays progress breakdown section', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('Progress Breakdown')).toBeInTheDocument();
    });

    it('shows all play status entries', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      // Multiple occurrences expected due to Progress Breakdown and Completion Journey
      expect(screen.getAllByText('Not Started').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('In Progress').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('Completed').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('Mastered').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('Dominated').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('Shelved').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('Dropped').length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText('Replay').length).toBeGreaterThanOrEqual(1);
    });

    it('displays game counts for each status', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('(30 games)')).toBeInTheDocument(); // not_started
      expect(screen.getByText('(15 games)')).toBeInTheDocument(); // in_progress
      expect(screen.getByText('(25 games)')).toBeInTheDocument(); // completed
    });
  });

  describe('Completion Journey', () => {
    it('displays completion journey section', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('Completion Journey')).toBeInTheDocument();
    });

    it('shows journey milestone descriptions', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('30 games waiting')).toBeInTheDocument();
      expect(screen.getByText('15 games active')).toBeInTheDocument();
      expect(screen.getByText('25 main stories finished')).toBeInTheDocument();
      expect(screen.getByText('10 games fully explored')).toBeInTheDocument();
      expect(screen.getByText('5 games at 100%')).toBeInTheDocument();
    });
  });

  describe('Time Investment', () => {
    it('displays time investment section when hours > 0', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      expect(screen.getByText('Time Investment')).toBeInTheDocument();
      expect(screen.getByText('Total Hours Played')).toBeInTheDocument();
      expect(screen.getByText('Average Hours per Game')).toBeInTheDocument();
      expect(screen.getByText('Average Completion Time')).toBeInTheDocument();
    });

    it('does not display time investment when hours is 0', () => {
      render(
        <ProgressStatistics stats={createMockStats({ totalHoursPlayed: 0 })} />
      );

      expect(screen.queryByText('Time Investment')).not.toBeInTheDocument();
    });

    it('calculates average hours per game correctly', () => {
      render(<ProgressStatistics stats={createMockStats()} />);

      // 1500 hours / 100 games = 15.0
      expect(screen.getByText('15.0')).toBeInTheDocument();
    });
  });

  describe('Edge cases', () => {
    it('handles zero total games', () => {
      const emptyStats = createMockStats({
        totalGames: 0,
        completionStats: {
          [PlayStatus.NOT_STARTED]: 0,
          [PlayStatus.IN_PROGRESS]: 0,
          [PlayStatus.COMPLETED]: 0,
          [PlayStatus.MASTERED]: 0,
          [PlayStatus.DOMINATED]: 0,
          [PlayStatus.SHELVED]: 0,
          [PlayStatus.DROPPED]: 0,
          [PlayStatus.REPLAY]: 0,
        },
        totalHoursPlayed: 0,
      });

      render(<ProgressStatistics stats={emptyStats} />);

      // Should render without crashing
      expect(screen.getByText('Total Games')).toBeInTheDocument();
      // Multiple 0s expected (total games, hours, percentages)
      expect(screen.getAllByText('0').length).toBeGreaterThanOrEqual(1);
    });

    it('handles missing completion stats gracefully', () => {
      const partialStats = createMockStats({
        completionStats: {
          [PlayStatus.IN_PROGRESS]: 5,
        } as Record<PlayStatus, number>,
      });

      render(<ProgressStatistics stats={partialStats} />);

      // Should render without crashing - In Progress appears in both progress breakdown and journey
      expect(screen.getAllByText('In Progress').length).toBeGreaterThanOrEqual(1);
    });

    it('applies custom className', () => {
      const { container } = render(
        <ProgressStatistics
          stats={createMockStats()}
          className="custom-container"
        />
      );

      expect(container.firstChild).toHaveClass('custom-container');
    });
  });
});
