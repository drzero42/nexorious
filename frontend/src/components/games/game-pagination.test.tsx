import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GamesPagination } from './game-pagination';

const defaultProps = {
  page: 1,
  perPage: 50,
  totalPages: 5,
  totalCount: 220,
  onPageChange: vi.fn(),
  onPerPageChange: vi.fn(),
  showPerPageSelector: false,
};

describe('GamesPagination', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('visibility', () => {
    it('renders nothing when totalPages is 1', () => {
      const { container } = render(
        <GamesPagination {...defaultProps} totalPages={1} totalCount={10} />
      );
      expect(container.firstChild).toBeNull();
    });

    it('renders nothing when totalPages is 0', () => {
      const { container } = render(
        <GamesPagination {...defaultProps} totalPages={0} totalCount={0} />
      );
      expect(container.firstChild).toBeNull();
    });

    it('renders when totalPages is 2', () => {
      render(<GamesPagination {...defaultProps} totalPages={2} />);
      expect(screen.getByRole('navigation', { name: /pagination/i })).toBeInTheDocument();
    });
  });

  describe('per-page selector', () => {
    it('does not render per-page selector when showPerPageSelector is false', () => {
      render(<GamesPagination {...defaultProps} showPerPageSelector={false} />);
      expect(screen.queryByText('Per page')).not.toBeInTheDocument();
    });

    it('renders per-page selector when showPerPageSelector is true', () => {
      render(<GamesPagination {...defaultProps} showPerPageSelector={true} perPage={50} />);
      expect(screen.getByText('Per page')).toBeInTheDocument();
    });

    it('shows all four per-page options', async () => {
      const user = userEvent.setup();
      render(<GamesPagination {...defaultProps} showPerPageSelector={true} perPage={50} />);
      await user.click(screen.getByRole('combobox'));
      expect(screen.getByRole('option', { name: '25' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: '50' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: '100' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: '500' })).toBeInTheDocument();
    });

    it('calls onPerPageChange with number when option is selected', async () => {
      const user = userEvent.setup();
      const onPerPageChange = vi.fn();
      render(
        <GamesPagination
          {...defaultProps}
          showPerPageSelector={true}
          perPage={50}
          onPerPageChange={onPerPageChange}
        />
      );
      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: '100' }));
      expect(onPerPageChange).toHaveBeenCalledWith(100);
      expect(onPerPageChange).toHaveBeenCalledTimes(1);
    });
  });

  describe('page navigation', () => {
    it('renders previous and next buttons', () => {
      render(<GamesPagination {...defaultProps} page={3} />);
      expect(screen.getByRole('link', { name: /previous/i })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: /next/i })).toBeInTheDocument();
    });

    it('previous button is aria-disabled on page 1', () => {
      render(<GamesPagination {...defaultProps} page={1} />);
      const prev = screen.getByRole('link', { name: /previous/i });
      expect(prev).toHaveAttribute('aria-disabled');
    });

    it('next button is aria-disabled on last page', () => {
      render(<GamesPagination {...defaultProps} page={5} totalPages={5} />);
      const next = screen.getByRole('link', { name: /next/i });
      expect(next).toHaveAttribute('aria-disabled');
    });

    it('does not call onPageChange when previous is clicked on page 1', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={1} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: /previous/i }));
      expect(onPageChange).not.toHaveBeenCalled();
    });

    it('does not call onPageChange when next is clicked on last page', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={5} totalPages={5} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: /next/i }));
      expect(onPageChange).not.toHaveBeenCalled();
    });

    it('clicking previous calls onPageChange with page - 1', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={3} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: /previous/i }));
      expect(onPageChange).toHaveBeenCalledWith(2);
    });

    it('clicking next calls onPageChange with page + 1', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={3} totalPages={5} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: /next/i }));
      expect(onPageChange).toHaveBeenCalledWith(4);
    });

    it('clicking a page number calls onPageChange with that page', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={1} totalPages={5} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: '3' }));
      expect(onPageChange).toHaveBeenCalledWith(3);
    });

    it('active page link has aria-current="page"', () => {
      render(<GamesPagination {...defaultProps} page={3} totalPages={5} />);
      const activeLink = screen.getByRole('link', { name: '3' });
      expect(activeLink).toHaveAttribute('aria-current', 'page');
    });
  });

  describe('page range with small page counts', () => {
    it('renders all pages when totalPages is 7 or fewer', () => {
      render(<GamesPagination {...defaultProps} page={1} totalPages={7} />);
      for (let i = 1; i <= 7; i++) {
        expect(screen.getByRole('link', { name: String(i) })).toBeInTheDocument();
      }
    });
  });

  describe('page range with ellipsis', () => {
    it('renders ellipsis for large page counts', () => {
      render(<GamesPagination {...defaultProps} page={5} totalPages={20} />);
      expect(screen.getByRole('link', { name: '1' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '20' })).toBeInTheDocument();
      const ellipses = screen.getAllByText('More pages');
      expect(ellipses.length).toBeGreaterThanOrEqual(1);
    });

    it('always renders first and last page', () => {
      render(<GamesPagination {...defaultProps} page={10} totalPages={20} />);
      expect(screen.getByRole('link', { name: '1' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '20' })).toBeInTheDocument();
    });

    it('renders current page and neighbours', () => {
      render(<GamesPagination {...defaultProps} page={10} totalPages={20} />);
      expect(screen.getByRole('link', { name: '9' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '10' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '11' })).toBeInTheDocument();
    });
  });
});
