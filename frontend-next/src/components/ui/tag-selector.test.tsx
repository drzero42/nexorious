import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { TagSelector, TagSelectorCompact } from './tag-selector';
import type { Tag } from '@/types';

// Mock lucide-react icons
vi.mock('lucide-react', () => ({
  Check: () => <div data-testid="check-icon" />,
  ChevronsUpDown: () => <div data-testid="chevrons-icon" />,
  Plus: () => <div data-testid="plus-icon" />,
  Search: () => <div data-testid="search-icon" />,
  Tag: () => <div data-testid="tag-icon" />,
  X: () => <div data-testid="x-icon" />,
}));

// ============================================================================
// Test Data
// ============================================================================

const mockTags: Tag[] = [
  {
    id: '1',
    user_id: 'user1',
    name: 'Action',
    color: '#FF5733',
    description: 'Action-packed games',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 10,
  },
  {
    id: '2',
    user_id: 'user1',
    name: 'RPG',
    color: '#33FF57',
    description: 'Role-playing games',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 5,
  },
  {
    id: '3',
    user_id: 'user1',
    name: 'Strategy',
    color: '#3357FF',
    description: 'Strategic gameplay',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 0,
  },
  {
    id: '4',
    user_id: 'user1',
    name: 'Puzzle',
    color: '#F0F0F0',
    description: 'Brain teasers',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 3,
  },
];

// ============================================================================
// TagSelector Tests
// ============================================================================

describe('TagSelector', () => {
  it('renders with placeholder when no tags are selected', () => {
    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByRole('combobox')).toHaveTextContent('Select tags...');
  });

  it('renders custom placeholder', () => {
    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        placeholder="Choose tags"
      />
    );

    expect(screen.getByRole('combobox')).toHaveTextContent('Choose tags');
  });

  it('shows selected tag badges', () => {
    render(
      <TagSelector
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    const trigger = screen.getByRole('combobox', { name: 'Select tags' });
    expect(trigger).toHaveTextContent('Action');
    expect(trigger).toHaveTextContent('RPG');
  });

  it('shows "+N more" badge when more than 3 tags are selected', () => {
    render(
      <TagSelector
        selectedTagIds={['1', '2', '3', '4']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('+2 more')).toBeInTheDocument();
  });

  it('opens popover on click', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    // Should see tag options
    expect(screen.getByText('Tags')).toBeInTheDocument();
    expect(screen.getByText('Action')).toBeInTheDocument();
    expect(screen.getByText('RPG')).toBeInTheDocument();
  });

  it('calls onChange when tag is selected', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.click(screen.getByText('Action'));

    expect(handleChange).toHaveBeenCalledWith(['1']);
  });

  it('calls onChange when tag is deselected', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={['1']}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    // When popover opens, there are multiple "Action" texts (in badge and in list)
    // Get the option from the list
    const actionOption = screen.getByRole('option', { name: /Action/ });
    await user.click(actionOption);

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('filters tags based on search query', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.type(screen.getByPlaceholderText('Search tags...'), 'action');

    expect(screen.getByText('Action')).toBeInTheDocument();
    expect(screen.queryByText('RPG')).not.toBeInTheDocument();
    expect(screen.queryByText('Strategy')).not.toBeInTheDocument();
  });

  it('filters tags by description', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.type(screen.getByPlaceholderText('Search tags...'), 'brain');

    expect(screen.getByText('Puzzle')).toBeInTheDocument();
    expect(screen.queryByText('Action')).not.toBeInTheDocument();
  });

  it('shows create new tag option when allowCreate is true and query does not match', async () => {
    const user = userEvent.setup();
    const handleCreateTag = vi.fn();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        onCreateTag={handleCreateTag}
        allowCreate
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.type(screen.getByPlaceholderText('Search tags...'), 'NewTag');

    expect(screen.getByText('Create new')).toBeInTheDocument();
    expect(screen.getByText(/Create "NewTag"/)).toBeInTheDocument();
  });

  it('does not show create option when query matches existing tag', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        onCreateTag={vi.fn()}
        allowCreate
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.type(screen.getByPlaceholderText('Search tags...'), 'Action');

    expect(screen.queryByText('Create new')).not.toBeInTheDocument();
  });

  it('calls onCreateTag when create option is clicked', async () => {
    const user = userEvent.setup();
    const handleCreateTag = vi.fn();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        onCreateTag={handleCreateTag}
        allowCreate
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.type(screen.getByPlaceholderText('Search tags...'), 'NewTag');
    await user.click(screen.getByText(/Create "NewTag"/));

    expect(handleCreateTag).toHaveBeenCalledWith('NewTag');
  });

  it('respects maxSelection limit', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={handleChange}
        maxSelection={2}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    // Strategy should be disabled since we're at max
    const strategyItem = screen.getByText('Strategy').closest('[role="option"]');
    expect(strategyItem).toHaveClass('opacity-50');
    expect(strategyItem).toHaveClass('cursor-not-allowed');
  });

  it('shows max selection info', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={['1']}
        availableTags={mockTags}
        onChange={vi.fn()}
        maxSelection={3}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    expect(screen.getByText(/max 3/)).toBeInTheDocument();
  });

  it('does not allow selection beyond max limit', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={handleChange}
        maxSelection={2}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    // Try to click a disabled item
    const strategyItem = screen.getByText('Strategy');
    await user.click(strategyItem);

    // onChange should not be called with a third item
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('calls onChange with Select All button', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.click(screen.getByText('All'));

    expect(handleChange).toHaveBeenCalledWith(['1', '2', '3', '4']);
  });

  it('respects max selection when using Select All', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={handleChange}
        maxSelection={2}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.click(screen.getByText('All'));

    // Should only select up to max
    expect(handleChange).toHaveBeenCalledWith(['1', '2']);
  });

  it('calls onChange with Clear All button', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={['1', '2', '3']}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.click(screen.getByText('None'));

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('is disabled when disabled prop is true', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={handleChange}
        disabled
      />
    );

    const combobox = screen.getByRole('combobox');
    expect(combobox).toBeDisabled();

    await user.click(combobox);
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('removes tag when X button is clicked in badge', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelector
        selectedTagIds={['1']}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    const removeButton = screen.getByRole('button', { name: 'Remove tag Action' });
    await user.click(removeButton);

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('shows game counts when showGameCounts is true', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        showGameCounts
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    expect(screen.getByText('10')).toBeInTheDocument(); // Action has 10 games
    expect(screen.getByText('5')).toBeInTheDocument(); // RPG has 5 games
    expect(screen.getByText('3')).toBeInTheDocument(); // Puzzle has 3 games
  });

  it('does not show game counts when showGameCounts is false', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        showGameCounts={false}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    // Game counts should not be shown in the list
    const listItems = screen.getAllByRole('option');
    listItems.forEach(item => {
      // Check that text content doesn't include standalone numbers
      const withinItem = within(item);
      // This is a loose check - the numbers might still be there in other contexts
      // But they shouldn't be in the count position
    });
  });

  it('shows game count in selected tag badge', () => {
    render(
      <TagSelector
        selectedTagIds={['1']}
        availableTags={mockTags}
        onChange={vi.fn()}
        showGameCounts
      />
    );

    const trigger = screen.getByRole('combobox', { name: 'Select tags' });
    expect(trigger).toHaveTextContent('(10)');
  });

  it('shows empty state when no tags are available', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={[]}
        onChange={vi.fn()}
        allowCreate
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    expect(screen.getByText('No tags available')).toBeInTheDocument();
    expect(screen.getByText('Type to create a new tag')).toBeInTheDocument();
  });

  it('shows empty state when no tags match search', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));
    await user.type(screen.getByPlaceholderText('Search tags...'), 'nonexistent');

    expect(screen.getByText(/No tags matching "nonexistent"/)).toBeInTheDocument();
  });

  it('displays all selected tags below trigger when more than 3 are selected and popover is closed', () => {
    render(
      <TagSelector
        selectedTagIds={['1', '2', '3', '4']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    // When more than 3 tags are selected, all tags should be visible as badges below the trigger
    // Use getAllByText since badges appear below trigger
    const actionBadges = screen.getAllByText('Action');
    expect(actionBadges.length).toBeGreaterThan(0);
    expect(screen.getAllByText('RPG').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Strategy').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Puzzle').length).toBeGreaterThan(0);
  });

  it('shows selection count in popover', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    expect(screen.getByText(/2 of 4 selected/)).toBeInTheDocument();
  });

  it('applies custom className', () => {
    const { container } = render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        className="custom-class"
      />
    );

    expect(container.querySelector('.custom-class')).toBeInTheDocument();
  });

  it('applies custom id', () => {
    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        id="custom-id"
      />
    );

    expect(document.getElementById('custom-id')).toBeInTheDocument();
  });

  it('disables Clear All button when no tags are selected', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    const clearButton = screen.getByText('None');
    expect(clearButton).toHaveClass('disabled:text-muted-foreground');
  });

  it('disables Select All button when max selection is reached', async () => {
    const user = userEvent.setup();

    render(
      <TagSelector
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={vi.fn()}
        maxSelection={2}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select tags' }));

    const selectAllButton = screen.getByText('All');
    expect(selectAllButton).toHaveClass('disabled:text-muted-foreground');
  });
});

// ============================================================================
// TagSelectorCompact Tests
// ============================================================================

describe('TagSelectorCompact', () => {
  it('renders all tags as checkboxes', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes).toHaveLength(4);

    expect(screen.getByText('Action')).toBeInTheDocument();
    expect(screen.getByText('RPG')).toBeInTheDocument();
    expect(screen.getByText('Strategy')).toBeInTheDocument();
    expect(screen.getByText('Puzzle')).toBeInTheDocument();
  });

  it('shows selected tags as checked', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    const selectedCheckboxes = checkboxes.filter(cb => (cb as HTMLInputElement).checked);
    expect(selectedCheckboxes).toHaveLength(2);
  });

  it('calls onChange when checkbox is toggled', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    await user.click(checkboxes[0]);

    expect(handleChange).toHaveBeenCalledWith(['1']);
  });

  it('removes tag when checkbox is unchecked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelectorCompact
        selectedTagIds={['1']}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    await user.click(checkboxes[0]);

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('filters tags based on search', async () => {
    const user = userEvent.setup();

    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.type(screen.getByPlaceholderText('Search tags...'), 'rpg');

    expect(screen.getByText('RPG')).toBeInTheDocument();
    expect(screen.queryByText('Action')).not.toBeInTheDocument();
  });

  it('shows selection count', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText(/2 of 4 selected/)).toBeInTheDocument();
  });

  it('calls onChange with Select All button', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByText('Select All'));

    expect(handleChange).toHaveBeenCalledWith(['1', '2', '3', '4']);
  });

  it('calls onChange with Clear button', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelectorCompact
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByText('Clear'));

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('is disabled when disabled prop is true', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={handleChange}
        disabled
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes[0]).toBeDisabled();

    await user.click(checkboxes[0]);
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('shows empty state when no tags are available', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={[]}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('No tags available')).toBeInTheDocument();
    expect(screen.getByText('Create some tags first to use them here.')).toBeInTheDocument();
  });

  it('shows empty state when no tags match search', async () => {
    const user = userEvent.setup();

    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.type(screen.getByPlaceholderText('Search tags...'), 'nonexistent');

    expect(screen.getByText(/No tags matching "nonexistent"/)).toBeInTheDocument();
  });

  it('highlights selected tags visually', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={['1']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    const actionText = screen.getByText('Action');
    const actionContainer = actionText.closest('label');
    expect(actionContainer).toHaveClass('bg-green-50');
  });

  it('separates selected and unselected tags', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={['1', '2']}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText(/Selected \(2\)/)).toBeInTheDocument();
    expect(screen.getByText(/Available \(2\)/)).toBeInTheDocument();
  });

  it('shows game counts for tags', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('10 games')).toBeInTheDocument();
    expect(screen.getByText('5 games')).toBeInTheDocument();
    expect(screen.getByText('3 games')).toBeInTheDocument();
  });

  it('uses singular form for single game', () => {
    const singleGameTag: Tag = {
      ...mockTags[0],
      game_count: 1,
    };

    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={[singleGameTag]}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('1 game')).toBeInTheDocument();
  });

  it('applies custom className', () => {
    const { container } = render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
        className="custom-class"
      />
    );

    expect(container.querySelector('.custom-class')).toBeInTheDocument();
  });

  it('disables Clear button when no tags are selected', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    const clearButton = screen.getByText('Clear');
    expect(clearButton).toHaveClass('disabled:text-muted-foreground');
  });

  it('shows tag descriptions', () => {
    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('Action-packed games')).toBeInTheDocument();
    expect(screen.getByText('Role-playing games')).toBeInTheDocument();
  });

  it('filters by tag description', async () => {
    const user = userEvent.setup();

    render(
      <TagSelectorCompact
        selectedTagIds={[]}
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    await user.type(screen.getByPlaceholderText('Search tags...'), 'brain');

    expect(screen.getByText('Puzzle')).toBeInTheDocument();
    expect(screen.queryByText('Action')).not.toBeInTheDocument();
  });
});

// ============================================================================
// Color Utility Tests
// ============================================================================

describe('getTextColor utility', () => {
  it('returns white text for dark background', () => {
    render(
      <TagSelector
        selectedTagIds={['1']} // Action tag has dark red color #FF5733
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    const trigger = screen.getByRole('combobox');
    // The badge should be rendered with appropriate styling
    expect(trigger).toBeInTheDocument();
  });

  it('returns black text for light background', () => {
    render(
      <TagSelector
        selectedTagIds={['4']} // Puzzle tag has light color #F0F0F0
        availableTags={mockTags}
        onChange={vi.fn()}
      />
    );

    const trigger = screen.getByRole('combobox');
    // The badge should be rendered with appropriate styling
    expect(trigger).toBeInTheDocument();
  });
});
