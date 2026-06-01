import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { notificationsApi } from '@/api/notifications';

export const notificationKeys = {
  all: ['notifications'] as const,
  channels: () => [...notificationKeys.all, 'channels'] as const,
  eventTypes: () => [...notificationKeys.all, 'event-types'] as const,
  subscriptions: () => [...notificationKeys.all, 'subscriptions'] as const,
};

export function useChannels() {
  return useQuery({
    queryKey: notificationKeys.channels(),
    queryFn: notificationsApi.listChannels,
  });
}

export function useEventTypes() {
  return useQuery({
    queryKey: notificationKeys.eventTypes(),
    queryFn: notificationsApi.listEventTypes,
  });
}

export function useSubscriptions() {
  return useQuery({
    queryKey: notificationKeys.subscriptions(),
    queryFn: notificationsApi.listSubscriptions,
  });
}

export function useCreateChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; url: string }) => notificationsApi.createChannel(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.channels() }),
  });
}

export function useUpdateChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: { name?: string; url?: string } }) =>
      notificationsApi.updateChannel(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.channels() }),
  });
}

export function useDeleteChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => notificationsApi.deleteChannel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.channels() }),
  });
}

export function useTestChannel() {
  return useMutation({ mutationFn: (id: string) => notificationsApi.testChannel(id) });
}

export function usePutSubscriptions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (eventTypes: string[]) => notificationsApi.putSubscriptions(eventTypes),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.subscriptions() }),
  });
}

export function useResetSubscriptions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => notificationsApi.resetSubscriptions(),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.subscriptions() }),
  });
}
