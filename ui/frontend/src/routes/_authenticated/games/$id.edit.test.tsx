import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { Route } from './$id.edit';

const { mockNavigate } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
}));

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
}));

// Mock GameEditForm so it does not render in these error-state-focused tests
vi.mock('@/components/games/game-edit-form', () => ({
  GameEditForm: () => <div data-testid="game-edit-form" />,
}));

describe('GameEditPage — error state Back navigation', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    sessionStorage.clear();

    // Mock Route.useParams so the component doesn't need a router context
    vi.spyOn(Route, 'useParams').mockReturnValue({ id: 'game-123' });

    const { useUserGame } = vi.mocked(await import('@/hooks'));
    // Default: put the component in error/not-found state
    useUserGame.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Not found'),
    } as ReturnType<typeof useUserGame>);
  });

  it('navigates to the stored referrer (with filters) when Back is clicked in error state', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem(
      'game_return',
      JSON.stringify({ to: '/games', label: 'Games', search: { q: 'zelda', sort: 'title' } }),
    );

    const { GameEditPage } = await import('./$id.edit');
    render(<GameEditPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { q: 'zelda', sort: 'title' },
    });
  });

  it('reflects the referrer label and target (Library Health)', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem(
      'game_return',
      JSON.stringify({ to: '/library-health', label: 'Library Health' }),
    );

    const { GameEditPage } = await import('./$id.edit');
    render(<GameEditPage />);

    await user.click(screen.getByRole('button', { name: /back to library health/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/library-health' });
  });

  it('falls back to "Back to Games" → bare /games when no referrer is stored', async () => {
    const user = userEvent.setup();

    const { GameEditPage } = await import('./$id.edit');
    render(<GameEditPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });
});
