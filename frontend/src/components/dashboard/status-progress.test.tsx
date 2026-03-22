import { describe, it, expect } from 'vitest';
import { render, screen } from '@/test/test-utils';
import { PlayStatus } from '@/types';
import { StatusProgress } from './status-progress';
import { statusColors, statusLabels, statusIcons, statusDescriptions } from './status-progress-data';

describe('StatusProgress', () => {
  it('renders the status label and count', () => {
    render(
      <StatusProgress
        status={PlayStatus.IN_PROGRESS}
        count={5}
        total={20}
      />
    );

    expect(screen.getByText('In Progress')).toBeInTheDocument();
    expect(screen.getByText('(5 games)')).toBeInTheDocument();
  });

  it('renders singular "game" when count is 1', () => {
    render(
      <StatusProgress
        status={PlayStatus.COMPLETED}
        count={1}
        total={10}
      />
    );

    expect(screen.getByText('(1 game)')).toBeInTheDocument();
  });

  it('displays correct percentage', () => {
    render(
      <StatusProgress
        status={PlayStatus.COMPLETED}
        count={25}
        total={100}
      />
    );

    expect(screen.getByText('25.0%')).toBeInTheDocument();
  });

  it('displays 0% when total is 0', () => {
    render(
      <StatusProgress
        status={PlayStatus.NOT_STARTED}
        count={0}
        total={0}
      />
    );

    expect(screen.getByText('0.0%')).toBeInTheDocument();
  });

  it('renders the status icon', () => {
    render(
      <StatusProgress
        status={PlayStatus.MASTERED}
        count={3}
        total={10}
      />
    );

    expect(screen.getByRole('img', { name: 'Mastered' })).toHaveTextContent('🏆');
  });

  it('shows description when showDescription is true', () => {
    render(
      <StatusProgress
        status={PlayStatus.SHELVED}
        count={2}
        total={10}
        showDescription
      />
    );

    expect(screen.getByText('On hold for later')).toBeInTheDocument();
  });

  it('does not show description by default', () => {
    render(
      <StatusProgress
        status={PlayStatus.SHELVED}
        count={2}
        total={10}
      />
    );

    expect(screen.queryByText('On hold for later')).not.toBeInTheDocument();
  });

  it('applies custom className', () => {
    const { container } = render(
      <StatusProgress
        status={PlayStatus.DROPPED}
        count={1}
        total={10}
        className="custom-class"
      />
    );

    expect(container.firstChild).toHaveClass('custom-class');
  });
});

describe('status exports', () => {
  it('has colors for all play statuses', () => {
    const allStatuses = Object.values(PlayStatus);
    allStatuses.forEach((status) => {
      expect(statusColors[status]).toBeDefined();
      expect(statusColors[status]).toMatch(/^bg-/);
    });
  });

  it('has labels for all play statuses', () => {
    const allStatuses = Object.values(PlayStatus);
    allStatuses.forEach((status) => {
      expect(statusLabels[status]).toBeDefined();
      expect(typeof statusLabels[status]).toBe('string');
    });
  });

  it('has icons for all play statuses', () => {
    const allStatuses = Object.values(PlayStatus);
    allStatuses.forEach((status) => {
      expect(statusIcons[status]).toBeDefined();
    });
  });

  it('has descriptions for all play statuses', () => {
    const allStatuses = Object.values(PlayStatus);
    allStatuses.forEach((status) => {
      expect(statusDescriptions[status]).toBeDefined();
      expect(typeof statusDescriptions[status]).toBe('string');
    });
  });
});
