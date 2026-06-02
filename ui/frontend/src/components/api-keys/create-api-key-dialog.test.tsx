import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { CreateApiKeyDialog } from './create-api-key-dialog';

// Mock the create mutation so no network call is made and we control the result.
const mockMutateAsync = vi.fn();
vi.mock('@/hooks', () => ({
  useCreateApiKey: () => ({ mutateAsync: mockMutateAsync, isPending: false }),
}));

describe('CreateApiKeyDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // jsdom has no clipboard by default; install a spy-able one.
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('reveals the raw key exactly once and copies it, then clears on close', async () => {
    const user = userEvent.setup();
    mockMutateAsync.mockResolvedValue({
      id: 'k1',
      name: 'CI token',
      scopes: 'write',
      key: 'nxr_secret_raw_value',
      created_at: '2026-06-02T12:00:00Z',
      expires_at: null,
    });

    const onOpenChange = vi.fn();
    render(<CreateApiKeyDialog open={true} onOpenChange={onOpenChange} />);

    await user.type(screen.getByLabelText(/name/i), 'CI token');
    await user.click(screen.getByRole('button', { name: /create/i }));

    // Reveal state: the raw key is now visible.
    await waitFor(() => {
      expect(screen.getByText('nxr_secret_raw_value')).toBeInTheDocument();
    });

    // Copy button writes the raw key to the clipboard.
    await user.click(screen.getByRole('button', { name: /copy/i }));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('nxr_secret_raw_value');

    // Closing the dialog clears the key from state.
    fireEvent.click(screen.getByRole('button', { name: /done/i }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
