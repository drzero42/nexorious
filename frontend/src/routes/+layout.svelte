<script lang="ts">
  import '../app.css';
  import { auth } from '$lib/stores';
  import { onMount } from 'svelte';
  import { initializePWA, initializeInstallPrompt } from '$lib/pwa';
  import PWAInstallButton from '$lib/components/PWAInstallButton.svelte';
  import PWAUpdateNotification from '$lib/components/PWAUpdateNotification.svelte';
  import OfflineIndicator from '$lib/components/OfflineIndicator.svelte';
  
  let mobileMenuOpen = false;
  
  onMount(() => {
    // Check if user is authenticated and refresh token if needed
    const authState = auth.value;
    if (authState.accessToken && authState.refreshToken) {
      // Optionally refresh token on app start
      auth.refreshAuth();
    }
    
    // Initialize PWA functionality
    initializePWA();
    initializeInstallPrompt();
  });
</script>

<svelte:head>
  <title>Nexorious Game Collection</title>
  <meta name="description" content="Self-hostable game collection management" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
</svelte:head>

<div class="min-h-screen bg-gray-50 dark:bg-gray-900">
  <header class="bg-white dark:bg-gray-800 shadow-sm">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex justify-between items-center h-16">
        <div class="flex items-center">
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
            Nexorious
          </h1>
        </div>
        
        <nav class="hidden md:flex items-center space-x-6">
          {#if auth.value.user}
            <a
              href="/games"
              class="text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors"
            >
              My Games
            </a>
            <a
              href="/wishlist"
              class="text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors"
            >
              Wishlist
            </a>
            <a
              href="/dashboard"
              class="text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors"
            >
              Dashboard
            </a>
          {/if}
        </nav>

        <div class="flex items-center space-x-4">
          <PWAInstallButton />
          {#if auth.value.user}
            <div class="hidden md:flex items-center space-x-4">
              <span class="text-sm text-gray-700 dark:text-gray-300">
                Welcome, {auth.value.user.username}
              </span>
              <button
                on:click={() => auth.logout()}
                class="text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors"
              >
                Logout
              </button>
            </div>
            <!-- Mobile menu button -->
            <button
              class="md:hidden p-2 rounded-md text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
              on:click={() => mobileMenuOpen = !mobileMenuOpen}
            >
              <svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                {#if mobileMenuOpen}
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                {:else}
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
                {/if}
              </svg>
            </button>
          {:else}
            <a
              href="/login"
              class="text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors"
            >
              Login
            </a>
          {/if}
        </div>
      </div>
    </div>
  </header>

  <!-- Mobile menu -->
  {#if mobileMenuOpen && auth.value.user}
    <div class="md:hidden bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700">
      <div class="px-2 pt-2 pb-3 space-y-1">
        <a
          href="/games"
          class="block px-3 py-2 rounded-md text-base font-medium text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-gray-700"
          on:click={() => mobileMenuOpen = false}
        >
          My Games
        </a>
        <a
          href="/wishlist"
          class="block px-3 py-2 rounded-md text-base font-medium text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-gray-700"
          on:click={() => mobileMenuOpen = false}
        >
          Wishlist
        </a>
        <a
          href="/dashboard"
          class="block px-3 py-2 rounded-md text-base font-medium text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-gray-700"
          on:click={() => mobileMenuOpen = false}
        >
          Dashboard
        </a>
      </div>
      <div class="pt-4 pb-3 border-t border-gray-200 dark:border-gray-700">
        <div class="flex items-center px-5">
          <div class="flex-shrink-0">
            <div class="h-10 w-10 rounded-full bg-gray-300 dark:bg-gray-600 flex items-center justify-center">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {auth.value.user.username?.charAt(0).toUpperCase()}
              </span>
            </div>
          </div>
          <div class="ml-3">
            <div class="text-base font-medium text-gray-800 dark:text-white">
              {auth.value.user.username}
            </div>
          </div>
        </div>
        <div class="mt-3 px-2 space-y-1">
          <button
            on:click={() => { auth.logout(); mobileMenuOpen = false; }}
            class="block w-full text-left px-3 py-2 rounded-md text-base font-medium text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-50 dark:hover:bg-gray-700"
          >
            Sign out
          </button>
        </div>
      </div>
    </div>
  {/if}

  <main class="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
    <slot />
  </main>
</div>

<!-- PWA Components -->
<OfflineIndicator />
<PWAUpdateNotification />

<style>
  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen,
      Ubuntu, Cantarell, sans-serif;
  }
</style>