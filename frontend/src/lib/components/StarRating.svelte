<script lang="ts">
  interface Props {
    value?: number | null | undefined;
    readonly?: boolean;
    size?: 'sm' | 'md' | 'lg';
    id?: string;
    class?: string;
    disabled?: boolean;
    clearable?: boolean;
    showLabel?: boolean;
    onchange?: (event: CustomEvent<{ value: number | null }>) => void;
  }

  let {
    value = $bindable(null),
    readonly = false,
    size = 'md',
    id,
    class: className = '',
    disabled = false,
    clearable = true,
    showLabel = false,
    onchange
  }: Props = $props();

  let hoveredStar = $state<number | null>(null);
  let isFocused = $state(false);
  let focusedStarIndex = $state<number | null>(null);

  // Derived values
  const isInteractive = $derived(!readonly && !disabled);
  const currentRating = $derived(hoveredStar !== null ? hoveredStar : value);
  const stars = $derived(Array.from({ length: 5 }, (_, i) => i + 1));

  // Size-dependent classes
  const sizeClasses = $derived({
    sm: {
      star: 'w-4 h-4',
      container: 'gap-0.5',
      text: 'text-xs'
    },
    md: {
      star: 'w-5 h-5',
      container: 'gap-1',
      text: 'text-sm'
    },
    lg: {
      star: 'w-6 h-6',
      container: 'gap-1.5',
      text: 'text-base'
    }
  }[size]);

  function handleStarClick(starValue: number) {
    if (!isInteractive) return;

    // If clearable and clicking on the same star, clear the rating
    if (clearable && value === starValue) {
      const newValue = null;
      value = newValue;
      onchange?.(new CustomEvent('change', { detail: { value: newValue } }));
      return;
    }

    value = starValue;
    onchange?.(new CustomEvent('change', { detail: { value: starValue } }));
  }

  function handleStarHover(starValue: number) {
    if (!isInteractive) return;
    hoveredStar = starValue;
  }

  function handleMouseLeave() {
    if (!isInteractive) return;
    hoveredStar = null;
  }

  function handleKeyDown(event: KeyboardEvent) {
    if (!isInteractive) return;

    const currentIndex = focusedStarIndex ?? (value ? value - 1 : 0);
    
    switch (event.key) {
      case 'ArrowLeft':
      case 'ArrowDown':
        event.preventDefault();
        focusedStarIndex = Math.max(0, currentIndex - 1);
        hoveredStar = focusedStarIndex + 1;
        break;
      case 'ArrowRight':
      case 'ArrowUp':
        event.preventDefault();
        focusedStarIndex = Math.min(4, currentIndex + 1);
        hoveredStar = focusedStarIndex + 1;
        break;
      case 'Enter':
      case ' ':
        event.preventDefault();
        if (focusedStarIndex !== null) {
          handleStarClick(focusedStarIndex + 1);
        }
        break;
      case 'Escape':
        event.preventDefault();
        hoveredStar = null;
        focusedStarIndex = null;
        break;
      case '0':
        event.preventDefault();
        if (clearable) {
          value = null;
          onchange?.(new CustomEvent('change', { detail: { value: null } }));
        }
        break;
      case '1':
      case '2':
      case '3':
      case '4':
      case '5':
        event.preventDefault();
        const rating = parseInt(event.key);
        value = rating;
        onchange?.(new CustomEvent('change', { detail: { value: rating } }));
        break;
    }
  }

  function handleFocus() {
    if (!isInteractive) return;
    isFocused = true;
    if (focusedStarIndex === null) {
      focusedStarIndex = value ? value - 1 : 0;
      hoveredStar = focusedStarIndex + 1;
    }
  }

  function handleBlur() {
    if (!isInteractive) return;
    isFocused = false;
    hoveredStar = null;
    focusedStarIndex = null;
  }

  // Generate accessible label
  function getAriaLabel() {
    if (readonly) {
      return value ? `Rated ${value} out of 5 stars` : 'Not rated';
    }
    return `Rate from 1 to 5 stars. Current rating: ${value || 'none'}. Use arrow keys to navigate, Enter to select, 0 to clear.`;
  }

  function getStarAriaLabel(starValue: number) {
    if (readonly) return undefined;
    return `${starValue} star${starValue > 1 ? 's' : ''}`;
  }
</script>

<div 
  class="star-rating-container {sizeClasses.container} {className}"
  class:star-rating-readonly={readonly}
  class:star-rating-disabled={disabled}
  class:star-rating-focused={isFocused}
>
  {#if isInteractive}
    <!-- Interactive rating input -->
    <div
      class="star-rating flex items-center {sizeClasses.container}"
      role="radiogroup"
      aria-label={getAriaLabel()}
      tabindex="0"
      onkeydown={handleKeyDown}
      onfocus={handleFocus}
      onblur={handleBlur}
      onmouseleave={handleMouseLeave}
      {id}
    >
      {#each stars as star}
        {@const isFilled = currentRating !== null && star <= currentRating}
        {@const isHovered = hoveredStar !== null && star <= hoveredStar}
        {@const isFocusedStar = focusedStarIndex === star - 1}
        
        <button
          type="button"
          class="star-button {sizeClasses.star} flex-shrink-0 transition-all duration-150 ease-in-out"
          class:star-filled={isFilled}
          class:star-empty={!isFilled}
          class:star-interactive={isInteractive}
          class:star-hovered={isHovered}
          class:star-focused={isFocusedStar && isFocused}
          aria-label={getStarAriaLabel(star)}
          aria-pressed={readonly ? undefined : (value === star)}
          tabindex="-1"
          disabled={!isInteractive}
          onclick={() => handleStarClick(star)}
          onmouseenter={() => handleStarHover(star)}
        >
          <svg
            class="w-full h-full"
            fill="currentColor"
            viewBox="0 0 24 24"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"
            />
          </svg>
        </button>
      {/each}
    </div>
  {:else}
    <!-- Readonly rating display -->
    <div
      class="star-rating flex items-center {sizeClasses.container}"
      role="img"
      aria-label={getAriaLabel()}
      {id}
    >
      {#each stars as star}
        {@const isFilled = currentRating !== null && star <= currentRating}
        {@const isHovered = hoveredStar !== null && star <= hoveredStar}
        {@const isFocusedStar = focusedStarIndex === star - 1}
        
        <button
          type="button"
          class="star-button {sizeClasses.star} flex-shrink-0 transition-all duration-150 ease-in-out"
          class:star-filled={isFilled}
          class:star-empty={!isFilled}
          class:star-interactive={isInteractive}
          class:star-hovered={isHovered}
          class:star-focused={isFocusedStar && isFocused}
          aria-label={getStarAriaLabel(star)}
          aria-pressed={readonly ? undefined : (value === star)}
          tabindex="-1"
          disabled={!isInteractive}
          onclick={() => handleStarClick(star)}
          onmouseenter={() => handleStarHover(star)}
        >
          <svg
            class="w-full h-full"
            fill="currentColor"
            viewBox="0 0 24 24"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"
            />
          </svg>
        </button>
      {/each}
    </div>
  {/if}
  
  {#if showLabel && (value !== null || readonly)}
    <span class="star-rating-label {sizeClasses.text} text-gray-600 ml-2">
      {#if value !== null}
        ({value}/5)
      {:else}
        Not rated
      {/if}
    </span>
  {/if}
</div>

<style>
  .star-rating-container {
    display: inline-flex;
    align-items: center;
  }

  .star-rating {
    outline: none;
  }

  .star-rating:focus-visible {
    outline: 2px solid theme('colors.blue.500');
    outline-offset: 2px;
    border-radius: 0.25rem;
  }

  .star-button {
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    outline: none;
  }

  .star-button:disabled {
    cursor: default;
  }

  /* Star states */
  .star-empty {
    color: theme('colors.gray.300');
  }

  .star-filled {
    color: theme('colors.yellow.400');
  }

  .star-interactive.star-empty:hover,
  .star-interactive.star-hovered {
    color: theme('colors.yellow.300');
  }

  .star-interactive.star-filled:hover {
    color: theme('colors.yellow.500');
  }

  .star-focused {
    color: theme('colors.yellow.500') !important;
    filter: drop-shadow(0 0 4px theme('colors.yellow.300'));
  }

  /* Readonly state */
  .star-rating-readonly .star-button {
    cursor: default;
  }

  /* Disabled state */
  .star-rating-disabled .star-empty {
    color: theme('colors.gray.200');
  }

  .star-rating-disabled .star-filled {
    color: theme('colors.gray.400');
  }

  /* Focus states */
  .star-rating-focused {
    outline: 2px solid theme('colors.blue.300');
    outline-offset: 2px;
    border-radius: 0.25rem;
  }

  /* Smooth transitions */
  .star-button {
    transition: color 0.15s ease-in-out, filter 0.15s ease-in-out;
  }
</style>