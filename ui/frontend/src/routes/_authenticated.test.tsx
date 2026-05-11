import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AuthenticatedLayout } from './_authenticated';

const mockUseHealthStatus = vi.fn();
vi.mock('@/hooks/use-health-status', () => ({
  useHealthStatus: () => mockUseHealthStatus(),
}));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Outlet: () => null,
}));

vi.mock('@/components/route-guard', () => ({
  RouteGuard: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('@/components/navigation', () => ({
  Sidebar: () => null,
  MobileNav: () => null,
}));

describe('AuthenticatedLayout IGDB banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows IGDB warning banner when igdb_configured is false', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_configured: false } });
    render(<AuthenticatedLayout />);
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText(/IGDB is not configured/)).toBeInTheDocument();
  });

  it('does not show IGDB banner when igdb_configured is true', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_configured: true } });
    render(<AuthenticatedLayout />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('does not show IGDB banner while health data is loading', () => {
    mockUseHealthStatus.mockReturnValue({ data: undefined });
    render(<AuthenticatedLayout />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
