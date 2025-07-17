<script lang="ts">
  import { ui } from '$lib/stores';
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  
  let showDropdown = false;
  
  function toggleTheme() {
    ui.toggleTheme();
  }
  
  function setTheme(theme: 'light' | 'dark' | 'system') {
    ui.setTheme(theme);
    showDropdown = false;
  }
  
  function getThemeIcon(theme: string) {
    switch (theme) {
      case 'light':
        return 'M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z';
      case 'dark':
        return 'M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z';
      case 'system':
        return 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.031 9-11.622 0-1.042-.133-2.052-.382-3.016z';
      default:
        return 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.031 9-11.622 0-1.042-.133-2.052-.382-3.016z';
    }
  }
  
  function getThemeLabel(theme: string) {
    switch (theme) {
      case 'light':
        return 'Light';
      case 'dark':
        return 'Dark';
      case 'system':
        return 'System';
      default:
        return 'System';
    }
  }
  
  function handleClickOutside(event: MouseEvent) {
    if (!browser) return;
    
    const target = event.target as HTMLElement;
    const dropdown = document.getElementById('theme-dropdown');
    const button = document.getElementById('theme-toggle-button');
    
    if (dropdown && button && !dropdown.contains(target) && !button.contains(target)) {
      showDropdown = false;
    }
  }
  
  onMount(() => {
    if (browser) {
      const cleanup = () => {
        document.removeEventListener('click', handleClickOutside);
      };
      
      return cleanup;
    }
  });
  
  $: if (browser && showDropdown) {
    document.addEventListener('click', handleClickOutside);
  } else if (browser) {
    document.removeEventListener('click', handleClickOutside);
  }
</script>

<div class="relative">
  <!-- Theme Toggle Button -->
  <button
    id="theme-toggle-button"
    on:click={() => showDropdown = !showDropdown}
    class="inline-flex items-center justify-center font-medium transition-all duration-200 outline-none focus:ring-2 focus:ring-blue-500/50 px-2 py-2 text-sm rounded-full text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
    aria-label="Toggle theme"
    aria-expanded={showDropdown}
    aria-haspopup="true"
  >
    <svg 
      class="w-5 h-5 text-gray-600 dark:text-gray-300" 
      fill="none" 
      stroke="currentColor" 
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <path 
        stroke-linecap="round" 
        stroke-linejoin="round" 
        stroke-width="2" 
        d={getThemeIcon(ui.value.theme)}
      />
    </svg>
  </button>
  
  <!-- Theme Dropdown -->
  {#if showDropdown}
    <div 
      id="theme-dropdown"
      class="absolute right-0 mt-2 w-36 bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 py-1 z-50 animate-in fade-in zoom-in-95 duration-150"
      role="menu"
      aria-orientation="vertical"
      aria-labelledby="theme-toggle-button"
    >
      <button
        on:click={() => setTheme('light')}
        class="flex items-center w-full px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors {ui.value.theme === 'light' ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-300' : ''}"
        role="menuitem"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getThemeIcon('light')} />
        </svg>
        Light
      </button>
      
      <button
        on:click={() => setTheme('dark')}
        class="flex items-center w-full px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors {ui.value.theme === 'dark' ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-300' : ''}"
        role="menuitem"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getThemeIcon('dark')} />
        </svg>
        Dark
      </button>
      
      <button
        on:click={() => setTheme('system')}
        class="flex items-center w-full px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors {ui.value.theme === 'system' ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-300' : ''}"
        role="menuitem"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={getThemeIcon('system')} />
        </svg>
        System
      </button>
    </div>
  {/if}
</div>

