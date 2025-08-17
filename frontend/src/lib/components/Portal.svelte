<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { browser } from '$app/environment';
  
  interface Props {
    target?: HTMLElement | string;
    onPortalReady?: (() => void) | null;
    onready?: () => void;
    children?: import('svelte').Snippet;
  }
  
  let { target = 'body', onPortalReady = null, onready, children }: Props = $props();
  
  let portalTarget: HTMLElement | null = null;
  let portalContainer: HTMLElement | null = null;
  let contentElement = $state<HTMLElement | null>(null);
  let isReady = $state(false);
  
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
    onready?.();
  });
  
  onDestroy(() => {
    if (portalContainer && portalContainer.parentNode) {
      portalContainer.parentNode.removeChild(portalContainer);
    }
  });
  
  // Move content to portal when ready
  $effect(() => {
    if (isReady && portalContainer && contentElement) {
      portalContainer.appendChild(contentElement);
      contentElement.style.display = '';
    }
  });
</script>

{#if browser}
  <!-- Content that will be moved to portal -->
  <div bind:this={contentElement} style="display: none;">
    {@render children?.()}
  </div>
{/if}