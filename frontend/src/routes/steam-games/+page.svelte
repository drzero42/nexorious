<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard, SteamGamesTable } from '$lib/components';
  import { steam, ui, auth } from '$lib/stores';
  import { steamGames, type SteamGameResponse, type SteamGamesListResponse } from '$lib/stores/steam-games.svelte';
  import type { SteamUserInfo } from '$lib/stores';

  // Page state
  let isLoading = $state(true);
  let unmatchedGames = $state<SteamGameResponse[]>([]);
  let matchedGames = $state<SteamGameResponse[]>([]);
  let ignoredGames = $state<SteamGameResponse[]>([]);
  let inSyncGames = $state<SteamGameResponse[]>([]);
  let activeTab = $state<'needs-attention' | 'ignored' | 'in-sync' | 'configuration'>('needs-attention');
  let searchQuery = $state('');
  let isRefreshing = $state(false);
  
  // Steam configuration state
  let webApiKey = $state('');
  let steamId = $state('');
  let vanityUrl = $state('');
  let showApiKey = $state(false);
  let isSubmitting = $state(false);
  let isDeleting = $state(false);

  // Validation state
  let apiKeyError = $state('');
  let steamIdError = $state('');
  let formError = $state('');

  // State for vanity URL resolution
  let showVanityResolver = $state(false);

  // State for Steam import functionality
  let isStartingImport = $state(false);
  let isPreviewingLibrary = $state(false);
  let libraryPreview: any = $state(null);
  let activeImportJob: any = $state(null);
  let isCheckingActiveJob = $state(false);
  
  // Collapsible table state
  let unmatchedCollapsed = $state(false);
  let matchedCollapsed = $state(false);
  
  // Stats
  let unmatchedCount = $state(0);
  let matchedCount = $state(0);
  let ignoredCount = $state(0);
  let syncedCount = $state(0);
  let totalCount = $state(0);

  onMount(async () => {
    // Check if Steam Games feature is enabled
    const user = auth.value.user;
    if (user && user.preferences?.ui?.steam_games_visible === false) {
      ui.showError('Steam Games feature is disabled. You can enable it in Profile Settings.');
      goto('/dashboard');
      return;
    }

    try {
      await steam.getConfig();
      
      // If we have existing config, populate Steam ID
      if (steam.value.config?.steam_id) {
        steamId = steam.value.config.steam_id;
      }

      // Check for active import jobs if configuration is verified
      await checkForActiveImportJob();
      
      // Load Steam games if configuration is verified
      if (steam.value.config?.has_api_key && steam.value.config?.is_verified) {
        await loadSteamGames();
      } else {
        // If Steam is not configured or not verified, show configuration tab
        activeTab = 'configuration';
      }
    } catch (error) {
      // Config doesn't exist yet, show configuration tab
      activeTab = 'configuration';
    } finally {
      isLoading = false;
    }
  });

  // Helper function to update counts from all games data
  function updateCounts(allGames: SteamGamesListResponse) {
    console.log('📊 [UPDATE-COUNTS] Updating counts from', allGames.games.length, 'games');
    
    totalCount = allGames.total;
    
    // Separate games by status with detailed logging
    const unmatchedGamesFiltered = allGames.games.filter((g: SteamGameResponse) => !g.igdb_id && !g.ignored);
    const matchedGamesFiltered = allGames.games.filter((g: SteamGameResponse) => g.igdb_id && !g.game_id && !g.ignored);
    const ignoredGamesFiltered = allGames.games.filter((g: SteamGameResponse) => g.ignored);
    const syncedGamesFiltered = allGames.games.filter((g: SteamGameResponse) => g.game_id);
    
    console.log('🔍 [UPDATE-COUNTS] Game categorization detailed:', {
      unmatched: {
        count: unmatchedGamesFiltered.length,
        games: unmatchedGamesFiltered.map((g: SteamGameResponse) => ({ name: g.game_name, igdb_id: g.igdb_id, ignored: g.ignored }))
      },
      matched: {
        count: matchedGamesFiltered.length,
        games: matchedGamesFiltered.map((g: SteamGameResponse) => ({ name: g.game_name, igdb_id: g.igdb_id, game_id: g.game_id, ignored: g.ignored }))
      },
      ignored: {
        count: ignoredGamesFiltered.length,
        games: ignoredGamesFiltered.map((g: SteamGameResponse) => ({ name: g.game_name, ignored: g.ignored }))
      },
      synced: {
        count: syncedGamesFiltered.length,
        games: syncedGamesFiltered.map((g: SteamGameResponse) => ({ name: g.game_name, game_id: g.game_id }))
      }
    });
    
    unmatchedCount = unmatchedGamesFiltered.length;
    matchedCount = matchedGamesFiltered.length;
    ignoredCount = ignoredGamesFiltered.length;
    syncedCount = syncedGamesFiltered.length;
    
    console.log('📊 [UPDATE-COUNTS] Final count assignments:', {
      unmatchedCount,
      matchedCount,
      ignoredCount,
      syncedCount,
      total: totalCount
    });

    // Return the filtered arrays for use by loadSteamGames
    return {
      unmatchedGamesFiltered,
      matchedGamesFiltered,
      ignoredGamesFiltered,
      syncedGamesFiltered
    };
  }

  // Hybrid refresh function that preserves search filters while updating counts
  async function handleIgnoreRefresh() {
    console.log('🔄 [IGNORE-REFRESH] Starting hybrid refresh to preserve search filter...');
    try {
      // First, update counts by loading all games data
      console.log('📡 [IGNORE-REFRESH] Loading all games for count update...');
      const allGames = await steamGames.listSteamGames(0, 1000);
      updateCounts(allGames); // We don't need the returned arrays here, just count updates
      
      // Then refresh the current tab with search filter preserved
      console.log('🔄 [IGNORE-REFRESH] Loading filtered tab data...');
      await loadTabData();
      
      console.log('✅ [IGNORE-REFRESH] Hybrid refresh completed');
    } catch (error) {
      console.error('❌ [IGNORE-REFRESH] Failed to refresh:', error);
      ui.showError('Failed to refresh data');
    }
  }

  async function loadSteamGames() {
    console.log('🔄 [LOAD-GAMES] Starting loadSteamGames...');
    try {
      isRefreshing = true;
      
      // Load all games to get counts
      console.log('📡 [LOAD-GAMES] Calling steamGames.listSteamGames(0, 1000)...');
      const allGames = await steamGames.listSteamGames(0, 1000);
      console.log('📨 [LOAD-GAMES] API Response:', {
        total: allGames.total,
        gamesCount: allGames.games.length,
        games: allGames.games.map(g => ({
          id: g.id,
          game_name: g.game_name,
          igdb_id: g.igdb_id,
          game_id: g.game_id,
          ignored: g.ignored
        }))
      });
      
      // Update counts using extracted function and get filtered arrays
      const { unmatchedGamesFiltered, matchedGamesFiltered, ignoredGamesFiltered, syncedGamesFiltered } = updateCounts(allGames);
      
      // Set initial tab based on what needs attention
      if (unmatchedCount > 0 || matchedCount > 0 || ignoredCount > 0) {
        activeTab = 'needs-attention';
      } else if (syncedCount > 0) {
        activeTab = 'in-sync';
      }
      
      console.log('🔀 [LOAD-GAMES] Active tab set to:', activeTab);
      console.log('🎯 [LOAD-GAMES] Sync All button should be visible:', matchedCount > 0);
      
      // Populate tab-specific state arrays from the filtered data we already have
      console.log('📋 [LOAD-GAMES] Populating tab-specific state arrays from existing data...');
      unmatchedGames = unmatchedGamesFiltered; // Assign filtered array to state variable
      matchedGames = matchedGamesFiltered;     // Assign filtered array to state variable
      ignoredGames = ignoredGamesFiltered;     // Assign filtered array to state variable  
      inSyncGames = syncedGamesFiltered;       // inSyncGames is the same as syncedGames
      
      console.log('✅ [LOAD-GAMES] Tab state arrays populated:', {
        unmatchedGames: unmatchedGames.length,
        matchedGames: matchedGames.length, 
        ignoredGames: ignoredGames.length,
        inSyncGames: inSyncGames.length
      });
    } catch (error) {
      console.error('❌ [LOAD-GAMES] Failed to load Steam games:', error);
      ui.showError('Failed to load Steam games');
    } finally {
      isRefreshing = false;
      console.log('✅ [LOAD-GAMES] loadSteamGames completed');
    }
  }

  async function loadTabData() {
    try {
      const searchTerm = searchQuery.trim();
      
      if (activeTab === 'needs-attention') {
        const [unmatched, matched] = await Promise.all([
          steamGames.listSteamGames(0, 1000, 'unmatched', searchTerm || undefined),
          steamGames.listSteamGames(0, 1000, 'matched', searchTerm || undefined)
        ]);
        
        unmatchedGames = unmatched.games;
        matchedGames = matched.games;
      } else if (activeTab === 'ignored') {
        const ignored = await steamGames.listSteamGames(0, 1000, 'ignored', searchTerm || undefined);
        ignoredGames = ignored.games;
      } else if (activeTab === 'in-sync') {
        const synced = await steamGames.listSteamGames(0, 1000, 'synced', searchTerm || undefined);
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

  async function handleUnmatchAll() {
    if (matchedCount === 0) {
      ui.showInfo('No matched games to unmatch.');
      return;
    }
    
    const confirmMessage = `This will remove IGDB matches from ${matchedCount} matched games, returning them to unmatched status. Are you sure?`;
    const confirmed = confirm(confirmMessage);
    if (!confirmed) return;
    
    try {
      await steamGames.unmatchAllGames();
      await loadSteamGames(); // Refresh data
    } catch (error) {
      // Error handled in store
    }
  }

  async function handleUnsyncAll() {
    if (syncedCount === 0) {
      ui.showInfo('No synced games to unsync.');
      return;
    }
    
    const confirmMessage = `This will remove ${syncedCount} games from your collection. IGDB matches will remain intact so you can re-sync them later. Are you sure?`;
    const confirmed = confirm(confirmMessage);
    if (!confirmed) return;
    
    try {
      await steamGames.unsyncAllGames();
      await loadSteamGames(); // Refresh data
    } catch (error) {
      // Error handled in store
    }
  }

  async function handleUnignoreAll() {
    const confirmed = confirm(`This will restore ${ignoredGames.length} ignored games back to your "Needs Attention" list. Are you sure?`);
    
    if (!confirmed) return;
    
    try {
      await steamGames.unignoreAllGames();
      await loadSteamGames(); // Refresh data
    } catch (error) {
      // Error handled in store
    }
  }

  async function handleAutoMatch() {
    console.log('🚀 [AUTO-MATCH] Starting auto-match process...');
    console.log('📊 [AUTO-MATCH] Pre-match counts:', {
      unmatched: unmatchedCount,
      matched: matchedCount,
      ignored: ignoredCount,
      synced: syncedCount,
      activeTab: activeTab,
      canAccessGameTabs: canAccessGameTabs
    });

    try {
      console.log('🔄 [AUTO-MATCH] Calling steamGames.retryAutoMatching()...');
      const result = await steamGames.retryAutoMatching();
      console.log('✅ [AUTO-MATCH] Auto-match API response:', result);
      
      // Optimistically update counts immediately based on auto-match result
      if (result.successful_matches > 0) {
        const oldUnmatched = unmatchedCount;
        const oldMatched = matchedCount;
        
        // Move games from unmatched to matched count optimistically
        unmatchedCount = Math.max(0, unmatchedCount - result.successful_matches);
        matchedCount = matchedCount + result.successful_matches;
        
        console.log('⚡ [AUTO-MATCH] Optimistic count updates:', {
          unmatchedCount: { old: oldUnmatched, new: unmatchedCount },
          matchedCount: { old: oldMatched, new: matchedCount },
          successful_matches: result.successful_matches
        });
      }
      
      // Switch to "Needs Attention" tab immediately if matches were found
      if (result.successful_matches > 0 && canAccessGameTabs) {
        console.log('🔀 [AUTO-MATCH] Switching to needs-attention tab...');
        await handleTabChange('needs-attention');
        console.log('✅ [AUTO-MATCH] Tab switched, activeTab is now:', activeTab);
      }
      
      // Add delay to ensure backend transaction is fully committed, then refresh
      console.log('⏱️  [AUTO-MATCH] Waiting 500ms for backend transaction commit...');
      await new Promise(resolve => setTimeout(resolve, 500));
      
      // Refresh data to get accurate state and verify our optimistic updates
      console.log('🔄 [AUTO-MATCH] Refreshing data via loadSteamGames()...');
      await loadSteamGames();
      
      console.log('📊 [AUTO-MATCH] Post-refresh counts:', {
        unmatched: unmatchedCount,
        matched: matchedCount,
        ignored: ignoredCount,
        synced: syncedCount,
        activeTab: activeTab
      });
      
      console.log('🎯 [AUTO-MATCH] Sync All button should be visible:', matchedCount > 0);
      
    } catch (error) {
      console.error('❌ [AUTO-MATCH] Error during auto-match:', error);
      // On error, refresh data to restore correct state
      await loadSteamGames();
    }
  }

  async function handleTabChange(tab: 'needs-attention' | 'ignored' | 'in-sync' | 'configuration') {
    activeTab = tab;
    if (tab !== 'configuration') {
      await loadTabData();
    }
  }

  function toggleUnmatchedCollapsed() {
    unmatchedCollapsed = !unmatchedCollapsed;
  }

  function toggleMatchedCollapsed() {
    matchedCollapsed = !matchedCollapsed;
  }

  // Steam configuration functions
  // Reactive validation using Svelte 5 $effect
  $effect(() => validateApiKey(webApiKey));
  $effect(() => validateSteamId(steamId));

  function validateApiKey(key: string) {
    apiKeyError = '';
    if (key && key.length !== 32) {
      apiKeyError = 'Steam Web API key must be exactly 32 characters';
    } else if (key && !/^[a-zA-Z0-9]+$/.test(key)) {
      apiKeyError = 'Steam Web API key must contain only alphanumeric characters';
    }
  }

  function validateSteamId(id: string) {
    steamIdError = '';
    if (id && (id.length !== 17 || !/^\d+$/.test(id))) {
      steamIdError = 'Steam ID must be exactly 17 digits';
    } else if (id && !id.startsWith('7656119')) {
      steamIdError = 'Invalid Steam ID format';
    }
  }

  async function handleVerify() {
    if (!webApiKey || apiKeyError) {
      formError = 'Please enter a valid Steam Web API key';
      return;
    }

    try {
      formError = '';
      await steam.verify(webApiKey, steamId || undefined);
      
      if (!steam.value.verificationResult?.is_valid) {
        formError = steam.value.verificationResult?.error_message || 'Verification failed';
      }
    } catch (error) {
      formError = error instanceof Error ? error.message : 'Verification failed';
    }
  }

  async function handleSave() {
    if (!webApiKey || apiKeyError || (steamId && steamIdError)) {
      formError = 'Please fix validation errors before saving';
      return;
    }

    isSubmitting = true;
    try {
      formError = '';
      await steam.setConfig(webApiKey, steamId || undefined);
      ui.showSuccess('Steam configuration saved successfully!');
      
      // Clear form
      webApiKey = '';
      showApiKey = false;
      steam.clearVerification();
      
      // Load games after successful configuration
      await loadSteamGames();
    } catch (error) {
      formError = error instanceof Error ? error.message : 'Failed to save configuration';
    } finally {
      isSubmitting = false;
    }
  }

  async function handleDelete() {
    if (!confirm('Are you sure you want to delete your Steam configuration? This cannot be undone.')) {
      return;
    }

    isDeleting = true;
    try {
      await steam.deleteConfig();
      ui.showSuccess('Steam configuration deleted successfully');
      
      // Clear form and reset state
      webApiKey = '';
      steamId = '';
      showApiKey = false;
      steam.clearVerification();
      
      // Reset games data
      unmatchedGames = [];
      matchedGames = [];
      ignoredGames = [];
      inSyncGames = [];
      totalCount = 0;
      unmatchedCount = 0;
      matchedCount = 0;
      ignoredCount = 0;
      syncedCount = 0;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to delete configuration';
      ui.showError(errorMessage);
    } finally {
      isDeleting = false;
    }
  }

  async function handleResolveVanity() {
    if (!vanityUrl.trim()) {
      return;
    }

    try {
      const result = await steam.resolveVanityUrl(vanityUrl.trim());
      
      if (result.success && result.steam_id) {
        steamId = result.steam_id;
        vanityUrl = '';
        showVanityResolver = false;
        ui.showSuccess('Steam ID resolved successfully!');
      } else {
        ui.showError(result.error_message || 'Failed to resolve vanity URL');
      }
    } catch (error) {
      ui.showError(error instanceof Error ? error.message : 'Failed to resolve vanity URL');
    }
  }

  function clearVerification() {
    steam.clearVerification();
    formError = '';
  }

  async function handleStartImport() {
    isStartingImport = true;
    try {
      await steam.startImport();
      // Navigation is handled in the steam store
    } catch (error) {
      // Error handling is done in the steam store
    } finally {
      isStartingImport = false;
    }
  }

  async function handlePreviewLibrary() {
    isPreviewingLibrary = true;
    try {
      libraryPreview = await steam.getLibraryPreview();
    } catch (error) {
      ui.showError(error instanceof Error ? error.message : 'Failed to preview library');
    } finally {
      isPreviewingLibrary = false;
    }
  }

  async function checkForActiveImportJob() {
    isCheckingActiveJob = true;
    try {
      activeImportJob = await steam.getActiveImportJob();
    } catch (error) {
      console.warn('Failed to check for active import job:', error);
      activeImportJob = null;
    } finally {
      isCheckingActiveJob = false;
    }
  }

  function handleViewActiveImport() {
    if (activeImportJob?.id) {
      goto(`/steam/import/status/${activeImportJob.id}`);
    }
  }

  // Reactive search using proper Svelte 5 dependency tracking
  $effect(() => {
    // Read searchQuery to establish dependency tracking
    searchQuery;
    
    // Only execute search if we have data loaded
    if (!isLoading && steam.value.config?.has_api_key && steam.value.config?.is_verified) {
      const debounceTimer = setTimeout(async () => {
        await loadTabData();
      }, 300);
      
      return () => {
        clearTimeout(debounceTimer);
      };
    }
    
    // Return undefined for the else path
    return undefined;
  });

  // Get current config for display using Svelte 5 $derived for proper reactivity
  const currentConfig = $derived(steam.value.config);
  const hasConfig = $derived(currentConfig?.has_api_key);
  const verificationResult = $derived(steam.value.verificationResult);
  const steamUserInfo = $derived(verificationResult?.steam_user_info as SteamUserInfo | undefined);

  // Derived values for reactive display
  const needsAttentionCount = $derived(unmatchedCount + matchedCount);
  const hasNeedsAttention = $derived(needsAttentionCount > 0);
  const hasInSync = $derived(syncedCount > 0);
  
  // Configuration status for conditional interactivity
  const isConfigValid = $derived(hasConfig && currentConfig?.is_verified);
  const canAccessGameTabs = $derived(isConfigValid);
</script>

<svelte:head>
  <title>Steam Games - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  {#if auth.value.user?.preferences?.ui?.steam_games_visible === false}
    <!-- Steam Games Disabled Message -->
    <div class="space-y-6">
      <div class="text-center py-16">
        <span class="text-6xl">🔥</span>
        <h1 class="mt-4 text-3xl font-bold text-gray-900">Steam Games Disabled</h1>
        <p class="mt-2 text-lg text-gray-600">
          The Steam Games feature has been disabled in your profile settings.
        </p>
        <div class="mt-6">
          <a 
            href="/profile" 
            class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
          >
            Go to Profile Settings
          </a>
        </div>
      </div>
    </div>
  {:else}
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
            disabled={!isConfigValid || steamGames.value.isImporting}
            class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
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
          
          {#if unmatchedCount > 0}
            <button
              onclick={handleAutoMatch}
              disabled={steamGames.value.isAutoMatching}
              class="btn-secondary disabled:opacity-50"
              title="Retry auto-matching for unmatched games"
            >
              {#if steamGames.value.isAutoMatching}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Auto-matching...
              {:else}
                <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                </svg>
                Auto-match
              {/if}
            </button>
          {/if}
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
            onclick={() => canAccessGameTabs && handleTabChange('needs-attention')}
            disabled={!canAccessGameTabs}
            class="border-b-2 py-2 px-1 text-sm font-medium {!canAccessGameTabs ? 'border-transparent text-gray-300 cursor-not-allowed' : (activeTab === 'needs-attention' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300')}"
          >
            Needs Attention
            {#if hasNeedsAttention}
              <span class="ml-2 bg-red-100 text-red-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {needsAttentionCount}
              </span>
            {/if}
          </button>
          <button
            onclick={() => canAccessGameTabs && handleTabChange('ignored')}
            disabled={!canAccessGameTabs}
            class="border-b-2 py-2 px-1 text-sm font-medium {!canAccessGameTabs ? 'border-transparent text-gray-300 cursor-not-allowed' : (activeTab === 'ignored' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300')}"
          >
            Ignored
            {#if ignoredCount > 0}
              <span class="ml-2 bg-gray-100 text-gray-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {ignoredCount}
              </span>
            {/if}
          </button>
          <button
            onclick={() => canAccessGameTabs && handleTabChange('in-sync')}
            disabled={!canAccessGameTabs}
            class="border-b-2 py-2 px-1 text-sm font-medium {!canAccessGameTabs ? 'border-transparent text-gray-300 cursor-not-allowed' : (activeTab === 'in-sync' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300')}"
          >
            In Sync
            {#if hasInSync}
              <span class="ml-2 bg-green-100 text-green-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {syncedCount}
              </span>
            {/if}
          </button>
          <button
            onclick={() => handleTabChange('configuration')}
            class="border-b-2 py-2 px-1 text-sm font-medium {activeTab === 'configuration' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
          >
            ⚙️ Configuration
            {#if !hasConfig || !currentConfig?.is_verified}
              <span class="ml-2 bg-yellow-100 text-yellow-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                Setup Required
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
      {#if activeTab === 'configuration'}
        <!-- Steam Configuration Tab Content -->
        <div class="space-y-6">
          <!-- Current Configuration Status -->
          {#if hasConfig}
            <div class="card">
              <div class="border-b border-gray-200 pb-4 mb-4">
                <h2 class="text-lg font-semibold text-gray-900">Current Configuration</h2>
              </div>
              
              <div class="space-y-4">
                <div>
                  <div class="form-label">Steam Web API Key</div>
                  <div class="mt-1 p-3 bg-gray-50 border border-gray-300 rounded-md text-gray-700 font-mono text-sm">
                    {currentConfig?.api_key_masked || 'Not configured'}
                  </div>
                </div>

                {#if currentConfig?.steam_id}
                  <div>
                    <div class="form-label">Steam ID</div>
                    <div class="mt-1 p-3 bg-gray-50 border border-gray-300 rounded-md text-gray-700 font-mono text-sm">
                      {currentConfig.steam_id}
                    </div>
                  </div>
                {/if}

                <div class="flex items-center space-x-2">
                  <div class="form-label">Status</div>
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {currentConfig?.is_verified ? 'bg-green-100 text-green-800' : 'bg-yellow-100 text-yellow-800'}">
                    {currentConfig?.is_verified ? 'Verified' : 'Not Verified'}
                  </span>
                </div>

                {#if currentConfig?.configured_at}
                  <div>
                    <div class="form-label">Last Updated</div>
                    <div class="mt-1 text-sm text-gray-600">
                      {currentConfig.configured_at.toLocaleString()}
                    </div>
                  </div>
                {/if}

                <div class="pt-4 flex space-x-3">
                  <button
                    onclick={handleDelete}
                    disabled={isDeleting}
                    class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {#if isDeleting}
                      <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                      Deleting...
                    {:else}
                      Delete Configuration
                    {/if}
                  </button>
                </div>
              </div>
            </div>
          {/if}

          <!-- Configuration Form -->
          <div class="card">
            <div class="border-b border-gray-200 pb-4 mb-6">
              <h2 class="text-lg font-semibold text-gray-900">
                {hasConfig ? 'Update' : 'Setup'} Steam Configuration
              </h2>
              <p class="mt-1 text-sm text-gray-500">
                You'll need a Steam Web API key to import your Steam library. 
                <a href="https://steamcommunity.com/dev/apikey" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-500">
                  Get your API key here
                </a>
              </p>
            </div>

            <!-- Steam Web API Key -->
            <div class="mb-4">
              <label for="webApiKey" class="form-label">Steam Web API Key *</label>
              <div class="mt-1 relative">
                <input
                  id="webApiKey"
                  type={showApiKey ? 'text' : 'password'}
                  bind:value={webApiKey}
                  placeholder="Enter your 32-character Steam Web API key"
                  class="form-input pr-10"
                  class:border-red-500={apiKeyError}
                  class:border-green-500={webApiKey && !apiKeyError}
                />
                <button
                  type="button"
                  onclick={() => showApiKey = !showApiKey}
                  class="absolute inset-y-0 right-0 flex items-center pr-3"
                  aria-label={showApiKey ? 'Hide API key' : 'Show API key'}
                >
                  {#if showApiKey}
                    <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                    </svg>
                  {:else}
                    <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                    </svg>
                  {/if}
                </button>
              </div>
              {#if apiKeyError}
                <p class="mt-2 text-sm text-red-600">{apiKeyError}</p>
              {/if}
            </div>

            <!-- Steam ID -->
            <div class="mb-4">
              <div class="flex items-center justify-between">
                <label for="steamId" class="form-label">Steam ID (Optional)</label>
                <button
                  type="button"
                  onclick={() => showVanityResolver = !showVanityResolver}
                  class="text-sm text-primary-600 hover:text-primary-500"
                >
                  {showVanityResolver ? 'Hide' : 'Resolve from vanity URL'}
                </button>
              </div>
              
              {#if showVanityResolver}
                <div class="mt-2 p-3 bg-gray-50 rounded-md">
                  <div class="flex space-x-2">
                    <input
                      type="text"
                      bind:value={vanityUrl}
                      placeholder="Enter your Steam vanity URL or custom ID"
                      class="flex-1 form-input text-sm"
                    />
                    <button
                      type="button"
                      onclick={handleResolveVanity}
                      disabled={steam.value.isResolvingVanity || !vanityUrl.trim()}
                      class="btn-secondary text-sm disabled:opacity-50"
                    >
                      {#if steam.value.isResolvingVanity}
                        <svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                      {:else}
                        Resolve
                      {/if}
                    </button>
                  </div>
                  <p class="mt-1 text-xs text-gray-500">
                    Example: "mynickname" from steamcommunity.com/id/mynickname
                  </p>
                </div>
              {/if}

              <div class="mt-1">
                <input
                  id="steamId"
                  type="text"
                  bind:value={steamId}
                  placeholder="76561198123456789"
                  class="form-input"
                  class:border-red-500={steamIdError}
                  class:border-green-500={steamId && !steamIdError}
                />
              </div>
              {#if steamIdError}
                <p class="mt-2 text-sm text-red-600">{steamIdError}</p>
              {:else}
                <p class="mt-2 text-sm text-gray-500">
                  17-digit Steam ID for importing your library. Leave empty if you only want to verify the API key.
                </p>
              {/if}
            </div>

            <!-- Verification Section -->
            {#if webApiKey && !apiKeyError}
              <div class="mb-6 p-4 bg-blue-50 rounded-md">
                <div class="flex items-center justify-between">
                  <div>
                    <h3 class="text-sm font-medium text-blue-900">Test Configuration</h3>
                    <p class="text-sm text-blue-700">Verify your settings before saving</p>
                  </div>
                  <button
                    type="button"
                    onclick={handleVerify}
                    disabled={steam.value.isVerifying}
                    class="btn-primary text-sm disabled:opacity-50"
                  >
                    {#if steam.value.isVerifying}
                      <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                      Verifying...
                    {:else}
                      Verify Configuration
                    {/if}
                  </button>
                </div>

                <!-- Verification Results -->
                {#if verificationResult}
                  <div class="mt-4">
                    {#if verificationResult.is_valid}
                      <div class="flex items-center text-green-700">
                        <svg class="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
                          <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                        </svg>
                        Configuration is valid!
                      </div>

                      {#if steamUserInfo}
                        <div class="mt-3 p-3 bg-white rounded border">
                          <div class="flex items-center space-x-3">
                            <img 
                              src={steamUserInfo.avatar_medium} 
                              alt="Steam avatar" 
                              class="w-10 h-10 rounded"
                            />
                            <div>
                              <div class="font-medium text-gray-900">{steamUserInfo.persona_name}</div>
                              <a 
                                href={steamUserInfo.profile_url} 
                                target="_blank" 
                                rel="noopener noreferrer"
                                class="text-sm text-primary-600 hover:text-primary-500"
                              >
                                View Steam Profile →
                              </a>
                            </div>
                          </div>
                        </div>
                      {/if}
                    {:else}
                      <div class="flex items-center text-red-700">
                        <svg class="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
                          <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
                        </svg>
                        {verificationResult.error_message}
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Form Error -->
            {#if formError}
              <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-md">
                <div class="flex">
                  <svg class="h-5 w-5 text-red-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
                  </svg>
                  <p class="text-sm text-red-800">{formError}</p>
                </div>
              </div>
            {/if}

            <!-- Action Buttons -->
            <div class="flex space-x-3">
              <button
                onclick={handleSave}
                disabled={!webApiKey || !!apiKeyError || (steamId && !!steamIdError) || isSubmitting}
                class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {#if isSubmitting}
                  <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  Saving...
                {:else}
                  Save Configuration
                {/if}
              </button>
              
              {#if verificationResult}
                <button
                  type="button"
                  onclick={clearVerification}
                  class="btn-secondary"
                >
                  Clear Verification
                </button>
              {/if}
            </div>
          </div>

          <!-- Steam Import Section -->
          {#if hasConfig && currentConfig?.is_verified}
            <div class="card">
              <div class="border-b border-gray-200 pb-4 mb-6">
                <div class="flex items-center justify-between">
                  <div>
                    <h2 class="text-lg font-semibold text-gray-900">Steam Library Import</h2>
                    <p class="mt-1 text-sm text-gray-500">
                      Import your Steam library to automatically add games to your collection
                    </p>
                  </div>
                </div>
              </div>

              <div class="space-y-4">
                <!-- Import Status/Info -->
                <div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
                  <div class="flex items-center">
                    <svg class="h-6 w-6 text-blue-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    <div>
                      <h3 class="text-sm font-medium text-blue-800">How Steam Import Works</h3>
                      <div class="text-sm text-blue-700 mt-1 space-y-1">
                        <p>• We'll retrieve your Steam library and automatically match games to our database</p>
                        <p>• You'll review any games that couldn't be automatically matched</p>
                        <p>• New games will be imported with full metadata and cover art</p>
                        <p>• Existing games will have Steam platform added to them</p>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- Active Import Status -->
                {#if activeImportJob}
                  <div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
                    <div class="flex items-center">
                      <svg class="h-6 w-6 text-blue-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                      <div>
                        <h3 class="text-sm font-medium text-blue-800">Active Import in Progress</h3>
                        <div class="text-sm text-blue-700 mt-1">
                          <p><strong>Status:</strong> {activeImportJob.status}</p>
                          <p><strong>Job ID:</strong> {activeImportJob.id}</p>
                          {#if activeImportJob.total_games > 0}
                            <p><strong>Progress:</strong> {activeImportJob.processed_games} of {activeImportJob.total_games} games processed</p>
                          {/if}
                          {#if activeImportJob.awaiting_review_games > 0}
                            <p><strong>Awaiting Review:</strong> {activeImportJob.awaiting_review_games} games need manual review</p>
                          {/if}
                        </div>
                      </div>
                    </div>
                  </div>
                {/if}

                <!-- Import Actions -->
                <div class="flex flex-col sm:flex-row gap-4">
                  {#if activeImportJob}
                    <!-- Active import exists - show view button -->
                    <button
                      onclick={handleViewActiveImport}
                      class="flex-1 btn-secondary"
                    >
                      <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                      </svg>
                      View Active Import ({activeImportJob.status})
                    </button>
                  {:else}
                    <!-- No active import - show start button -->
                    <button
                      onclick={handleStartImport}
                      disabled={isStartingImport || isCheckingActiveJob}
                      class="flex-1 btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {#if isStartingImport}
                        <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        Starting Import...
                      {:else if isCheckingActiveJob}
                        <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        Checking for active imports...
                      {:else}
                        <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                        </svg>
                        Start Steam Import
                      {/if}
                    </button>
                  {/if}

                  <button
                    onclick={handlePreviewLibrary}
                    disabled={isPreviewingLibrary}
                    class="flex-1 btn-secondary disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {#if isPreviewingLibrary}
                      <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                      Loading Preview...
                    {:else}
                      <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                      </svg>
                      Preview Library
                    {/if}
                  </button>
                </div>

                <!-- Library Preview -->
                {#if libraryPreview}
                  <div class="border border-gray-200 rounded-lg p-4">
                    <div class="flex items-center justify-between mb-3">
                      <h4 class="text-sm font-medium text-gray-900">
                        Steam Library Preview
                      </h4>
                      <span class="text-sm text-gray-600">
                        {libraryPreview.total_games} games found
                      </span>
                    </div>
                    
                    <div class="text-sm text-gray-600 mb-3">
                      <p><strong>Profile:</strong> {libraryPreview.steam_user_info.persona_name}</p>
                      <p><strong>Steam ID:</strong> {libraryPreview.steam_user_info.steam_id}</p>
                    </div>

                    {#if libraryPreview.games && libraryPreview.games.length > 0}
                      <div class="space-y-2 max-h-40 overflow-y-auto">
                        <div class="text-xs font-medium text-gray-700 mb-2">Sample Games:</div>
                        {#each libraryPreview.games.slice(0, 10) as game}
                          <div class="text-xs text-gray-600 bg-gray-50 px-2 py-1 rounded">
                            {game.name} (ID: {game.appid})
                          </div>
                        {/each}
                        {#if libraryPreview.games.length > 10}
                          <div class="text-xs text-gray-500 text-center">
                            +{libraryPreview.games.length - 10} more games
                          </div>
                        {/if}
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            </div>
          {:else if hasConfig && !currentConfig?.is_verified}
            <div class="card bg-yellow-50 border-yellow-200">
              <div class="flex items-center">
                <svg class="h-6 w-6 text-yellow-400 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                </svg>
                <div>
                  <h3 class="text-sm font-medium text-yellow-800">Steam Configuration Required</h3>
                  <p class="text-sm text-yellow-700 mt-1">
                    Please verify your Steam configuration before importing your library.
                  </p>
                </div>
              </div>
            </div>
          {/if}

          <!-- Help Information -->
          <div class="card max-w-2xl">
            <h3 class="text-sm font-semibold text-gray-900 mb-3">Getting Your Steam Web API Key</h3>
            <div class="text-sm text-gray-600 space-y-2">
              <p>1. Go to <a href="https://steamcommunity.com/dev/apikey" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-500">Steam Web API Key page</a></p>
              <p>2. Log in with your Steam account</p>
              <p>3. Enter any domain name (e.g., "localhost" for personal use)</p>
              <p>4. Copy the generated 32-character API key</p>
            </div>
            
            <h3 class="text-sm font-semibold text-gray-900 mb-3 mt-6">Finding Your Steam ID</h3>
            <div class="text-sm text-gray-600 space-y-2">
              <p>• Use the vanity URL resolver above if you have a custom Steam URL</p>
              <p>• Or visit <a href="https://steamid.io/" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-500">SteamID.io</a> to find your 17-digit Steam ID</p>
              <p>• Your profile must be public to import your library</p>
            </div>
          </div>
        </div>
      {:else if activeTab === 'needs-attention'}
        <div class="space-y-6">
          {#if !isConfigValid}
            <!-- Steam not configured message -->
            <div class="text-center py-12">
              <div class="mx-auto h-12 w-12 text-gray-400">
                <span class="text-4xl">⚙️</span>
              </div>
              <h3 class="mt-2 text-sm font-semibold text-gray-900">Steam Configuration Required</h3>
              <p class="mt-1 text-sm text-gray-500">Please configure Steam in the Configuration tab to view games that need attention.</p>
              <div class="mt-6">
                <button
                  onclick={() => handleTabChange('configuration')}
                  class="btn-primary"
                >
                  Go to Configuration
                </button>
              </div>
            </div>
          {:else}
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
                <div class="flex space-x-3">
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
                  <button
                    onclick={handleUnmatchAll}
                    disabled={steamGames.value.isUnmatchingAll}
                    class="btn-secondary text-orange-600 hover:text-orange-700 disabled:opacity-50"
                  >
                    {#if steamGames.value.isUnmatchingAll}
                      <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                      Unmatching...
                    {:else}
                      Unmatch All
                    {/if}
                  </button>
                </div>
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
                onRefresh={handleIgnoreRefresh}
                collapsible={true}
                collapsed={unmatchedCollapsed}
                onToggleCollapse={toggleUnmatchedCollapsed}
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
                showUnmatchButton={true}
                onRefresh={handleIgnoreRefresh}
                collapsible={true}
                collapsed={matchedCollapsed}
                onToggleCollapse={toggleMatchedCollapsed}
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
          {/if}
        </div>
      {:else if activeTab === 'ignored'}
        <div class="space-y-6">
          {#if !isConfigValid}
            <!-- Steam not configured message -->
            <div class="text-center py-12">
              <div class="mx-auto h-12 w-12 text-gray-400">
                <span class="text-4xl">⚙️</span>
              </div>
              <h3 class="mt-2 text-sm font-semibold text-gray-900">Steam Configuration Required</h3>
              <p class="mt-1 text-sm text-gray-500">Please configure Steam in the Configuration tab to view ignored games.</p>
              <div class="mt-6">
                <button
                  onclick={() => handleTabChange('configuration')}
                  class="btn-primary"
                >
                  Go to Configuration
                </button>
              </div>
            </div>
          {:else}
          <!-- Bulk Actions for Ignored Games -->
          {#if ignoredGames.length > 0}
            <div class="bg-orange-50 border border-orange-200 rounded-lg p-4">
              <div class="flex items-center justify-between">
                <div class="flex items-center">
                  <svg class="h-6 w-6 text-orange-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.268 16.5c-.77.833.192 2.5 1.732 2.5z" />
                  </svg>
                  <div>
                    <h3 class="text-sm font-medium text-orange-800">Ignored Games Available</h3>
                    <p class="text-sm text-orange-700 mt-1">
                      {ignoredGames.length} {ignoredGames.length === 1 ? 'game is' : 'games are'} currently ignored and can be restored
                    </p>
                  </div>
                </div>
                <button
                  onclick={handleUnignoreAll}
                  disabled={steamGames.value.isUnignoringAll}
                  class="btn-primary disabled:opacity-50"
                >
                  {#if steamGames.value.isUnignoringAll}
                    <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                      <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Restoring...
                  {:else}
                    Unignore All
                  {/if}
                </button>
              </div>
            </div>
          {/if}

          <!-- Ignored Games Section -->
          {#if ignoredGames.length > 0}
            <SteamGamesTable
              title="Ignored Games"
              description="These games have been marked as ignored and won't be imported to your collection."
              icon="🚫"
              games={ignoredGames}
              emptyMessage="No ignored games found"
              showUnignoreButton={true}
              showUnmatchButton={true}
              onRefresh={handleIgnoreRefresh}
              collapsible={false}
            />
          {:else}
            <div class="text-center py-12">
              <span class="text-6xl">🚫</span>
              <h3 class="mt-2 text-lg font-medium text-gray-900">No ignored games</h3>
              <p class="mt-1 text-sm text-gray-500">
                Games you mark as ignored will appear here.
              </p>
            </div>
          {/if}
          {/if}
        </div>
      {:else if activeTab === 'in-sync'}
        <div class="space-y-6">
          {#if !isConfigValid}
            <!-- Steam not configured message -->
            <div class="text-center py-12">
              <div class="mx-auto h-12 w-12 text-gray-400">
                <span class="text-4xl">⚙️</span>
              </div>
              <h3 class="mt-2 text-sm font-semibold text-gray-900">Steam Configuration Required</h3>
              <p class="mt-1 text-sm text-gray-500">Please configure Steam in the Configuration tab to view synced games.</p>
              <div class="mt-6">
                <button
                  onclick={() => handleTabChange('configuration')}
                  class="btn-primary"
                >
                  Go to Configuration
                </button>
              </div>
            </div>
          {:else}
          <!-- Bulk Actions for In Sync -->
          {#if syncedCount > 0}
            <div class="bg-red-50 border border-red-200 rounded-lg p-4 mb-6">
              <div class="flex items-center justify-between">
                <div class="flex items-center">
                  <svg class="h-6 w-6 text-red-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v3m0 0v3m0-3h3m-3 0H9m12 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <div>
                    <h3 class="text-sm font-medium text-red-800">Collection Management</h3>
                    <p class="text-sm text-red-700 mt-1">
                      {syncedCount} {syncedCount === 1 ? 'game is' : 'games are'} synced to your collection
                    </p>
                  </div>
                </div>
                <button
                  onclick={handleUnsyncAll}
                  disabled={steamGames.value.isUnsyncingAll}
                  class="btn-secondary text-red-600 hover:text-red-700 disabled:opacity-50"
                >
                  {#if steamGames.value.isUnsyncingAll}
                    <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                      <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Unsyncing...
                  {:else}
                    Unsync All
                  {/if}
                </button>
              </div>
            </div>
          {/if}

          <!-- In Sync Games Section -->
          {#if inSyncGames.length > 0}
            <SteamGamesTable
              title="Games in Collection"
              description="These Steam games have been successfully added to your main game collection."
              icon="🔥"
              games={inSyncGames}
              emptyMessage="No games synced yet"
              showUnsyncButton={true}
              showGameLink={true}
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
          {/if}
        </div>
      {/if}
    {/if}

  </div>
  {/if}
</RouteGuard>