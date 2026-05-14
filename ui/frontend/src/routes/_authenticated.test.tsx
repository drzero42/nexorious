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

vi.mock('@/hooks', () => ({
  useActiveJob: () => ({ data: undefined }),
}));

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

describe('AuthenticatedLayout IGDB banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows "not configured" banner when igdb_status is not_configured', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'not_configured' } });
    render(<AuthenticatedLayout />);
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText(/IGDB is not configured/)).toBeInTheDocument();
    expect(screen.getByText(/IGDB_CLIENT_ID/)).toBeInTheDocument();
  });

  it('shows "invalid credentials" banner when igdb_status is invalid_credentials', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'invalid_credentials' } });
    render(<AuthenticatedLayout />);
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText(/IGDB credentials are invalid/)).toBeInTheDocument();
  });

  it('does not show IGDB banner when igdb_status is ok', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'ok' } });
    render(<AuthenticatedLayout />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('does not show IGDB banner while health data is loading', () => {
    mockUseHealthStatus.mockReturnValue({ data: undefined });
    render(<AuthenticatedLayout />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
