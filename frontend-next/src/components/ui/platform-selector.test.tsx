import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import {
  PlatformSelector,
  PlatformSelectorCompact,
  type PlatformSelection,
} from './platform-selector';
import type { Platform, Storefront } from '@/types';

// ============================================================================
// Test Data
// ============================================================================

const mockStorefronts: Storefront[] = [
  {
    id: 'steam',
    name: 'steam',
    display_name: 'Steam',
    is_active: true,
    source: 'official',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'epic',
    name: 'epic',
    display_name: 'Epic Games Store',
    is_active: true,
    source: 'official',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'gog',
    name: 'gog',
    display_name: 'GOG',
    is_active: true,
    source: 'official',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

const mockPlatforms: Platform[] = [
  {
    id: 'pc',
    name: 'pc',
    display_name: 'PC',
    is_active: true,
    source: 'official',
    default_storefront_id: 'steam',
    storefronts: mockStorefronts,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'ps5',
    name: 'ps5',
    display_name: 'PlayStation 5',
    is_active: true,
    source: 'official',
    storefronts: [
      {
        id: 'psn',
        name: 'psn',
        display_name: 'PlayStation Store',
        is_active: true,
        source: 'official',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'xbox',
    name: 'xbox',
    display_name: 'Xbox Series X|S',
    is_active: true,
    source: 'official',
    storefronts: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'switch',
    name: 'switch',
    display_name: 'Nintendo Switch',
    is_active: true,
    source: 'official',
    storefronts: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

// ============================================================================
// PlatformSelector Tests
// ============================================================================

describe('PlatformSelector', () => {
  it('renders with placeholder when no platforms are selected', () => {
    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByRole('combobox')).toHaveTextContent('Select platforms...');
  });

  it('renders custom placeholder', () => {
    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
        placeholder="Choose your platforms"
      />
    );

    expect(screen.getByRole('combobox')).toHaveTextContent('Choose your platforms');
  });

  it('shows selected platform badge', () => {
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    // Badge shows in trigger (without remove button to avoid nested buttons)
    const trigger = screen.getByRole('combobox', { name: 'Select platforms' });
    expect(trigger).toHaveTextContent('PC');
    expect(trigger).toHaveTextContent('Steam');
  });

  it('shows "+N more" badge when many platforms are selected', () => {
    const selected: PlatformSelection[] = [
      { platform_id: 'pc' },
      { platform_id: 'ps5' },
      { platform_id: 'xbox' },
    ];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('+2 more')).toBeInTheDocument();
  });

  it('opens popover on click', async () => {
    const user = userEvent.setup();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));

    // Should see platform options
    expect(screen.getByText('Platforms')).toBeInTheDocument();
    expect(screen.getByText('PC')).toBeInTheDocument();
    expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
  });

  it('calls onChange when platform is selected', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));
    await user.click(screen.getByText('PC'));

    expect(handleChange).toHaveBeenCalledWith([
      { platform_id: 'pc', storefront_id: 'steam' }, // default storefront
    ]);
  });

  it('calls onChange when platform is deselected', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));
    // In the popover, click on the PC item to toggle it off
    const pcItem = screen.getByRole('option', { name: /PC/ });
    await user.click(pcItem);

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('shows storefront count for platforms with storefronts', async () => {
    const user = userEvent.setup();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));

    expect(screen.getByText('3 stores')).toBeInTheDocument(); // PC has 3 storefronts
    expect(screen.getByText('1 store')).toBeInTheDocument(); // PS5 has 1 storefront
  });

  it('filters platforms based on search', async () => {
    const user = userEvent.setup();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));
    await user.type(screen.getByPlaceholderText('Search platforms...'), 'play');

    expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
    expect(screen.queryByText('Xbox Series X|S')).not.toBeInTheDocument();
    expect(screen.queryByText('Nintendo Switch')).not.toBeInTheDocument();
  });

  it('displays storefront selector for selected platforms', () => {
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    // Should show the platform card below the trigger with storefront selector
    // The card has a remove button for that platform
    expect(screen.getByRole('button', { name: /remove pc/i })).toBeInTheDocument();
  });

  it('shows storefront selector for platforms with storefronts', () => {
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    // The storefront selector should be present (second combobox after main trigger)
    const comboboxes = screen.getAllByRole('combobox');
    expect(comboboxes).toHaveLength(2); // Main trigger + storefront selector
  });

  it('clears all selections when clear button is clicked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const selected: PlatformSelection[] = [
      { platform_id: 'pc', storefront_id: 'steam' },
      { platform_id: 'ps5' },
    ];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));
    await user.click(screen.getByText('Clear'));

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('respects maxSelection limit', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const selected: PlatformSelection[] = [
      { platform_id: 'pc' },
      { platform_id: 'ps5' },
    ];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
        maxSelection={2}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));

    // Xbox should be disabled since we're at max
    const xboxItem = screen.getByText('Xbox Series X|S').closest('[role="option"]');
    expect(xboxItem).toHaveClass('opacity-50');
  });

  it('shows max selection info', async () => {
    const user = userEvent.setup();
    const selected: PlatformSelection[] = [{ platform_id: 'pc' }];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
        maxSelection={3}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));

    expect(screen.getByText(/max 3/)).toBeInTheDocument();
  });

  it('is disabled when disabled prop is true', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
        disabled
      />
    );

    const combobox = screen.getByRole('combobox');
    expect(combobox).toBeDisabled();

    await user.click(combobox);
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('removes platform when X button is clicked in platform card', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />
    );

    // The remove button is in the platform card below the trigger, not in the trigger badge
    const removeButton = screen.getByRole('button', { name: /remove pc/i });
    await user.click(removeButton);

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('shows empty state when no platforms match search', async () => {
    const user = userEvent.setup();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));
    await user.type(screen.getByPlaceholderText('Search platforms...'), 'nonexistent');

    expect(screen.getByText(/no platforms matching/i)).toBeInTheDocument();
  });

  it('shows empty state when no platforms are available', async () => {
    const user = userEvent.setup();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={[]}
        onChange={vi.fn()}
      />
    );

    await user.click(screen.getByRole('combobox', { name: 'Select platforms' }));

    expect(screen.getByText('No platforms available')).toBeInTheDocument();
  });
});

// ============================================================================
// PlatformSelectorCompact Tests
// ============================================================================

describe('PlatformSelectorCompact', () => {
  it('renders all platforms as checkboxes', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes).toHaveLength(4); // All 4 mock platforms

    expect(screen.getByText('PC')).toBeInTheDocument();
    expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
    expect(screen.getByText('Xbox Series X|S')).toBeInTheDocument();
    expect(screen.getByText('Nintendo Switch')).toBeInTheDocument();
  });

  it('shows selected platforms as checked', () => {
    const selected: PlatformSelection[] = [{ platform_id: 'pc' }];

    render(
      <PlatformSelectorCompact
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    const pcCheckbox = checkboxes[0]; // PC is first

    expect(pcCheckbox).toBeChecked();
  });

  it('calls onChange when checkbox is toggled', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelectorCompact
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    await user.click(checkboxes[0]); // Click PC

    expect(handleChange).toHaveBeenCalledWith([
      { platform_id: 'pc', storefront_id: 'steam' }, // default storefront
    ]);
  });

  it('removes platform when unchecked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelectorCompact
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    await user.click(checkboxes[0]); // Uncheck PC

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('shows storefront selector for selected platforms with storefronts', async () => {
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelectorCompact
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    // Should show storefront label
    expect(screen.getByText('Storefront:')).toBeInTheDocument();
  });

  it('shows storefront selector when platform with storefronts is selected', () => {
    const selected: PlatformSelection[] = [{ platform_id: 'pc', storefront_id: 'steam' }];

    render(
      <PlatformSelectorCompact
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    // The storefront selector combobox should be present
    expect(screen.getByRole('combobox')).toBeInTheDocument();
  });

  it('does not show storefront selector for platforms without storefronts', () => {
    const selected: PlatformSelection[] = [{ platform_id: 'xbox' }];

    render(
      <PlatformSelectorCompact
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    // Xbox has no storefronts, so no storefront label should appear
    expect(screen.queryByText('Storefront:')).not.toBeInTheDocument();
  });

  it('shows empty state when no platforms are available', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[]}
        availablePlatforms={[]}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('No platforms available')).toBeInTheDocument();
  });

  it('is disabled when disabled prop is true', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelectorCompact
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
        disabled
      />
    );

    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes[0]).toBeDisabled();

    await user.click(checkboxes[0]);
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('highlights selected platforms visually', () => {
    const selected: PlatformSelection[] = [{ platform_id: 'pc' }];

    render(
      <PlatformSelectorCompact
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />
    );

    // The PC platform container should have the selected styling
    const pcText = screen.getByText('PC');
    const pcContainer = pcText.closest('.rounded-lg');
    expect(pcContainer).toHaveClass('bg-primary/5');
  });
});
