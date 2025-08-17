<script lang="ts">
  import '../app.css';
  import { auth } from '$lib/stores';
  import { platforms, type Storefront } from '$lib/stores/platforms.svelte';
  import { ToastContainer } from '$lib/components';
  import { steamAvailability } from '$lib/stores/steam-availability.svelte';
  import { buildIconUrl } from '$lib/utils/icon-utils';
  import { onMount } from 'svelte';
  
  interface Props {
    children?: import('svelte').Snippet;
  }
  
  let { children }: Props = $props();
  let mobileMenuOpen = $state(false);
  
  // Get Steam storefront icon URL
  const steamStorefront = $derived(platforms.value.storefronts.find((storefront: Storefront) => storefront.name === 'steam'));
  const steamIconUrl = $derived(steamStorefront ? buildIconUrl(steamStorefront.icon_url) : null);
  
  onMount(async () => {
    // Check if user is authenticated and refresh token if needed
    const authState = auth.value;
    
    if (authState.accessToken && authState.refreshToken) {
      // Optionally refresh token on app start
      await auth.refreshAuth();
      
      // Load platforms data for authenticated users
      // This is needed for platform management and navigation
      if (auth.value.user) {
        try {
          // Admin users need all platforms (active + inactive) for management
          // Regular users only need active platforms for efficiency
          console.log('🔄 [LAYOUT] Loading platforms data for user:', {
            userId: auth.value.user.id,
            username: auth.value.user.username,
            isAdmin: auth.value.user.isAdmin
          });
          
          if (auth.value.user.isAdmin) {
            await platforms.fetchAll();
          } else {
            await platforms.fetchActivePlatformsAndStorefronts();
          }
          
          console.log('✅ [LAYOUT] Platforms data loaded successfully');
        } catch (error) {
          console.error('❌ [LAYOUT] Failed to load platforms data:', error);
          // Don't block app loading if platforms fail to load
        }

        // Check Steam availability for sidebar navigation
        console.log('🔄 [LAYOUT] Checking Steam availability for navigation...');
        try {
          await steamAvailability.checkAvailability();
          console.log('✅ [LAYOUT] Steam availability check completed');
        } catch (error) {
          console.error('❌ [LAYOUT] Failed to check Steam availability:', error);
          // Don't block app loading if availability check fails
        }
      }
    }
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

<div class="min-h-screen bg-gray-50">
  {#if auth.value.user}
    <!-- Desktop Layout with Sidebar -->
    <div class="hidden lg:fixed lg:inset-y-0 lg:z-50 lg:flex lg:w-72 lg:flex-col">
      <!-- Sidebar -->
      <div class="flex grow flex-col gap-y-5 overflow-y-auto bg-gray-700 px-6 pb-4">
        <!-- Logo/Brand -->
        <div class="flex h-16 shrink-0 items-center border-b border-gray-600">
          <a href="/" class="flex items-center space-x-2">
            <h1 class="text-xl font-bold text-white">Nexorious</h1>
          </a>
        </div>
        
        <!-- Navigation -->
        <nav class="flex flex-1 flex-col">
          <ul role="list" class="flex flex-1 flex-col gap-y-7">
            <li>
              <ul role="list" class="-mx-2 space-y-1">
                <li>
                  <a
                    href="/dashboard"
                    class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                  >
                    <span class="text-lg">📊</span>
                    Dashboard
                  </a>
                </li>
                <li>
                  <a
                    href="/games"
                    class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                  >
                    <span class="text-lg">🎮</span>
                    My Games
                  </a>
                </li>
                <li>
                  <a
                    href="/games/add"
                    class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                  >
                    <span class="text-lg">➕</span>
                    Add Game
                  </a>
                </li>
                <!-- Import Sources Section -->
                <li>
                  <div class="text-xs font-semibold leading-6 text-gray-400 uppercase tracking-wide">
                    Import
                  </div>
                  <ul role="list" class="-mx-2 mt-2 space-y-1">
                    {#if steamAvailability.isAvailable}
                      <li>
                        <a
                          href="/import/steam"
                          class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                        >
                          {#if steamIconUrl}
                            <img 
                              src="{steamIconUrl}" 
                              alt="Steam icon" 
                              class="w-5 h-5"
                              loading="lazy"
                              onerror={(e) => {
                                const img = e.target as HTMLImageElement;
                                const fallback = img.nextElementSibling as HTMLElement;
                                if (img && fallback) {
                                  img.style.display = 'none';
                                  fallback.style.display = 'inline';
                                }
                              }}
                            />
                            <span class="text-lg hidden">🔥</span>
                          {:else}
                            <span class="text-lg">🔥</span>
                          {/if}
                          Steam Library
                        </a>
                      </li>
                    {/if}
                    
                    <!-- Future import sources will go here -->
                    <!-- 
                    <li>
                      <a
                        href="/import/epic"
                        class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                      >
                        <span class="text-lg">🎮</span>
                        Epic Games Store
                      </a>
                    </li>
                    <li>
                      <a
                        href="/import/gog"
                        class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                      >
                        <span class="text-lg">🏪</span>
                        GOG
                      </a>
                    </li>
                    -->
                  </ul>
                </li>
              </ul>
            </li>
            
            <!-- Admin Section -->
            {#if auth.value.user?.isAdmin}
              <li>
                <div class="text-xs font-semibold leading-6 text-gray-400 uppercase tracking-wide">
                  Administration
                </div>
                <ul role="list" class="-mx-2 mt-2 space-y-1">
                  <li>
                    <a
                      href="/admin/dashboard"
                      class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                    >
                      <span class="text-lg">📊</span>
                      Admin Dashboard
                    </a>
                  </li>
                  <li>
                    <a
                      href="/admin/users"
                      class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                    >
                      <span class="text-lg">👥</span>
                      Manage Users
                    </a>
                  </li>
                  <li>
                    <a
                      href="/admin/platforms"
                      class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                    >
                      <span class="text-lg">🏢</span>
                      Manage Platforms
                    </a>
                  </li>
                </ul>
              </li>
            {/if}
            <li class="mt-auto">
              <a
                href="/profile"
                class="group -mx-2 flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
              >
                <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-500">
                  <span class="text-xs font-medium text-white">
                    {auth.value.user.username?.charAt(0).toUpperCase()}
                  </span>
                </div>
                <span class="flex flex-col">
                  <span class="text-sm">{auth.value.user.username}</span>
                  <span class="text-xs text-gray-400">Profile Settings</span>
                </span>
              </a>
              <button
                onclick={() => auth.logout()}
                class="group -mx-2 flex w-full gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
              >
                <span class="text-lg">↪️</span>
                Logout
              </button>
            </li>
          </ul>
        </nav>
      </div>
    </div>

    <!-- Mobile Header -->
    <div class="lg:hidden">
      <div class="sticky top-0 z-40 flex h-16 shrink-0 items-center gap-x-4 border-b border-gray-200 bg-white px-4 shadow-sm sm:gap-x-6 sm:px-6">
        <button
          onclick={() => mobileMenuOpen = !mobileMenuOpen}
          class="-m-2.5 p-2.5 text-gray-700 lg:hidden"
          aria-label="Toggle mobile menu"
        >
          <span class="sr-only">Open sidebar</span>
          {#if mobileMenuOpen}
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          {:else}
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
            </svg>
          {/if}
        </button>
        
        <!-- Mobile Logo -->
        <div class="flex flex-1">
          <a href="/" class="flex items-center">
            <h1 class="text-lg font-bold text-gray-900">Nexorious</h1>
          </a>
        </div>

        <!-- Mobile user menu -->
        <div class="flex items-center gap-x-4 lg:gap-x-6">
          <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-500">
            <span class="text-xs font-medium text-white">
              {auth.value.user.username?.charAt(0).toUpperCase()}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- Mobile menu overlay -->
    {#if mobileMenuOpen}
      <div class="relative z-50 lg:hidden" role="dialog" aria-modal="true">
        <div class="fixed inset-0 bg-gray-900/80"></div>
        <div class="fixed inset-0 flex">
          <div class="relative mr-16 flex w-full max-w-xs flex-1">
            <div class="absolute left-full top-0 flex w-16 justify-center pt-5">
              <button
                onclick={closeMobileMenu}
                class="-m-2.5 p-2.5"
                aria-label="Close sidebar"
              >
                <span class="sr-only">Close sidebar</span>
                <svg class="h-6 w-6 text-white" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            
            <div class="flex grow flex-col gap-y-5 overflow-y-auto bg-gray-700 px-6 pb-4">
              <!-- Mobile Logo -->
              <div class="flex h-16 shrink-0 items-center border-b border-gray-600">
                <a href="/" class="flex items-center space-x-2">
                  <h1 class="text-xl font-bold text-white">Nexorious</h1>
                </a>
              </div>
              
              <!-- Mobile Navigation -->
              <nav class="flex flex-1 flex-col">
                <ul role="list" class="flex flex-1 flex-col gap-y-7">
                  <li>
                    <ul role="list" class="-mx-2 space-y-1">
                      <li>
                        <a
                          href="/dashboard"
                          onclick={closeMobileMenu}
                          class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                        >
                          <span class="text-lg">📊</span>
                          Dashboard
                        </a>
                      </li>
                      <li>
                        <a
                          href="/games"
                          onclick={closeMobileMenu}
                          class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                        >
                          <span class="text-lg">🎮</span>
                          My Games
                        </a>
                      </li>
                      <li>
                        <a
                          href="/games/add"
                          onclick={closeMobileMenu}
                          class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                        >
                          <span class="text-lg">➕</span>
                          Add Game
                        </a>
                      </li>
                      <!-- Import Sources Section -->
                      <li>
                        <div class="text-xs font-semibold leading-6 text-gray-400 uppercase tracking-wide">
                          Import
                        </div>
                        <ul role="list" class="-mx-2 mt-2 space-y-1">
                          {#if steamAvailability.isAvailable}
                            <li>
                              <a
                                href="/import/steam"
                                onclick={closeMobileMenu}
                                class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                              >
                                {#if steamIconUrl}
                                  <img 
                                    src="{steamIconUrl}" 
                                    alt="Steam icon" 
                                    class="w-5 h-5"
                                    loading="lazy"
                                    onerror={(e) => {
                                      const img = e.target as HTMLImageElement;
                                      const fallback = img.nextElementSibling as HTMLElement;
                                      if (img && fallback) {
                                        img.style.display = 'none';
                                        fallback.style.display = 'inline';
                                      }
                                    }}
                                  />
                                  <span class="text-lg hidden">🔥</span>
                                {:else}
                                  <span class="text-lg">🔥</span>
                                {/if}
                                Steam Library
                              </a>
                            </li>
                          {/if}
                          
                          <!-- Future import sources will go here -->
                        </ul>
                      </li>
                    </ul>
                  </li>
                  
                  <!-- Mobile Admin Section -->
                  {#if auth.value.user?.isAdmin}
                    <li>
                      <div class="text-xs font-semibold leading-6 text-gray-400 uppercase tracking-wide">
                        Administration
                      </div>
                      <ul role="list" class="-mx-2 mt-2 space-y-1">
                        <li>
                          <a
                            href="/admin/dashboard"
                            onclick={closeMobileMenu}
                            class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                          >
                            <span class="text-lg">📊</span>
                            Admin Dashboard
                          </a>
                        </li>
                        <li>
                          <a
                            href="/admin/users"
                            onclick={closeMobileMenu}
                            class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                          >
                            <span class="text-lg">👥</span>
                            Manage Users
                          </a>
                        </li>
                        <li>
                          <a
                            href="/admin/platforms"
                            onclick={closeMobileMenu}
                            class="group flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                          >
                            <span class="text-lg">🏢</span>
                            Manage Platforms
                          </a>
                        </li>
                      </ul>
                    </li>
                  {/if}
                  <li class="mt-auto">
                    <a
                      href="/profile"
                      onclick={closeMobileMenu}
                      class="group -mx-2 flex gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                    >
                      <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-500">
                        <span class="text-xs font-medium text-white">
                          {auth.value.user.username?.charAt(0).toUpperCase()}
                        </span>
                      </div>
                      <span class="flex flex-col">
                        <span class="text-sm">{auth.value.user.username}</span>
                        <span class="text-xs text-gray-400">Profile Settings</span>
                      </span>
                    </a>
                    <button
                      onclick={() => { auth.logout(); closeMobileMenu(); }}
                      class="group -mx-2 flex w-full gap-x-3 rounded-md p-2 text-sm leading-6 font-semibold text-gray-300 hover:text-white hover:bg-gray-600"
                    >
                      <span class="text-lg">↪️</span>
                      Sign out
                    </button>
                  </li>
                </ul>
              </nav>
            </div>
          </div>
        </div>
      </div>
    {/if}

    <!-- Main content -->
    <div class="lg:pl-72">
      <main class="py-10">
        <div class="px-4 sm:px-6 lg:px-8">
          {@render children?.()}
        </div>
      </main>
    </div>
  {:else}
    <!-- Non-authenticated layout -->
    <header class="bg-white border-b border-gray-200">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div class="flex justify-between items-center py-6 md:justify-start md:space-x-10">
          <div class="flex justify-start lg:w-0 lg:flex-1">
            <a href="/" class="flex items-center">
              <h1 class="text-xl font-bold text-gray-900">Nexorious</h1>
            </a>
          </div>
          <div class="md:flex items-center justify-end md:flex-1 lg:w-0">
            <a href="/login" class="btn-primary">
              Login
            </a>
          </div>
        </div>
      </div>
    </header>
    
    <main class="flex-1">
      <div class="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
        {@render children?.()}
      </div>
    </main>
  {/if}
</div>

<!-- Toast notifications -->
<ToastContainer />
