import { describe, it, expect, vi } from 'vitest';
import { useState } from 'react';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { StarRating } from './star-rating';

function filledStarCount(buttons: HTMLElement[]): number {
  return buttons.filter((b) => b.querySelector('.fill-yellow-400')).length;
}

describe('StarRating', () => {
  it('displays the current rating value', () => {
    render(<StarRating value={3} showLabel />);

    expect(screen.getByText('(3/5)')).toBeInTheDocument();
  });

  it("displays 'Not rated' when value is null and showLabel is true", () => {
    render(<StarRating value={null} showLabel />);

    expect(screen.getByText('Not rated')).toBeInTheDocument();
  });

  it('calls onChange when a star is clicked', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating onChange={handleChange} />);

    const buttons = screen.getAllByRole('button');
    await user.click(buttons[2]); // Click the 3rd star

    expect(handleChange).toHaveBeenCalledWith(3);
  });

  it('immediately fills the clicked star when starting unrated (regression #845)', async () => {
    const user = userEvent.setup();

    // Mirror the edit form: a controlled StarRating whose value comes from state.
    function Controlled() {
      const [value, setValue] = useState<number | null>(null);
      return <StarRating value={value} onChange={setValue} clearable />;
    }

    render(<Controlled />);

    const buttons = screen.getAllByRole('button');
    await user.click(buttons[4]); // Click the 5th star

    // Without moving the mouse away, all 5 stars must read as filled — not the
    // stale pre-click value (which previously leaked through the focus handler).
    expect(filledStarCount(buttons)).toBe(5);
  });

  it('immediately empties the stars when clicking to clear (regression #845)', async () => {
    const user = userEvent.setup();

    function Controlled() {
      const [value, setValue] = useState<number | null>(3);
      return <StarRating value={value} onChange={setValue} clearable />;
    }

    render(<Controlled />);

    const buttons = screen.getAllByRole('button');
    await user.click(buttons[2]); // Click the 3rd star (already selected) to clear

    // The cleared state must show right away, even while the pointer still
    // hovers the just-clicked star.
    expect(filledStarCount(buttons)).toBe(0);
  });

  it('clears rating when clicking the same star (clearable mode)', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating value={3} onChange={handleChange} clearable />);

    const buttons = screen.getAllByRole('button');
    await user.click(buttons[2]); // Click the 3rd star (already selected)

    expect(handleChange).toHaveBeenCalledWith(null);
  });

  it('does not call onChange when readonly', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating value={3} onChange={handleChange} readonly />);

    const buttons = screen.getAllByRole('button');
    await user.click(buttons[4]); // Try to click the 5th star

    expect(handleChange).not.toHaveBeenCalled();
  });

  it('does not call onChange when disabled', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();

    render(<StarRating value={3} onChange={handleChange} disabled />);

    const buttons = screen.getAllByRole('button');
    await user.click(buttons[4]); // Try to click the 5th star

    expect(handleChange).not.toHaveBeenCalled();
  });

  it('has correct aria-label for readonly mode', () => {
    render(<StarRating value={4} readonly />);

    expect(screen.getByRole('img')).toHaveAttribute('aria-label', 'Rated 4 out of 5 stars');
  });

  it('has correct aria-label for interactive mode', () => {
    const handleChange = vi.fn();
    render(<StarRating value={2} onChange={handleChange} />);

    expect(screen.getByRole('radiogroup')).toHaveAttribute(
      'aria-label',
      expect.stringContaining('Current rating: 2'),
    );
  });
});
