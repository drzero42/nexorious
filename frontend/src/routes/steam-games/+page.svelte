<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard, SteamGamesTable } from '$lib/components';
  import { steam } from '$lib/stores/steam.svelte';
  import { steamGames, type SteamGameResponse } from '$lib/stores/steam-games.svelte';
  import { ui } from '$lib/stores/ui.svelte';

  // Page state
  let isLoading = $state(true);
  let unmatchedGames = $state<SteamGameResponse[]>([]);
  let matchedGames = $state<SteamGameResponse[]>([]);
  let ignoredGames = $state<SteamGameResponse[]>([]);
  let inSyncGames = $state<SteamGameResponse[]>([]);
  let activeTab = $state<'needs-attention' | 'in-sync'>('needs-attention');
  let searchQuery = $state('');
  let isRefreshing = $state(false);
  
  // Stats
  let unmatchedCount = $state(0);
  let matchedCount = $state(0);
  let ignoredCount = $state(0);
  let syncedCount = $state(0);
  let totalCount = $state(0);

  onMount(async () => {
    // Check if user has Steam configuration
    try {
      await steam.getConfig();
      
      if (!steam.value.config?.has_api_key || !steam.value.config?.is_verified) {
        ui.showWarning('Steam configuration required. Redirecting to settings...');
        setTimeout(() => {
          goto('/settings/steam');
        }, 2000);
        return;
      }
      
      await loadSteamGames();
    } catch (error) {
      console.warn('No Steam configuration found, redirecting to settings');
      goto('/settings/steam');
    } finally {
      isLoading = false;
    }
  });

  async function loadSteamGames() {
    try {
      isRefreshing = true;
      
      // Load all games to get counts
      const allGames = await steamGames.listSteamGames(0, 1000);
      totalCount = allGames.total;
      
      // Separate games by status
      unmatchedCount = allGames.games.filter(g => !g.igdb_id && !g.ignored).length;
      matchedCount = allGames.games.filter(g => g.igdb_id && !g.game_id && !g.ignored).length;
      ignoredCount = allGames.games.filter(g => g.ignored).length;
      syncedCount = allGames.games.filter(g => g.game_id).length;
      
      // Set initial tab based on what needs attention
      if (unmatchedCount > 0 || matchedCount > 0 || ignoredCount > 0) {
        activeTab = 'needs-attention';
      } else if (syncedCount > 0) {
        activeTab = 'in-sync';
      }
      
      await loadTabData();
    } catch (error) {
      console.error('Failed to load Steam games:', error);
      ui.showError('Failed to load Steam games');
    } finally {
      isRefreshing = false;
    }
  }

  async function loadTabData() {
    try {
      if (activeTab === 'needs-attention') {
        const [unmatched, matched, ignored] = await Promise.all([
          steamGames.listSteamGames(0, 1000, 'unmatched', searchQuery),
          steamGames.listSteamGames(0, 1000, 'matched', searchQuery),
          steamGames.listSteamGames(0, 1000, 'ignored', searchQuery)
        ]);
        
        unmatchedGames = unmatched.games;
        matchedGames = matched.games;
        ignoredGames = ignored.games;
      } else {
        const synced = await steamGames.listSteamGames(0, 1000, 'synced', searchQuery);
        inSyncGames = synced.games;
      }
    } catch (error) {
      console.error('Failed to load tab data:', error);
    }
  }

  async function handleRefresh() {
    await loadSteamGames();
  }

  async function handleImportLibrary() {
    try {
      await steamGames.importSteamLibrary();
      // Refresh after a delay to allow background import to process
      setTimeout(async () => {
        await loadSteamGames();
      }, 3000);
    } catch (error) {
      // Error handled in store
    }
  }

  async function handleSyncAll() {
    try {
      await steamGames.syncAllMatchedGames();
      await loadSteamGames(); // Refresh data
    } catch (error) {
      // Error handled in store
    }
  }

  async function handleTabChange(tab: 'needs-attention' | 'in-sync') {
    activeTab = tab;
    await loadTabData();
  }

  // Reactive search
  $effect(() => {
    const debounceTimer = setTimeout(async () => {
      await loadTabData();
    }, 300);

    return () => clearTimeout(debounceTimer);
  });

  // Derived values for reactive display
  const needsAttentionCount = $derived(unmatchedCount + matchedCount + ignoredCount);
  const hasNeedsAttention = $derived(needsAttentionCount > 0);
  const hasInSync = $derived(syncedCount > 0);
</script>

<svelte:head>
  <title>Steam Games - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div>
      <nav class="flex text-sm text-gray-500" aria-label="Breadcrumb">
        <ol class="inline-flex items-center space-x-1 md:space-x-3">
          <li>
            <a href="/dashboard" class="hover:text-gray-700">Dashboard</a>
          </li>
          <li>
            <span>›</span>
          </li>
          <li>
            <span class="text-gray-900 font-medium">Steam Games</span>
          </li>
        </ol>
      </nav>
      
      <div class="mt-4 flex flex-col sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">
            Steam Games Management
          </h1>
          <p class="mt-1 text-sm text-gray-500">
            Import and sync your Steam library to your game collection
          </p>
        </div>
        
        <div class="mt-4 sm:mt-0 flex space-x-3">
          <button
            onclick={handleRefresh}
            disabled={isRefreshing}
            class="btn-secondary disabled:opacity-50"
            aria-label="Refresh Steam games data"
          >
            {#if isRefreshing}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Refreshing...
            {:else}
              <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Refresh
            {/if}
          </button>
          
          <button
            onclick={handleImportLibrary}
            disabled={steamGames.value.isImporting}
            class="btn-primary disabled:opacity-50"
          >
            {#if steamGames.value.isImporting}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Importing...
            {:else}
              <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              Import Library
            {/if}
          </button>
        </div>
      </div>
    </div>

    {#if isLoading}
      <!-- Loading State -->
      <div class="flex items-center justify-center py-12">
        <div class="text-center">
          <svg class="animate-spin h-8 w-8 mx-auto text-gray-400" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <p class="mt-2 text-sm text-gray-500">Loading Steam games...</p>
        </div>
      </div>
    {:else if totalCount === 0}
      <!-- Empty State -->
      <div class="text-center py-12">
        <div class="mx-auto h-12 w-12 text-gray-400">
          <span class="text-4xl">🔥</span>
        </div>
        <h3 class="mt-2 text-sm font-semibold text-gray-900">No Steam games found</h3>
        <p class="mt-1 text-sm text-gray-500">Get started by importing your Steam library.</p>
        <div class="mt-6">
          <button
            onclick={handleImportLibrary}
            disabled={steamGames.value.isImporting}
            class="btn-primary"
          >
            {#if steamGames.value.isImporting}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Importing Library...
            {:else}
              <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              Import Steam Library
            {/if}
          </button>
        </div>
      </div>
    {:else}
      <!-- Stats Overview -->
      <div class="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-5">
        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <span class="text-2xl">📚</span>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">Total Games</dt>
                  <dd class="text-lg font-medium text-gray-900">{totalCount}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>

        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <span class="text-2xl">❓</span>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">Unmatched</dt>
                  <dd class="text-lg font-medium text-gray-900">{unmatchedCount}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>

        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <span class="text-2xl">✅</span>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">Matched</dt>
                  <dd class="text-lg font-medium text-gray-900">{matchedCount}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>

        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <span class="text-2xl">🚫</span>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">Ignored</dt>
                  <dd class="text-lg font-medium text-gray-900">{ignoredCount}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>

        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <span class="text-2xl">🔥</span>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">In Collection</dt>
                  <dd class="text-lg font-medium text-gray-900">{syncedCount}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab Navigation -->
      <div class="border-b border-gray-200">
        <nav class="-mb-px flex space-x-8" aria-label="Tabs">
          <button
            onclick={() => handleTabChange('needs-attention')}
            class="border-b-2 py-2 px-1 text-sm font-medium {activeTab === 'needs-attention' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
          >
            Needs Attention
            {#if hasNeedsAttention}
              <span class="ml-2 bg-red-100 text-red-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {needsAttentionCount}
              </span>
            {/if}
          </button>
          <button
            onclick={() => handleTabChange('in-sync')}
            class="border-b-2 py-2 px-1 text-sm font-medium {activeTab === 'in-sync' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
          >
            In Sync
            {#if hasInSync}
              <span class="ml-2 bg-green-100 text-green-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {syncedCount}
              </span>
            {/if}
          </button>
        </nav>
      </div>

      <!-- Search Bar -->
      <div class="max-w-md">
        <div class="relative">
          <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
            <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </div>
          <input
            type="text"
            bind:value={searchQuery}
            placeholder="Search Steam games..."
            class="block w-full pl-10 pr-3 py-2 border border-gray-300 rounded-md leading-5 bg-white placeholder-gray-500 focus:outline-none focus:placeholder-gray-400 focus:ring-1 focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
          />
        </div>
      </div>

      <!-- Tab Content -->
      {#if activeTab === 'needs-attention'}
        <div class="space-y-6">
          <!-- Bulk Actions for Needs Attention -->
          {#if matchedCount > 0}
            <div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
              <div class="flex items-center justify-between">
                <div class="flex items-center">
                  <svg class="h-6 w-6 text-blue-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <div>
                    <h3 class="text-sm font-medium text-blue-800">Ready to Sync</h3>
                    <p class="text-sm text-blue-700 mt-1">
                      {matchedCount} matched {matchedCount === 1 ? 'game is' : 'games are'} ready to be added to your collection
                    </p>
                  </div>
                </div>
                <button
                  onclick={handleSyncAll}
                  disabled={steamGames.value.isSyncing}
                  class="btn-primary disabled:opacity-50"
                >
                  {#if steamGames.value.isSyncing}
                    <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                      <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Syncing...
                  {:else}
                    Sync All Matched
                  {/if}
                </button>
              </div>
            </div>
          {/if}

          <!-- Needs Attention Tables -->
          <div class="space-y-8">
            <!-- Unmatched Games Section -->
            {#if unmatchedGames.length > 0}
              <SteamGamesTable
                title="Unmatched Games"
                description="These games need to be matched to IGDB entries before they can be imported."
                icon="❓"
                games={unmatchedGames}
                emptyMessage="No unmatched games found"
                showMatchButton={true}
                showIgnoreButton={true}
                onRefresh={loadTabData}
              />
            {/if}

            <!-- Matched Games Section -->
            {#if matchedGames.length > 0}
              <SteamGamesTable
                title="Matched Games"
                description="These games are matched to IGDB and ready to be added to your collection."
                icon="✅"
                games={matchedGames}
                emptyMessage="No matched games found"
                showSyncButton={true}
                showIgnoreButton={true}
                onRefresh={loadTabData}
              />
            {/if}

            <!-- Ignored Games Section -->
            {#if ignoredGames.length > 0}
              <SteamGamesTable
                title="Ignored Games"
                description="These games have been marked as ignored and won't be imported."
                icon="🚫"
                games={ignoredGames}
                emptyMessage="No ignored games found"
                showUnignoreButton={true}
                onRefresh={loadTabData}
              />
            {/if}

            {#if needsAttentionCount === 0}
              <div class="text-center py-12">
                <span class="text-6xl">🎉</span>
                <h3 class="mt-2 text-lg font-medium text-gray-900">All caught up!</h3>
                <p class="mt-1 text-sm text-gray-500">
                  No Steam games need attention right now.
                </p>
              </div>
            {/if}
          </div>
        </div>
      {:else if activeTab === 'in-sync'}
        <div class="space-y-6">
          <!-- In Sync Games Section -->
          {#if inSyncGames.length > 0}
            <SteamGamesTable
              title="Games in Collection"
              description="These Steam games have been successfully added to your main game collection."
              icon="🔥"
              games={inSyncGames}
              emptyMessage="No games synced yet"
              onRefresh={loadTabData}
            />
          {:else}
            <div class="text-center py-12">
              <span class="text-6xl">🔥</span>
              <h3 class="mt-2 text-lg font-medium text-gray-900">No games synced yet</h3>
              <p class="mt-1 text-sm text-gray-500">
                Import and sync your Steam games to see them here.
              </p>
            </div>
          {/if}
        </div>
      {/if}
    {/if}

    <!-- Steam Configuration Link -->
    <div class="card bg-gray-50">
      <div class="flex items-center">
        <svg class="h-6 w-6 text-gray-400 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
        <div>
          <h3 class="text-sm font-medium text-gray-900">Steam Settings</h3>
          <p class="text-sm text-gray-600">
            Manage your Steam Web API configuration and import settings.
          </p>
        </div>
        <div class="ml-auto">
          <a href="/settings/steam" class="btn-secondary">
            Configure Steam
          </a>
        </div>
      </div>
    </div>
  </div>
</RouteGuard>