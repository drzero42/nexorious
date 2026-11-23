import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AuthenticatedLayout } from './_authenticated';

const mockUseHealthStatus = vi.fn();
vi.mock('@/hooks/use-health-status', () => ({
  useHealthStatus: () => mockUseHealthStatus(),
}));

const mockUseAuth = vi.fn();
vi.mock('@/providers', () => ({
  useAuth: () => mockUseAuth(),
}));

const mockUseAdminHealthWarnings = vi.fn();
vi.mock('@/hooks/use-admin-health-warnings', () => ({
  useAdminHealthWarnings: (enabled: boolean) => mockUseAdminHealthWarnings(enabled),
}));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Outlet: () => null,
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}));

vi.mock('@/components/route-guard', () => ({
  RouteGuard: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('@/components/navigation', () => ({
  Sidebar: () => null,
  MobileNav: () => null,
}));

vi.mock('@/hooks', () => ({
  useJobTypeStatus: () => ({ data: undefined }),
  useJobCompletionEffect: () => {},
  useSettings: () => ({ data: undefined }),
  useThemePreference: () => ({ pref: 'system', setPref: () => {} }),
}));

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

describe('AuthenticatedLayout IGDB banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({ user: { isAdmin: false } });
    mockUseAdminHealthWarnings.mockReturnValue({ data: undefined });
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

describe('AuthenticatedLayout database role banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'ok' } });
  });

  it('shows the warning to admins when the database role is privileged', () => {
    mockUseAuth.mockReturnValue({ user: { isAdmin: true } });
    mockUseAdminHealthWarnings.mockReturnValue({
      data: [{ code: 'db_role_privileged', role: 'nexorious' }],
    });
    render(<AuthenticatedLayout />);
    expect(mockUseAdminHealthWarnings).toHaveBeenCalledWith(true);
    expect(screen.getByRole('alert')).toHaveTextContent(/database role is a superuser/);
    expect(screen.getByText('nexorious')).toBeInTheDocument();
  });

  it('does not query admin warnings for non-admins', () => {
    mockUseAuth.mockReturnValue({ user: { isAdmin: false } });
    mockUseAdminHealthWarnings.mockReturnValue({ data: undefined });
    render(<AuthenticatedLayout />);
    expect(mockUseAdminHealthWarnings).toHaveBeenCalledWith(false);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('shows nothing when there are no warnings', () => {
    mockUseAuth.mockReturnValue({ user: { isAdmin: true } });
    mockUseAdminHealthWarnings.mockReturnValue({ data: [] });
    render(<AuthenticatedLayout />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
