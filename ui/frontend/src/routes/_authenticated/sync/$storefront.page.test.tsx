import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor } from '@/test/test-utils';
import { server } from '@/test/mocks/server';
import { Route } from './$storefront';

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => (opts: Record<string, unknown>) => ({
    options: opts,
    useParams: () => ({ storefront: 'gog' }),
  }),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/sync/gog' } }),
}));

const API_URL = '/api';

describe('store page connection toggle', () => {
  // The header label is the only way to the connection card, so pressing it
  // from the keyboard must close and reopen the card on the real page.
  it('opens and closes the connection card from the keyboard', async () => {
    server.use(
      http.get(`${API_URL}/sync/config/gog`, () =>
        HttpResponse.json({
          id: 'c1',
          user_id: 'u1',
          storefront: 'gog',
          frequency: 'manual',
          last_synced_at: null,
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
          is_configured: false,
        }),
      ),
      http.get(`${API_URL}/sync/:storefront/status`, ({ params }) =>
        HttpResponse.json({
          storefront: params.storefront,
          is_syncing: false,
          last_synced_at: null,
          active_job_id: null,
          external_game_count: 0,
        }),
      ),
      http.get(`${API_URL}/sync/:storefront/connection`, () =>
        HttpResponse.json({ connected: false }),
      ),
      http.get(`${API_URL}/sync/playstation-store/status`, () =>
        HttpResponse.json({ is_configured: false }),
      ),
      http.get(`${API_URL}/settings`, () => HttpResponse.json({})),
      http.get(`${API_URL}/jobs/recent`, () => HttpResponse.json({ jobs: [] })),
      http.get(`${API_URL}/sync/gog/external-games`, () => HttpResponse.json([])),
      http.get(`${API_URL}/platforms/storefronts/:name`, ({ params }) =>
        HttpResponse.json({ name: params.name, display_name: 'GOG', icon_url: null }),
      ),
    );
    const Page = Route.options.component as React.ComponentType;
    render(<Page />);

    const toggle = await screen.findByRole('button', { name: /not configured/i });
    // A store that is not connected opens its connection card.
    await waitFor(() => expect(toggle).toHaveAttribute('aria-expanded', 'true'));
    expect(screen.getByText('GOG Connection')).toBeInTheDocument();

    toggle.focus();
    await userEvent.keyboard('{Enter}');
    await waitFor(() => expect(toggle).toHaveAttribute('aria-expanded', 'false'));
    expect(screen.queryByText('GOG Connection')).not.toBeInTheDocument();

    await userEvent.keyboard(' ');
    await waitFor(() => expect(toggle).toHaveAttribute('aria-expanded', 'true'));
    expect(screen.getByText('GOG Connection')).toBeInTheDocument();
  });
});
