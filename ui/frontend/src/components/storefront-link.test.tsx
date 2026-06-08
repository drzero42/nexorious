import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { StorefrontLabel } from './storefront-link';

describe('StorefrontLabel', () => {
  it('renders a new-tab link when store_url is present', () => {
    render(
      <StorefrontLabel displayName="Steam" storeUrl="https://store.steampowered.com/app/440/" />,
    );
    const link = screen.getByRole('link', { name: /steam/i });
    expect(link).toHaveAttribute('href', 'https://store.steampowered.com/app/440/');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('renders a plain label when store_url is absent', () => {
    render(<StorefrontLabel displayName="Humble" />);
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText(/humble/i)).toBeInTheDocument();
  });
});
