import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { NavBadge } from './nav-badge';

describe('NavBadge', () => {
  it('renders count when greater than 0', () => {
    render(<NavBadge count={5} />);
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('renders nothing when count is 0', () => {
    const { container } = render(<NavBadge count={0} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when count is negative', () => {
    const { container } = render(<NavBadge count={-1} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('caps display at 99+', () => {
    render(<NavBadge count={150} />);
    expect(screen.getByText('99+')).toBeInTheDocument();
  });

  it('calls onClick when clicked', async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    render(<NavBadge count={5} onClick={handleClick} />);

    await user.click(screen.getByText('5'));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('stops event propagation on click', async () => {
    const user = userEvent.setup();
    const parentClick = vi.fn();
    const badgeClick = vi.fn();

    render(
      <div onClick={parentClick}>
        <NavBadge count={5} onClick={badgeClick} />
      </div>
    );

    await user.click(screen.getByText('5'));
    expect(badgeClick).toHaveBeenCalled();
    expect(parentClick).not.toHaveBeenCalled();
  });
});
