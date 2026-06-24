import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StorefrontIcon } from './platform-icon';
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

describe('StorefrontIcon', () => {
  it('renders the storefront image when icon_url is present', () => {
    render(<StorefrontIcon storefront={steam} />);
    expect(screen.getByRole('img', { name: 'Steam' })).toHaveAttribute(
      'src',
      '/logos/storefronts/steam/steam-icon-light.svg',
    );
  });

  it('renders a first-letter badge when icon_url is absent', () => {
    render(<StorefrontIcon storefront={{ ...steam, icon_url: undefined }} />);
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.getByText('S')).toBeInTheDocument();
  });
});
