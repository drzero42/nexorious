import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor } from '@/test/test-utils';
import { server } from '@/test/mocks/server';
import { SyncPage } from './index';
import { syncKeys } from '@/hooks';
import { toast } from 'sonner';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

const API_URL = '/api';

function overviewHandlers() {
  return [
    http.get(`${API_URL}/sync/config`, () =>
      HttpResponse.json({
        configs: [
          {
            id: 'c1',
            user_id: 'u1',
            storefront: 'steam',
            frequency: 'manual',
            last_synced_at: null,
            created_at: '2025-01-01T00:00:00Z',
            updated_at: '2025-01-01T00:00:00Z',
            is_configured: true,
          },
        ],
        total: 1,
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
    http.get(`${API_URL}/jobs/pending-review-count`, () =>
      HttpResponse.json({ counts_by_source: {} }),
    ),
    http.get(`${API_URL}/platforms/storefronts/:name`, ({ params }) =>
      HttpResponse.json({ name: params.name, display_name: String(params.name), icon_url: null }),
    ),
    http.get(`${API_URL}/settings`, () => HttpResponse.json({})),
  ];
}

const enabledSyncNow = () =>
  screen.getAllByRole('button', { name: /sync now/i }).filter((b) => !b.hasAttribute('disabled'));

describe('SyncPage', () => {
  // A refused start (here a 409, a sync already running) is reported by the
  // page and leaves the tile usable, with no unhandled rejection.
  it('leaves the tile usable when its sync request is refused', async () => {
    server.use(
      ...overviewHandlers(),
      http.post(`${API_URL}/sync/steam`, () =>
        HttpResponse.json({ message: 'sync already in progress' }, { status: 409 }),
      ),
    );
    render(<SyncPage />);
    await screen.findAllByRole('button', { name: /sync now/i });
    expect(enabledSyncNow()).toHaveLength(1);
    await userEvent.click(enabledSyncNow()[0]);
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(enabledSyncNow()).toHaveLength(1));
  });

  // A tile refreshes its own connection when its sync ends, so a sign-in the
  // sync flagged shows on the overview without a reload.
  it('shows a sign-in flagged by a sync that just ended', async () => {
    let steamStatus: Record<string, unknown> = {
      storefront: 'steam',
      is_syncing: true,
      last_synced_at: null,
      active_job_id: 'job-1',
      external_game_count: 0,
    };
    let credentialsError = false;
    let steamConnectionRequests = 0;
    // The specific handlers come first: msw answers with the first match.
    server.use(
      http.get(`${API_URL}/sync/steam/status`, () => HttpResponse.json(steamStatus)),
      http.get(`${API_URL}/sync/steam/connection`, () => {
        steamConnectionRequests += 1;
        return HttpResponse.json({
          connected: true,
          credentials_error: credentialsError,
          username: 'u',
        });
      }),
      ...overviewHandlers(),
    );
    const { queryClient } = render(<SyncPage />);
    await screen.findAllByRole('button', { name: /syncing/i });
    await waitFor(() => expect(steamConnectionRequests).toBe(1));
    expect(screen.queryByText('Credentials Error')).not.toBeInTheDocument();

    credentialsError = true;
    steamStatus = { ...steamStatus, is_syncing: false, active_job_id: null };
    await queryClient.refetchQueries({ queryKey: syncKeys.status('steam' as never) });
    expect(await screen.findByText('Credentials Error')).toBeInTheDocument();
    expect(steamConnectionRequests).toBe(2);
  });

  // A tile's Sync Now must say the sync is starting while the request is in
  // flight: before this, each tile read the pending state of a separate,
  // never-used mutation, so the button showed nothing until the server answered.
  it('shows the tile as syncing while its sync request is in flight', async () => {
    let release: () => void = () => {};
    server.use(
      http.get(`${API_URL}/sync/config`, () =>
        HttpResponse.json({
          configs: [
            {
              id: 'c1',
              user_id: 'u1',
              storefront: 'steam',
              frequency: 'manual',
              last_synced_at: null,
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
              is_configured: true,
            },
          ],
          total: 1,
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
      http.get(`${API_URL}/jobs/pending-review-count`, () =>
        HttpResponse.json({ counts_by_source: {} }),
      ),
      http.get(`${API_URL}/platforms/storefronts/:name`, ({ params }) =>
        HttpResponse.json({ name: params.name, display_name: String(params.name), icon_url: null }),
      ),
      http.get(`${API_URL}/settings`, () => HttpResponse.json({})),
      http.post(
        `${API_URL}/sync/steam`,
        () =>
          new Promise<Response>((resolve) => {
            release = () =>
              resolve(
                HttpResponse.json({
                  message: 'started',
                  job_id: 'job-1',
                  storefront: 'steam',
                  status: 'pending',
                }),
              );
          }),
      ),
    );

    render(<SyncPage />);
    // Only Steam is configured, so its Sync Now is the one enabled button.
    await screen.findAllByRole('button', { name: /sync now/i });
    const steam = screen
      .getAllByRole('button', { name: /sync now/i })
      .filter((b) => !b.hasAttribute('disabled'));
    expect(steam).toHaveLength(1);
    await userEvent.click(steam[0]);

    expect(await screen.findByRole('button', { name: /syncing/i })).toBeDisabled();
    release();
    // Once the server answers and the status says the sync is not running,
    // the tile offers Sync Now again; waiting for it lets the request settle.
    await waitFor(() =>
      expect(
        screen
          .getAllByRole('button', { name: /sync now/i })
          .filter((b) => !b.hasAttribute('disabled')),
      ).toHaveLength(1),
    );
  });
});
