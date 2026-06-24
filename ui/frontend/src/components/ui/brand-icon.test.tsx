import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { BrandIcon } from './brand-icon';

let mockResolvedTheme: string | undefined = 'light';
vi.mock('next-themes', () => ({
  useTheme: () => ({ resolvedTheme: mockResolvedTheme }),
}));

describe('BrandIcon', () => {
  beforeEach(() => {
    mockResolvedTheme = 'light';
  });

  it('renders the image when iconUrl is present', () => {
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    const img = screen.getByRole('img', { name: 'PC' });
    expect(img).toHaveAttribute('src', '/logos/platforms/pc/pc-icon-light.svg');
  });

  it('renders a first-letter badge when iconUrl is absent', () => {
    render(<BrandIcon displayName="Steam" />);
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.getByText('S')).toBeInTheDocument();
  });

  it('selects the dark variant under a dark resolvedTheme', () => {
    mockResolvedTheme = 'dark';
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    expect(screen.getByRole('img', { name: 'PC' })).toHaveAttribute(
      'src',
      '/logos/platforms/pc/pc-icon-dark.svg',
    );
  });

  it('does not swap when the path has no -icon-light.svg token', () => {
    mockResolvedTheme = 'dark';
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc.png" displayName="PC" />);
    expect(screen.getByRole('img', { name: 'PC' })).toHaveAttribute(
      'src',
      '/logos/platforms/pc/pc.png',
    );
  });

  it('falls back to the stored (light) asset when the dark variant errors', () => {
    mockResolvedTheme = 'dark';
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    const img = screen.getByRole('img', { name: 'PC' });
    expect(img).toHaveAttribute('src', '/logos/platforms/pc/pc-icon-dark.svg');
    fireEvent.error(img);
    expect(screen.getByRole('img', { name: 'PC' })).toHaveAttribute(
      'src',
      '/logos/platforms/pc/pc-icon-light.svg',
    );
  });

  it('falls back to the first-letter badge when the image fails with no further variant', () => {
    render(<BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" />);
    fireEvent.error(screen.getByRole('img', { name: 'PC' }));
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.getByText('P')).toBeInTheDocument();
  });

  it('renders the label inline when showLabel is set', () => {
    render(<BrandIcon iconUrl="/logos/x/x-icon-light.svg" displayName="Epic" showLabel />);
    expect(screen.getByText('Epic')).toBeInTheDocument();
  });

  it('hides the image from screen readers when decorative', () => {
    render(
      <BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" decorative />,
    );
    // alt="" makes the image presentational — no accessible name to double up with the label.
    expect(screen.queryByRole('img', { name: 'PC' })).toBeNull();
    expect(screen.getByRole('presentation', { hidden: true })).toHaveAttribute('alt', '');
  });

  it('hides the first-letter badge from screen readers when decorative', () => {
    render(<BrandIcon displayName="Steam" decorative />);
    expect(screen.getByText('S')).toHaveAttribute('aria-hidden', 'true');
  });

  it('hides the icon from screen readers when showLabel is set (label carries the name)', () => {
    render(<BrandIcon iconUrl="/logos/x/x-icon-light.svg" displayName="Epic" showLabel />);
    expect(screen.queryByRole('img', { name: 'Epic' })).toBeNull();
    expect(screen.getByText('Epic')).toBeInTheDocument();
  });

  it('keeps the icon accessible name when standalone (not decorative)', () => {
    render(
      <BrandIcon iconUrl="/logos/platforms/pc/pc-icon-light.svg" displayName="PC" showTooltip />,
    );
    expect(screen.getByRole('img', { name: 'PC' })).toBeInTheDocument();
  });
});
