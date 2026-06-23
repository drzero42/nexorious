import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper, createTestQueryClient } from '@/test/test-utils';
import { QueryClientProvider } from '@tanstack/react-query';
import {
  useUserGames,
  useUserGame,
  useSearchIGDB,
  useCollectionStats,
  useActiveGames,
  useUserGameGenres,
  useCreateUserGame,
  useUpdateUserGame,
  useDeleteUserGame,
  useImportFromIGDB,
  useBulkUpdateUserGames,
  useBulkDeleteUserGames,
  useAddPlatformToUserGame,
  useUpdatePlatformAssociation,
  useRemovePlatformFromUserGame,
} from './use-games';
import { OwnershipStatus } from '@/types';
import type { PlayStatus, GameId } from '@/types';

const API_URL = '/api';

// Mock game data (API format - snake_case)
const mockGameApi = {
  id: 12345,
  title: 'Test Game',
  description: 'A test game description',
  genre: 'Action',
  developer: 'Test Developer',
  publisher: 'Test Publisher',
  release_date: '2024-01-15',
  cover_art_url: 'https://example.com/cover.jpg',
  rating_average: 8.5,
  rating_count: 100,
  game_metadata: null,
  howlongtobeat_main: 15,
  howlongtobeat_extra: 25,
  howlongtobeat_completionist: 40,
  igdb_slug: 'test-game',
  igdb_platform_names: 'PC, PlayStation 5',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockPlatformApi = {
  name: 'pc',
  display_name: 'PC',
  icon_url: 'https://example.com/pc.png',
  is_active: true,
  source: 'official',
  default_storefront: 'steam',
  storefronts: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockStorefrontApi = {
  name: 'steam',
  display_name: 'Steam',
  icon_url: 'https://example.com/steam.png',
  base_url: 'https://store.steampowered.com',
  is_active: true,
  source: 'official',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockUserGamePlatformApi = {
  id: 'ugp-1',
  platform: 'pc',
  storefront: 'steam',
  platform_details: mockPlatformApi,
  storefront_details: mockStorefrontApi,
  is_available: true,
  hours_played: 50,
  ownership_status: OwnershipStatus.OWNED,
  created_at: '2024-01-01T00:00:00Z',
};

const mockTagApi = {
  id: 'tag-1',
  user_id: 'user-1',
  name: 'Favorite',
  color: '#ff0000',
  description: 'My favorite games',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  game_count: 5,
};

const mockUserGameApi = {
  id: 'ug-12345678-1234-4123-8123-123456789012',
  game: mockGameApi,
  personal_rating: 9,
  is_loved: true,
  play_status: 'completed' as PlayStatus,
  hours_played: 50,
  personal_notes: 'Great game!',
  platforms: [mockUserGamePlatformApi],
  tags: [mockTagApi],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockIGDBGameApi = {
  igdb_id: 99999,
  igdb_slug: 'igdb-game',
  title: 'IGDB Game',
  release_date: '2024-06-15',
  cover_art_url: 'https://example.com/igdb-cover.jpg',
  description: 'An IGDB game',
  platforms: ['PC', 'PlayStation 5'],
  howlongtobeat_main: 10,
  howlongtobeat_extra: 20,
  howlongtobeat_completionist: 30,
};

describe('use-games hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('useUserGames', () => {
    it('fetches user games list successfully', async () => {
      server.use(
        http.get(`${API_URL}/user-games`, () => {
          return HttpResponse.json({
            user_games: [mockUserGameApi],
            total: 1,
            page: 1,
            per_page: 20,
            pages: 1,
          });
        }),
      );

      const { result } = renderHook(() => useUserGames(), {
        wrapper: QueryWrapper,
      });

      expect(result.current.isLoading).toBe(true);

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.items).toHaveLength(1);
      expect(result.current.data?.total).toBe(1);
      expect(result.current.data?.items[0].game.title).toBe('Test Game');
      expect(result.current.data?.items[0].play_status).toBe('completed');
    });

    it('passes filter parameters correctly', async () => {
      server.use(
        http.get(`${API_URL}/user-games`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('play_status')).toBe('in_progress');
          expect(url.searchParams.get('ownership_status')).toBe('owned');
          expect(url.searchParams.get('q')).toBe('zelda');
          expect(url.searchParams.get('page')).toBe('2');
          expect(url.searchParams.get('per_page')).toBe('10');

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 2,
            per_page: 10,
            pages: 0,
          });
        }),
      );

      const { result } = renderHook(
        () =>
          useUserGames({
            status: ['in_progress'],
            ownershipStatus: 'owned' as OwnershipStatus,
            search: 'zelda',
            page: 2,
            perPage: 10,
          }),
        { wrapper: QueryWrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });

    it('handles error state', async () => {
      server.use(
        http.get(`${API_URL}/user-games`, () => {
          return HttpResponse.json({ detail: 'Failed to fetch games' }, { status: 500 });
        }),
      );

      const { result } = renderHook(() => useUserGames(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Failed to fetch games');
    });
  });

  describe('useUserGame', () => {
    it('fetches single user game by ID', async () => {
      const gameId = mockUserGameApi.id;
      server.use(
        http.get(`${API_URL}/user-games/${gameId}`, () => {
          return HttpResponse.json(mockUserGameApi);
        }),
      );

      const { result } = renderHook(() => useUserGame(gameId), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.id).toBe(gameId);
      expect(result.current.data?.game.title).toBe('Test Game');
      expect(result.current.data?.platforms).toHaveLength(1);
      expect(result.current.data?.tags).toHaveLength(1);
    });

    it('does not fetch when ID is undefined', async () => {
      const fetchSpy = vi.fn();

      server.use(
        http.get(`${API_URL}/user-games/*`, () => {
          fetchSpy();
          return HttpResponse.json(mockUserGameApi);
        }),
      );

      const { result } = renderHook(() => useUserGame(undefined), {
        wrapper: QueryWrapper,
      });

      // Disabled query: stays pending and never fires the request.
      expect(result.current.isPending).toBe(true);
      expect(result.current.fetchStatus).toBe('idle');
      expect(fetchSpy).not.toHaveBeenCalled();
    });

    it('handles 404 error', async () => {
      server.use(
        http.get(`${API_URL}/user-games/non-existent`, () => {
          return HttpResponse.json({ detail: 'Game not found' }, { status: 404 });
        }),
      );

      const { result } = renderHook(() => useUserGame('non-existent'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Game not found');
    });
  });

  describe('useSearchIGDB', () => {
    it('searches IGDB when query is 3+ characters', async () => {
      server.use(
        http.post(`${API_URL}/games/search/igdb`, () => {
          return HttpResponse.json({
            games: [mockIGDBGameApi],
            total: 1,
          });
        }),
      );

      const { result } = renderHook(() => useSearchIGDB('zelda'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toHaveLength(1);
      expect(result.current.data?.[0].title).toBe('IGDB Game');
      expect(result.current.data?.[0].platforms).toEqual(['PC', 'PlayStation 5']);
    });

    it('does not search when query is less than 3 characters', async () => {
      const fetchSpy = vi.fn();

      server.use(
        http.post(`${API_URL}/games/search/igdb`, () => {
          fetchSpy();
          return HttpResponse.json({ games: [], total: 0 });
        }),
      );

      const { result } = renderHook(() => useSearchIGDB('ze'), {
        wrapper: QueryWrapper,
      });

      // Below the 3-char minimum neither query is enabled: the combined result
      // stays pending and the search endpoint is never hit.
      expect(result.current.isPending).toBe(true);
      expect(result.current.isFetching).toBe(false);
      expect(fetchSpy).not.toHaveBeenCalled();
    });

    it('passes limit parameter', async () => {
      server.use(
        http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
          const body = (await request.json()) as { query: string; limit: number };
          expect(body.limit).toBe(5);
          return HttpResponse.json({ games: [], total: 0 });
        }),
      );

      const { result } = renderHook(() => useSearchIGDB('zelda', { limit: 5 }), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });

    it('forwards an "igdb:NNNN" query verbatim to the search endpoint (inference is server-side)', async () => {
      let capturedQuery: string | null = null;
      const idLookupSpy = vi.fn();

      server.use(
        http.get(`${API_URL}/games/igdb/:id`, () => {
          idLookupSpy();
          return HttpResponse.json({ games: [], total: 0 });
        }),
        http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
          capturedQuery = ((await request.json()) as { query: string }).query;
          return HttpResponse.json({ games: [mockIGDBGameApi], total: 1 });
        }),
      );

      const { result } = renderHook(() => useSearchIGDB('igdb:12345'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // The hook no longer parses the query; it posts it as-is and the backend
      // performs the ID inference. The per-ID GET endpoint is never hit here.
      expect(capturedQuery).toBe('igdb:12345');
      expect(idLookupSpy).not.toHaveBeenCalled();
      expect(result.current.data).toHaveLength(1);
    });

    it('forwards a bare numeric query verbatim to the search endpoint', async () => {
      let capturedQuery: string | null = null;
      server.use(
        http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
          capturedQuery = ((await request.json()) as { query: string }).query;
          return HttpResponse.json({ games: [mockIGDBGameApi], total: 1 });
        }),
      );

      const { result } = renderHook(() => useSearchIGDB('2048'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(capturedQuery).toBe('2048');
    });

    it('enables a short purely-numeric query below the 3-char minimum', async () => {
      let capturedQuery: string | null = null;
      server.use(
        http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
          capturedQuery = ((await request.json()) as { query: string }).query;
          return HttpResponse.json({ games: [mockIGDBGameApi], total: 1 });
        }),
      );

      const { result } = renderHook(() => useSearchIGDB('12'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(capturedQuery).toBe('12');
    });
  });

  describe('useSearchIGDB with externalGameId', () => {
    it('forwards externalGameId to the API client', async () => {
      let capturedBody: Record<string, unknown> | null = null;
      server.use(
        http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
          capturedBody = (await request.json()) as Record<string, unknown>;
          return HttpResponse.json({ games: [], total: 0 });
        }),
      );

      const { result } = renderHook(() => useSearchIGDB('zelda', { externalGameId: 'eg-xyz' }), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(capturedBody).not.toBeNull();
      expect(capturedBody!.external_game_id).toBe('eg-xyz');
    });

    it('produces a different cache key when externalGameId changes', async () => {
      let calls = 0;
      server.use(
        http.post(`${API_URL}/games/search/igdb`, () => {
          calls++;
          return HttpResponse.json({ games: [], total: 0 });
        }),
      );

      renderHook(() => useSearchIGDB('zelda', { externalGameId: 'eg-a' }), {
        wrapper: QueryWrapper,
      });
      renderHook(() => useSearchIGDB('zelda', { externalGameId: 'eg-b' }), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => expect(calls).toBe(2));
    });
  });

  describe('useCollectionStats', () => {
    it('fetches collection statistics', async () => {
      const mockStats = {
        total_games: 100,
        completion_stats: {
          not_started: 30,
          in_progress: 20,
          completed: 40,
          mastered: 5,
          dominated: 2,
          shelved: 2,
          dropped: 1,
          replay: 0,
        },
        ownership_stats: {
          owned: 80,
          borrowed: 5,
          rented: 5,
          subscription: 10,
          no_longer_owned: 0,
        },
        platform_stats: { PC: 60, 'PlayStation 5': 40 },
        genre_stats: { Action: 50, RPG: 30, Adventure: 20 },
        pile_of_shame: 30,
        completion_rate: 0.4,
        average_rating: 7.5,
        total_hours_played: 500,
      };

      server.use(
        http.get(`${API_URL}/user-games/stats`, () => {
          return HttpResponse.json(mockStats);
        }),
      );

      const { result } = renderHook(() => useCollectionStats(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.totalGames).toBe(100);
      expect(result.current.data?.pileOfShame).toBe(30);
      expect(result.current.data?.completionRate).toBe(0.4);
      expect(result.current.data?.averageRating).toBe(7.5);
    });
  });

  describe('useCreateUserGame', () => {
    it('creates a new user game successfully', async () => {
      server.use(
        http.post(`${API_URL}/user-games`, async ({ request }) => {
          const body = (await request.json()) as { game_id: number };
          expect(body.game_id).toBe(12345);
          return HttpResponse.json(mockUserGameApi);
        }),
      );

      const { result } = renderHook(() => useCreateUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          gameId: 12345 as GameId,
          playStatus: 'not_started' as PlayStatus,
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.game.title).toBe('Test Game');
      expect(result.current.data?.platforms[0].ownership_status).toBe(OwnershipStatus.OWNED);
    });

    it('handles creation error', async () => {
      server.use(
        http.post(`${API_URL}/user-games`, () => {
          return HttpResponse.json({ detail: 'Game already in collection' }, { status: 400 });
        }),
      );

      const { result } = renderHook(() => useCreateUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync({
            gameId: 12345 as GameId,
          });
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Game already in collection');
    });
  });

  describe('useUpdateUserGame', () => {
    it('updates user game successfully', async () => {
      const gameId = mockUserGameApi.id;
      const updatedGame = {
        ...mockUserGameApi,
        personal_rating: 10,
        play_status: 'mastered',
      };

      server.use(
        http.put(`${API_URL}/user-games/${gameId}`, () => {
          return HttpResponse.json(updatedGame);
        }),
      );

      const { result } = renderHook(() => useUpdateUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          id: gameId,
          data: { personalRating: 10, playStatus: 'mastered' as PlayStatus },
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.personal_rating).toBe(10);
      expect(result.current.data?.play_status).toBe('mastered');
    });

    it('handles update error', async () => {
      server.use(
        http.put(`${API_URL}/user-games/non-existent`, () => {
          return HttpResponse.json({ detail: 'Game not found' }, { status: 404 });
        }),
      );

      const { result } = renderHook(() => useUpdateUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync({
            id: 'non-existent',
            data: { playStatus: 'completed' as PlayStatus },
          });
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Game not found');
    });
  });

  describe('useDeleteUserGame', () => {
    it('deletes user game successfully', async () => {
      const gameId = mockUserGameApi.id;

      server.use(
        http.delete(`${API_URL}/user-games/${gameId}`, () => {
          return new HttpResponse(null, { status: 204 });
        }),
      );

      const { result } = renderHook(() => useDeleteUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync(gameId);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });

    it('handles delete error', async () => {
      server.use(
        http.delete(`${API_URL}/user-games/non-existent`, () => {
          return HttpResponse.json({ detail: 'Game not found' }, { status: 404 });
        }),
      );

      const { result } = renderHook(() => useDeleteUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('non-existent');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Game not found');
    });
  });

  describe('useImportFromIGDB', () => {
    it('imports game from IGDB', async () => {
      server.use(
        http.post(`${API_URL}/games/igdb-import`, async ({ request }) => {
          const body = (await request.json()) as { igdb_id: number };
          expect(body.igdb_id).toBe(99999);
          return HttpResponse.json(mockGameApi);
        }),
      );

      const { result } = renderHook(() => useImportFromIGDB(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          igdbId: 99999 as GameId,
          downloadCoverArt: true,
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.title).toBe('Test Game');
    });
  });

  describe('useBulkUpdateUserGames', () => {
    it('bulk updates multiple games successfully', async () => {
      server.use(
        http.put(`${API_URL}/user-games/bulk-update`, async ({ request }) => {
          const body = (await request.json()) as {
            user_game_ids: string[];
            play_status?: PlayStatus;
          };
          expect(body.user_game_ids).toEqual(['game-1', 'game-2', 'game-3']);
          expect(body.play_status).toBe('completed');

          return HttpResponse.json({
            message: 'Updated successfully',
            updated_count: 3,
            failed_count: 0,
          });
        }),
      );

      const { result } = renderHook(() => useBulkUpdateUserGames(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          ids: ['game-1', 'game-2', 'game-3'],
          updates: { playStatus: 'completed' as PlayStatus },
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.updatedCount).toBe(3);
      expect(result.current.data?.failedCount).toBe(0);
      expect(result.current.data?.message).toBe('Updated successfully');
    });
  });

  describe('useBulkDeleteUserGames', () => {
    it('bulk deletes multiple games successfully', async () => {
      server.use(
        http.delete(`${API_URL}/user-games/bulk-delete`, async ({ request }) => {
          const body = (await request.json()) as { user_game_ids: string[] };
          expect(body.user_game_ids).toEqual(['game-1', 'game-2']);

          return HttpResponse.json({
            message: 'Deleted successfully',
            deleted_count: 2,
            failed_count: 0,
          });
        }),
      );

      const { result } = renderHook(() => useBulkDeleteUserGames(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync(['game-1', 'game-2']);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.deletedCount).toBe(2);
      expect(result.current.data?.failedCount).toBe(0);
      expect(result.current.data?.message).toBe('Deleted successfully');
    });
  });

  describe('useAddPlatformToUserGame', () => {
    it('adds platform to user game successfully', async () => {
      const userGameId = mockUserGameApi.id;

      server.use(
        http.post(`${API_URL}/user-games/${userGameId}/platforms`, async ({ request }) => {
          const body = (await request.json()) as {
            platform: string;
            storefront?: string;
          };
          expect(body.platform).toBe('ps5');
          expect(body.storefront).toBe('playstation-store');

          return HttpResponse.json({
            ...mockUserGamePlatformApi,
            id: 'ugp-2',
            platform: 'ps5',
            storefront: 'playstation-store',
          });
        }),
      );

      const { result } = renderHook(() => useAddPlatformToUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          userGameId,
          data: {
            platform: 'ps5',
            storefront: 'playstation-store',
          },
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.platform).toBe('ps5');
      expect(result.current.data?.storefront).toBe('playstation-store');
    });
  });

  describe('useUpdatePlatformAssociation', () => {
    it('updates platform association successfully', async () => {
      const userGameId = mockUserGameApi.id;
      const platformAssociationId = 'ugp-1';

      server.use(
        http.put(
          `${API_URL}/user-games/${userGameId}/platforms/${platformAssociationId}`,
          async ({ request }) => {
            const body = (await request.json()) as { is_available: boolean };
            expect(body.is_available).toBe(false);

            return HttpResponse.json({
              ...mockUserGamePlatformApi,
              is_available: false,
            });
          },
        ),
      );

      const { result } = renderHook(() => useUpdatePlatformAssociation(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          userGameId,
          platformAssociationId,
          data: {
            platform: 'platform-1',
            isAvailable: false,
          },
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.is_available).toBe(false);
    });
  });

  describe('useRemovePlatformFromUserGame', () => {
    it('removes platform from user game successfully', async () => {
      const userGameId = mockUserGameApi.id;
      const platformAssociationId = 'ugp-1';

      server.use(
        http.delete(
          `${API_URL}/user-games/${userGameId}/platforms/${platformAssociationId}`,
          () => {
            return new HttpResponse(null, { status: 204 });
          },
        ),
      );

      const { result } = renderHook(() => useRemovePlatformFromUserGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          userGameId,
          platformAssociationId,
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });
  });

  describe('useActiveGames', () => {
    it('fetches IN_PROGRESS and REPLAY games in a single multi-status call', async () => {
      const inProgressGame = {
        ...mockUserGameApi,
        id: 'ug-1',
        play_status: 'in_progress' as PlayStatus,
      };

      const replayGame = {
        ...mockUserGameApi,
        id: 'ug-2',
        play_status: 'replay' as PlayStatus,
      };

      let requestCount = 0;
      let playStatusValues: string[] = [];

      server.use(
        http.get(`${API_URL}/user-games`, ({ request }) => {
          requestCount++;
          const url = new URL(request.url);
          playStatusValues = url.searchParams.getAll('play_status');

          return HttpResponse.json({
            user_games: [inProgressGame, replayGame],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        }),
      );

      const { result } = renderHook(() => useActiveGames(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(requestCount).toBe(1);
      expect(playStatusValues).toEqual(['in_progress', 'replay']);
      expect(result.current.data?.items).toHaveLength(2);
      expect(result.current.data?.total).toBe(2);
      expect(result.current.data?.items[0].play_status).toBe('in_progress');
      expect(result.current.data?.items[1].play_status).toBe('replay');
    });

    it('returns empty array when no active games exist', async () => {
      server.use(
        http.get(`${API_URL}/user-games`, () => {
          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 50,
            pages: 0,
          });
        }),
      );

      const { result } = renderHook(() => useActiveGames(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.items).toEqual([]);
      expect(result.current.data?.total).toBe(0);
    });
  });

  describe('query caching behavior', () => {
    it('fetches separately for different params', async () => {
      let fetchCount = 0;

      server.use(
        http.get(`${API_URL}/user-games`, () => {
          fetchCount++;
          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 20,
            pages: 0,
          });
        }),
      );

      const queryClient = createTestQueryClient();

      const { result: result1 } = renderHook(() => useUserGames({ status: ['completed'] }), {
        wrapper: ({ children }) => (
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        ),
      });

      await waitFor(() => {
        expect(result1.current.isSuccess).toBe(true);
      });

      const { result: result2 } = renderHook(() => useUserGames({ status: ['in_progress'] }), {
        wrapper: ({ children }) => (
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        ),
      });

      await waitFor(() => {
        expect(result2.current.isSuccess).toBe(true);
      });

      // Should have fetched twice due to different params
      expect(fetchCount).toBe(2);
    });
  });

  describe('useUserGameGenres', () => {
    it('returns genres array', async () => {
      server.use(
        http.get(`${API_URL}/user-games/genres`, () => {
          return HttpResponse.json({ genres: ['Action', 'Adventure', 'RPG'] });
        }),
      );

      const { result } = renderHook(() => useUserGameGenres(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(Array.isArray(result.current.data)).toBe(true);
      expect(result.current.data).toEqual(['Action', 'Adventure', 'RPG']);
    });

    it('returns empty array when no genres exist', async () => {
      server.use(
        http.get(`${API_URL}/user-games/genres`, () => {
          return HttpResponse.json({ genres: [] });
        }),
      );

      const { result } = renderHook(() => useUserGameGenres(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual([]);
    });

    it('handles error state', async () => {
      server.use(
        http.get(`${API_URL}/user-games/genres`, () => {
          return HttpResponse.json({ detail: 'Failed to fetch genres' }, { status: 500 });
        }),
      );

      const { result } = renderHook(() => useUserGameGenres(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Failed to fetch genres');
    });
  });
});
