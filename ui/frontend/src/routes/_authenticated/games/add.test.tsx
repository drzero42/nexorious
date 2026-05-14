import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AddGamePage } from './add.index';

const mockUseHealthStatus = vi.fn();
vi.mock('@/hooks/use-health-status', () => ({
  useHealthStatus: () => mockUseHealthStatus(),
}));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
  useNavigate: () => vi.fn(),
}));

vi.mock('@/components/games/igdb-search', () => ({
  IGDBSearch: ({ disabled }: { disabled?: boolean }) => (
    <div data-testid="igdb-search" data-disabled={String(disabled ?? false)} />
  ),
}));

describe('AddGamePage IGDB disabled state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('disables IGDB search when igdb_status is not_configured', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'not_configured' } });
    render(<AddGamePage />);
    expect(screen.getByTestId('igdb-search')).toHaveAttribute('data-disabled', 'true');
  });

  it('disables IGDB search when igdb_status is invalid_credentials', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'invalid_credentials' } });
    render(<AddGamePage />);
    expect(screen.getByTestId('igdb-search')).toHaveAttribute('data-disabled', 'true');
  });

  it('enables IGDB search when igdb_status is ok', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_status: 'ok' } });
    render(<AddGamePage />);
    expect(screen.getByTestId('igdb-search')).toHaveAttribute('data-disabled', 'false');
  });
});
