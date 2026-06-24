import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import {
  PlatformSelector,
  PlatformSelectorCompact,
  type PlatformSelection,
} from './platform-selector';
import type { Platform, Storefront } from '@/types';

vi.mock('next-themes', () => ({ useTheme: () => ({ resolvedTheme: 'light' }) }));

// Helper: build a PlatformSelection with the now-required `key`.
// NOTE: pass an explicit `key` when the same platform appears twice (the default collides).
const sel = (platform: string, storefront?: string, key = `k-${platform}`): PlatformSelection => ({
  key,
  platform,
  storefront,
});

// ============================================================================
// Test Data
// ============================================================================

const mockStorefronts: Storefront[] = [
  {
    name: 'steam',
    display_name: 'Steam',
    icon_url: '/logos/storefronts/steam/steam-icon-light.svg',
    is_active: true,
    source: 'official',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'epic-games-store',
    display_name: 'Epic Games Store',
    is_active: true,
    source: 'official',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
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
    name: 'pc',
    display_name: 'PC',
    is_active: true,
    source: 'official',
    default_storefront: 'steam',
    storefronts: mockStorefronts,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'ps5',
    display_name: 'PlayStation 5',
    is_active: true,
    source: 'official',
    storefronts: [
      {
        name: 'playstation-store',
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
    name: 'xbox',
    display_name: 'Xbox Series X|S',
    is_active: true,
    source: 'official',
    storefronts: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'switch',
    display_name: 'Nintendo Switch',
    is_active: true,
    source: 'official',
    storefronts: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

// A blank row, as produced by the "Add platform" button before a platform is chosen.
const blank = (key: string): PlatformSelection => ({ key, platform: '', storefront: undefined });

// ============================================================================
// PlatformSelector (row-based editor)
// ============================================================================

describe('PlatformSelector', () => {
  it('renders an "Add platform" button', () => {
    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: /add platform/i })).toBeInTheDocument();
  });

  it('appends a blank row when "Add platform" is clicked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getByRole('button', { name: /add platform/i }));

    expect(handleChange).toHaveBeenCalledTimes(1);
    const next = handleChange.mock.calls[0][0] as PlatformSelection[];
    expect(next).toHaveLength(1);
    expect(next[0].platform).toBe('');
    expect(next[0].key).toBeTruthy();
  });

  it('renders one row per selection showing the platform name', () => {
    render(
      <PlatformSelector
        selectedPlatforms={[sel('pc', 'steam'), sel('ps5', 'playstation-store')]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Remove PC / Steam' })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Remove PlayStation 5 / PlayStation Store' }),
    ).toBeInTheDocument();
  });

  it('sets the platform and a default storefront when a platform is picked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelector
        selectedPlatforms={[blank('new-1')]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getByRole('combobox', { name: /select platform/i }));
    await user.click(screen.getByRole('option', { name: /PC$/ }));

    expect(handleChange).toHaveBeenCalledWith([
      { key: 'new-1', platform: 'pc', storefront: 'steam' },
    ]);
  });

  it('defaults a second copy of a platform to a free storefront, never duplicating (#848)', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelector
        selectedPlatforms={[sel('pc', 'steam', 'k-1'), blank('k-2')]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getByRole('combobox', { name: /select platform/i }));
    await user.click(screen.getByRole('option', { name: /PC$/ }));

    expect(handleChange).toHaveBeenCalledWith([
      { key: 'k-1', platform: 'pc', storefront: 'steam' },
      { key: 'k-2', platform: 'pc', storefront: 'epic-games-store' }, // not steam — that slot is taken
    ]);
  });

  it('disables a platform whose every storefront slot is already taken', async () => {
    const user = userEvent.setup();
    const exhausted: PlatformSelection[] = [
      sel('pc', 'steam', 'k-1'),
      sel('pc', 'epic-games-store', 'k-2'),
      sel('pc', 'gog', 'k-3'),
      sel('pc', undefined, 'k-4'),
    ];

    render(
      <PlatformSelector
        selectedPlatforms={[...exhausted, blank('k-5')]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('combobox', { name: /select platform/i }));

    expect(screen.getByRole('option', { name: /PC$/ })).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByRole('option', { name: /PlayStation 5$/ })).not.toHaveAttribute(
      'aria-disabled',
      'true',
    );
  });

  it('removes only the targeted row when two rows share a platform (#846)', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const selected: PlatformSelection[] = [
      { key: 'k-1', platform: 'pc', storefront: 'steam' },
      { key: 'k-2', platform: 'pc', storefront: 'epic-games-store' },
    ];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Remove PC / Epic Games Store' }));

    expect(handleChange).toHaveBeenCalledWith([
      { key: 'k-1', platform: 'pc', storefront: 'steam' },
    ]);
  });

  it('disables the Add platform button when disabled', () => {
    render(
      <PlatformSelector
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
        disabled
      />,
    );

    expect(screen.getByRole('button', { name: /add platform/i })).toBeDisabled();
  });
});

// ============================================================================
// PlatformSelectorCompact (add wizard)
// ============================================================================

describe('PlatformSelectorCompact', () => {
  it('renders all platforms as checkboxes', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getAllByRole('checkbox')).toHaveLength(4);
    expect(screen.getByText('PC')).toBeInTheDocument();
    expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
  });

  it('shows selected platforms as checked', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('pc')]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getAllByRole('checkbox')[0]).toBeChecked();
  });

  it('adds a row with the default storefront when a checkbox is checked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelectorCompact
        selectedPlatforms={[]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getAllByRole('checkbox')[0]); // PC

    expect(handleChange).toHaveBeenCalledWith([
      expect.objectContaining({ platform: 'pc', storefront: 'steam' }),
    ]);
  });

  it('removes all rows of a platform when unchecked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('pc', 'steam', 'k-1'), sel('pc', 'epic-games-store', 'k-2')]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getAllByRole('checkbox')[0]); // uncheck PC

    expect(handleChange).toHaveBeenCalledWith([]);
  });

  it('shows an "add another storefront" affordance for a checked platform with free slots', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('pc', 'steam')]}
        availablePlatforms={[mockPlatforms[0]]}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: /add another storefront/i })).toBeInTheDocument();
  });

  it('adds a second copy with a different storefront via "add another storefront" (#848)', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('pc', 'steam', 'k-1')]}
        availablePlatforms={[mockPlatforms[0]]}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getByRole('button', { name: /add another storefront/i }));

    expect(handleChange).toHaveBeenCalledTimes(1);
    const next = handleChange.mock.calls[0][0] as PlatformSelection[];
    expect(next).toHaveLength(2);
    expect(next[1]).toEqual(
      expect.objectContaining({ platform: 'pc', storefront: 'epic-games-store' }),
    );
  });

  it('hides "add another storefront" when the platform is exhausted', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[
          sel('pc', 'steam', 'k-1'),
          sel('pc', 'epic-games-store', 'k-2'),
          sel('pc', 'gog', 'k-3'),
          sel('pc', undefined, 'k-4'),
        ]}
        availablePlatforms={[mockPlatforms[0]]}
        onChange={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole('button', { name: /add another storefront/i }),
    ).not.toBeInTheDocument();
  });

  it('lets an extra storefront row be removed individually', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('pc', 'steam', 'k-1'), sel('pc', 'epic-games-store', 'k-2')]}
        availablePlatforms={[mockPlatforms[0]]}
        onChange={handleChange}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Remove PC / Epic Games Store' }));

    expect(handleChange).toHaveBeenCalledWith([
      { key: 'k-1', platform: 'pc', storefront: 'steam' },
    ]);
  });

  it('does not show a storefront selector for platforms without storefronts', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('xbox')]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );

    expect(screen.queryByText('Storefront:')).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /add another storefront/i }),
    ).not.toBeInTheDocument();
  });

  it('shows empty state when no platforms are available', () => {
    render(
      <PlatformSelectorCompact selectedPlatforms={[]} availablePlatforms={[]} onChange={vi.fn()} />,
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
      />,
    );

    expect(screen.getAllByRole('checkbox')[0]).toBeDisabled();
    await user.click(screen.getAllByRole('checkbox')[0]);
    expect(handleChange).not.toHaveBeenCalled();
  });

  it('highlights selected platforms visually', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('pc')]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );

    const pcContainer = screen.getByText('PC').closest('.rounded-lg');
    expect(pcContainer).toHaveClass('bg-primary/5');
  });

  it('renders a storefront selector per row for a checked platform', () => {
    render(
      <PlatformSelectorCompact
        selectedPlatforms={[sel('pc', 'steam', 'k-1'), sel('pc', 'epic-games-store', 'k-2')]}
        availablePlatforms={[mockPlatforms[0]]}
        onChange={vi.fn()}
      />,
    );

    const pcCard = screen.getByText('PC').closest('.rounded-lg') as HTMLElement;
    expect(within(pcCard).getAllByText('Storefront:')).toHaveLength(2);
  });
});

// ============================================================================
// StorefrontSelector (combobox)
// ============================================================================

describe('StorefrontSelector (via PlatformSelector row editor)', () => {
  it('renders a searchable combobox and selects a storefront with an icon', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <PlatformSelector
        selectedPlatforms={[sel('pc', 'steam')]}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );
    // The storefront trigger is a combobox button, not a native <select>.
    const triggers = screen.getAllByRole('combobox');
    // Open the storefront combobox (the row's second combobox) and pick Epic Games Store.
    await user.click(triggers[triggers.length - 1]);
    await user.click(screen.getByText('Epic Games Store'));
    expect(handleChange).toHaveBeenCalledWith([
      { key: 'k-pc', platform: 'pc', storefront: 'epic-games-store' },
    ]);
  });

  it('renders the storefront icon in the combobox trigger when a storefront is selected', () => {
    render(
      <PlatformSelector
        selectedPlatforms={[sel('pc', 'steam')]}
        availablePlatforms={mockPlatforms}
        onChange={vi.fn()}
      />,
    );
    // Steam has an icon_url — the trigger should render an img with the steam icon.
    const steamIcon = screen.getByRole('img', { name: 'Steam' });
    expect(steamIcon).toBeInTheDocument();
  });
});
