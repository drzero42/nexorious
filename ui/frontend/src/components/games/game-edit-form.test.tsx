import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameEditForm } from './game-edit-form';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame, GameId, UserGameId } from '@/types';

// Mock @tanstack/react-router navigation
const mockNavigate = vi.fn();

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useParams: () => ({}),
    useSearch: () => ({}),
    useRouterState: vi.fn((opts?: { select?: (s: unknown) => unknown }) => {
      const state = { location: { pathname: '/', search: '', hash: '' } };
      return opts?.select ? opts.select(state) : state;
    }),
  };
});

// Mock sonner toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

// Mock hooks
vi.mock('@/hooks', () => ({
  useUpdateUserGame: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useAddPlatformToUserGame: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useRemovePlatformFromUserGame: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useUpdatePlatformAssociation: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useAssignTagsToGame: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useRemoveTagsFromGame: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useAllPlatforms: () => ({ data: [], isLoading: false }),
  useAllTags: () => ({ data: [], isLoading: false }),
  useCreateOrGetTag: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useSyncConfig: () => ({ data: null }),
}));

const mockGame: UserGame = {
  id: 'f47ac10b-58cc-4372-a567-0e02b2c3d479' as UserGameId,
  game: {
    id: 123 as GameId,
    title: 'Test Game',
    description: 'A test game description',
    genre: 'RPG',
    developer: 'Test Developer',
    publisher: 'Test Publisher',
    release_date: '2024-01-01',
    cover_art_url: '/covers/test.jpg',
    rating_average: 4.5,
    rating_count: 100,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  personal_rating: 4,
  is_loved: false,
  play_status: PlayStatus.IN_PROGRESS,
  hours_played: 10,
  personal_notes: '<p>Some notes</p>',
  platforms: [
    {
      id: 'ugp-1',
      platform: 'pc',
      storefront: 'steam',
      platform_details: {
        name: 'pc',
        display_name: 'PC',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      storefront_details: {
        name: 'steam',
        display_name: 'Steam',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      is_available: true,
      hours_played: 10,
      ownership_status: OwnershipStatus.OWNED,
      acquired_date: '2024-01-15',
      created_at: '2024-01-01T00:00:00Z',
    },
  ],
  tags: [
    {
      id: 'tag-1',
      user_id: 'test-user-id',
      name: 'RPG',
      color: '#FF5733',
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    },
  ],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

describe('GameEditForm', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockNavigate.mockReset();
  });

  it('renders the form with game title', async () => {
    render(<GameEditForm game={mockGame} />);

    expect(screen.getByText('Test Game')).toBeInTheDocument();
    expect(screen.getByText('Test Developer')).toBeInTheDocument();
  });

  it('renders status dropdowns with current values', async () => {
    render(<GameEditForm game={mockGame} />);

    // Check play status is shown
    expect(screen.getByText('In Progress')).toBeInTheDocument();

    // Check ownership status is shown per platform (in Platforms section)
    expect(screen.getByText('Owned')).toBeInTheDocument();
  });

  it('renders hours played summary', () => {
    render(<GameEditForm game={mockGame} />);

    // Hours played is now shown as aggregate total, not as an editable input
    // The per-platform playtime inputs are in the Platforms section
    expect(screen.getByText('10 hours total')).toBeInTheDocument();
  });

  it('renders acquired date input per platform with current value', () => {
    const { container } = render(<GameEditForm game={mockGame} />);

    // Acquired date is now per platform, find the date input in the platform section
    // The label isn't connected with htmlFor, so find via input type
    const dateInput = container.querySelector('input[type="date"]');
    expect(dateInput).toHaveValue('2024-01-15');
  });

  it('renders cancel button that navigates back', async () => {
    const user = userEvent.setup();
    render(<GameEditForm game={mockGame} />);

    const cancelButtons = screen.getAllByRole('button', { name: /cancel/i });
    await user.click(cancelButtons[0]);

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games/f47ac10b-58cc-4372-a567-0e02b2c3d479' });
  });

  it('renders save button', () => {
    render(<GameEditForm game={mockGame} />);

    const saveButtons = screen.getAllByRole('button', { name: /save changes/i });
    expect(saveButtons.length).toBeGreaterThanOrEqual(1);
  });

  it('renders is loved checkbox', () => {
    render(<GameEditForm game={mockGame} />);

    expect(screen.getByRole('checkbox', { name: /mark as loved/i })).toBeInTheDocument();
  });

  it('toggles is loved checkbox', async () => {
    const user = userEvent.setup();
    render(<GameEditForm game={mockGame} />);

    const checkbox = screen.getByRole('checkbox', { name: /mark as loved/i });
    expect(checkbox).not.toBeChecked();

    await user.click(checkbox);
    expect(checkbox).toBeChecked();
  });
});
