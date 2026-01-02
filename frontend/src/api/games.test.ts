import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { setAuthHandlers } from './client';
import { PlayStatus, OwnershipStatus, type GameId } from '@/types';
import {
  getUserGames,
  getUserGame,
  getUserGameIds,
  createUserGame,
  updateUserGame,
  deleteUserGame,
  searchIGDB,
  getGameByIGDBId,
  importFromIGDB,
  bulkUpdateUserGames,
  bulkDeleteUserGames,
  getCollectionStats,
  getUserGameGenres,
  addPlatformToUserGame,
  updatePlatformAssociation,
  removePlatformFromUserGame,
} from './games';

const API_URL = '/api';

// Mock data
const mockGameApi = {
  id: 12345,
  title: 'Test Game',
  description: 'A test game description',
  genre: 'RPG',
  developer: 'Test Developer',
  publisher: 'Test Publisher',
  release_date: '2024-01-15',
  cover_art_url: 'https://example.com/cover.jpg',
  rating_average: 8.5,
  rating_count: 100,
  game_metadata: '{}',
  estimated_playtime_hours: 40,
  howlongtobeat_main: 35,
  howlongtobeat_extra: 50,
  howlongtobeat_completionist: 80,
  igdb_slug: 'test-game',
  igdb_platform_names: 'PC, PS5',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockPlatformApi = {
  name: 'pc',
  display_name: 'PC',
  icon_url: null,
  is_active: true,
  source: 'official',
  default_storefront: 'steam',
  storefronts: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockUserGamePlatformApi = {
  id: 'ugp-1',
  platform: 'pc',
  storefront: 'steam',
  platform_details: mockPlatformApi,
  storefront_details: null,
  store_game_id: 'steam-12345',
  store_url: 'https://store.steampowered.com/app/12345',
  is_available: true,
  original_platform_name: null,
  created_at: '2024-01-01T00:00:00Z',
};

const mockTagApi = {
  id: 'tag-1',
  user_id: 'user-123',
  name: 'RPG',
  color: '#FF5733',
  description: 'Role-playing games',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  game_count: 5,
};

const mockUserGameApi = {
  id: 'user-game-123',
  game: mockGameApi,
  ownership_status: OwnershipStatus.OWNED,
  personal_rating: 9,
  is_loved: true,
  play_status: PlayStatus.COMPLETED,
  hours_played: 50,
  personal_notes: 'Great game!',
  acquired_date: '2024-01-01',
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
  description: 'A game from IGDB',
  platforms: ['PC', 'PlayStation 5'],
  howlongtobeat_main: 25,
  howlongtobeat_extra: 40,
  howlongtobeat_completionist: 60,
};

describe('games.ts', () => {
  let mockGetAccessToken: Mock<() => string | null>;
  let mockRefreshTokens: Mock<() => Promise<boolean>>;
  let mockLogout: Mock<() => void>;

  beforeEach(() => {
    vi.clearAllMocks();

    mockGetAccessToken = vi.fn<() => string | null>().mockReturnValue('test-access-token');
    mockRefreshTokens = vi.fn<() => Promise<boolean>>().mockResolvedValue(false);
    mockLogout = vi.fn<() => void>();

    setAuthHandlers(mockGetAccessToken, mockRefreshTokens, mockLogout);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('getUserGames', () => {
    it('returns paginated user games list', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, () => {
          return HttpResponse.json({
            user_games: [mockUserGameApi],
            total: 1,
            page: 1,
            per_page: 20,
            pages: 1,
          });
        })
      );

      const result = await getUserGames();

      expect(result.items).toHaveLength(1);
      expect(result.total).toBe(1);
      expect(result.page).toBe(1);
      expect(result.perPage).toBe(20);
      expect(result.pages).toBe(1);

      // Verify transformation
      const game = result.items[0];
      expect(game.id).toBe('user-game-123');
      expect(game.game.id).toBe(12345);
      expect(game.game.title).toBe('Test Game');
      expect(game.ownership_status).toBe(OwnershipStatus.OWNED);
      expect(game.is_loved).toBe(true);
      expect(game.platforms).toHaveLength(1);
      expect(game.tags).toHaveLength(1);
    });

    it('passes filter parameters correctly', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('play_status')).toBe(PlayStatus.IN_PROGRESS);
          expect(url.searchParams.get('ownership_status')).toBe(OwnershipStatus.OWNED);
          expect(url.searchParams.get('platform')).toBe('pc');
          expect(url.searchParams.get('q')).toBe('test');
          expect(url.searchParams.get('sort_by')).toBe('title');
          expect(url.searchParams.get('sort_order')).toBe('asc');
          expect(url.searchParams.get('page')).toBe('2');
          expect(url.searchParams.get('per_page')).toBe('50');
          expect(url.searchParams.get('is_loved')).toBe('true');
          expect(url.searchParams.get('rating_min')).toBe('7');
          expect(url.searchParams.get('rating_max')).toBe('10');

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 2,
            per_page: 50,
            pages: 0,
          });
        })
      );

      await getUserGames({
        status: PlayStatus.IN_PROGRESS,
        ownershipStatus: OwnershipStatus.OWNED,
        platform: 'pc',
        search: 'test',
        sortBy: 'title',
        sortOrder: 'asc',
        page: 2,
        perPage: 50,
        isLoved: true,
        ratingMin: 7,
        ratingMax: 10,
      });
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(getUserGames()).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('handles multiple platform values', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, ({ request }) => {
          const url = new URL(request.url);
          const platforms = url.searchParams.getAll('platform');
          expect(platforms).toEqual(['windows', 'playstation_5']);

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 20,
            pages: 0,
          });
        })
      );

      const result = await getUserGames({
        platform: ['windows', 'playstation_5'],
      });
      expect(result).toBeDefined();
      expect(result.items).toEqual([]);
    });

    it('handles multiple storefront values', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, ({ request }) => {
          const url = new URL(request.url);
          const storefronts = url.searchParams.getAll('storefront');
          expect(storefronts).toEqual(['steam', 'epic']);

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 20,
            pages: 0,
          });
        })
      );

      const result = await getUserGames({
        storefront: ['steam', 'epic'],
      });
      expect(result).toBeDefined();
      expect(result.items).toEqual([]);
    });

    it('handles multiple genre values', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, ({ request }) => {
          const url = new URL(request.url);
          const genres = url.searchParams.getAll('genre');
          expect(genres).toEqual(['RPG', 'Action']);

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 20,
            pages: 0,
          });
        })
      );

      const result = await getUserGames({
        genre: ['RPG', 'Action'],
      });
      expect(result).toBeDefined();
      expect(result.items).toEqual([]);
    });

    it('handles multiple tag values', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, ({ request }) => {
          const url = new URL(request.url);
          const tags = url.searchParams.getAll('tags');
          expect(tags).toEqual(['tag-id-1', 'tag-id-2']);

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 20,
            pages: 0,
          });
        })
      );

      const result = await getUserGames({
        tags: ['tag-id-1', 'tag-id-2'],
      });
      expect(result).toBeDefined();
      expect(result.items).toEqual([]);
    });

    it('handles single platform as string', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('platform')).toBe('windows');
          // Should only have one platform value
          expect(url.searchParams.getAll('platform')).toEqual(['windows']);

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 20,
            pages: 0,
          });
        })
      );

      const result = await getUserGames({
        platform: 'windows',
      });
      expect(result).toBeDefined();
    });

    it('handles mixed array and single value params', async () => {
      server.use(
        http.get(`${API_URL}/user-games/`, ({ request }) => {
          const url = new URL(request.url);
          // Array params
          expect(url.searchParams.getAll('platform')).toEqual(['windows', 'ps5']);
          expect(url.searchParams.getAll('genre')).toEqual(['RPG']);
          // Single value params
          expect(url.searchParams.get('play_status')).toBe(PlayStatus.IN_PROGRESS);
          expect(url.searchParams.get('q')).toBe('zelda');

          return HttpResponse.json({
            user_games: [],
            total: 0,
            page: 1,
            per_page: 20,
            pages: 0,
          });
        })
      );

      const result = await getUserGames({
        platform: ['windows', 'ps5'],
        genre: ['RPG'],
        status: PlayStatus.IN_PROGRESS,
        search: 'zelda',
      });
      expect(result).toBeDefined();
    });
  });

  describe('getUserGame', () => {
    it('returns single user game by ID', async () => {
      server.use(
        http.get(`${API_URL}/user-games/user-game-123`, () => {
          return HttpResponse.json(mockUserGameApi);
        })
      );

      const result = await getUserGame('user-game-123');

      expect(result.id).toBe('user-game-123');
      expect(result.game.title).toBe('Test Game');
      expect(result.platforms).toHaveLength(1);
    });

    it('throws error for non-existent game', async () => {
      server.use(
        http.get(`${API_URL}/user-games/non-existent`, () => {
          return HttpResponse.json({ detail: 'User game not found' }, { status: 404 });
        })
      );

      await expect(getUserGame('non-existent')).rejects.toMatchObject({
        message: 'User game not found',
        status: 404,
      });
    });
  });

  describe('createUserGame', () => {
    it('creates a new user game', async () => {
      server.use(
        http.post(`${API_URL}/user-games/`, async ({ request }) => {
          const body = (await request.json()) as Record<string, unknown>;
          expect(body.game_id).toBe(12345);
          expect(body.ownership_status).toBe(OwnershipStatus.OWNED);
          expect(body.play_status).toBe(PlayStatus.NOT_STARTED);

          return HttpResponse.json(mockUserGameApi);
        })
      );

      const result = await createUserGame({
        gameId: 12345 as GameId,
        ownershipStatus: OwnershipStatus.OWNED,
        playStatus: PlayStatus.NOT_STARTED,
      });

      expect(result.id).toBe('user-game-123');
      expect(result.game.id).toBe(12345);
    });

    it('creates user game with platforms', async () => {
      server.use(
        http.post(`${API_URL}/user-games/`, async ({ request }) => {
          const body = (await request.json()) as {
            platforms?: Array<{
              platform: string;
              storefront?: string;
              store_game_id?: string;
              is_available?: boolean;
            }>;
          };
          expect(body.platforms).toHaveLength(1);
          expect(body.platforms?.[0].platform).toBe('pc');
          expect(body.platforms?.[0].storefront).toBe('steam');
          expect(body.platforms?.[0].is_available).toBe(true);

          return HttpResponse.json(mockUserGameApi);
        })
      );

      await createUserGame({
        gameId: 12345 as GameId,
        platforms: [
          {
            platform: 'pc',
            storefront: 'steam',
            isAvailable: true,
          },
        ],
      });
    });

    it('handles all optional fields', async () => {
      server.use(
        http.post(`${API_URL}/user-games/`, async ({ request }) => {
          const body = (await request.json()) as Record<string, unknown>;
          expect(body.personal_rating).toBe(8);
          expect(body.is_loved).toBe(true);
          expect(body.hours_played).toBe(10);
          expect(body.personal_notes).toBe('Starting!');
          expect(body.acquired_date).toBe('2024-06-01');

          return HttpResponse.json(mockUserGameApi);
        })
      );

      await createUserGame({
        gameId: 12345 as GameId,
        personalRating: 8,
        isLoved: true,
        hoursPlayed: 10,
        personalNotes: 'Starting!',
        acquiredDate: '2024-06-01',
      });
    });
  });

  describe('updateUserGame', () => {
    it('updates user game properties', async () => {
      server.use(
        http.put(`${API_URL}/user-games/user-game-123`, async ({ request }) => {
          const body = (await request.json()) as Record<string, unknown>;
          expect(body.play_status).toBe(PlayStatus.COMPLETED);
          expect(body.personal_rating).toBe(10);

          return HttpResponse.json({
            ...mockUserGameApi,
            play_status: PlayStatus.COMPLETED,
            personal_rating: 10,
          });
        })
      );

      const result = await updateUserGame('user-game-123', {
        playStatus: PlayStatus.COMPLETED,
        personalRating: 10,
      });

      expect(result.play_status).toBe(PlayStatus.COMPLETED);
      expect(result.personal_rating).toBe(10);
    });

    it('only sends specified fields', async () => {
      server.use(
        http.put(`${API_URL}/user-games/user-game-123`, async ({ request }) => {
          const body = (await request.json()) as Record<string, unknown>;
          expect(body.hours_played).toBe(100);
          expect(body.play_status).toBeUndefined();
          expect(body.personal_rating).toBeUndefined();

          return HttpResponse.json({
            ...mockUserGameApi,
            hours_played: 100,
          });
        })
      );

      await updateUserGame('user-game-123', { hoursPlayed: 100 });
    });

    it('can set personal_rating to null', async () => {
      server.use(
        http.put(`${API_URL}/user-games/user-game-123`, async ({ request }) => {
          const body = (await request.json()) as Record<string, unknown>;
          expect(body.personal_rating).toBeNull();

          return HttpResponse.json({
            ...mockUserGameApi,
            personal_rating: null,
          });
        })
      );

      await updateUserGame('user-game-123', { personalRating: null });
    });
  });

  describe('deleteUserGame', () => {
    it('deletes a user game', async () => {
      server.use(
        http.delete(`${API_URL}/user-games/user-game-123`, () => {
          return new HttpResponse(null, { status: 204 });
        })
      );

      await expect(deleteUserGame('user-game-123')).resolves.toBeUndefined();
    });

    it('throws error for non-existent game', async () => {
      server.use(
        http.delete(`${API_URL}/user-games/non-existent`, () => {
          return HttpResponse.json({ detail: 'User game not found' }, { status: 404 });
        })
      );

      await expect(deleteUserGame('non-existent')).rejects.toMatchObject({
        message: 'User game not found',
        status: 404,
      });
    });
  });

  describe('searchIGDB', () => {
    it('searches IGDB and returns candidates', async () => {
      server.use(
        http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
          const body = (await request.json()) as { query: string; limit: number };
          expect(body.query).toBe('zelda');
          expect(body.limit).toBe(10);

          return HttpResponse.json({
            games: [
              {
                igdb_id: 54321,
                igdb_slug: 'zelda-totk',
                title: 'The Legend of Zelda: Tears of the Kingdom',
                release_date: '2023-05-12',
                cover_art_url: 'https://example.com/zelda.jpg',
                description: 'Link awakens...',
                platforms: ['Nintendo Switch'],
                howlongtobeat_main: 50,
                howlongtobeat_extra: 100,
                howlongtobeat_completionist: 200,
              },
            ],
            total: 1,
          });
        })
      );

      const result = await searchIGDB('zelda');

      expect(result).toHaveLength(1);
      expect(result[0].igdb_id).toBe(54321);
      expect(result[0].title).toBe('The Legend of Zelda: Tears of the Kingdom');
      expect(result[0].platforms).toEqual(['Nintendo Switch']);
    });

    it('passes custom limit', async () => {
      server.use(
        http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
          const body = (await request.json()) as { query: string; limit: number };
          expect(body.limit).toBe(5);

          return HttpResponse.json({ games: [], total: 0 });
        })
      );

      await searchIGDB('test', 5);
    });
  });

  describe('getGameByIGDBId', () => {
    it('fetches game by IGDB ID successfully', async () => {
      server.use(
        http.get(`${API_URL}/games/igdb/12345`, () => {
          return HttpResponse.json({
            games: [mockIGDBGameApi],
            total: 1,
          });
        })
      );

      const result = await getGameByIGDBId(12345);

      expect(result).toHaveLength(1);
      expect(result[0].igdb_id).toBe(99999);
      expect(result[0].title).toBe('IGDB Game');
    });

    it('returns empty array when game not found', async () => {
      server.use(
        http.get(`${API_URL}/games/igdb/99999999`, () => {
          return HttpResponse.json({
            games: [],
            total: 0,
          });
        })
      );

      const result = await getGameByIGDBId(99999999);

      expect(result).toHaveLength(0);
    });
  });

  describe('importFromIGDB', () => {
    it('imports game from IGDB', async () => {
      server.use(
        http.post(`${API_URL}/games/igdb-import`, async ({ request }) => {
          const body = (await request.json()) as { igdb_id: number };
          const url = new URL(request.url);
          expect(body.igdb_id).toBe(54321);
          expect(url.searchParams.get('download_cover_art')).toBe('true');

          return HttpResponse.json(mockGameApi);
        })
      );

      const result = await importFromIGDB(54321 as GameId);

      expect(result.id).toBe(12345);
      expect(result.title).toBe('Test Game');
    });

    it('respects downloadCoverArt parameter', async () => {
      server.use(
        http.post(`${API_URL}/games/igdb-import`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('download_cover_art')).toBe('false');

          return HttpResponse.json(mockGameApi);
        })
      );

      await importFromIGDB(54321 as GameId, false);
    });
  });

  describe('bulkUpdateUserGames', () => {
    it('bulk updates multiple games', async () => {
      server.use(
        http.put(`${API_URL}/user-games/bulk-update`, async ({ request }) => {
          const body = (await request.json()) as {
            user_game_ids: string[];
            play_status?: string;
            is_loved?: boolean;
          };
          expect(body.user_game_ids).toEqual(['game-1', 'game-2', 'game-3']);
          expect(body.play_status).toBe(PlayStatus.COMPLETED);
          expect(body.is_loved).toBe(true);

          return HttpResponse.json({
            message: 'Successfully updated 3 games',
            updated_count: 3,
            failed_count: 0,
          });
        })
      );

      const result = await bulkUpdateUserGames(['game-1', 'game-2', 'game-3'], {
        playStatus: PlayStatus.COMPLETED,
        isLoved: true,
      });

      expect(result.message).toBe('Successfully updated 3 games');
      expect(result.updatedCount).toBe(3);
      expect(result.failedCount).toBe(0);
    });
  });

  describe('bulkDeleteUserGames', () => {
    it('bulk deletes multiple games', async () => {
      server.use(
        http.delete(`${API_URL}/user-games/bulk-delete`, async ({ request }) => {
          const body = (await request.json()) as { user_game_ids: string[] };
          expect(body.user_game_ids).toEqual(['game-1', 'game-2']);

          return HttpResponse.json({
            message: 'Successfully deleted 2 games',
            deleted_count: 2,
            failed_count: 0,
          });
        })
      );

      const result = await bulkDeleteUserGames(['game-1', 'game-2']);

      expect(result.message).toBe('Successfully deleted 2 games');
      expect(result.deletedCount).toBe(2);
      expect(result.failedCount).toBe(0);
    });
  });

  describe('getCollectionStats', () => {
    it('returns collection statistics', async () => {
      server.use(
        http.get(`${API_URL}/user-games/stats`, () => {
          return HttpResponse.json({
            total_games: 100,
            completion_stats: {
              [PlayStatus.NOT_STARTED]: 30,
              [PlayStatus.IN_PROGRESS]: 20,
              [PlayStatus.COMPLETED]: 40,
              [PlayStatus.MASTERED]: 5,
              [PlayStatus.DOMINATED]: 2,
              [PlayStatus.SHELVED]: 3,
              [PlayStatus.DROPPED]: 0,
              [PlayStatus.REPLAY]: 0,
            },
            ownership_stats: {
              [OwnershipStatus.OWNED]: 90,
              [OwnershipStatus.BORROWED]: 5,
              [OwnershipStatus.RENTED]: 2,
              [OwnershipStatus.SUBSCRIPTION]: 3,
              [OwnershipStatus.NO_LONGER_OWNED]: 0,
            },
            platform_stats: { PC: 70, 'PlayStation 5': 20, 'Nintendo Switch': 10 },
            genre_stats: { RPG: 40, Action: 30, Adventure: 30 },
            pile_of_shame: 30,
            completion_rate: 0.47,
            average_rating: 8.2,
            total_hours_played: 500,
          });
        })
      );

      const result = await getCollectionStats();

      expect(result.totalGames).toBe(100);
      expect(result.pileOfShame).toBe(30);
      expect(result.completionRate).toBe(0.47);
      expect(result.averageRating).toBe(8.2);
      expect(result.totalHoursPlayed).toBe(500);
      expect(result.completionStats[PlayStatus.COMPLETED]).toBe(40);
      expect(result.ownershipStats[OwnershipStatus.OWNED]).toBe(90);
      expect(result.platformStats['PC']).toBe(70);
    });
  });

  describe('addPlatformToUserGame', () => {
    it('adds platform to user game', async () => {
      server.use(
        http.post(`${API_URL}/user-games/user-game-123/platforms`, async ({ request }) => {
          const body = (await request.json()) as {
            platform: string;
            storefront?: string;
            is_available: boolean;
          };
          expect(body.platform).toBe('pc');
          expect(body.storefront).toBe('steam');
          expect(body.is_available).toBe(true);

          return HttpResponse.json(mockUserGamePlatformApi);
        })
      );

      const result = await addPlatformToUserGame('user-game-123', {
        platform: 'pc',
        storefront: 'steam',
        isAvailable: true,
      });

      expect(result.id).toBe('ugp-1');
      expect(result.platform).toBe('pc');
    });
  });

  describe('updatePlatformAssociation', () => {
    it('updates platform association', async () => {
      server.use(
        http.put(`${API_URL}/user-games/user-game-123/platforms/ugp-1`, async ({ request }) => {
          const body = (await request.json()) as {
            platform: string;
            storefront?: string;
          };
          expect(body.platform).toBe('playstation-5');

          return HttpResponse.json({
            ...mockUserGamePlatformApi,
            platform: 'playstation-5',
          });
        })
      );

      const result = await updatePlatformAssociation('user-game-123', 'ugp-1', {
        platform: 'playstation-5',
      });

      expect(result.platform).toBe('playstation-5');
    });
  });

  describe('removePlatformFromUserGame', () => {
    it('removes platform from user game', async () => {
      server.use(
        http.delete(`${API_URL}/user-games/user-game-123/platforms/ugp-1`, () => {
          return new HttpResponse(null, { status: 204 });
        })
      );

      await expect(
        removePlatformFromUserGame('user-game-123', 'ugp-1')
      ).resolves.toBeUndefined();
    });
  });

  describe('getUserGameGenres', () => {
    it('fetches unique genres from user collection', async () => {
      server.use(
        http.get(`${API_URL}/user-games/genres`, () => {
          return HttpResponse.json({ genres: ['Action', 'Adventure', 'RPG'] });
        })
      );

      const genres = await getUserGameGenres();

      expect(Array.isArray(genres)).toBe(true);
      expect(genres).toEqual(['Action', 'Adventure', 'RPG']);
    });

    it('returns empty array when no genres exist', async () => {
      server.use(
        http.get(`${API_URL}/user-games/genres`, () => {
          return HttpResponse.json({ genres: [] });
        })
      );

      const genres = await getUserGameGenres();

      expect(genres).toEqual([]);
    });
  });

  describe('getUserGameIds', () => {
    it('fetches game IDs without filters', async () => {
      server.use(
        http.get(`${API_URL}/user-games/ids`, () => {
          return HttpResponse.json({ ids: ['game-1', 'game-2', 'game-3'] });
        })
      );

      const ids = await getUserGameIds();

      expect(ids).toEqual(['game-1', 'game-2', 'game-3']);
    });

    it('handles multiple platform values', async () => {
      server.use(
        http.get(`${API_URL}/user-games/ids`, ({ request }) => {
          const url = new URL(request.url);
          const platforms = url.searchParams.getAll('platform');
          expect(platforms).toEqual(['windows', 'playstation_5']);

          return HttpResponse.json({ ids: ['game-1', 'game-2'] });
        })
      );

      const ids = await getUserGameIds({
        platform: ['windows', 'playstation_5'],
      });
      expect(ids).toEqual(['game-1', 'game-2']);
    });

    it('handles multiple genre values', async () => {
      server.use(
        http.get(`${API_URL}/user-games/ids`, ({ request }) => {
          const url = new URL(request.url);
          const genres = url.searchParams.getAll('genre');
          expect(genres).toEqual(['RPG', 'Action']);

          return HttpResponse.json({ ids: ['game-1'] });
        })
      );

      const ids = await getUserGameIds({
        genre: ['RPG', 'Action'],
      });
      expect(ids).toEqual(['game-1']);
    });

    it('handles multiple tag values', async () => {
      server.use(
        http.get(`${API_URL}/user-games/ids`, ({ request }) => {
          const url = new URL(request.url);
          const tags = url.searchParams.getAll('tags');
          expect(tags).toEqual(['tag-id-1', 'tag-id-2']);

          return HttpResponse.json({ ids: ['game-1', 'game-2'] });
        })
      );

      const ids = await getUserGameIds({
        tags: ['tag-id-1', 'tag-id-2'],
      });
      expect(ids).toEqual(['game-1', 'game-2']);
    });
  });
});
