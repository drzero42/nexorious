import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import StarRating from './StarRating.svelte';

// Test wrapper component to test two-way binding
import StarRatingTestWrapper from './StarRatingTestWrapper.test.svelte';

describe('StarRating', () => {
  it('should render 5 stars', () => {
    render(StarRating);
    
    const stars = screen.getAllByRole('button');
    expect(stars).toHaveLength(5);
  });

  it('should render with correct accessibility attributes', () => {
    render(StarRating, {
      props: {
        value: 3
      }
    });

    const starRating = screen.getByRole('radiogroup');
    expect(starRating).toHaveAttribute('aria-label', 'Rate from 1 to 5 stars. Current rating: 3. Use arrow keys to navigate, Enter to select, 0 to clear.');
  });

  it('should render readonly mode with correct accessibility', () => {
    render(StarRating, {
      props: {
        value: 4,
        readonly: true
      }
    });

    const starRating = screen.getByRole('img');
    expect(starRating).toHaveAttribute('aria-label', 'Rated 4 out of 5 stars');
    expect(starRating).not.toHaveAttribute('tabindex');
  });

  it('should display correct number of filled stars', () => {
    render(StarRating, {
      props: {
        value: 3
      }
    });

    const stars = screen.getAllByRole('button');
    
    // First 3 stars should be filled
    expect(stars[0]).toHaveClass('star-filled');
    expect(stars[1]).toHaveClass('star-filled');
    expect(stars[2]).toHaveClass('star-filled');
    
    // Last 2 stars should be empty
    expect(stars[3]).toHaveClass('star-empty');
    expect(stars[4]).toHaveClass('star-empty');
  });

  it('should update value when star is clicked', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: null
      }
    });

    const stars = screen.getAllByRole('button');
    await user.click(stars[2]!); // Click 3rd star (rating of 3)

    // Check that the component's value was updated
    expect(component.getValue()).toBe(3);
  });

  it('should clear rating when clicking on selected star and clearable is true', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: 3,
        clearable: true
      }
    });

    const stars = screen.getAllByRole('button');
    await user.click(stars[2]!); // Click 3rd star (currently selected)

    // Check that the rating was cleared
    expect(component.getValue()).toBe(null);
  });

  it('should not clear rating when clearable is false', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: 3,
        clearable: false
      }
    });

    const stars = screen.getAllByRole('button');
    await user.click(stars[2]!); // Click 3rd star (currently selected)

    // Value should remain the same
    expect(component.getValue()).toBe(3);
  });

  it('should not respond to clicks when readonly', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: 2,
        readonly: true
      }
    });

    const stars = screen.getAllByRole('button');
    await user.click(stars[3]!); // Click 4th star

    // Value should remain unchanged
    expect(component.getValue()).toBe(2);
  });

  it('should not respond to clicks when disabled', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: null,
        disabled: true
      }
    });

    const stars = screen.getAllByRole('button');
    await user.click(stars[2]!);

    // Value should remain null
    expect(component.getValue()).toBe(null);
  });

  it('should show label when showLabel is true', () => {
    render(StarRating, {
      props: {
        value: 4,
        showLabel: true
      }
    });

    expect(screen.getByText('(4/5)')).toBeInTheDocument();
  });

  it('should show "Not rated" label when value is null and showLabel is true', () => {
    render(StarRating, {
      props: {
        value: null,
        showLabel: true,
        readonly: true
      }
    });

    expect(screen.getByText('Not rated')).toBeInTheDocument();
  });

  it('should handle keyboard navigation with arrow keys', async () => {
    const user = userEvent.setup();
    
    render(StarRating, {
      props: {
        value: null
      }
    });

    const starRating = screen.getByRole('radiogroup');
    
    // Focus the component
    await user.click(starRating);
    
    // Navigate right with arrow key
    await user.keyboard('[ArrowRight]');
    
    // Check if second star is hovered/focused
    const stars = screen.getAllByRole('button');
    expect(stars[1]).toHaveClass('star-hovered');
  });

  it('should select rating with Enter key', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: null
      }
    });

    const starRating = screen.getByRole('radiogroup');
    
    // Focus and navigate to third star
    await user.click(starRating);
    await user.keyboard('[ArrowRight][ArrowRight]');
    await user.keyboard('[Enter]');

    expect(component.getValue()).toBe(3);
  });

  it('should select rating with Space key', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: null
      }
    });

    const starRating = screen.getByRole('radiogroup');
    
    // Focus and navigate to second star
    await user.click(starRating);
    await user.keyboard('[ArrowRight]');
    await user.keyboard('[Space]');

    expect(component.getValue()).toBe(2);
  });

  it('should clear rating with 0 key when clearable', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: 3,
        clearable: true
      }
    });

    const starRating = screen.getByRole('radiogroup');
    await user.click(starRating);
    await user.keyboard('0');

    expect(component.getValue()).toBe(null);
  });

  it('should set rating with number keys', async () => {
    const user = userEvent.setup();
    
    const { component } = render(StarRatingTestWrapper, {
      props: {
        initialValue: null
      }
    });

    const starRating = screen.getByRole('radiogroup');
    await user.click(starRating);
    await user.keyboard('4');

    expect(component.getValue()).toBe(4);
  });

  it('should apply size classes correctly', () => {
    // Test small size
    const { unmount: unmountSm } = render(StarRating, {
      props: {
        size: 'sm'
      }
    });

    let stars = screen.getAllByRole('button');
    expect(stars[0]).toHaveClass('w-4', 'h-4');
    
    unmountSm();

    // Test large size separately
    render(StarRating, {
      props: {
        size: 'lg'
      }
    });

    stars = screen.getAllByRole('button');
    expect(stars[0]).toHaveClass('w-6', 'h-6');
  });

  it('should apply custom class names', () => {
    render(StarRating, {
      props: {
        class: 'custom-class'
      }
    });

    const container = screen.getByRole('radiogroup').parentElement;
    expect(container).toHaveClass('custom-class');
  });

  it('should handle hover events for interactive mode', async () => {
    const user = userEvent.setup();
    
    render(StarRating);

    const stars = screen.getAllByRole('button');
    
    // Hover over third star
    await user.hover(stars[2]!);
    
    // First 3 stars should show hovered state
    expect(stars[0]).toHaveClass('star-hovered');
    expect(stars[1]).toHaveClass('star-hovered');
    expect(stars[2]).toHaveClass('star-hovered');
    
    // Last 2 stars should not
    expect(stars[3]).not.toHaveClass('star-hovered');
    expect(stars[4]).not.toHaveClass('star-hovered');
  });

  it('should not show hover effects in readonly mode', async () => {
    const user = userEvent.setup();
    
    render(StarRating, {
      props: {
        readonly: true
      }
    });

    const stars = screen.getAllByRole('button');
    await user.hover(stars[2]!);
    
    // No stars should show hovered state in readonly mode
    stars.forEach(star => {
      expect(star).not.toHaveClass('star-hovered');
    });
  });

  it('should handle mouse leave to clear hover state', async () => {
    const user = userEvent.setup();
    
    render(StarRating);

    const starRating = screen.getByRole('radiogroup');
    const stars = screen.getAllByRole('button');
    
    // Hover over a star
    await user.hover(stars[2]!);
    expect(stars[2]!).toHaveClass('star-hovered');
    
    // Mouse leave should clear hover
    await user.unhover(starRating);
    expect(stars[2]!).not.toHaveClass('star-hovered');
  });

  it('should show correct aria-pressed values', () => {
    render(StarRating, {
      props: {
        value: 3
      }
    });

    const stars = screen.getAllByRole('button');
    
    // Third star should have aria-pressed="true"
    expect(stars[2]).toHaveAttribute('aria-pressed', 'true');
    
    // Other stars should have aria-pressed="false"
    expect(stars[0]).toHaveAttribute('aria-pressed', 'false');
    expect(stars[1]).toHaveAttribute('aria-pressed', 'false');
    expect(stars[3]).toHaveAttribute('aria-pressed', 'false');
    expect(stars[4]).toHaveAttribute('aria-pressed', 'false');
  });

  it('should have correct star labels for accessibility', () => {
    render(StarRating);

    const stars = screen.getAllByRole('button');
    
    expect(stars[0]).toHaveAttribute('aria-label', '1 star');
    expect(stars[1]).toHaveAttribute('aria-label', '2 stars');
    expect(stars[2]).toHaveAttribute('aria-label', '3 stars');
    expect(stars[3]).toHaveAttribute('aria-label', '4 stars');
    expect(stars[4]).toHaveAttribute('aria-label', '5 stars');
  });

  it('should handle keyboard navigation boundaries correctly', async () => {
    const user = userEvent.setup();
    
    render(StarRating);

    const starRating = screen.getByRole('radiogroup');
    await user.click(starRating);
    
    // Navigate to first star
    await user.keyboard('[ArrowLeft]');
    
    // Try to go past first star - should stay at first
    await user.keyboard('[ArrowLeft]');
    
    const stars = screen.getAllByRole('button');
    expect(stars[0]).toHaveClass('star-hovered');
    
    // Navigate to last star
    await user.keyboard('[ArrowRight][ArrowRight][ArrowRight][ArrowRight]');
    
    // Try to go past last star - should stay at last
    await user.keyboard('[ArrowRight]');
    
    expect(stars[4]).toHaveClass('star-hovered');
  });

  it('should reset hover state on Escape key', async () => {
    const user = userEvent.setup();
    
    render(StarRating);

    const starRating = screen.getByRole('radiogroup');
    await user.click(starRating);
    
    // Navigate to a star
    await user.keyboard('[ArrowRight][ArrowRight]');
    
    const stars = screen.getAllByRole('button');
    expect(stars[2]).toHaveClass('star-hovered');
    
    // Press escape
    await user.keyboard('[Escape]');
    
    // Hover state should be cleared
    expect(stars[2]!).not.toHaveClass('star-hovered');
  });
});