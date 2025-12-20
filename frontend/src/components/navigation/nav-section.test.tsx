// frontend/src/components/navigation/nav-section.test.tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { NavSectionCollapsible } from './nav-section';
import { Settings, Tag, User } from 'lucide-react';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: vi.fn(() => '/other'),
  useRouter: vi.fn(() => ({ push: vi.fn() })),
}));

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
});
