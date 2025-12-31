import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameEditForm } from './game-edit-form';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame, GameId, UserGameId } from '@/types';

// Mock next/navigation
const mockPush = vi.fn();
const mockBack = vi.fn();

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    back: mockBack,
  }),
  useParams: () => ({ id: 'test-game-id' }),
}));

// Mock sonner toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
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
  ownership_status: OwnershipStatus.OWNED,
  personal_rating: 4,
  is_loved: false,
  play_status: PlayStatus.IN_PROGRESS,
  hours_played: 10,
  personal_notes: '<p>Some notes</p>',
  acquired_date: '2024-01-15',
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
      hours_played: 0,
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

    // Check ownership status is shown
    expect(screen.getByText('Owned')).toBeInTheDocument();
  });

  it('renders hours played summary', () => {
    render(<GameEditForm game={mockGame} />);

    // Hours played is now shown as aggregate total, not as an editable input
    // The per-platform playtime inputs are in the Platforms section
    expect(screen.getByText('10 hours total')).toBeInTheDocument();
  });

  it('renders acquired date input with current value', () => {
    render(<GameEditForm game={mockGame} />);

    const dateInput = screen.getByLabelText('Acquired Date');
    expect(dateInput).toHaveValue('2024-01-15');
  });

  it('renders cancel button that navigates back', async () => {
    const user = userEvent.setup();
    render(<GameEditForm game={mockGame} />);

    const cancelButtons = screen.getAllByRole('button', { name: /cancel/i });
    await user.click(cancelButtons[0]);

    expect(mockPush).toHaveBeenCalledWith('/games/f47ac10b-58cc-4372-a567-0e02b2c3d479');
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
