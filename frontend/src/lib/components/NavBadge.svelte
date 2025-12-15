<script lang="ts">
  /**
   * NavBadge - Displays a count badge for navigation items.
   *
   * Used to show pending review counts on Import/Export and Sync nav items.
   * Badge only appears when count > 0.
   * Clicking the badge navigates to a filtered review page.
   */
  import { goto } from '$app/navigation';

  interface Props {
    /** Number to display in the badge */
    count: number;
    /** URL to navigate to when badge is clicked */
    href?: string;
    /** Accessible label for the badge */
    label?: string;
    /** Additional CSS classes */
    class?: string;
  }

  let {
    count,
    href,
    label,
    class: className = ''
  }: Props = $props();

  // Compute default label reactively if not provided
  const computedLabel = $derived(label ?? `${count} pending`);

  function handleClick(event: MouseEvent) {
    if (href) {
      event.preventDefault();
      event.stopPropagation();
      goto(href);
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if ((event.key === 'Enter' || event.key === ' ') && href) {
      event.preventDefault();
      event.stopPropagation();
      goto(href);
    }
  }
</script>

{#if count > 0}
  {#if href}
    <button
      type="button"
      class="
        inline-flex items-center justify-center
        min-w-5 h-5 px-1.5
        text-xs font-semibold
        bg-primary-500 text-white
        rounded-full
        hover:bg-primary-600 focus:outline-none focus:ring-2 focus:ring-primary-400 focus:ring-offset-1 focus:ring-offset-gray-700
        transition-colors duration-150
        {className}
      "
      aria-label={computedLabel}
      onclick={handleClick}
      onkeydown={handleKeydown}
      data-testid="nav-badge"
    >
      {count}
    </button>
  {:else}
    <span
      class="
        inline-flex items-center justify-center
        min-w-5 h-5 px-1.5
        text-xs font-semibold
        bg-primary-500 text-white
        rounded-full
        {className}
      "
      aria-label={computedLabel}
      data-testid="nav-badge"
    >
      {count}
    </span>
  {/if}
{/if}
