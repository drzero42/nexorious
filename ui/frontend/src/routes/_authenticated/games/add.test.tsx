import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { IGDBGameCandidate } from '@/types';
import { AddGamePage } from './add.index';

const mockUseHealthStatus = vi.fn();
vi.mock('@/hooks/use-health-status', () => ({
  useHealthStatus: () => mockUseHealthStatus(),
}));

const { mockNavigate } = vi.hoisted(() => ({ mockNavigate: vi.fn() }));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
  useNavigate: () => mockNavigate,
}));

// Mock IGDBSearch to expose its onSelect callback and the showLibraryStatus prop
// so the page's selection branching can be exercised directly.
let lastShowLibraryStatus: boolean | undefined;
const ownedGame: IGDBGameCandidate = {
  igdb_id: 90101 as IGDBGameCandidate['igdb_id'],
  title: 'Owned Game',
  platforms: [],
  user_game_id: 'ug-1',
};
const newGame: IGDBGameCandidate = {
  igdb_id: 90202 as IGDBGameCandidate['igdb_id'],
  title: 'New Game',
  platforms: [],
};

vi.mock('@/components/games/igdb-search', () => ({
  IGDBSearch: ({
    disabled,
    showLibraryStatus,
    onSelect,
  }: {
    disabled?: boolean;
    showLibraryStatus?: boolean;
    onSelect: (game: IGDBGameCandidate) => void;
  }) => {
    lastShowLibraryStatus = showLibraryStatus;
    return (
      <div data-testid="igdb-search" data-disabled={String(disabled ?? false)}>
        <button onClick={() => onSelect(ownedGame)}>select-owned</button>
        <button onClick={() => onSelect(newGame)}>select-new</button>
      </div>
    );
  },
}));

describe('AddGamePage IGDB disabled state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it.each(['not_configured', 'invalid_credentials'])(
    'disables IGDB search when igdb_status is %s',
    (igdbStatus) => {
      mockUseHealthStatus.mockReturnValue({ data: { igdb_status: igdbStatus } });
      render(<AddGamePage />);
      expect(screen.getByTestId('igdb-search')).toHaveAttribute('data-disabled', 'true');
    },
  );

  it('enables IGDB search when igdb_status is ok', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'ok' } });
    render(<AddGamePage />);
    expect(screen.getByTestId('igdb-search')).toHaveAttribute('data-disabled', 'false');
  });
});

describe('AddGamePage selection routing (#856)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'ok' } });
  });

  it('passes showLibraryStatus to the search component', () => {
    render(<AddGamePage />);
    expect(lastShowLibraryStatus).toBe(true);
  });

  it('navigates to the detail page when the selected game is already in the library or wishlist', async () => {
    const user = userEvent.setup();
    render(<AddGamePage />);

    await user.click(screen.getByText('select-owned'));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games/$id',
      params: { id: 'ug-1' },
    });
  });

  it('navigates to the confirm page when the selected game is not in the library', async () => {
    const user = userEvent.setup();
    render(<AddGamePage />);

    await user.click(screen.getByText('select-new'));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games/add/confirm',
      search: { igdb_id: '90202' },
    });
    expect(sessionStorage.getItem('nexorious_selected_game')).toContain('90202');
  });
});
