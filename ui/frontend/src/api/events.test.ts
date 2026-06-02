import { describe, it, expect, vi, beforeEach } from 'vitest';
import { eventsApi } from './events';
import { api } from './client';

vi.mock('./client', () => ({
  api: { get: vi.fn() },
}));

const apiResponse = {
  events: [
    {
      id: 'evt-1',
      type: 'sync.completed',
      category: 'Sync',
      scope: 'user',
      occurred_at: '2026-06-02T10:00:00Z',
      actor_user_id: 'u-1',
      actor_username: 'alice',
      title: 'Sync completed',
      body: 'all good',
      payload: { added: 3 },
    },
  ],
  next_cursor: 'CURSOR123',
};

describe('eventsApi.list', () => {
  beforeEach(() => vi.clearAllMocks());

  it('maps snake_case response to camelCase', async () => {
    vi.mocked(api.get).mockResolvedValue(apiResponse);
    const res = await eventsApi.list({});
    expect(res.nextCursor).toBe('CURSOR123');
    expect(res.events[0]).toMatchObject({
      id: 'evt-1',
      occurredAt: '2026-06-02T10:00:00Z',
      actorUsername: 'alice',
    });
  });

  it('forwards filters and cursor as query params, omitting empties', async () => {
    vi.mocked(api.get).mockResolvedValue(apiResponse);
    await eventsApi.list({ scope: 'admin', type: '', user: 'bob' }, 'CUR', 25);
    expect(api.get).toHaveBeenCalledWith('/admin/events', {
      params: { scope: 'admin', user: 'bob', before: 'CUR', limit: 25 },
    });
  });
});
