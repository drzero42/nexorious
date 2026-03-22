import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { Route } from './$id.index';

// vi.hoisted ensures mockNavigate is captured by the vi.mock factory below
const { mockNavigate } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
}));

// Override the global setup.ts mock for this test file only
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useSearch: () => ({}),
  };
});

vi.mock('@/hooks', () => ({
  useUserGame: vi.fn(),
  useDeleteUserGame: vi.fn(),
}));

// Minimal mock game — only the fields the component null-checks against
const mockGame = {
  id: 'game-123',
  play_status: 'not_started' as const,
  personal_rating: null,
  is_loved: false,
  hours_played: 0,
  personal_notes: null,
  platforms: [],
  game: {
    id: 1,
    title: 'Test Game',
    cover_art_url: null,
    developer: null,
    publisher: null,
    genre: null,
    release_date: null,
    game_modes: null,
    themes: null,
    player_perspectives: null,
    igdb_slug: null,
    description: null,
    howlongtobeat_main: null,
    howlongtobeat_extra: null,
    howlongtobeat_completionist: null,
  },
};

describe('GameDetailPage — Back to Games navigation', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    sessionStorage.clear();

    // Mock Route.useParams so the component doesn't need a router context
    vi.spyOn(Route, 'useParams').mockReturnValue({ id: 'game-123' });

    const { useUserGame, useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: mockGame,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useUserGame>);
    useDeleteUserGame.mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue(undefined),
    } as unknown as ReturnType<typeof useDeleteUserGame>);
  });

  it('navigates to stored return URL when Back to Games is clicked', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', JSON.stringify({ q: 'foo', status: 'completed' }));

    const { GameDetailPage } = await import(
      './$id.index'
    );
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { q: 'foo', status: 'completed' },
    });
  });

  it('navigates to bare /games when no return URL is stored', async () => {
    const user = userEvent.setup();
    // sessionStorage is empty (cleared in beforeEach)

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });

  it('error state Back to Games uses stored return URL', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', JSON.stringify({ status: 'in_progress' }));

    const { useUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Not found'),
    } as ReturnType<typeof useUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { status: 'in_progress' },
    });
  });

  it('error state Back to Games falls back to /games when no URL stored', async () => {
    const user = userEvent.setup();

    const { useUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Not found'),
    } as ReturnType<typeof useUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });

  it('navigates to stored return URL after deleting a game', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', JSON.stringify({ q: 'rpg' }));

    const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
    const { useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useDeleteUserGame.mockReturnValue({
      mutateAsync: mockMutateAsync,
    } as unknown as ReturnType<typeof useDeleteUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    // Open the delete confirmation dialog
    await user.click(screen.getByRole('button', { name: /remove/i }));
    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    // Confirm deletion (trigger is now inert behind the modal)
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith('game-123');
    });
    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { q: 'rpg' },
    });
  });

  it('restores page number without double-encoding (regression: page="2" not page="\\"2\\"")', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', JSON.stringify({ page: '2', status: 'completed' }));

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { page: '2', status: 'completed' },
    });
  });

  it('navigates to bare /games after deleting when no return URL stored', async () => {
    const user = userEvent.setup();

    const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
    const { useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useDeleteUserGame.mockReturnValue({
      mutateAsync: mockMutateAsync,
    } as unknown as ReturnType<typeof useDeleteUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /remove/i }));
    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalled();
    });
    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });
});
