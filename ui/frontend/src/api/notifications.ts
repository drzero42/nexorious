import { api } from './client';

export interface NotificationChannel {
  id: string;
  name: string;
  created_at: string;
}

export interface EventTypeMeta {
  type: string;
  scope: 'user' | 'admin';
  category: string;
  label: string;
  default_on: boolean;
}

export const notificationsApi = {
  listChannels: () => api.get<NotificationChannel[]>('/notifications/channels'),
  createChannel: (data: { name: string; url: string }) =>
    api.post<NotificationChannel>('/notifications/channels', data),
  updateChannel: (id: string, data: { name?: string; url?: string }) =>
    api.patch<NotificationChannel>(`/notifications/channels/${id}`, data),
  deleteChannel: (id: string) => api.delete<void>(`/notifications/channels/${id}`),
  testChannel: (id: string) => api.post<void>(`/notifications/channels/${id}/test`),
  listEventTypes: () => api.get<EventTypeMeta[]>('/notifications/event-types'),
  listSubscriptions: () => api.get<{ event_types: string[] }>('/notifications/subscriptions'),
  putSubscriptions: (eventTypes: string[]) =>
    api.put<{ event_types: string[] }>('/notifications/subscriptions', {
      event_types: eventTypes,
    }),
  resetSubscriptions: () =>
    api.post<{ event_types: string[] }>('/notifications/subscriptions/reset'),
};
