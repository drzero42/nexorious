import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { VersionFooter } from './version-footer';

const mockUseVersion = vi.fn();
vi.mock('@/hooks', () => ({
  useVersion: () => mockUseVersion(),
}));

describe('VersionFooter', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders the update link when an update is available', () => {
    mockUseVersion.mockReturnValue({
      data: {
        version: '0.9.0',
        commit: 'abc1234',
        update_check_enabled: true,
        update_available: true,
        latest_version: '0.10.0',
        release_url: 'https://github.com/drzero42/nexorious/releases/tag/v0.10.0',
      },
    });
    render(<VersionFooter />);
    expect(screen.getByText('Version: 0.9.0')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: /a newer version is available/i });
    expect(link).toHaveAttribute(
      'href',
      'https://github.com/drzero42/nexorious/releases/tag/v0.10.0',
    );
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('renders no link when no update is available', () => {
    mockUseVersion.mockReturnValue({
      data: {
        version: '0.9.0',
        commit: 'abc1234',
        update_check_enabled: true,
        update_available: false,
        latest_version: '',
        release_url: '',
      },
    });
    render(<VersionFooter />);
    expect(screen.getByText('Version: 0.9.0')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('renders nothing while version data is missing', () => {
    mockUseVersion.mockReturnValue({ data: undefined });
    const { container } = render(<VersionFooter />);
    expect(container).toBeEmptyDOMElement();
  });
});
