<script lang="ts">
  import { onMount, onDestroy, tick, createEventDispatcher } from 'svelte';
  import { browser } from '$app/environment';
  
  export let target: HTMLElement | string = 'body';
  export let onPortalReady: (() => void) | null = null;
  
  const dispatch = createEventDispatcher<{ ready: void }>();
  
  let portalTarget: HTMLElement | null = null;
  let portalContainer: HTMLElement | null = null;
  let contentElement: HTMLElement | null = null;
  let isReady = false;
  
  onMount(async () => {
    if (!browser) return;
    
    // Get the target element
    portalTarget = typeof target === 'string' 
      ? document.querySelector(target) as HTMLElement
      : target;
      
    if (!portalTarget) {
      console.error('Portal target not found:', target);
      return;
    }
    
    // Create portal container
    portalContainer = document.createElement('div');
    portalContainer.className = 'portal-container';
    portalTarget.appendChild(portalContainer);
    
    await tick(); // Wait for DOM updates
    isReady = true;
    
    // Notify that portal is ready
    if (onPortalReady) {
      onPortalReady();
    }
    dispatch('ready');
  });
  
  onDestroy(() => {
    if (portalContainer && portalContainer.parentNode) {
      portalContainer.parentNode.removeChild(portalContainer);
    }
  });
  
  // Move content to portal when ready
  $: if (isReady && portalContainer && contentElement) {
    portalContainer.appendChild(contentElement);
    contentElement.style.display = '';
  }
</script>

{#if browser}
  <!-- Content that will be moved to portal -->
  <div bind:this={contentElement} style="display: none;">
    <slot />
  </div>
{/if}