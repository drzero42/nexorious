import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { NavLink } from './nav-link';
import { Library } from 'lucide-react';

// Mock @tanstack/react-router - override useRouterState to return '/games' as pathname
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useRouterState: vi.fn((opts?: { select?: (s: unknown) => unknown }) => {
      const state = { location: { pathname: '/games', search: '', hash: '' } };
      return opts?.select ? opts.select(state) : state;
    }),
    Link: ({ children, to, ...props }: { children: React.ReactNode; to: string; [key: string]: unknown }) => (
      <a href={to} {...props as React.HTMLAttributes<HTMLAnchorElement>}>{children}</a>
    ),
  };
});

describe('NavLink', () => {
  const defaultProps = {
    href: '/games',
    label: 'Library',
    icon: <Library className="h-4 w-4" data-testid="icon" />,
  };

  it('renders label and icon', () => {
    render(<NavLink {...defaultProps} />);
    expect(screen.getByText('Library')).toBeInTheDocument();
    expect(screen.getByTestId('icon')).toBeInTheDocument();
  });

  it('renders as a link', () => {
    render(<NavLink {...defaultProps} />);
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', '/games');
  });

  it('shows badge when count > 0', () => {
    render(<NavLink {...defaultProps} badge={5} />);
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('hides badge when count is 0', () => {
    render(<NavLink {...defaultProps} badge={0} />);
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('applies active styles when pathname matches href', () => {
    render(<NavLink {...defaultProps} />);
    const link = screen.getByRole('link');
    expect(link).toHaveClass('bg-primary');
  });

  it('calls onNavigate when clicked', async () => {
    const user = userEvent.setup();
    const onNavigate = vi.fn();
    render(<NavLink {...defaultProps} onNavigate={onNavigate} />);

    await user.click(screen.getByRole('link'));
    expect(onNavigate).toHaveBeenCalled();
  });
});
