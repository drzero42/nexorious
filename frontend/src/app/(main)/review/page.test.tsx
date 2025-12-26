import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';

// Use vi.hoisted for variables used inside vi.mock factories
const { mockReplace, mockToastInfo } = vi.hoisted(() => ({
  mockReplace: vi.fn(),
  mockToastInfo: vi.fn(),
}));

// Mock next/navigation
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    replace: mockReplace,
  }),
}));

// Mock sonner toast
vi.mock('sonner', () => ({
  toast: {
    info: mockToastInfo,
  },
}));

// Import the component after mocking
import ReviewPage from './page';

describe('ReviewPage Redirect', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows redirecting message', () => {
    render(<ReviewPage />);
    expect(screen.getByText('Redirecting to Sync...')).toBeInTheDocument();
  });

  it('redirects to /sync', async () => {
    render(<ReviewPage />);

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith('/sync');
    });
  });

  it('shows toast notification about page move', async () => {
    render(<ReviewPage />);

    await waitFor(() => {
      expect(mockToastInfo).toHaveBeenCalledWith('Review items are now on the Sync page');
    });
  });
});
