export interface AdminEvent {
  id: string;
  type: string;
  category: string;
  scope: 'user' | 'admin';
  occurredAt: string;
  actorUserId: string | null;
  actorUsername: string | null;
  title: string;
  body: string;
  payload: unknown;
}

export interface AdminEventListResponse {
  events: AdminEvent[];
  nextCursor: string | null;
}

export interface AdminEventFilters {
  type?: string;
  category?: string;
  scope?: 'user' | 'admin';
  user?: string;
  since?: string;
  until?: string;
}
