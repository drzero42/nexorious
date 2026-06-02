import { api } from './client';
import type { AdminEvent, AdminEventFilters, AdminEventListResponse } from '@/types';

interface AdminEventApiResponse {
  id: string;
  type: string;
  category: string;
  scope: 'user' | 'admin';
  occurred_at: string;
  actor_user_id: string | null;
  actor_username: string | null;
  title: string;
  body: string;
  payload: unknown;
}

interface AdminEventListApiResponse {
  events: AdminEventApiResponse[];
  next_cursor: string | null;
}

function transformEvent(e: AdminEventApiResponse): AdminEvent {
  return {
    id: e.id,
    type: e.type,
    category: e.category,
    scope: e.scope,
    occurredAt: e.occurred_at,
    actorUserId: e.actor_user_id,
    actorUsername: e.actor_username,
    title: e.title,
    body: e.body,
    payload: e.payload,
  };
}

export const eventsApi = {
  list: async (
    filters: AdminEventFilters,
    before?: string,
    limit = 50,
  ): Promise<AdminEventListResponse> => {
    const params: Record<string, string | number> = {};
    if (filters.type) params.type = filters.type;
    if (filters.category) params.category = filters.category;
    if (filters.scope) params.scope = filters.scope;
    if (filters.user) params.user = filters.user;
    if (filters.since) params.since = filters.since;
    if (filters.until) params.until = filters.until;
    if (before) params.before = before;
    params.limit = limit;

    const response = await api.get<AdminEventListApiResponse>('/admin/events', {
      params,
    });
    return {
      events: response.events.map(transformEvent),
      nextCursor: response.next_cursor,
    };
  },
};
