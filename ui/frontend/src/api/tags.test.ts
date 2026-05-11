import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { setAuthHandlers } from './client';
import {
  getTags,
  getAllTags,
  getTag,
  createTag,
  createOrGetTag,
  updateTag,
  deleteTag,
  assignTagsToGame,
  removeTagsFromGame,
  bulkAssignTags,
  bulkRemoveTags,
} from './tags';

const API_URL = '/api';

// Mock tag data
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

const mockTag2Api = {
  id: 'tag-2',
  user_id: 'user-123',
  name: 'Action',
  color: '#33FF57',
  description: 'Action games',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  game_count: 3,
};

describe('tags.ts', () => {
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

  describe('getTags', () => {
    it('returns paginated tags list', async () => {
      server.use(
        http.get(`${API_URL}/tags/`, () => {
          return HttpResponse.json({
            tags: [mockTagApi, mockTag2Api],
            total: 2,
            page: 1,
            per_page: 100,
            total_pages: 1,
          });
        })
      );

      const result = await getTags();

      expect(result.tags).toHaveLength(2);
      expect(result.total).toBe(2);
      expect(result.page).toBe(1);
      expect(result.perPage).toBe(100);
      expect(result.totalPages).toBe(1);

      // Verify tag transformation
      expect(result.tags[0]).toEqual({
        id: 'tag-1',
        user_id: 'user-123',
        name: 'RPG',
        color: '#FF5733',
        description: 'Role-playing games',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
        game_count: 5,
      });
    });

    it('passes custom parameters', async () => {
      server.use(
        http.get(`${API_URL}/tags/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('page')).toBe('2');
          expect(url.searchParams.get('per_page')).toBe('50');
          expect(url.searchParams.get('include_game_count')).toBe('true');

          return HttpResponse.json({
            tags: [],
            total: 0,
            page: 2,
            per_page: 50,
            total_pages: 0,
          });
        })
      );

      await getTags({ page: 2, perPage: 50, includeGameCount: true });
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(getTags()).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });
  });

  describe('getAllTags', () => {
    it('returns all tags by paginating through all pages', async () => {
      let callCount = 0;

      server.use(
        http.get(`${API_URL}/tags/`, ({ request }) => {
          callCount++;
          const url = new URL(request.url);
          const page = parseInt(url.searchParams.get('page') || '1');

          if (page === 1) {
            return HttpResponse.json({
              tags: [mockTagApi],
              total: 2,
              page: 1,
              per_page: 100,
              total_pages: 2,
            });
          }

          return HttpResponse.json({
            tags: [mockTag2Api],
            total: 2,
            page: 2,
            per_page: 100,
            total_pages: 2,
          });
        })
      );

      const result = await getAllTags();

      expect(result).toHaveLength(2);
      expect(result[0].id).toBe('tag-1');
      expect(result[1].id).toBe('tag-2');
      expect(callCount).toBe(2);
    });

    it('returns empty array when no tags exist', async () => {
      server.use(
        http.get(`${API_URL}/tags/`, () => {
          return HttpResponse.json({
            tags: [],
            total: 0,
            page: 1,
            per_page: 100,
            total_pages: 0,
          });
        })
      );

      const result = await getAllTags();

      expect(result).toEqual([]);
    });
  });

  describe('getTag', () => {
    it('returns single tag by ID', async () => {
      server.use(
        http.get(`${API_URL}/tags/tag-1`, () => {
          return HttpResponse.json(mockTagApi);
        })
      );

      const result = await getTag('tag-1');

      expect(result.id).toBe('tag-1');
      expect(result.name).toBe('RPG');
      expect(result.color).toBe('#FF5733');
    });

    it('throws error for non-existent tag', async () => {
      server.use(
        http.get(`${API_URL}/tags/non-existent`, () => {
          return HttpResponse.json({ detail: 'Tag not found' }, { status: 404 });
        })
      );

      await expect(getTag('non-existent')).rejects.toMatchObject({
        message: 'Tag not found',
        status: 404,
      });
    });
  });

  describe('createTag', () => {
    it('creates a new tag', async () => {
      server.use(
        http.post(`${API_URL}/tags/`, async ({ request }) => {
          const body = (await request.json()) as {
            name: string;
            color?: string;
            description?: string;
          };
          expect(body.name).toBe('New Tag');
          expect(body.color).toBe('#123456');
          expect(body.description).toBe('A new tag');

          return HttpResponse.json({
            id: 'tag-new',
            user_id: 'user-123',
            name: 'New Tag',
            color: '#123456',
            description: 'A new tag',
            created_at: '2024-01-02T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
          });
        })
      );

      const result = await createTag({
        name: 'New Tag',
        color: '#123456',
        description: 'A new tag',
      });

      expect(result.id).toBe('tag-new');
      expect(result.name).toBe('New Tag');
    });

    it('creates tag with minimal data', async () => {
      server.use(
        http.post(`${API_URL}/tags/`, async ({ request }) => {
          const body = (await request.json()) as { name: string };
          expect(body.name).toBe('Simple Tag');

          return HttpResponse.json({
            id: 'tag-simple',
            user_id: 'user-123',
            name: 'Simple Tag',
            color: '#808080',
            created_at: '2024-01-02T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
          });
        })
      );

      const result = await createTag({ name: 'Simple Tag' });

      expect(result.name).toBe('Simple Tag');
    });

    it('throws error for duplicate tag name', async () => {
      server.use(
        http.post(`${API_URL}/tags/`, () => {
          return HttpResponse.json({ detail: 'Tag with this name already exists' }, { status: 409 });
        })
      );

      await expect(createTag({ name: 'RPG' })).rejects.toMatchObject({
        message: 'Tag with this name already exists',
        status: 409,
      });
    });
  });

  describe('createOrGetTag', () => {
    it('creates new tag when it does not exist', async () => {
      server.use(
        http.post(`${API_URL}/tags/create-or-get`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('name')).toBe('New Tag');
          expect(url.searchParams.get('color')).toBe('#FF0000');

          return HttpResponse.json({
            tag: {
              id: 'tag-new',
              user_id: 'user-123',
              name: 'New Tag',
              color: '#FF0000',
              created_at: '2024-01-02T00:00:00Z',
              updated_at: '2024-01-02T00:00:00Z',
            },
            created: true,
          });
        })
      );

      const result = await createOrGetTag('New Tag', '#FF0000');

      expect(result.tag.id).toBe('tag-new');
      expect(result.tag.name).toBe('New Tag');
      expect(result.created).toBe(true);
    });

    it('returns existing tag when it already exists', async () => {
      server.use(
        http.post(`${API_URL}/tags/create-or-get`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('name')).toBe('RPG');

          return HttpResponse.json({
            tag: mockTagApi,
            created: false,
          });
        })
      );

      const result = await createOrGetTag('RPG');

      expect(result.tag.id).toBe('tag-1');
      expect(result.created).toBe(false);
    });
  });

  describe('updateTag', () => {
    it('updates tag properties', async () => {
      server.use(
        http.put(`${API_URL}/tags/tag-1`, async ({ request }) => {
          const body = (await request.json()) as {
            name?: string;
            color?: string;
            description?: string;
          };
          expect(body.name).toBe('Updated RPG');
          expect(body.color).toBe('#AABBCC');

          return HttpResponse.json({
            ...mockTagApi,
            name: 'Updated RPG',
            color: '#AABBCC',
          });
        })
      );

      const result = await updateTag('tag-1', {
        name: 'Updated RPG',
        color: '#AABBCC',
      });

      expect(result.name).toBe('Updated RPG');
      expect(result.color).toBe('#AABBCC');
    });

    it('updates only specified properties', async () => {
      server.use(
        http.put(`${API_URL}/tags/tag-1`, async ({ request }) => {
          const body = (await request.json()) as Record<string, unknown>;
          expect(body.description).toBe('New description');
          expect(body.name).toBeUndefined();

          return HttpResponse.json({
            ...mockTagApi,
            description: 'New description',
          });
        })
      );

      const result = await updateTag('tag-1', { description: 'New description' });

      expect(result.description).toBe('New description');
    });

    it('throws error for non-existent tag', async () => {
      server.use(
        http.put(`${API_URL}/tags/non-existent`, () => {
          return HttpResponse.json({ detail: 'Tag not found' }, { status: 404 });
        })
      );

      await expect(updateTag('non-existent', { name: 'New Name' })).rejects.toMatchObject({
        message: 'Tag not found',
        status: 404,
      });
    });
  });

  describe('deleteTag', () => {
    it('deletes a tag', async () => {
      server.use(
        http.delete(`${API_URL}/tags/tag-1`, () => {
          return new HttpResponse(null, { status: 204 });
        })
      );

      await expect(deleteTag('tag-1')).resolves.toBeUndefined();
    });

    it('throws error for non-existent tag', async () => {
      server.use(
        http.delete(`${API_URL}/tags/non-existent`, () => {
          return HttpResponse.json({ detail: 'Tag not found' }, { status: 404 });
        })
      );

      await expect(deleteTag('non-existent')).rejects.toMatchObject({
        message: 'Tag not found',
        status: 404,
      });
    });
  });

  describe('assignTagsToGame', () => {
    it('assigns tags to a game', async () => {
      server.use(
        http.post(`${API_URL}/tags/assign/game-123`, async ({ request }) => {
          const body = (await request.json()) as { tag_ids: string[] };
          expect(body.tag_ids).toEqual(['tag-1', 'tag-2']);

          return HttpResponse.json({
            message: 'Tags assigned successfully',
            new_associations: 2,
            total_requested: 2,
          });
        })
      );

      const result = await assignTagsToGame('game-123', ['tag-1', 'tag-2']);

      expect(result.message).toBe('Tags assigned successfully');
      expect(result.newAssociations).toBe(2);
      expect(result.totalRequested).toBe(2);
    });

    it('handles partial assignment (some already assigned)', async () => {
      server.use(
        http.post(`${API_URL}/tags/assign/game-123`, () => {
          return HttpResponse.json({
            message: 'Tags assigned successfully',
            new_associations: 1,
            total_requested: 2,
          });
        })
      );

      const result = await assignTagsToGame('game-123', ['tag-1', 'tag-2']);

      expect(result.newAssociations).toBe(1);
      expect(result.totalRequested).toBe(2);
    });
  });

  describe('removeTagsFromGame', () => {
    it('removes tags from a game', async () => {
      server.use(
        http.delete(`${API_URL}/tags/remove/game-123`, async ({ request }) => {
          const body = (await request.json()) as { tag_ids: string[] };
          expect(body.tag_ids).toEqual(['tag-1']);

          return HttpResponse.json({
            message: 'Tags removed successfully',
            removed_associations: 1,
            total_requested: 1,
          });
        })
      );

      const result = await removeTagsFromGame('game-123', ['tag-1']);

      expect(result.message).toBe('Tags removed successfully');
      expect(result.removedAssociations).toBe(1);
      expect(result.totalRequested).toBe(1);
    });
  });

  describe('bulkAssignTags', () => {
    it('assigns tags to multiple games', async () => {
      server.use(
        http.post(`${API_URL}/tags/bulk-assign`, async ({ request }) => {
          const body = (await request.json()) as {
            user_game_ids: string[];
            tag_ids: string[];
          };
          expect(body.user_game_ids).toEqual(['game-1', 'game-2', 'game-3']);
          expect(body.tag_ids).toEqual(['tag-1', 'tag-2']);

          return HttpResponse.json({
            message: 'Tags bulk assigned successfully',
            total_new_associations: 6,
            games_processed: 3,
          });
        })
      );

      const result = await bulkAssignTags(['game-1', 'game-2', 'game-3'], ['tag-1', 'tag-2']);

      expect(result.message).toBe('Tags bulk assigned successfully');
      expect(result.totalNewAssociations).toBe(6);
      expect(result.gamesProcessed).toBe(3);
    });
  });

  describe('bulkRemoveTags', () => {
    it('removes tags from multiple games', async () => {
      server.use(
        http.delete(`${API_URL}/tags/bulk-remove`, async ({ request }) => {
          const body = (await request.json()) as {
            user_game_ids: string[];
            tag_ids: string[];
          };
          expect(body.user_game_ids).toEqual(['game-1', 'game-2']);
          expect(body.tag_ids).toEqual(['tag-1']);

          return HttpResponse.json({
            message: 'Tags bulk removed successfully',
            total_removed_associations: 2,
            games_processed: 2,
          });
        })
      );

      const result = await bulkRemoveTags(['game-1', 'game-2'], ['tag-1']);

      expect(result.message).toBe('Tags bulk removed successfully');
      expect(result.totalRemovedAssociations).toBe(2);
      expect(result.gamesProcessed).toBe(2);
    });
  });
});
