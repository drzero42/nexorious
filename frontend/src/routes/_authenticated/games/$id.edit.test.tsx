import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';

const { mockNavigate } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
}));

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useParams: () => ({ id: 'game-123' }),
    useSearch: () => ({}),
  };
});

vi.mock('@/hooks', () => ({
  useUserGame: vi.fn(),
}));

// Mock GameEditForm so it does not render in these error-state-focused tests
vi.mock('@/components/games/game-edit-form', () => ({
  GameEditForm: () => <div data-testid="game-edit-form" />,
}));

describe('GameEditPage — error state Back to Games navigation', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    sessionStorage.clear();

    const { useUserGame } = vi.mocked(await import('@/hooks'));
    // Default: put the component in error/not-found state
    useUserGame.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Not found'),
    } as ReturnType<typeof useUserGame>);
  });

  it('navigates to stored return URL when Back to Games is clicked in error state', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', '?q=zelda&sort=title');

    const { GameEditPage } = await import('./$id.edit');
    render(<GameEditPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { q: 'zelda', sort: 'title' },
    });
  });

  it('navigates to bare /games when no return URL is stored', async () => {
    const user = userEvent.setup();

    const { GameEditPage } = await import('./$id.edit');
    render(<GameEditPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });
});
