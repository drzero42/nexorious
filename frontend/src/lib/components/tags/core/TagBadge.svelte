<script lang="ts">
  import { goto } from '$app/navigation';
  import type { Tag } from '$lib/stores';

  interface Props {
    tag: Tag;
    clickable?: boolean;
    removable?: boolean;
    size?: 'sm' | 'md' | 'lg';
    showCount?: boolean;
    class?: string;
    onclick?: (tag: Tag) => void;
    onremove?: (tag: Tag) => void;
  }

  let {
    tag,
    clickable = false,
    removable = false,
    size = 'md',
    showCount = false,
    class: className = '',
    onclick,
    onremove
  }: Props = $props();

  // Calculate text color based on background color luminance
  const getTextColor = (hexColor: string): string => {
    // Convert hex to RGB
    const hex = hexColor.replace('#', '');
    const r = parseInt(hex.substr(0, 2), 16);
    const g = parseInt(hex.substr(2, 2), 16);
    const b = parseInt(hex.substr(4, 2), 16);
    
    // Calculate luminance
    const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
    
    // Return white text for dark backgrounds, black for light backgrounds
    return luminance > 0.5 ? '#000000' : '#FFFFFF';
  };

  // Size classes
  const sizeClasses = {
    sm: 'px-2 py-0.5 text-xs',
    md: 'px-2.5 py-1 text-sm',
    lg: 'px-3 py-1.5 text-base'
  };

  const handleClick = (event: MouseEvent | KeyboardEvent) => {
    if (clickable && onclick) {
      event.preventDefault();
      onclick(tag);
    } else if (clickable) {
      // Default behavior: navigate to games page with tag filter
      event.preventDefault();
      goto(`/games?tag=${tag.id}`);
    }
  };

  const handleRemove = (event: MouseEvent | KeyboardEvent) => {
    event.stopPropagation();
    if (onremove) {
      onremove(tag);
    }
  };

  // Generate accessible label
  const ariaLabel = $derived(() => {
    let label = `Tag: ${tag.name}`;
    if (showCount && tag.game_count !== undefined) {
      label += `, ${tag.game_count} game${tag.game_count !== 1 ? 's' : ''}`;
    }
    if (clickable) {
      label += ', click to filter games';
    }
    if (removable) {
      label += ', removable';
    }
    return label;
  });
</script>

<!-- 
TagBadge Component
Displays a tag as a colored badge with optional click and remove functionality.

Props:
- tag: Tag object to display
- clickable: Whether the badge can be clicked (default: false)
- removable: Whether the badge shows a remove button (default: false) 
- size: Size variant - 'sm' | 'md' | 'lg' (default: 'md')
- showCount: Whether to show the game count (default: false)
- onclick: Custom click handler (optional)
- onremove: Remove handler (optional)
-->

{#if clickable && removable}
  <!-- Special case: both clickable and removable -->
  <div
    class="
      inline-flex items-center gap-1 rounded-full font-medium transition-all duration-200
      {sizeClasses[size]}
      {className}
    "
    style="background-color: {tag.color}; color: {getTextColor(tag.color)};"
  >
    <!-- Clickable tag content -->
    <button
      type="button"
      class="flex items-center gap-1 cursor-pointer hover:scale-105 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 rounded-full"
      aria-label={ariaLabel()}
      onclick={handleClick}
    >
      <!-- Tag name -->
      <span class="truncate max-w-32">{tag.name}</span>
      
      <!-- Game count -->
      {#if showCount && tag.game_count !== undefined}
        <span class="opacity-75 text-xs">
          ({tag.game_count})
        </span>
      {/if}
    </button>
    
    <!-- Remove button -->
    <button
      type="button"
      class="
        ml-1 inline-flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full
        hover:bg-white/20 focus:outline-none focus:ring-1 focus:ring-white/50
        transition-colors duration-200
      "
      aria-label="Remove tag {tag.name}"
      onclick={handleRemove}
      onkeydown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.stopPropagation();
          event.preventDefault();
          handleRemove(event);
        }
      }}
    >
      <svg class="h-2.5 w-2.5" fill="currentColor" viewBox="0 0 20 20">
        <path
          fill-rule="evenodd"
          d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
          clip-rule="evenodd"
        />
      </svg>
    </button>
  </div>
{:else if clickable}
  <!-- Only clickable -->
  <button
    type="button"
    class="
      inline-flex items-center gap-1 rounded-full font-medium transition-all duration-200
      {sizeClasses[size]}
      cursor-pointer hover:scale-105 hover:shadow-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500
      {className}
    "
    style="background-color: {tag.color}; color: {getTextColor(tag.color)};"
    aria-label={ariaLabel()}
    onclick={handleClick}
  >
    <!-- Tag name -->
    <span class="truncate max-w-32">{tag.name}</span>
    
    <!-- Game count -->
    {#if showCount && tag.game_count !== undefined}
      <span class="opacity-75 text-xs">
        ({tag.game_count})
      </span>
    {/if}
  </button>
{:else}
  <!-- Not clickable (might still be removable) -->
  <span
    class="
      inline-flex items-center gap-1 rounded-full font-medium transition-all duration-200
      {sizeClasses[size]}
      {className}
    "
    style="background-color: {tag.color}; color: {getTextColor(tag.color)};"
  >
    <!-- Tag name -->
    <span class="truncate max-w-32">{tag.name}</span>
    
    <!-- Game count -->
    {#if showCount && tag.game_count !== undefined}
      <span class="opacity-75 text-xs">
        ({tag.game_count})
      </span>
    {/if}
    
    <!-- Remove button -->
    {#if removable}
      <button
        type="button"
        class="
          ml-1 inline-flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full
          hover:bg-white/20 focus:outline-none focus:ring-1 focus:ring-white/50
          transition-colors duration-200
        "
        aria-label="Remove tag {tag.name}"
        onclick={handleRemove}
        onkeydown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.stopPropagation();
            event.preventDefault();
            handleRemove(event);
          }
        }}
      >
        <svg class="h-2.5 w-2.5" fill="currentColor" viewBox="0 0 20 20">
          <path
            fill-rule="evenodd"
            d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
            clip-rule="evenodd"
          />
        </svg>
      </button>
    {/if}
  </span>
{/if}

<style>
  /* Ensure consistent styling across browsers */
  button {
    user-select: none;
  }
</style>