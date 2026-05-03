
import * as React from 'react';
import { cn } from '@/lib/utils';
import { Star } from 'lucide-react';

export interface StarRatingProps {
  value?: number | null;
  onChange?: (value: number | null) => void;
  readonly?: boolean;
  disabled?: boolean;
  size?: 'sm' | 'md' | 'lg';
  clearable?: boolean;
  showLabel?: boolean;
  className?: string;
  id?: string;
}

const sizeConfig = {
  sm: { star: 'w-4 h-4', gap: 'gap-0.5', text: 'text-xs' },
  md: { star: 'w-5 h-5', gap: 'gap-1', text: 'text-sm' },
  lg: { star: 'w-6 h-6', gap: 'gap-1.5', text: 'text-base' },
};

export function StarRating({
  value,
  onChange,
  readonly = false,
  disabled = false,
  size = 'md',
  clearable = true,
  showLabel = false,
  className,
  id,
}: StarRatingProps) {
  const [hoveredStar, setHoveredStar] = React.useState<number | null>(null);
  const [isFocused, setIsFocused] = React.useState(false);
  const [focusedIndex, setFocusedIndex] = React.useState<number | null>(null);

  const isInteractive = !readonly && !disabled;
  const currentRating = hoveredStar !== null ? hoveredStar : (value ?? null);
  const stars = [1, 2, 3, 4, 5];
  const config = sizeConfig[size];

  const handleClick = (starValue: number) => {
    if (!isInteractive || !onChange) return;

    // If clearable and clicking on the same star, clear the rating
    if (clearable && value === starValue) {
      onChange(null);
      return;
    }

    onChange(starValue);
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (!isInteractive || !onChange) return;

    const currentIndex = focusedIndex ?? ((value ?? 1) - 1);

    switch (event.key) {
      case 'ArrowLeft':
      case 'ArrowDown':
        event.preventDefault();
        setFocusedIndex(Math.max(0, currentIndex - 1));
        setHoveredStar(Math.max(1, currentIndex));
        break;
      case 'ArrowRight':
      case 'ArrowUp':
        event.preventDefault();
        setFocusedIndex(Math.min(4, currentIndex + 1));
        setHoveredStar(Math.min(5, currentIndex + 2));
        break;
      case 'Enter':
      case ' ':
        event.preventDefault();
        if (focusedIndex !== null) {
          handleClick(focusedIndex + 1);
        }
        break;
      case 'Escape':
        event.preventDefault();
        setHoveredStar(null);
        setFocusedIndex(null);
        break;
      case '0':
        event.preventDefault();
        if (clearable) {
          onChange(null);
        }
        break;
      case '1':
      case '2':
      case '3':
      case '4':
      case '5':
        event.preventDefault();
        onChange(parseInt(event.key));
        break;
    }
  };

  const handleFocus = () => {
    if (!isInteractive) return;
    setIsFocused(true);
    if (focusedIndex === null) {
      setFocusedIndex(value ? value - 1 : 0);
      setHoveredStar(value ?? 1);
    }
  };

  const handleBlur = () => {
    setIsFocused(false);
    setHoveredStar(null);
    setFocusedIndex(null);
  };

  const getAriaLabel = () => {
    if (readonly) {
      return value ? `Rated ${value} out of 5 stars` : 'Not rated';
    }
    return `Rate from 1 to 5 stars. Current rating: ${value || 'none'}. Use arrow keys to navigate, Enter to select${clearable ? ', 0 to clear' : ''}.`;
  };

  return (
    <div className={cn('inline-flex items-center', config.gap, className)}>
      <div
        id={id}
        role={isInteractive ? 'radiogroup' : 'img'}
        aria-label={getAriaLabel()}
        tabIndex={isInteractive ? 0 : undefined}
        onKeyDown={handleKeyDown}
        onFocus={handleFocus}
        onBlur={handleBlur}
        onMouseLeave={() => isInteractive && setHoveredStar(null)}
        className={cn(
          'inline-flex items-center',
          config.gap,
          isInteractive && 'focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 rounded-sm'
        )}
      >
        {stars.map((star) => {
          const isFilled = currentRating !== null && star <= currentRating;
          const isHovered = hoveredStar !== null && star <= hoveredStar;
          const isFocusedStar = isFocused && focusedIndex === star - 1;

          return (
            <button
              key={star}
              type="button"
              disabled={!isInteractive}
              tabIndex={-1}
              onClick={() => handleClick(star)}
              onMouseEnter={() => isInteractive && setHoveredStar(star)}
              aria-label={isInteractive ? `${star} star${star > 1 ? 's' : ''}` : undefined}
              aria-pressed={!readonly ? value === star : undefined}
              className={cn(
                'transition-all duration-150 ease-in-out',
                config.star,
                isInteractive && 'cursor-pointer hover:scale-110',
                !isInteractive && 'cursor-default',
                isFocusedStar && 'ring-2 ring-yellow-400 ring-offset-1 rounded-sm'
              )}
            >
              <Star
                className={cn(
                  'w-full h-full transition-colors',
                  isFilled || isHovered
                    ? 'fill-yellow-400 text-yellow-400'
                    : 'fill-transparent text-muted-foreground/40',
                  isInteractive && !isFilled && !isHovered && 'hover:text-yellow-300',
                  disabled && 'opacity-50'
                )}
              />
            </button>
          );
        })}
      </div>

      {showLabel && (
        <span className={cn('text-muted-foreground ml-2', config.text)}>
          {value !== null ? `(${value}/5)` : 'Not rated'}
        </span>
      )}
    </div>
  );
}
