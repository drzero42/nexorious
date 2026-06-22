import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import { WhatsNew } from './whats-new';

const mockUseChangelogUnseen = vi.fn();
const mockUseChangelogContent = vi.fn();
vi.mock('@/hooks', () => ({
  useChangelogUnseen: () => mockUseChangelogUnseen(),
  useChangelogContent: () => mockUseChangelogContent(),
  changelogKeys: { unseen: () => ['changelog', 'unseen'] },
}));

describe('WhatsNew', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseChangelogContent.mockReturnValue({ data: undefined, isLoading: false });
  });

  it('shows the dot when has_unseen is true', () => {
    mockUseChangelogUnseen.mockReturnValue({ data: { has_unseen: true } });
    render(<WhatsNew />);
    expect(screen.getByLabelText('new changelog entries')).toBeInTheDocument();
  });

  it('does not show the dot when has_unseen is false', () => {
    mockUseChangelogUnseen.mockReturnValue({ data: { has_unseen: false } });
    render(<WhatsNew />);
    expect(screen.queryByLabelText('new changelog entries')).not.toBeInTheDocument();
  });
});
