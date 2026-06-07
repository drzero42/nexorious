import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { IGDBGameCandidate } from '@/types';
import { IGDBSearch } from './igdb-search';

const mockUseSearchIGDB = vi.fn();
vi.mock('@/hooks/use-games', () => ({
  useSearchIGDB: (...args: unknown[]) => mockUseSearchIGDB(...args),
}));

function candidate(overrides: Partial<IGDBGameCandidate> = {}): IGDBGameCandidate {
  return {
    igdb_id: 90101 as IGDBGameCandidate['igdb_id'],
    title: 'Owned Game',
    platforms: [],
    ...overrides,
  };
}

function mockResults(results: IGDBGameCandidate[]) {
  mockUseSearchIGDB.mockReturnValue({
    data: results,
    isLoading: false,
    isFetching: false,
    error: null,
  });
}

describe('IGDBSearch library status (#856)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('marks a result already in the library when showLibraryStatus is set', () => {
    mockResults([candidate({ user_game_id: 'ug-1' })]);
    render(<IGDBSearch onSelect={vi.fn()} showLibraryStatus />);

    expect(screen.getByText('In library')).toBeInTheDocument();
    expect(screen.getByText(/click to edit/i)).toBeInTheDocument();
  });

  it('does not mark library results when showLibraryStatus is not set', () => {
    mockResults([candidate({ user_game_id: 'ug-1' })]);
    render(<IGDBSearch onSelect={vi.fn()} />);

    expect(screen.queryByText('In library')).not.toBeInTheDocument();
  });

  it('does not mark a result that is not in the library', () => {
    mockResults([candidate({ user_game_id: undefined })]);
    render(<IGDBSearch onSelect={vi.fn()} showLibraryStatus />);

    expect(screen.queryByText('In library')).not.toBeInTheDocument();
  });
});
