import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { NotificationsSection } from './notifications-section';

// Hoist putMock so it can be referenced inside the vi.mock factory.
const { putMock } = vi.hoisted(() => ({ putMock: vi.fn().mockResolvedValue(undefined) }));

vi.mock('@/hooks/use-notifications', () => ({
  notificationKeys: {
    all: ['notifications'] as const,
    channels: () => ['notifications', 'channels'] as const,
    eventTypes: () => ['notifications', 'event-types'] as const,
    subscriptions: () => ['notifications', 'subscriptions'] as const,
  },

  // Query hooks
  useChannels: () => ({
    data: [{ id: 'c1', name: 'Phone', created_at: '2026-01-01T00:00:00Z' }],
    isLoading: false,
    isPending: false,
    isError: false,
  }),
  useEventTypes: () => ({
    data: [
      {
        type: 'sync.failed',
        scope: 'user',
        category: 'Sync',
        label: 'Sync failed',
        default_on: true,
      },
    ],
    isLoading: false,
    isPending: false,
    isError: false,
  }),
  useSubscriptions: () => ({
    data: { event_types: [] },
    isLoading: false,
    isPending: false,
    isError: false,
  }),

  // Mutation hooks
  useCreateChannel: () => ({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    mutate: vi.fn(),
    isPending: false,
  }),
  useUpdateChannel: () => ({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    mutate: vi.fn(),
    isPending: false,
  }),
  useDeleteChannel: () => ({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    mutate: vi.fn(),
    isPending: false,
  }),
  useTestChannel: () => ({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    mutate: vi.fn(),
    isPending: false,
  }),
  usePutSubscriptions: () => ({
    mutateAsync: putMock,
    mutate: vi.fn(),
    isPending: false,
  }),
  useResetSubscriptions: () => ({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    mutate: vi.fn(),
    isPending: false,
  }),
}));

describe('NotificationsSection', () => {
  it('renders channels and event-type toggles', () => {
    render(<NotificationsSection />);

    expect(screen.getByText('Phone')).toBeInTheDocument();
    expect(screen.getByText('Sync failed')).toBeInTheDocument();
  });

  it('toggling an event type calls putSubscriptions', async () => {
    const user = userEvent.setup();
    putMock.mockClear();

    render(<NotificationsSection />);

    const toggle = screen.getByRole('switch', { name: 'Sync failed' });
    await user.click(toggle);

    expect(putMock).toHaveBeenCalledOnce();
    expect(putMock).toHaveBeenCalledWith(expect.arrayContaining(['sync.failed']));
  });
});
