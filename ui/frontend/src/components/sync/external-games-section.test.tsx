import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import userEvent from '@testing-library/user-event';
import { server } from '@/test/mocks/server';
import { render, screen, waitFor, within } from '@/test/test-utils';
import { ExternalGamesSection } from './external-games-section';
import { SyncStorefront } from '@/types';
import type { ExternalGame } from '@/types';

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useRouterState: vi.fn((opts?: { select?: (s: unknown) => unknown }) => {
      const state = { location: { hash: '' } };
      return opts?.select ? opts.select(state) : state;
    }),
  };
});

const API_URL = '/api';

function game(overrides: Partial<ExternalGame>): ExternalGame {
  return {
    id: 'eg-1',
    storefront: 'steam',
    external_id: '1',
    title: 'Game',
    resolved_igdb_id: null,
    is_skipped: false,
    is_available: true,
    is_subscription: false,
    has_user_game: false,
    user_game_id: null,
    igdb_title: null,
    user_game_other_platform_count: 0,
    sync_status: 'unmatched',
    failed_job_item_id: null,
    platforms: ['pc-windows'],
    store_url: null,
    ...overrides,
  };
}

describe('ExternalGamesSection', () => {
  it('lists unmatched games in their own section, where they can be skipped', async () => {
    let skipped: string | undefined;
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([
          game({ id: 'eg-unmatched', title: 'Lost Game' }),
          game({
            id: 'eg-matched',
            title: 'Found Game',
            sync_status: 'matched',
            resolved_igdb_id: 7,
            igdb_title: 'Found Game',
          }),
        ]),
      ),
      http.post(`${API_URL}/sync/ignored/:id`, ({ params }) => {
        skipped = params.id as string;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    const heading = await screen.findByText('Unmatched (1)');
    const card = heading.closest('#unmatched') as HTMLElement;
    expect(card).not.toBeNull();
    expect(within(card).getByText('Lost Game')).toBeInTheDocument();
    expect(within(card).queryByText('Found Game')).not.toBeInTheDocument();
    expect(within(card).getByRole('button', { name: 'Find Match' })).toBeInTheDocument();

    await userEvent.click(within(card).getByRole('button', { name: 'Skip' }));
    await waitFor(() => expect(skipped).toBe('eg-unmatched'));
  });
});
