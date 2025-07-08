<script lang="ts">
  import '../app.css';
  import { auth } from '$lib/stores';
  import { onMount } from 'svelte';
  
  onMount(() => {
    // Check if user is authenticated and refresh token if needed
    const authState = auth.value;
    if (authState.accessToken && authState.refreshToken) {
      // Optionally refresh token on app start
      auth.refreshAuth();
    }
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
        
        <nav class="flex items-center space-x-4">
          {#if auth.value.user}
            <span class="text-sm text-gray-700 dark:text-gray-300">
              Welcome, {auth.value.user.username}
            </span>
            <button
              on:click={() => auth.logout()}
              class="text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
            >
              Logout
            </button>
          {:else}
            <a
              href="/login"
              class="text-sm text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
            >
              Login
            </a>
          {/if}
        </nav>
      </div>
    </div>
  </header>

  <main class="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
    <slot />
  </main>
</div>

<style>
  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen,
      Ubuntu, Cantarell, sans-serif;
  }
</style>