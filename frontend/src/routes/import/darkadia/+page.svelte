<script lang="ts">
  import { onDestroy } from 'svelte';
  import { RouteGuard, DarkadiaGamesTable, DarkadiaFileUpload, BatchProgressModal, ImportProgressModal, PlatformStorefrontModal } from '$lib/components';
  import IgdbWarningBanner from '$lib/components/IgdbWarningBanner.svelte';
  import { darkadia, ui, auth } from '$lib/stores';
  import { platforms } from '$lib/stores/platforms.svelte';
  import type { 
    DarkadiaGameResponse, 
    DarkadiaGamesListResponse,
    DarkadiaUploadResponse 
  } from '$lib/types/darkadia';

  // Page state
  let isLoading = $state(true);
  let unmatchedGames = $state<DarkadiaGameResponse[]>([]);
  let matchedGames = $state<DarkadiaGameResponse[]>([]);
  let ignoredGames = $state<DarkadiaGameResponse[]>([]);
  let inSyncGames = $state<DarkadiaGameResponse[]>([]);
  let activeTab = $state<'needs-attention' | 'ignored' | 'in-sync' | 'upload'>('upload');
  let searchQuery = $state('');
  let isRefreshing = $state(false);
  
  // Collapsible table state
  let unmatchedCollapsed = $state(false);
  let matchedCollapsed = $state(false);
  
  // Stats
  let unmatchedCount = $state(0);
  let matchedCount = $state(0);
  let ignoredCount = $state(0);
  let syncedCount = $state(0);
  let totalCount = $state(0);

  // Batch processing state
  let showBatchModal = $state(false);
  let batchProcessingActive = $state(false);
  let processingTimeout: NodeJS.Timeout | null = $state(null);
  let isCancelling = $state(false);

  // Import progress modal state
  let showImportModal = $state(false);
  let importCancelling = $state(false);

  // Platform and storefront resolution state
  let showPlatformStorefrontModal = $state(false);
  let pendingPlatformResolutions = $state(0);
  let pendingStorefrontResolutions = $state(0);

  // Reset confirmation state
  let showResetModal = $state(false);

  // Initialize Darkadia data when auth is ready
  let hasInitialized = $state(false);
  
  $effect(() => {
    // Only run when we have a valid authenticated user
    if (auth.value.user && !hasInitialized) {
      console.log('🔄 [DARKADIA-PAGE] Auth ready, initializing Darkadia data...');
      hasInitialized = true;
      initializeDarkadiaData();
    }
  });

  onDestroy(() => {
    // Clean up any running batch processing intervals to prevent memory leaks
    stopBatchProcessing();
  });

  // Helper function to update counts from all games data
  function updateCounts(allGames: DarkadiaGamesListResponse) {
    console.log('📊 [UPDATE-COUNTS] Updating counts from', allGames.games.length, 'games');
    
    totalCount = allGames.total;
    
    // Separate games by status with detailed logging
    const unmatchedGamesFiltered = allGames.games.filter((g: DarkadiaGameResponse) => !g.igdb_id && !g.ignored);
    const matchedGamesFiltered = allGames.games.filter((g: DarkadiaGameResponse) => g.igdb_id && !g.game_id && !g.ignored);
    const ignoredGamesFiltered = allGames.games.filter((g: DarkadiaGameResponse) => g.ignored);
    const syncedGamesFiltered = allGames.games.filter((g: DarkadiaGameResponse) => g.game_id);
    
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

    // Return the filtered arrays for use by loadDarkadiaGames
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
      const allGames = await darkadia.listDarkadiaGames(0, 100000);
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

  async function loadDarkadiaGames() {
    console.log('🔄 [LOAD-GAMES] Starting loadDarkadiaGames...');
    try {
      isRefreshing = true;
      
      // Load all games to get counts
      console.log('📡 [LOAD-GAMES] Calling darkadia.listDarkadiaGames(0, 100000)...');
      const allGames = await darkadia.listDarkadiaGames(0, 100000);
      console.log('📨 [LOAD-GAMES] API Response:', {
        total: allGames.total,
        gamesCount: allGames.games.length,
        games: allGames.games.map((g: DarkadiaGameResponse) => ({
          id: g.id,
          game_name: g.name,
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
      console.error('❌ [LOAD-GAMES] Failed to load Darkadia games:', error);
      ui.showError('Failed to load Darkadia games');
    } finally {
      isRefreshing = false;
      console.log('✅ [LOAD-GAMES] loadDarkadiaGames completed');
    }
  }

  async function loadTabData() {
    try {
      const searchTerm = searchQuery.trim();
      
      if (activeTab === 'needs-attention') {
        const [unmatched, matched] = await Promise.all([
          darkadia.listDarkadiaGames(0, 100000, 'unmatched', searchTerm || undefined),
          darkadia.listDarkadiaGames(0, 100000, 'matched', searchTerm || undefined)
        ]);
        
        unmatchedGames = unmatched.games;
        matchedGames = matched.games;
      } else if (activeTab === 'ignored') {
        const ignored = await darkadia.listDarkadiaGames(0, 100000, 'ignored', searchTerm || undefined);
        ignoredGames = ignored.games;
      } else if (activeTab === 'in-sync') {
        const synced = await darkadia.listDarkadiaGames(0, 100000, 'synced', searchTerm || undefined);
        inSyncGames = synced.games;
      }
    } catch (error) {
      console.error('Failed to load tab data:', error);
    }
  }

  async function handleRefresh() {
    await loadDarkadiaGames();
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
      await darkadia.unmatchAllGames();
      await loadDarkadiaGames(); // Refresh data
    } catch (error) {
      // Error handled in store
    }
  }

  async function handleUnignoreAll() {
    const confirmed = confirm(`This will restore ${ignoredGames.length} ignored games back to your "Needs Attention" list. Are you sure?`);
    
    if (!confirmed) return;
    
    try {
      await darkadia.unignoreAllGames();
      await loadDarkadiaGames(); // Refresh data
    } catch (error) {
      // Error handled in store
    }
  }

  async function handleBatchAutoMatch() {
    console.log('🎯 [BATCH-AUTO-MATCH] Function called, unmatchedCount:', unmatchedCount);
    
    if (unmatchedCount === 0) {
      console.log('❌ [BATCH-AUTO-MATCH] No unmatched games to process');
      ui.showInfo('No unmatched games found to auto-match.');
      return;
    }
    
    const confirmed = confirm(
      `Start batch auto-matching for ${unmatchedCount} unmatched games? ` +
      `This will process games in small batches and you can cancel at any time.`
    );
    
    if (!confirmed) {
      console.log('⏹️ [BATCH-AUTO-MATCH] User cancelled operation');
      return;
    }

    try {
      console.log('🚀 [BATCH-AUTO-MATCH] Starting batch auto-match process...');
      
      // Start the batch session
      const session = await darkadia.startBatchAutoMatch();
      console.log('📋 [BATCH-AUTO-MATCH] Session response:', session);
      
      if (!session) {
        console.error('❌ [BATCH-AUTO-MATCH] No session object returned');
        ui.showError('Failed to start auto-match: No session returned');
        return;
      }
      
      if (!session.session_id) {
        console.error('❌ [BATCH-AUTO-MATCH] Session missing session_id:', session);
        ui.showError('Failed to start auto-match: Invalid session ID');
        return;
      }
      
      console.log('✅ [BATCH-AUTO-MATCH] Valid session received, showing modal');
      
      // Show progress modal
      showBatchModal = true;
      batchProcessingActive = true;
      
      console.log('🎬 [BATCH-AUTO-MATCH] Modal state set - showBatchModal:', showBatchModal, 'batchProcessingActive:', batchProcessingActive);
      
      // Start interval-based batch processing
      startBatchProcessing(session.session_id, 'auto_match');
      
    } catch (error) {
      console.error('❌ [BATCH-AUTO-MATCH] Error during batch auto-match:', error);
      ui.showError(`Failed to start auto-match: ${error instanceof Error ? error.message : 'Unknown error'}`);
      batchProcessingActive = false;
    }
  }

  async function handleBatchSync() {
    if (matchedCount === 0) {
      ui.showInfo('No matched games found to sync.');
      return;
    }
    
    const confirmed = confirm(
      `Start batch sync for ${matchedCount} matched games? ` +
      `This will add games to your collection in small batches and you can cancel at any time.`
    );
    
    if (!confirmed) return;

    try {
      console.log('🚀 [BATCH-SYNC] Starting batch sync process...');
      
      // Start the batch session
      const session = await darkadia.startBatchSync();
      
      if (!session.session_id) {
        // No games to process
        return;
      }
      
      // Show progress modal
      showBatchModal = true;
      batchProcessingActive = true;
      
      // Start interval-based batch processing
      startBatchProcessing(session.session_id, 'sync');
      
    } catch (error) {
      console.error('❌ [BATCH-SYNC] Error during batch sync:', error);
      batchProcessingActive = false;
    }
  }

  function startBatchProcessing(sessionId: string, operationType: 'auto_match' | 'sync') {
    console.log(`🚀 [BATCH-${operationType.toUpperCase()}] Starting sequential batch processing`);
    
    // Clear any existing timeout
    if (processingTimeout) {
      clearTimeout(processingTimeout);
    }
    
    // Start the sequential processing chain
    processNextBatch(sessionId, operationType);
  }
  
  async function processNextBatch(sessionId: string, operationType: 'auto_match' | 'sync') {
    try {
      // Check for cancellation immediately - before any processing
      if (!batchProcessingActive || isCancelling) {
        console.log(`❌ [BATCH-${operationType.toUpperCase()}] Processing cancelled by user`);
        stopBatchProcessing();
        return;
      }
      
      const batchSession = darkadia.value.activeBatchSession;
      
      // Check if processing is complete
      if (!batchSession || batchSession.isComplete || batchSession.status !== 'active') {
        console.log(`✅ [BATCH-${operationType.toUpperCase()}] Batch processing completed`);
        await completeBatchProcessing();
        return;
      }
      
      console.log(`🔄 [BATCH-${operationType.toUpperCase()}] Processing next batch...`);
      
      // Process next batch using the real batch endpoint
      await darkadia.processBatchNext(sessionId);
      
      // Check for cancellation again after batch processing, before UI refresh
      if (!batchProcessingActive || isCancelling) {
        console.log(`❌ [BATCH-${operationType.toUpperCase()}] Processing cancelled after batch completion`);
        stopBatchProcessing();
        return;
      }
      
      // Refresh the current tab data to show progress
      await loadTabData();
      
      // Check for cancellation one more time before scheduling next batch
      if (!batchProcessingActive || isCancelling) {
        console.log(`❌ [BATCH-${operationType.toUpperCase()}] Processing cancelled after UI refresh`);
        stopBatchProcessing();
        return;
      }
      
      // Schedule next batch after a short delay (sequential, not parallel)
      processingTimeout = setTimeout(() => {
        processNextBatch(sessionId, operationType);
      }, 50);
      
    } catch (error) {
      console.error(`❌ [BATCH-${operationType.toUpperCase()}] Error during batch processing:`, error);
      stopBatchProcessing();
    }
  }

  async function completeBatchProcessing() {
    console.log('🎉 [BATCH-COMPLETE] Completing batch processing...');
    stopBatchProcessing();
    
    // Final refresh when done
    try {
      await loadDarkadiaGames();
    } catch (error) {
      console.error('❌ [BATCH-COMPLETE] Error during final refresh:', error);
    }
  }

  function stopBatchProcessing() {
    console.log('⏹️ [BATCH-STOP] Stopping batch processing...');
    
    if (processingTimeout) {
      clearTimeout(processingTimeout);
      processingTimeout = null;
    }
    
    batchProcessingActive = false;
    isCancelling = false;
  }

  function handleBatchModalClose() {
    console.log('🔄 [MODAL-CLOSE] Closing batch modal...');
    showBatchModal = false;
    stopBatchProcessing();
    darkadia.clearBatchSession();
  }

  async function handleBatchCancel() {
    console.log('❌ [CANCEL] User requested cancellation...');
    
    // Set cancellation flag immediately for responsive UI
    isCancelling = true;
    
    // If there's an active session, cancel it properly
    const sessionId = darkadia.value.activeBatchSession?.sessionId;
    if (sessionId) {
      try {
        // Note: This endpoint may need to be implemented
        // await darkadia.cancelBatchOperation(sessionId);
      } catch (error) {
        console.error('❌ [CANCEL] Error cancelling batch operation:', error);
      }
    }
    
    // Stop processing and close modal
    stopBatchProcessing();
    showBatchModal = false;
  }

  async function handleTabChange(tab: 'needs-attention' | 'ignored' | 'in-sync' | 'upload') {
    activeTab = tab;
    if (tab !== 'upload') {
      await loadTabData();
    }
  }

  function toggleUnmatchedCollapsed() {
    unmatchedCollapsed = !unmatchedCollapsed;
  }

  function toggleMatchedCollapsed() {
    matchedCollapsed = !matchedCollapsed;
  }

  // Upload handlers
  async function handleUploadComplete(result: DarkadiaUploadResponse) {
    console.log('Upload completed:', result);
    ui.showSuccess(`Successfully uploaded ${result.total_games} games from your Darkadia CSV`);
    
    // Show import progress modal since import starts automatically after upload
    showImportModal = true;
    
    // Check for pending platform and storefront resolutions (but don't auto-show modal)
    try {
      const [platformResolutions, storefrontResolutions] = await Promise.all([
        platforms.getPendingResolutions(1, 1),
        platforms.getPendingStorefrontResolutions(1, 1)
      ]);
      pendingPlatformResolutions = platformResolutions.total;
      pendingStorefrontResolutions = storefrontResolutions.total;
      console.log('🔄 [PAGE] Fetched fresh resolution counts from backend:', {
        platforms: platformResolutions.total,
        storefronts: storefrontResolutions.total
      });
    } catch (error) {
      console.warn('Failed to check for pending platform/storefront resolutions:', error);
    }
    
    // Switch to needs attention tab after successful upload/import
    setTimeout(async () => {
      await loadDarkadiaGames();
    }, 2000);
  }

  function handleUploadError(error: string) {
    console.error('Upload error:', error);
    ui.showError(`Upload failed: ${error}`);
  }

  // Import progress modal handlers
  function handleImportModalClose() {
    showImportModal = false;
    importCancelling = false;
    darkadia.clearImportJob();
  }

  async function handleImportCancel() {
    if (!darkadia.value.currentImportJob?.id) return;
    
    importCancelling = true;
    
    try {
      await darkadia.cancelImportJob(darkadia.value.currentImportJob.id);
    } catch (error) {
      console.error('Failed to cancel import:', error);
      importCancelling = false;
    }
  }

  // Platform and storefront resolution handlers
  function handleOpenPlatformStorefront() {
    showPlatformStorefrontModal = true;
  }

  function handleClosePlatformStorefront() {
    console.log('🔒 [PAGE] Closing platform storefront modal');
    showPlatformStorefrontModal = false;
  }

  function handlePlatformStorefrontResolutionsComplete(resolvedCount: number) {
    console.log('📥 [PAGE] handlePlatformStorefrontResolutionsComplete called with resolvedCount:', resolvedCount);
    console.log('📊 [PAGE] Current pending counts before update:', {
      platforms: pendingPlatformResolutions,
      storefronts: pendingStorefrontResolutions
    });
    
    ui.showSuccess(`Successfully resolved ${resolvedCount} item${resolvedCount === 1 ? '' : 's'}`);
    
    // Update pending counts (we don't know the exact split, so reduce both conservatively)
    const totalPending = pendingPlatformResolutions + pendingStorefrontResolutions;
    if (totalPending > 0) {
      // Reduce total by resolved count, distribute proportionally
      const platformRatio = pendingPlatformResolutions / totalPending;
      
      const platformReduction = Math.min(pendingPlatformResolutions, Math.ceil(resolvedCount * platformRatio));
      const storefrontReduction = Math.min(pendingStorefrontResolutions, resolvedCount - platformReduction);
      
      pendingPlatformResolutions = Math.max(0, pendingPlatformResolutions - platformReduction);
      pendingStorefrontResolutions = Math.max(0, pendingStorefrontResolutions - storefrontReduction);
    }
    
    console.log('📊 [PAGE] New pending counts after update:', {
      platforms: pendingPlatformResolutions,
      storefronts: pendingStorefrontResolutions
    });
    
    // If all resolutions are complete, refresh the data
    if (pendingPlatformResolutions === 0 && pendingStorefrontResolutions === 0) {
      console.log('🔄 [PAGE] All resolutions complete, refreshing data in 1 second');
      setTimeout(async () => {
        await loadDarkadiaGames();
      }, 1000);
    }
  }

  // Debug platform storefront button visibility
  $effect(() => {
    console.log('🎯 [PAGE] Platform storefront button visibility check:', {
      platformsPending: pendingPlatformResolutions,
      storefrontsPending: pendingStorefrontResolutions,
      totalPending: pendingPlatformResolutions + pendingStorefrontResolutions,
      shouldShow: (pendingPlatformResolutions + pendingStorefrontResolutions) > 0
    });
  });

  // Reactive search using proper Svelte 5 dependency tracking
  $effect(() => {
    // Read searchQuery to establish dependency tracking
    searchQuery;
    
    // Only execute search if we have data loaded
    if (!isLoading && totalCount > 0) {
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

  // Derived values for reactive display
  const needsAttentionCount = $derived(unmatchedCount + matchedCount);
  const hasNeedsAttention = $derived(needsAttentionCount > 0);
  const hasInSync = $derived(syncedCount > 0);
  const hasAnyGames = $derived(totalCount > 0);

  // Initialize Darkadia configuration and games data
  async function initializeDarkadiaData() {
    try {
      console.log('🔧 [DARKADIA-PAGE] Starting Darkadia data initialization...');
      
      // Check if we already have games imported
      await loadDarkadiaGames();
      
      // If no games, show upload tab by default
      if (totalCount === 0) {
        console.log('📤 [DARKADIA-PAGE] No games found, showing upload tab');
        activeTab = 'upload';
      } else {
        console.log('🎮 [DARKADIA-PAGE] Games found, showing appropriate tab');
        
        // Check for pending platform and storefront resolutions
        try {
          const [platformResolutions, storefrontResolutions] = await Promise.all([
            platforms.getPendingResolutions(1, 1),
            platforms.getPendingStorefrontResolutions(1, 1)
          ]);
          pendingPlatformResolutions = platformResolutions.total;
          pendingStorefrontResolutions = storefrontResolutions.total;
          console.log(`🔗 [DARKADIA-PAGE] Found pending resolutions:`, {
            platforms: pendingPlatformResolutions,
            storefronts: pendingStorefrontResolutions
          });
        } catch (error) {
          console.warn('Failed to check for pending platform/storefront resolutions on init:', error);
        }
      }
    } catch (error) {
      // No games imported yet, show upload tab
      console.log('📤 [DARKADIA-PAGE] No games imported yet, showing upload tab:', error);
      activeTab = 'upload';
    } finally {
      console.log('✅ [DARKADIA-PAGE] Darkadia data initialization completed');
      isLoading = false;
    }
  }

  // Reset handlers
  function handleOpenResetModal() {
    showResetModal = true;
  }

  function handleCloseResetModal() {
    showResetModal = false;
  }

  async function handleConfirmReset() {
    try {
      await darkadia.resetImport();
      showResetModal = false;
      
      // Refresh the data after reset
      await initializeDarkadiaData();
    } catch (error) {
      console.error('Failed to reset Darkadia import:', error);
    }
  }

</script>

<svelte:head>
  <title>Darkadia Import - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="space-y-6">
    <!-- IGDB Warning Banner -->
    <IgdbWarningBanner />

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
            <span class="text-gray-900 font-medium">Darkadia Import</span>
          </li>
        </ol>
      </nav>
      
      <div class="mt-4 flex flex-col sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">
            Darkadia CSV Import
          </h1>
          <p class="mt-1 text-sm text-gray-500">
            Import and manage your game collection from Darkadia CSV exports
          </p>
        </div>
        
        <div class="mt-4 sm:mt-0 flex space-x-3">
          {#if hasAnyGames}
            <button
              onclick={handleRefresh}
              disabled={isRefreshing}
              class="btn-secondary disabled:opacity-50"
              aria-label="Refresh Darkadia games data"
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
            
            {#if unmatchedCount > 0}
              <button
                onclick={handleBatchAutoMatch}
                disabled={darkadia.value.isBatchProcessing || batchProcessingActive}
                class="btn-secondary disabled:opacity-50"
                title="Auto-match unmatched games to IGDB"
              >
                {#if darkadia.value.isBatchProcessing || batchProcessingActive}
                  <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
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
            
            {#if (pendingPlatformResolutions + pendingStorefrontResolutions) > 0}
              <button
                onclick={handleOpenPlatformStorefront}
                class="btn-secondary"
                title="Resolve platforms and storefronts from CSV import"
              >
                <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                </svg>
                Platforms and Storefronts ({pendingPlatformResolutions + pendingStorefrontResolutions})
              </button>
            {:else}
              <!-- Show button for reviewing mappings when no pending resolutions -->
              <button
                onclick={handleOpenPlatformStorefront}
                class="btn-secondary"
                title="Review platform and storefront mappings"
              >
                <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
                </svg>
                Platforms and Storefronts
              </button>
            {/if}

            <!-- Reset Button -->
            <button
              onclick={handleOpenResetModal}
              class="inline-flex items-center px-3 py-2 border border-red-300 shadow-sm text-sm leading-4 font-medium rounded-md text-red-700 bg-white hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500"
              title="Reset all Darkadia import data"
            >
              <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
              Reset Import
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
            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <p class="mt-2 text-sm text-gray-500">Loading Darkadia games...</p>
        </div>
      </div>
    {:else}
      <!-- Stats Overview -->
      {#if hasAnyGames}
        <div class="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
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

          {#if (pendingPlatformResolutions + pendingStorefrontResolutions) > 0}
            <div class="bg-white overflow-hidden shadow rounded-lg">
              <div class="p-5">
                <div class="flex items-center">
                  <div class="flex-shrink-0">
                    <span class="text-2xl">🔗</span>
                  </div>
                  <div class="ml-5 w-0 flex-1">
                    <dl>
                      <dt class="text-sm font-medium text-gray-500 truncate">Resolution Needed</dt>
                      <dd class="text-lg font-medium text-gray-900">
                        {pendingPlatformResolutions + pendingStorefrontResolutions}
                        <div class="text-xs text-gray-500 mt-1">
                          {pendingPlatformResolutions} platforms, {pendingStorefrontResolutions} storefronts
                        </div>
                      </dd>
                    </dl>
                  </div>
                </div>
              </div>
              <div class="bg-yellow-50 px-5 py-3">
                <button
                  onclick={handleOpenPlatformStorefront}
                  class="text-xs font-medium text-yellow-800 hover:text-yellow-900 underline"
                >
                  Resolve Now →
                </button>
              </div>
            </div>
          {/if}
        </div>
      {/if}

      <!-- Tab Navigation -->
      <div class="border-b border-gray-200">
        <nav class="-mb-px flex space-x-8" aria-label="Tabs">
          <button
            onclick={() => handleTabChange('needs-attention')}
            disabled={!hasAnyGames && activeTab !== 'needs-attention'}
            class="border-b-2 py-2 px-1 text-sm font-medium {!hasAnyGames && activeTab !== 'needs-attention' ? 'border-transparent text-gray-300 cursor-not-allowed' : (activeTab === 'needs-attention' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300')}"
          >
            Needs Attention
            {#if hasNeedsAttention}
              <span class="ml-2 bg-red-100 text-red-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {needsAttentionCount}
              </span>
            {/if}
          </button>
          <button
            onclick={() => handleTabChange('ignored')}
            disabled={!hasAnyGames && activeTab !== 'ignored'}
            class="border-b-2 py-2 px-1 text-sm font-medium {!hasAnyGames && activeTab !== 'ignored' ? 'border-transparent text-gray-300 cursor-not-allowed' : (activeTab === 'ignored' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300')}"
          >
            Ignored
            {#if ignoredCount > 0}
              <span class="ml-2 bg-gray-100 text-gray-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {ignoredCount}
              </span>
            {/if}
          </button>
          <button
            onclick={() => handleTabChange('in-sync')}
            disabled={!hasAnyGames && activeTab !== 'in-sync'}
            class="border-b-2 py-2 px-1 text-sm font-medium {!hasAnyGames && activeTab !== 'in-sync' ? 'border-transparent text-gray-300 cursor-not-allowed' : (activeTab === 'in-sync' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300')}"
          >
            In Sync
            {#if hasInSync}
              <span class="ml-2 bg-green-100 text-green-600 py-0.5 px-2.5 rounded-full text-xs font-medium">
                {syncedCount}
              </span>
            {/if}
          </button>
          <button
            onclick={() => handleTabChange('upload')}
            class="border-b-2 py-2 px-1 text-sm font-medium {activeTab === 'upload' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
          >
            📤 Upload CSV
          </button>
        </nav>
      </div>

      <!-- Search Bar (only show for game tabs) -->
      {#if activeTab !== 'upload' && hasAnyGames}
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
              placeholder="Search Darkadia games..."
              class="block w-full pl-10 pr-3 py-2 border border-gray-300 rounded-md leading-5 bg-white placeholder-gray-500 focus:outline-none focus:placeholder-gray-400 focus:ring-1 focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
            />
          </div>
        </div>
      {/if}

      <!-- Tab Content -->
      {#if activeTab === 'upload'}
        <!-- Upload Tab Content -->
        <div class="space-y-6">
          <div class="bg-white rounded-lg shadow-lg p-8">
            <h2 class="text-xl font-semibold text-gray-900 mb-6">Upload Darkadia CSV</h2>
            
            <DarkadiaFileUpload
              onUploadComplete={handleUploadComplete}
              onUploadError={handleUploadError}
            />
          </div>

          <!-- Instructions -->
          <div class="bg-white rounded-lg shadow-lg p-8">
            <h2 class="text-xl font-semibold text-gray-900 mb-6">How to Export from Darkadia</h2>
            
            <div class="space-y-6">
              <div class="flex items-start">
                <div class="flex-shrink-0 w-8 h-8 bg-blue-600 text-white rounded-full flex items-center justify-center text-sm font-semibold">
                  1
                </div>
                <div class="ml-4">
                  <h3 class="text-lg font-medium text-gray-900">Export Your Collection</h3>
                  <p class="text-gray-600 mt-1">
                    In Darkadia:
                    <br />1. Login to your account
                    <br />2. Go to Settings → Extras
                    <br />3. Click the Download button to export your collection as CSV
                  </p>
                </div>
              </div>

              <div class="flex items-start">
                <div class="flex-shrink-0 w-8 h-8 bg-blue-600 text-white rounded-full flex items-center justify-center text-sm font-semibold">
                  2
                </div>
                <div class="ml-4">
                  <h3 class="text-lg font-medium text-gray-900">Upload Here</h3>
                  <p class="text-gray-600 mt-1">
                    Drag and drop your CSV file above or click to select it. The import process will start automatically.
                  </p>
                </div>
              </div>

              <div class="flex items-start">
                <div class="flex-shrink-0 w-8 h-8 bg-blue-600 text-white rounded-full flex items-center justify-center text-sm font-semibold">
                  3
                </div>
                <div class="ml-4">
                  <h3 class="text-lg font-medium text-gray-900">Review & Manage</h3>
                  <p class="text-gray-600 mt-1">
                    After import, review your games in the other tabs. You can manually match unrecognized titles and sync games to your collection.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      {:else if activeTab === 'needs-attention'}
        <div class="space-y-6">
          {#if !hasAnyGames}
            <!-- No games imported yet -->
            <div class="text-center py-12">
              <span class="text-6xl">📤</span>
              <h3 class="mt-2 text-lg font-medium text-gray-900">No games imported yet</h3>
              <p class="mt-1 text-sm text-gray-500">
                Upload your Darkadia CSV file to start importing your games.
              </p>
              <div class="mt-6">
                <button
                  onclick={() => handleTabChange('upload')}
                  class="btn-primary"
                >
                  Upload CSV File
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
                      onclick={handleBatchSync}
                      disabled={darkadia.value.isBatchProcessing || batchProcessingActive}
                      class="btn-primary disabled:opacity-50"
                    >
                      {#if darkadia.value.isBatchProcessing || batchProcessingActive}
                        <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        Syncing...
                      {:else}
                        Sync All Matched
                      {/if}
                    </button>
                    <button
                      onclick={handleUnmatchAll}
                      disabled={darkadia.value.isUnmatchingAll}
                      class="btn-secondary text-orange-600 hover:text-orange-700 disabled:opacity-50"
                    >
                      {#if darkadia.value.isUnmatchingAll}
                        <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                          <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        Unmatching...
                      {:else}
                        Unmatch All
                      {/if}
                    </button>
                    
                    <!-- Platform and Storefront Resolution Button -->
                    {#if (pendingPlatformResolutions + pendingStorefrontResolutions) > 0}
                      <button
                        onclick={() => {
                          console.log('🎯 [PAGE] Platform storefront button clicked. Current counts:', {
                            platforms: pendingPlatformResolutions,
                            storefronts: pendingStorefrontResolutions
                          });
                          handleOpenPlatformStorefront();
                        }}
                        class="btn-secondary text-purple-600 hover:text-purple-700 border-purple-300 hover:border-purple-400"
                      >
                        <svg class="-ml-1 mr-2 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                        </svg>
                        Platforms and Storefronts
                        <span class="ml-2 bg-purple-100 text-purple-600 py-0.5 px-2 rounded-full text-xs font-medium">
                          {pendingPlatformResolutions + pendingStorefrontResolutions}
                        </span>
                      </button>
                    {/if}
                  </div>
                </div>
              </div>
            {/if}

            <!-- Needs Attention Tables -->
            <div class="space-y-8">
              <!-- Unmatched Games Section -->
              {#if unmatchedGames.length > 0}
                <DarkadiaGamesTable
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
                <DarkadiaGamesTable
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
                    No Darkadia games need attention right now.
                  </p>
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {:else if activeTab === 'ignored'}
        <div class="space-y-6">
          {#if !hasAnyGames}
            <!-- No games imported yet -->
            <div class="text-center py-12">
              <span class="text-6xl">📤</span>
              <h3 class="mt-2 text-lg font-medium text-gray-900">No games imported yet</h3>
              <p class="mt-1 text-sm text-gray-500">
                Upload your Darkadia CSV file to start importing your games.
              </p>
              <div class="mt-6">
                <button
                  onclick={() => handleTabChange('upload')}
                  class="btn-primary"
                >
                  Upload CSV File
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
                    disabled={darkadia.value.isUnignoringAll}
                    class="btn-primary disabled:opacity-50"
                  >
                    {#if darkadia.value.isUnignoringAll}
                      <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
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
              <DarkadiaGamesTable
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
          {#if !hasAnyGames}
            <!-- No games imported yet -->
            <div class="text-center py-12">
              <span class="text-6xl">📤</span>
              <h3 class="mt-2 text-lg font-medium text-gray-900">No games imported yet</h3>
              <p class="mt-1 text-sm text-gray-500">
                Upload your Darkadia CSV file to start importing your games.
              </p>
              <div class="mt-6">
                <button
                  onclick={() => handleTabChange('upload')}
                  class="btn-primary"
                >
                  Upload CSV File
                </button>
              </div>
            </div>
          {:else}
            <!-- In Sync Games Section -->
            {#if inSyncGames.length > 0}
              <DarkadiaGamesTable
                title="Games in Collection"
                description="These Darkadia games have been successfully added to your main game collection."
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
                  Import and sync your Darkadia games to see them here.
                </p>
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    {/if}
  </div>
</RouteGuard>

<!-- Batch Progress Modal -->
<BatchProgressModal
  isOpen={showBatchModal}
  onClose={handleBatchModalClose}
  onCancel={handleBatchCancel}
  isCancelling={isCancelling}
  store="darkadia"
/>

<!-- Import Progress Modal -->
<ImportProgressModal
  isOpen={showImportModal}
  onClose={handleImportModalClose}
  onCancel={handleImportCancel}
  isCancelling={importCancelling}
  importJob={darkadia.value.currentImportJob}
/>

<!-- Platform and Storefront Resolution Modal -->
<PlatformStorefrontModal
  isOpen={showPlatformStorefrontModal}
  onClose={handleClosePlatformStorefront}
  onResolutionsComplete={handlePlatformStorefrontResolutionsComplete}
/>

<!-- Reset Confirmation Modal -->
{#if showResetModal}
  <div 
    class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50" 
    onclick={handleCloseResetModal}
    onkeydown={(e) => e.key === 'Escape' && handleCloseResetModal()}
    role="dialog"
    aria-modal="true"
    aria-labelledby="reset-modal-title"
    tabindex="-1"
  >
    <div 
      class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white" 
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      role="presentation"
    >
      <div class="mt-3">
        <!-- Warning Icon -->
        <div class="mx-auto flex items-center justify-center h-12 w-12 rounded-full bg-red-100 mb-4">
          <svg class="h-8 w-8 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.268 16.5c-.77.833.192 2.5 1.732 2.5z" />
          </svg>
        </div>
        
        <!-- Title -->
        <h3 id="reset-modal-title" class="text-lg font-semibold text-gray-900 text-center mb-2">
          Reset Darkadia Import
        </h3>
        
        <!-- Warning Message -->
        <div class="bg-red-50 border border-red-200 rounded-lg p-4 mb-4">
          <div class="text-sm text-red-800">
            <p class="font-medium mb-2">⚠️ This action cannot be undone!</p>
            <p class="mb-2">This will permanently:</p>
            <ul class="list-disc list-inside space-y-1 text-xs">
              <li>Remove all synced games from your collection</li>
              <li>Delete all Darkadia staging games</li>
              <li>Clear all import tracking records</li>
              <li>Reset your Darkadia configuration</li>
              <li>Delete your uploaded CSV file</li>
            </ul>
          </div>
        </div>
        
        <!-- Buttons -->
        <div class="flex gap-3">
          <button
            onclick={handleCloseResetModal}
            class="flex-1 btn-secondary"
          >
            Cancel
          </button>
          <button
            onclick={handleConfirmReset}
            class="flex-1 inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500"
          >
            <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
            Yes, Reset Import
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}