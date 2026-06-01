import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { ChannelDialog } from './channel-dialog';

// Hoist mutation mocks so they can be referenced inside the vi.mock factory
// and also asserted against in tests.
const { createMutateAsync, updateMutateAsync, testUrlMutateAsync } = vi.hoisted(() => ({
  createMutateAsync: vi.fn().mockResolvedValue(undefined),
  updateMutateAsync: vi.fn().mockResolvedValue(undefined),
  testUrlMutateAsync: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('@/hooks/use-notifications', () => ({
  useCreateChannel: () => ({
    mutateAsync: createMutateAsync,
    mutate: vi.fn(),
    isPending: false,
  }),
  useUpdateChannel: () => ({
    mutateAsync: updateMutateAsync,
    mutate: vi.fn(),
    isPending: false,
  }),
  useTestChannel: () => ({
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    mutate: vi.fn(),
    isPending: false,
  }),
  useTestUrl: () => ({
    mutateAsync: testUrlMutateAsync,
    mutate: vi.fn(),
    isPending: false,
  }),
}));

const noop = () => {};

describe('ChannelDialog', () => {
  it('add mode rejects a blank URL', async () => {
    const user = userEvent.setup();
    createMutateAsync.mockClear();

    render(<ChannelDialog open={true} onOpenChange={noop} channel={null} />);

    // Fill name but leave URL blank
    await user.type(screen.getByLabelText(/^name$/i), 'My Channel');
    await user.click(screen.getByRole('button', { name: /add channel/i }));

    // Validation should block the call and show an inline error
    await waitFor(() => {
      expect(screen.getByText('URL is required')).toBeInTheDocument();
    });
    expect(createMutateAsync).not.toHaveBeenCalled();
  });

  it('add mode: typing a URL and clicking Send test calls testUrl with that URL', async () => {
    const user = userEvent.setup();
    testUrlMutateAsync.mockClear();

    render(<ChannelDialog open={true} onOpenChange={noop} channel={null} />);

    // Type a URL into the URL field
    const urlInput = screen.getByLabelText(/shoutrrr url/i);
    await user.type(urlInput, 'ntfy://ntfy.sh/my-topic');

    await user.click(screen.getByRole('button', { name: /send test/i }));

    await waitFor(() => {
      expect(testUrlMutateAsync).toHaveBeenCalledWith('ntfy://ntfy.sh/my-topic');
    });
  });

  it('edit mode allows a blank URL and omits it from the update payload', async () => {
    const user = userEvent.setup();
    updateMutateAsync.mockClear();

    const channel = { id: 'c1', name: 'Old Name', created_at: '2026-01-01T00:00:00Z' };
    render(<ChannelDialog open={true} onOpenChange={noop} channel={channel} />);

    // Clear the name field and type a new one; leave URL blank
    const nameInput = screen.getByLabelText(/^name$/i);
    await user.clear(nameInput);
    await user.type(nameInput, 'New Name');

    await user.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      expect(updateMutateAsync).toHaveBeenCalledOnce();
    });

    const callArg = updateMutateAsync.mock.calls[0][0] as {
      id: string;
      data: Record<string, unknown>;
    };
    expect(callArg.id).toBe('c1');
    expect(callArg.data).toEqual({ name: 'New Name' });
    expect(callArg.data).not.toHaveProperty('url');
  });
});
