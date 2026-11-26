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
  const id = overrides.id ?? 'eg-1';
  return {
    editions: [
      {
        id,
        external_id: overrides.external_id ?? '1',
        platforms: overrides.platforms ?? ['pc-windows'],
        store_url: null,
        is_available: true,
      },
    ],
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

  // An entry of two editions of one storefront package.
  const twoEditions = (overrides: Partial<ExternalGame>) =>
    game({
      id: 'eg-rep',
      title: 'Spider-Man',
      external_id: '100',
      platforms: ['pc-linux', 'pc-windows'],
      ...overrides,
      editions: [
        {
          id: 'eg-rep',
          external_id: '100',
          platforms: ['pc-windows'],
          store_url: null,
          is_available: true,
        },
        {
          id: 'eg-ed',
          external_id: '200',
          platforms: ['pc-linux'],
          store_url: null,
          is_available: true,
        },
      ],
    });

  function recordDecisions() {
    const requests: string[] = [];
    const record = ({ request }: { request: Request }) => {
      const url = new URL(request.url);
      requests.push(`${request.method} ${url.pathname}${url.search}`);
      return new HttpResponse(null, { status: 204 });
    };
    server.use(
      http.post(`${API_URL}/sync/ignored/:id`, record),
      http.delete(`${API_URL}/sync/ignored/:id`, record),
    );
    return requests;
  }

  it('skips an entry of one edition at once, with no choice', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([game({ id: 'eg-1', title: 'Solo' })]),
      ),
    );
    const requests = recordDecisions();
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByRole('button', { name: 'Skip' }));
    await waitFor(() => expect(requests).toEqual(['POST /api/sync/ignored/eg-1']));
    expect(screen.queryByText('All editions')).not.toBeInTheDocument();
  });

  it('asks which editions to skip, and sends one edition with the option', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([twoEditions({ sync_status: 'needs_review' })]),
      ),
    );
    const requests = recordDecisions();
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByRole('button', { name: 'Skip' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByLabelText('All editions')).toBeChecked();
    await userEvent.click(within(dialog).getByLabelText(/Only pc-linux · ID 200/));
    // An unmatched edition skipped alone may not stay apart: the dialog says so.
    expect(within(dialog).getByText(/they take this skip too/)).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Skip' }));

    await waitFor(() =>
      expect(requests).toEqual(['POST /api/sync/ignored/eg-ed?only_this_edition=true']),
    );
  });

  it('unskips all editions by default', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([
          twoEditions({ sync_status: 'skipped', is_skipped: true, resolved_igdb_id: 7 }),
        ]),
      ),
    );
    const requests = recordDecisions();
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByText('Skipped (1)'));
    await userEvent.click(await screen.findByRole('button', { name: 'Unskip' }));
    const dialog = await screen.findByRole('alertdialog');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Unskip' }));

    await waitFor(() => expect(requests).toEqual(['DELETE /api/sync/ignored/eg-rep']));
  });

  it('unskips one chosen edition with the option', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([
          twoEditions({ sync_status: 'skipped', is_skipped: true, resolved_igdb_id: 7 }),
        ]),
      ),
    );
    const requests = recordDecisions();
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByText('Skipped (1)'));
    await userEvent.click(await screen.findByRole('button', { name: 'Unskip' }));
    const dialog = await screen.findByRole('alertdialog');
    await userEvent.click(within(dialog).getByLabelText(/Only pc-linux · ID 200/));
    // A matched edition unskipped alone stays apart: no warning.
    expect(within(dialog).queryByText(/skips it again/)).not.toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Unskip' }));

    await waitFor(() =>
      expect(requests).toEqual(['DELETE /api/sync/ignored/eg-ed?only_this_edition=true']),
    );
  });

  it('warns that an unmatched edition unskipped alone is skipped again at the next sync', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([twoEditions({ sync_status: 'skipped', is_skipped: true })]),
      ),
    );
    recordDecisions();
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByText('Skipped (1)'));
    await userEvent.click(await screen.findByRole('button', { name: 'Unskip' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).queryByText(/skips it again/)).not.toBeInTheDocument();
    await userEvent.click(within(dialog).getByLabelText(/Only pc-linux · ID 200/));
    expect(within(dialog).getByText(/skips it again/)).toBeInTheDocument();
  });

  it('unskips an entry of one edition at once, with no choice', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([
          game({ id: 'eg-1', title: 'Solo', sync_status: 'skipped', is_skipped: true }),
        ]),
      ),
    );
    const requests = recordDecisions();
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByText('Skipped (1)'));
    await userEvent.click(await screen.findByRole('button', { name: 'Unskip' }));
    await waitFor(() => expect(requests).toEqual(['DELETE /api/sync/ignored/eg-1']));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('gives matched entries of more than one edition a Skip button, and only those', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([
          twoEditions({ sync_status: 'matched', resolved_igdb_id: 7, igdb_title: 'IGDB Package' }),
          game({
            id: 'eg-solo',
            title: 'Solo',
            sync_status: 'matched',
            resolved_igdb_id: 8,
            igdb_title: 'IGDB Solo',
          }),
        ]),
      ),
    );
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByText('Matched (2)'));
    const rows = await screen.findAllByRole('row');
    const pkg = rows.find((r) => within(r).queryByText('IGDB Package'));
    const solo = rows.find((r) => within(r).queryByText('IGDB Solo'));
    expect(pkg && within(pkg).getByRole('button', { name: 'Skip' })).toBeTruthy();
    expect(solo && within(solo).queryByRole('button', { name: 'Skip' })).toBeNull();
  });

  it('tells same-title entries apart by platforms and storefront id', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([
          game({
            id: 'eg-a',
            title: 'Alone in the Dark',
            external_id: '11',
            platforms: ['pc-dos'],
          }),
          game({
            id: 'eg-b',
            title: 'Alone in the Dark',
            external_id: '22',
            platforms: ['pc-windows'],
            sync_status: 'needs_review',
          }),
          game({ id: 'eg-c', title: 'Unique', external_id: '33' }),
        ]),
      ),
    );
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    expect(await screen.findByText('pc-dos · ID 11')).toBeInTheDocument();
    expect(screen.getByText('pc-windows · ID 22')).toBeInTheDocument();
    expect(screen.queryByText(/ID 33/)).not.toBeInTheDocument();
  });

  it('rematches one edition with the option, and asks keep or remove on the orphan 409', async () => {
    const rematches: string[] = [];
    const searchedFor: string[] = [];
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([
          twoEditions({
            sync_status: 'matched',
            resolved_igdb_id: 7,
            igdb_title: 'Spider-Man',
            has_user_game: true,
            user_game_other_platform_count: 0,
          }),
        ]),
      ),
      http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
        const body = (await request.json()) as { external_game_id?: string };
        searchedFor.push(body.external_game_id ?? '');
        return HttpResponse.json({
          games: [{ igdb_id: 99, title: 'Spider-Man 2', platforms: [] }],
        });
      }),
      http.post(`${API_URL}/sync/external-games/:id/rematch`, async ({ request, params }) => {
        const url = new URL(request.url);
        const body = (await request.json()) as { orphan_action: string };
        rematches.push(`${params.id as string}${url.search} ${body.orphan_action}`);
        if (body.orphan_action === '') {
          return HttpResponse.json(
            { message: 'orphan_action required: game would lose its only storefront link' },
            { status: 409 },
          );
        }
        return new HttpResponse(null, { status: 204 });
      }),
    );
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByText('Matched (1)'));
    await userEvent.click(await screen.findByRole('button', { name: 'Change Match' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByLabelText('All editions')).toBeChecked();
    await userEvent.click(within(dialog).getByLabelText(/Only pc-linux · ID 200/));
    await userEvent.click(await within(dialog).findByText('Spider-Man 2'));
    // The search is scoped by the chosen edition's id.
    expect(searchedFor.at(-1)).toBe('eg-ed');

    // Sent without orphan_action: the entry's orphan fields are for the set.
    await waitFor(() => expect(rematches).toEqual(['eg-ed?only_this_edition=true ']));
    const question = await screen.findByRole('alertdialog');
    await userEvent.click(within(question).getByRole('button', { name: 'Keep in Library' }));
    await waitFor(() =>
      expect(rematches).toEqual([
        'eg-ed?only_this_edition=true ',
        'eg-ed?only_this_edition=true keep',
      ]),
    );
  });

  it('offers no edition choice when matching an entry of one edition', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/external-games`, () =>
        HttpResponse.json([game({ id: 'eg-1', title: 'Solo', sync_status: 'needs_review' })]),
      ),
    );
    render(<ExternalGamesSection storefront={SyncStorefront.STEAM} />);

    await userEvent.click(await screen.findByRole('button', { name: 'Find Match' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByText('All editions')).not.toBeInTheDocument();
  });
});
