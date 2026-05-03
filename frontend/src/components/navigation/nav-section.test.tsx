import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect } from 'vitest';
import { NavSectionCollapsible } from './nav-section';
import { Settings, Tag, User } from 'lucide-react';

describe('NavSectionCollapsible', () => {
  const defaultProps = {
    label: 'Settings',
    icon: <Settings className="h-4 w-4" data-testid="section-icon" />,
    items: [
      { href: '/tags', label: 'Tags', icon: <Tag className="h-4 w-4" /> },
      { href: '/profile', label: 'Profile', icon: <User className="h-4 w-4" /> },
    ],
  };

  it('renders section label', () => {
    render(<NavSectionCollapsible {...defaultProps} />);
    expect(screen.getByText('Settings')).toBeInTheDocument();
  });

  it('is collapsed by default', () => {
    render(<NavSectionCollapsible {...defaultProps} />);
    expect(screen.queryByText('Tags')).not.toBeInTheDocument();
  });

  it('expands when clicked', async () => {
    const user = userEvent.setup();
    render(<NavSectionCollapsible {...defaultProps} />);

    await user.click(screen.getByText('Settings'));
    expect(screen.getByText('Tags')).toBeInTheDocument();
    expect(screen.getByText('Profile')).toBeInTheDocument();
  });

  it('collapses when clicked again', async () => {
    const user = userEvent.setup();
    render(<NavSectionCollapsible {...defaultProps} />);

    await user.click(screen.getByText('Settings'));
    expect(screen.getByText('Tags')).toBeInTheDocument();

    await user.click(screen.getByText('Settings'));
    expect(screen.queryByText('Tags')).not.toBeInTheDocument();
  });

  it('respects defaultOpen prop', () => {
    render(<NavSectionCollapsible {...defaultProps} defaultOpen={true} />);
    expect(screen.getByText('Tags')).toBeInTheDocument();
  });

  describe('needsAttention behavior', () => {
    it('should auto-expand when needsAttention is true', () => {
      render(
        <NavSectionCollapsible
          {...defaultProps}
          defaultOpen={false}
          needsAttention={true}
        />
      );

      // Content should be visible because needsAttention overrides defaultOpen
      expect(screen.getByText('Tags')).toBeInTheDocument();
    });

    it('should stay collapsed when needsAttention is false and defaultOpen is false', () => {
      render(
        <NavSectionCollapsible
          {...defaultProps}
          defaultOpen={false}
          needsAttention={false}
        />
      );

      // Content should not be visible
      expect(screen.queryByText('Tags')).not.toBeInTheDocument();
    });

    it('should expand when needsAttention changes from false to true', async () => {
      const { rerender } = render(
        <NavSectionCollapsible
          {...defaultProps}
          defaultOpen={false}
          needsAttention={false}
        />
      );

      // Initially collapsed
      expect(screen.queryByText('Tags')).not.toBeInTheDocument();

      // Rerender with needsAttention=true
      rerender(
        <NavSectionCollapsible
          {...defaultProps}
          defaultOpen={false}
          needsAttention={true}
        />
      );

      // Should now be expanded
      expect(screen.getByText('Tags')).toBeInTheDocument();
    });
  });
});
