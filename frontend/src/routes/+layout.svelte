<script lang="ts">
  import '../app.css';
  import { auth, ui } from '$lib/stores';
  import { onMount } from 'svelte';
  import { ThemeToggle } from '$lib/components';
  
  let mobileMenuOpen = false;
  
  onMount(() => {
    // Check if user is authenticated and refresh token if needed
    const authState = auth.value;
    if (authState.accessToken && authState.refreshToken) {
      // Optionally refresh token on app start
      auth.refreshAuth();
    }
    
    
    // Initialize system theme listener
    ui.initSystemThemeListener();
  });
  
  function closeMobileMenu() {
    mobileMenuOpen = false;
  }
</script>

<svelte:head>
  <title>Nexorious Game Collection</title>
  <meta name="description" content="Self-hostable game collection management" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
</svelte:head>

<div class="min-h-screen bg-gray-50 dark:bg-slate-900 transition-theme">
  <header class="bg-white dark:bg-slate-800 shadow-sm border-b border-gray-200 dark:border-gray-700 transition-theme">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex justify-between items-center h-16">
        <!-- Logo/Brand -->
        <div class="flex items-center">
          <a href="/" class="flex items-center space-x-2 group">
            <div class="w-8 h-8 bg-gradient-gaming rounded-lg flex items-center justify-center">
              <svg class="w-5 h-5 text-white" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <h1 class="text-xl font-bold text-gradient group-hover:scale-105 transition-transform">
              Nexorious
            </h1>
          </a>
        </div>
        
        <!-- Desktop Navigation -->
        <nav class="hidden md:flex items-center space-x-1">
          {#if auth.value.user}
            <a
              href="/games"
              class="px-3 py-2 rounded-lg text-sm font-medium text-gray-700 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-all duration-200"
            >
              My Games
            </a>
            <a
              href="/wishlist"
              class="px-3 py-2 rounded-lg text-sm font-medium text-gray-700 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-all duration-200"
            >
              Wishlist
            </a>
            <a
              href="/dashboard"
              class="px-3 py-2 rounded-lg text-sm font-medium text-gray-700 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-all duration-200"
            >
              Dashboard
            </a>
          {/if}
        </nav>

        <!-- Right Side Actions -->
        <div class="flex items-center space-x-2">
          <ThemeToggle />
          
          {#if auth.value.user}
            <div class="hidden md:flex items-center space-x-3">
              <div class="flex items-center space-x-2">
                <div class="w-7 h-7 bg-gradient-gaming rounded-full flex items-center justify-center">
                  <span class="text-sm font-medium text-white">
                    {auth.value.user.username?.charAt(0).toUpperCase()}
                  </span>
                </div>
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {auth.value.user.username}
                </span>
              </div>
              <button
                on:click={() => auth.logout()}
                class="btn btn-ghost btn-sm text-gray-600 dark:text-gray-400 hover:text-red-600 dark:hover:text-red-400"
              >
                <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                </svg>
                Logout
              </button>
            </div>
            
            <!-- Mobile menu button -->
            <button
              class="md:hidden p-2 rounded-lg text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-700 transition-all duration-200"
              on:click={() => mobileMenuOpen = !mobileMenuOpen}
              aria-label="Toggle mobile menu"
            >
              <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                {#if mobileMenuOpen}
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                {:else}
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
                {/if}
              </svg>
            </button>
          {:else}
            <a href="/login" class="btn btn-primary btn-sm">
              Login
            </a>
          {/if}
        </div>
      </div>
    </div>
  </header>

  <!-- Mobile menu -->
  {#if mobileMenuOpen && auth.value.user}
    <div class="md:hidden bg-white dark:bg-slate-800 border-b border-gray-200 dark:border-gray-700 animate-fade-in">
      <div class="px-2 pt-2 pb-3 space-y-1">
        <a
          href="/games"
          class="block px-3 py-2 rounded-lg text-base font-medium text-gray-700 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-all duration-200"
          on:click={closeMobileMenu}
        >
          <svg class="w-4 h-4 mr-2 inline-block" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
          My Games
        </a>
        <a
          href="/wishlist"
          class="block px-3 py-2 rounded-lg text-base font-medium text-gray-700 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-all duration-200"
          on:click={closeMobileMenu}
        >
          <svg class="w-4 h-4 mr-2 inline-block" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
          </svg>
          Wishlist
        </a>
        <a
          href="/dashboard"
          class="block px-3 py-2 rounded-lg text-base font-medium text-gray-700 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-all duration-200"
          on:click={closeMobileMenu}
        >
          <svg class="w-4 h-4 mr-2 inline-block" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
          </svg>
          Dashboard
        </a>
      </div>
      <div class="pt-4 pb-3 border-t border-gray-200 dark:border-gray-700">
        <div class="flex items-center px-5">
          <div class="flex-shrink-0">
            <div class="h-8 w-8 rounded-full bg-gradient-gaming flex items-center justify-center">
              <span class="text-sm font-medium text-white">
                {auth.value.user.username?.charAt(0).toUpperCase()}
              </span>
            </div>
          </div>
          <div class="ml-3">
            <div class="text-base font-medium text-gray-800 dark:text-white">
              {auth.value.user.username}
            </div>
            <div class="text-sm text-gray-500 dark:text-gray-400">
              {auth.value.user.email}
            </div>
          </div>
        </div>
        <div class="mt-3 px-2 space-y-1">
          <button
            on:click={() => { auth.logout(); closeMobileMenu(); }}
            class="flex items-center w-full px-3 py-2 rounded-lg text-base font-medium text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 transition-all duration-200"
          >
            <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
            </svg>
            Sign out
          </button>
        </div>
      </div>
    </div>
  {/if}

  <main class="max-w-7xl mx-auto py-6 px-4 sm:px-6 lg:px-8">
    <div class="animate-fade-in">
      <slot />
    </div>
  </main>
</div>


<style>
  :global(body) {
    margin: 0;
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }
</style>