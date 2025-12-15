import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import {
  setupFetchMock,
  resetFetchMock,
  mockConfig
} from '../../test-utils/api-mocks';
import { resetStoresMocks } from '../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../test-utils/auth-mocks';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock the auth module
vi.mock('$lib/stores/auth.svelte', () => ({
  auth: {
    value: {
      accessToken: 'test-token',
      user: { id: '1', username: 'testuser' }
    }
  }
}));

// Mock RouteGuard
vi.mock('$lib/components/RouteGuard.svelte', () => {
  return import('../../test-utils/MockRouteGuard.svelte');
});

// Mock the components index file
vi.mock('$lib/components', async () => {
  const MockRouteGuard = await import('../../test-utils/MockRouteGuard.svelte');
  return {
    RouteGuard: MockRouteGuard.default
  };
});

// Import component after mocks
import ImportLandingPage from './+page.svelte';

describe('Import Landing Page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetAuthMocks();
    setupFetchMock();
    setAuthenticatedState();
  });

  describe('Core Rendering', () => {
    it('should render the page title', () => {
      render(ImportLandingPage);

      expect(screen.getByText('Import Your Games')).toBeInTheDocument();
    });

    it('should render the page description', () => {
      render(ImportLandingPage);

      expect(screen.getByText(/Choose how you'd like to import your game collection/)).toBeInTheDocument();
    });

    it('should set document title correctly', () => {
      render(ImportLandingPage);

      const titleElement = document.querySelector('title');
      expect(titleElement?.textContent).toBe('Import Games - Nexorious');
    });
  });

  describe('Import Source Cards', () => {
    it('should render Nexorious JSON import card', () => {
      render(ImportLandingPage);

      expect(screen.getByText('Nexorious JSON')).toBeInTheDocument();
      expect(screen.getByText(/Restore a previous Nexorious export/)).toBeInTheDocument();
    });

    it('should render Darkadia CSV import card', () => {
      render(ImportLandingPage);

      expect(screen.getByText('Darkadia CSV')).toBeInTheDocument();
      expect(screen.getByText(/Import your game collection from Darkadia/)).toBeInTheDocument();
    });

    it('should NOT render Steam Library import card (Steam uses sync, not import)', () => {
      render(ImportLandingPage);

      // Steam import was removed - Steam now only uses sync at /sync
      expect(screen.queryByText('Steam Library')).not.toBeInTheDocument();
    });

    it('should display feature lists for each import source', () => {
      render(ImportLandingPage);

      // Nexorious features
      expect(screen.getByText('Full metadata restoration')).toBeInTheDocument();
      expect(screen.getByText('Preserves ratings and notes')).toBeInTheDocument();

      // Darkadia features
      expect(screen.getByText('CSV file upload')).toBeInTheDocument();
      expect(screen.getByText('Automatic IGDB matching')).toBeInTheDocument();
    });

    it('should render exactly 2 import sources (Nexorious and Darkadia)', () => {
      render(ImportLandingPage);

      // Should only have 2 Start Import links
      const importLinks = screen.getAllByRole('link', { name: /Start Import/i });
      expect(importLinks).toHaveLength(2);
    });
  });

  describe('Navigation Links', () => {
    it('should have link to Nexorious import page', () => {
      render(ImportLandingPage);

      const nexoriousLinks = screen.getAllByRole('link', { name: /Start Import/i });
      const nexoriousLink = nexoriousLinks.find(link => link.getAttribute('href') === '/import/nexorious');
      expect(nexoriousLink).toBeTruthy();
    });

    it('should have link to Darkadia import page', () => {
      render(ImportLandingPage);

      const darkadiaLinks = screen.getAllByRole('link', { name: /Start Import/i });
      const darkadiaLink = darkadiaLinks.find(link => link.getAttribute('href') === '/import/darkadia');
      expect(darkadiaLink).toBeTruthy();
    });

    it('should have breadcrumb navigation', () => {
      render(ImportLandingPage);

      expect(screen.getByText('Dashboard')).toBeInTheDocument();
      expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('href', '/dashboard');
    });
  });

  describe('Import Tips Section', () => {
    it('should display import tips section', () => {
      render(ImportLandingPage);

      expect(screen.getByText('Import Tips')).toBeInTheDocument();
    });

    it('should display "Before You Import" tips', () => {
      render(ImportLandingPage);

      expect(screen.getByText('Before You Import')).toBeInTheDocument();
      expect(screen.getByText(/Imports are additive/)).toBeInTheDocument();
      expect(screen.getByText(/Duplicate games are automatically detected/)).toBeInTheDocument();
    });

    it('should display "Import Workflow" steps', () => {
      render(ImportLandingPage);

      expect(screen.getByText('Import Workflow')).toBeInTheDocument();
      expect(screen.getByText(/Upload your file/)).toBeInTheDocument();
      expect(screen.getByText(/Games are matched to IGDB/)).toBeInTheDocument();
      expect(screen.getByText(/Review any games that couldn't be auto-matched/)).toBeInTheDocument();
    });
  });

  describe('Quick Links', () => {
    it('should have link to Steam Sync page', () => {
      render(ImportLandingPage);

      const syncLink = screen.getByRole('link', { name: /Steam Sync/i });
      expect(syncLink).toHaveAttribute('href', '/sync');
    });

    it('should have link to jobs page', () => {
      render(ImportLandingPage);

      const jobsLink = screen.getByRole('link', { name: /View Jobs/i });
      expect(jobsLink).toHaveAttribute('href', '/jobs');
    });

    it('should have link to review page', () => {
      render(ImportLandingPage);

      const reviewLink = screen.getByRole('link', { name: /Review Pending Items/i });
      expect(reviewLink).toHaveAttribute('href', '/review');
    });

    it('should have link to games collection', () => {
      render(ImportLandingPage);

      const gamesLink = screen.getByRole('link', { name: /View Collection/i });
      expect(gamesLink).toHaveAttribute('href', '/games');
    });
  });
});
