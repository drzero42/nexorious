import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { StorefrontLabel } from './storefront-link';
import type { Storefront } from '@/types';

vi.mock('next-themes', () => ({ useTheme: () => ({ resolvedTheme: 'light' }) }));

const steam: Storefront = {
  name: 'steam',
  display_name: 'Steam',
  is_active: true,
  source: 'official',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  icon_url: '/logos/storefronts/steam/steam-icon-light.svg',
};

describe('StorefrontLabel', () => {
  it('renders a new-tab link (with icon) when store_url is present', () => {
    render(
      <StorefrontLabel storefront={steam} storeUrl="https://store.steampowered.com/app/440/" />,
    );
    const link = screen.getByRole('link', { name: /steam/i });
    expect(link).toHaveAttribute('href', 'https://store.steampowered.com/app/440/');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    // The icon sits next to the visible "Steam" label, so it is decorative
    // (alt="") — the label carries the accessible name.
    const icon = link.querySelector('img[src="/logos/storefronts/steam/steam-icon-light.svg"]');
    expect(icon).toBeInTheDocument();
    expect(icon).toHaveAttribute('alt', '');
  });

  it('renders a plain label (no parens) when store_url is absent', () => {
    render(<StorefrontLabel storefront={{ ...steam, display_name: 'Humble' }} />);
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('Humble')).toBeInTheDocument();
  });
});
