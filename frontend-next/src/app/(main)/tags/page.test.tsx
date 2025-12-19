import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import TagsPage from './page';

// Mock next/navigation
const mockPush = vi.fn();
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
  }),
}));

// Mock sonner toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

// Mock the hooks
vi.mock('@/hooks', async () => {
  const actual = await vi.importActual('@/hooks');
  return {
    ...actual,
    useAllTags: vi.fn(),
    useCreateTag: vi.fn(),
    useUpdateTag: vi.fn(),
    useDeleteTag: vi.fn(),
  };
});

import { useAllTags, useCreateTag, useUpdateTag, useDeleteTag } from '@/hooks';
import { toast } from 'sonner';

const mockedUseAllTags = vi.mocked(useAllTags);
const mockedUseCreateTag = vi.mocked(useCreateTag);
const mockedUseUpdateTag = vi.mocked(useUpdateTag);
const mockedUseDeleteTag = vi.mocked(useDeleteTag);

const mockTags = [
  {
    id: 'tag-1',
    user_id: 'user-1',
    name: 'RPG',
    color: '#EF4444',
    description: 'Role-playing games',
    created_at: '2024-01-15T10:00:00Z',
    updated_at: '2024-01-15T10:00:00Z',
    game_count: 10,
  },
  {
    id: 'tag-2',
    user_id: 'user-1',
    name: 'Action',
    color: '#3B82F6',
    description: 'Action games',
    created_at: '2024-01-10T10:00:00Z',
    updated_at: '2024-01-10T10:00:00Z',
    game_count: 5,
  },
  {
    id: 'tag-3',
    user_id: 'user-1',
    name: 'Unused Tag',
    color: '#6B7280',
    description: '',
    created_at: '2024-01-05T10:00:00Z',
    updated_at: '2024-01-05T10:00:00Z',
    game_count: 0,
  },
];

const createMockMutation = (mutateAsyncFn = vi.fn()) => ({
  mutateAsync: mutateAsyncFn,
  mutate: vi.fn(),
  isPending: false,
  isError: false,
  isSuccess: false,
  isIdle: true,
  data: undefined,
  error: null,
  reset: vi.fn(),
  status: 'idle' as const,
  variables: undefined,
  failureCount: 0,
  failureReason: null,
  context: undefined,
  submittedAt: 0,
  isPaused: false,
});

describe('TagsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockPush.mockClear();

    // Default mock implementations
    mockedUseCreateTag.mockReturnValue(
      createMockMutation() as unknown as ReturnType<typeof useCreateTag>
    );
    mockedUseUpdateTag.mockReturnValue(
      createMockMutation() as unknown as ReturnType<typeof useUpdateTag>
    );
    mockedUseDeleteTag.mockReturnValue(
      createMockMutation() as unknown as ReturnType<typeof useDeleteTag>
    );
  });

  describe('Loading State', () => {
    it('renders loading skeleton when loading', () => {
      mockedUseAllTags.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // Should show skeleton loaders
      const skeletons = document.querySelectorAll('[class*="animate-pulse"]');
      expect(skeletons.length).toBeGreaterThan(0);
    });
  });

  describe('Error State', () => {
    it('displays error message when loading fails', () => {
      mockedUseAllTags.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Failed to load tags'),
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // The error message appears both as heading and description
      expect(screen.getAllByText('Failed to load tags').length).toBeGreaterThan(0);
      expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument();
    });

    it('calls refetch when Try Again button is clicked', async () => {
      const refetchFn = vi.fn();
      mockedUseAllTags.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Failed to load tags'),
        refetch: refetchFn,
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      const tryAgainButton = screen.getByRole('button', { name: /try again/i });
      await userEvent.click(tryAgainButton);

      expect(refetchFn).toHaveBeenCalled();
    });
  });

  describe('Page Header', () => {
    it('renders page title and description', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByText('Tag Management')).toBeInTheDocument();
      expect(
        screen.getByText(/organize your games with custom tags/i)
      ).toBeInTheDocument();
    });

    it('renders Create Tag button', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByRole('button', { name: /create tag/i })).toBeInTheDocument();
    });
  });

  describe('Statistics Cards', () => {
    it('displays correct tag statistics', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // Total Tags: 3
      expect(screen.getByText('Total Tags')).toBeInTheDocument();
      expect(screen.getByText('3')).toBeInTheDocument();

      // Used Tags: 2 (RPG and Action have game_count > 0)
      expect(screen.getByText('Used Tags')).toBeInTheDocument();
      expect(screen.getByText('2')).toBeInTheDocument();

      // Unused Tags: 1
      expect(screen.getByText('Unused Tags')).toBeInTheDocument();
      expect(screen.getByText('1')).toBeInTheDocument();

      // Total Usage: 15 (10 + 5 + 0)
      expect(screen.getByText('Total Usage')).toBeInTheDocument();
      expect(screen.getByText('15')).toBeInTheDocument();

      // Average per Tag: 5.0 (15 / 3)
      expect(screen.getByText('Avg per Tag')).toBeInTheDocument();
      expect(screen.getByText('5.0')).toBeInTheDocument();
    });
  });

  describe('Tags List', () => {
    it('displays all tags', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByText('RPG')).toBeInTheDocument();
      expect(screen.getByText('Action')).toBeInTheDocument();
      expect(screen.getByText('Unused Tag')).toBeInTheDocument();
    });

    it('displays tag descriptions', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByText('Role-playing games')).toBeInTheDocument();
      expect(screen.getByText('Action games')).toBeInTheDocument();
    });

    it('displays game count for used tags', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByText('10 games')).toBeInTheDocument();
      expect(screen.getByText('5 games')).toBeInTheDocument();
    });

    it('displays Unused badge for tags with no games', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByText('Unused')).toBeInTheDocument();
    });

    it('displays empty state when no tags exist', () => {
      mockedUseAllTags.mockReturnValue({
        data: [],
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByText('No tags')).toBeInTheDocument();
      expect(
        screen.getByText(/get started by creating your first tag/i)
      ).toBeInTheDocument();
      expect(
        screen.getByRole('button', { name: /create your first tag/i })
      ).toBeInTheDocument();
    });
  });

  describe('Search and Sort', () => {
    it('filters tags by search query', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      const searchInput = screen.getByPlaceholderText(/search tags/i);
      await userEvent.type(searchInput, 'RPG');

      expect(screen.getByText('RPG')).toBeInTheDocument();
      expect(screen.queryByText('Action')).not.toBeInTheDocument();
      expect(screen.queryByText('Unused Tag')).not.toBeInTheDocument();
    });

    it('shows no results message when search has no matches', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      const searchInput = screen.getByPlaceholderText(/search tags/i);
      await userEvent.type(searchInput, 'NonExistent');

      expect(screen.getByText('No tags found')).toBeInTheDocument();
    });

    it('has sort buttons for name, usage, and date', () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      expect(screen.getByRole('button', { name: /^name/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /usage/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /date/i })).toBeInTheDocument();
    });
  });

  describe('Tag Navigation', () => {
    it('navigates to games page with tag filter when tag is clicked', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // Click on the RPG tag (the clickable area with the tag info)
      const rpgButton = screen.getByRole('button', { name: /rpg.*10 games/i });
      await userEvent.click(rpgButton);

      expect(mockPush).toHaveBeenCalledWith('/games?tag=tag-1');
    });
  });

  describe('Create Tag Dialog', () => {
    it('opens create dialog when Create Tag button is clicked', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      const createButton = screen.getByRole('button', { name: /create tag/i });
      await userEvent.click(createButton);

      expect(screen.getByText('Create New Tag')).toBeInTheDocument();
      expect(screen.getByLabelText(/name/i)).toBeInTheDocument();
    });

    it('calls createTag mutation when form is submitted', async () => {
      const mutateAsyncFn = vi.fn().mockResolvedValue({
        id: 'new-tag',
        name: 'New Tag',
        color: '#EF4444',
      });

      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      mockedUseCreateTag.mockReturnValue(
        createMockMutation(mutateAsyncFn) as unknown as ReturnType<typeof useCreateTag>
      );

      render(<TagsPage />);

      // Open dialog
      const createButton = screen.getByRole('button', { name: /create tag/i });
      await userEvent.click(createButton);

      // Fill form
      const nameInput = screen.getByLabelText(/name/i);
      await userEvent.type(nameInput, 'New Tag');

      // Submit
      const submitButton = screen.getByRole('button', { name: /^create tag$/i });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(mutateAsyncFn).toHaveBeenCalledWith(
          expect.objectContaining({
            name: 'New Tag',
          })
        );
      });
    });

    it('shows error toast when create fails', async () => {
      const mutateAsyncFn = vi.fn().mockRejectedValue(new Error('Tag already exists'));

      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      mockedUseCreateTag.mockReturnValue(
        createMockMutation(mutateAsyncFn) as unknown as ReturnType<typeof useCreateTag>
      );

      render(<TagsPage />);

      // Open dialog and submit
      const createButton = screen.getByRole('button', { name: /create tag/i });
      await userEvent.click(createButton);

      const nameInput = screen.getByLabelText(/name/i);
      await userEvent.type(nameInput, 'New Tag');

      const submitButton = screen.getByRole('button', { name: /^create tag$/i });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith('Tag already exists');
      });
    });
  });

  describe('Edit Tag Dialog', () => {
    it('opens edit dialog when edit button is clicked', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // Click edit button (there are 3 tags, click the first one)
      const editButtons = screen.getAllByRole('button', { name: /edit/i });
      await userEvent.click(editButtons[0]);

      expect(screen.getByText('Edit Tag')).toBeInTheDocument();
    });

    it('pre-fills form with tag data', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // Tags are sorted alphabetically by name, so order is: Action, RPG, Unused Tag
      // Click edit button for first tag (Action)
      const editButtons = screen.getAllByRole('button', { name: /edit/i });
      await userEvent.click(editButtons[0]);

      // Edit dialog uses different id for name input
      const nameInput = document.getElementById('edit-tag-name') as HTMLInputElement;
      expect(nameInput.value).toBe('Action');
    });
  });

  describe('Delete Tag Dialog', () => {
    it('opens delete confirmation when delete button is clicked', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // Click delete button for first tag
      const deleteButtons = screen.getAllByRole('button', { name: /delete/i });
      await userEvent.click(deleteButtons[0]);

      expect(screen.getByText(/delete tag/i)).toBeInTheDocument();
    });

    it('shows game count warning for tags with games', async () => {
      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      render(<TagsPage />);

      // Tags are sorted alphabetically by name, so order is: Action, RPG, Unused Tag
      // Click delete button for first tag (Action with 5 games)
      const deleteButtons = screen.getAllByRole('button', { name: /delete/i });
      await userEvent.click(deleteButtons[0]);

      // Check for the warning message which includes the tag name and game count
      expect(screen.getByText(/This will remove it from 5 games/i)).toBeInTheDocument();
    });

    it('calls deleteTag mutation when confirmed', async () => {
      const mutateAsyncFn = vi.fn().mockResolvedValue(undefined);

      mockedUseAllTags.mockReturnValue({
        data: mockTags,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      } as unknown as ReturnType<typeof useAllTags>);

      mockedUseDeleteTag.mockReturnValue(
        createMockMutation(mutateAsyncFn) as unknown as ReturnType<typeof useDeleteTag>
      );

      render(<TagsPage />);

      // Tags are sorted alphabetically by name, so order is: Action, RPG, Unused Tag
      // Click delete button for first tag (Action with id tag-2)
      const deleteButtons = screen.getAllByRole('button', { name: /delete/i });
      await userEvent.click(deleteButtons[0]);

      // Wait for dialog to appear
      await waitFor(() => {
        expect(screen.getByText('Delete Tag')).toBeInTheDocument();
      });

      // Find and click the confirmation button in the dialog
      // The AlertDialogAction has bg-destructive class
      const confirmButton = screen.getByRole('button', { name: /^delete$/i });
      await userEvent.click(confirmButton);

      await waitFor(() => {
        // First sorted tag is Action (tag-2)
        expect(mutateAsyncFn).toHaveBeenCalledWith('tag-2');
      });
    });
  });
});
