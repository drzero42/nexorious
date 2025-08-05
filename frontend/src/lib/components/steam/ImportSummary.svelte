<script lang="ts">
  import { steamImport } from '$lib/stores/steam-import.svelte';

  // Get current job data
  $: job = steamImport.value.currentJob;
  $: userDecisions = steamImport.value.userDecisions;

  // Calculate final import statistics
  $: finalStats = (() => {
    if (!job) return null;

    // Count games by their final import action
    const newGamesCount = Object.values(userDecisions).filter(
      decision => decision.action === 'import' && decision.igdb_id
    ).length + job.matched_games;

    const skippedCount = Object.values(userDecisions).filter(
      decision => decision.action === 'skip'
    ).length;

    const existingGamesCount = job.platform_added_games;

    return {
      totalGames: job.total_games,
      newGames: newGamesCount,
      updatedGames: existingGamesCount,
      skippedGames: skippedCount,
      processedGames: newGamesCount + existingGamesCount + skippedCount
    };
  })();

  // Get categorized games for preview
  $: categorizedGames = (() => {
    if (!job?.games) return { newGames: [], updatedGames: [], skippedGames: [] };

    const newGames = job.games.filter(game => 
      game.status === 'matched' || 
      (game.status === 'awaiting_user' && userDecisions[game.steam_appid.toString()]?.action === 'import')
    );

    const updatedGames = job.games.filter(game => game.status === 'platform_added');

    const skippedGames = job.games.filter(game => 
      game.status === 'skipped' || 
      (game.status === 'awaiting_user' && userDecisions[game.steam_appid.toString()]?.action === 'skip')
    );

    return { newGames, updatedGames, skippedGames };
  })();

  function getEstimatedStorageSize(): string {
    if (!finalStats) return '0 MB';
    
    // Rough estimate: ~500KB per game for cover art and metadata
    const estimatedMB = Math.ceil((finalStats.newGames * 0.5) + (finalStats.updatedGames * 0.1));
    
    if (estimatedMB < 1) return '< 1 MB';
    if (estimatedMB < 1024) return `${estimatedMB} MB`;
    return `${(estimatedMB / 1024).toFixed(1)} GB`;
  }

  function getProcessingTimeEstimate(): string {
    if (!finalStats) return '0 minutes';
    
    // Rough estimate: 2-3 seconds per new game, 1 second per update
    const estimatedSeconds = (finalStats.newGames * 2.5) + (finalStats.updatedGames * 1);
    
    if (estimatedSeconds < 60) return `${Math.ceil(estimatedSeconds)} seconds`;
    const minutes = Math.ceil(estimatedSeconds / 60);
    if (minutes === 1) return '1 minute';
    if (minutes < 60) return `${minutes} minutes`;
    return `${Math.ceil(minutes / 60)} hour${minutes >= 120 ? 's' : ''}`;
  }
</script>

{#if job && finalStats}
  <div class="card space-y-6">
    <!-- Header -->
    <div>
      <h2 class="text-xl font-bold text-gray-900 mb-2">Import Summary</h2>
      <p class="text-gray-600">
        Review what will be imported to your game collection
      </p>
    </div>

    <!-- Overview Statistics -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4 p-4 bg-gray-50 rounded-lg">
      <div class="text-center">
        <div class="text-2xl font-bold text-green-600">{finalStats.newGames}</div>
        <div class="text-sm text-gray-700 font-medium">New Games</div>
        <div class="text-xs text-gray-500">Will be imported</div>
      </div>
      
      <div class="text-center">
        <div class="text-2xl font-bold text-blue-600">{finalStats.updatedGames}</div>
        <div class="text-sm text-gray-700 font-medium">Updated Games</div>
        <div class="text-xs text-gray-500">Steam platform added</div>
      </div>
      
      <div class="text-center">
        <div class="text-2xl font-bold text-orange-600">{finalStats.skippedGames}</div>
        <div class="text-sm text-gray-700 font-medium">Skipped Games</div>
        <div class="text-xs text-gray-500">Will not be imported</div>
      </div>
      
      <div class="text-center">
        <div class="text-2xl font-bold text-purple-600">{finalStats.totalGames}</div>
        <div class="text-sm text-gray-700 font-medium">Total Games</div>
        <div class="text-xs text-gray-500">From Steam library</div>
      </div>
    </div>

    <!-- Import Impact -->
    <div class="space-y-4">
      <h3 class="text-lg font-semibold text-gray-900">Import Impact</h3>
      
      <div class="grid md:grid-cols-2 gap-4">
        <!-- Storage Impact -->
        <div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
          <div class="flex items-center">
            <svg class="h-6 w-6 text-blue-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2.21 1.79 4 4 4h8c2.21 0 4-1.79 4-4V7c0-2.21-1.79-4-4-4H8c-2.21 0-4 1.79-4 4z" />
            </svg>
            <div>
              <div class="text-sm font-medium text-blue-800">Estimated Storage</div>
              <div class="text-lg font-bold text-blue-900">{getEstimatedStorageSize()}</div>
              <div class="text-xs text-blue-600">Cover art and metadata</div>
            </div>
          </div>
        </div>

        <!-- Processing Time -->
        <div class="bg-green-50 border border-green-200 rounded-lg p-4">
          <div class="flex items-center">
            <svg class="h-6 w-6 text-green-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <div>
              <div class="text-sm font-medium text-green-800">Processing Time</div>
              <div class="text-lg font-bold text-green-900">{getProcessingTimeEstimate()}</div>
              <div class="text-xs text-green-600">Estimated duration</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Game Previews -->
    <div class="space-y-4">
      <h3 class="text-lg font-semibold text-gray-900">Game Previews</h3>
      
      <!-- New Games Preview -->
      {#if categorizedGames.newGames.length > 0}
        <div class="border border-green-200 rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h4 class="text-sm font-medium text-green-800 flex items-center">
              <svg class="h-4 w-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 3a1 1 0 011 1v5h5a1 1 0 110 2h-5v5a1 1 0 11-2 0v-5H4a1 1 0 110-2h5V4a1 1 0 011-1z" clip-rule="evenodd" />
              </svg>
              New Games ({categorizedGames.newGames.length})
            </h4>
            <span class="text-xs text-green-600 bg-green-100 px-2 py-1 rounded">
              Will be imported from IGDB
            </span>
          </div>
          
          <div class="grid gap-2 max-h-32 overflow-y-auto">
            {#each categorizedGames.newGames.slice(0, 10) as game}
              <div class="flex items-center justify-between py-1 px-2 bg-green-50 rounded text-sm">
                <span class="font-medium text-green-800 truncate mr-2">{game.steam_name}</span>
                <span class="text-xs text-green-600 bg-green-100 px-2 py-0.5 rounded flex-shrink-0">
                  Steam ID: {game.steam_appid}
                </span>
              </div>
            {/each}
            {#if categorizedGames.newGames.length > 10}
              <div class="text-xs text-green-600 text-center py-1">
                +{categorizedGames.newGames.length - 10} more games
              </div>
            {/if}
          </div>
        </div>
      {/if}

      <!-- Updated Games Preview -->
      {#if categorizedGames.updatedGames.length > 0}
        <div class="border border-blue-200 rounded-lg p-4">
          <div class="flex items-center justify-between mb-3">
            <h4 class="text-sm font-medium text-blue-800 flex items-center">
              <svg class="h-4 w-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z" clip-rule="evenodd" />
              </svg>
              Updated Games ({categorizedGames.updatedGames.length})
            </h4>
            <span class="text-xs text-blue-600 bg-blue-100 px-2 py-1 rounded">
              Steam platform will be added
            </span>
          </div>
          
          <div class="grid gap-2 max-h-32 overflow-y-auto">
            {#each categorizedGames.updatedGames.slice(0, 10) as game}
              <div class="flex items-center justify-between py-1 px-2 bg-blue-50 rounded text-sm">
                <span class="font-medium text-blue-800 truncate mr-2">{game.steam_name}</span>
                <span class="text-xs text-blue-600 bg-blue-100 px-2 py-0.5 rounded flex-shrink-0">
                  Already owned
                </span>
              </div>
            {/each}
            {#if categorizedGames.updatedGames.length > 10}
              <div class="text-xs text-blue-600 text-center py-1">
                +{categorizedGames.updatedGames.length - 10} more games
              </div>
            {/if}
          </div>
        </div>
      {/if}

      <!-- Skipped Games Preview -->
      {#if categorizedGames.skippedGames.length > 0}
        <details class="border border-orange-200 rounded-lg">
          <summary class="p-4 cursor-pointer hover:bg-orange-50">
            <div class="flex items-center justify-between">
              <h4 class="text-sm font-medium text-orange-800 flex items-center">
                <svg class="h-4 w-4 mr-2" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M13.477 14.89A6 6 0 015.11 6.524l8.367 8.368zm1.414-1.414L6.524 5.11a6 6 0 018.367 8.367zM18 10a8 8 0 11-16 0 8 8 0 0116 0z" clip-rule="evenodd" />
                </svg>
                Skipped Games ({categorizedGames.skippedGames.length})
              </h4>
              <span class="text-xs text-orange-600 bg-orange-100 px-2 py-1 rounded">
                Will not be imported
              </span>
            </div>
          </summary>
          
          <div class="px-4 pb-4">
            <div class="grid gap-2 max-h-32 overflow-y-auto">
              {#each categorizedGames.skippedGames.slice(0, 10) as game}
                <div class="flex items-center justify-between py-1 px-2 bg-orange-50 rounded text-sm">
                  <span class="font-medium text-orange-800 truncate mr-2">{game.steam_name}</span>
                  <span class="text-xs text-orange-600 bg-orange-100 px-2 py-0.5 rounded flex-shrink-0">
                    Skipped
                  </span>
                </div>
              {/each}
              {#if categorizedGames.skippedGames.length > 10}
                <div class="text-xs text-orange-600 text-center py-1">
                  +{categorizedGames.skippedGames.length - 10} more games
                </div>
              {/if}
            </div>
          </div>
        </details>
      {/if}
    </div>

    <!-- Final Summary -->
    <div class="bg-gradient-to-r from-blue-50 to-green-50 border border-blue-200 rounded-lg p-4">
      <div class="flex items-center">
        <svg class="h-6 w-6 text-blue-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4M7.835 4.697a3.42 3.42 0 001.946-.806 3.42 3.42 0 014.438 0 3.42 3.42 0 001.946.806 3.42 3.42 0 013.138 3.138 3.42 3.42 0 00.806 1.946 3.42 3.42 0 010 4.438 3.42 3.42 0 00-.806 1.946 3.42 3.42 0 01-3.138 3.138 3.42 3.42 0 00-1.946.806 3.42 3.42 0 01-4.438 0 3.42 3.42 0 00-1.946-.806 3.42 3.42 0 01-3.138-3.138 3.42 3.42 0 00-.806-1.946 3.42 3.42 0 010-4.438 3.42 3.42 0 00.806-1.946 3.42 3.42 0 013.138-3.138z" />
        </svg>
        <div>
          <h4 class="text-sm font-medium text-blue-800">Ready to Import</h4>
          <p class="text-sm text-blue-700">
            {finalStats.newGames + finalStats.updatedGames} games will be added to your collection out of {finalStats.totalGames} Steam games processed.
          </p>
        </div>
      </div>
    </div>
  </div>
{:else}
  <!-- Loading State -->
  <div class="card">
    <div class="animate-pulse">
      <div class="h-4 bg-gray-200 rounded w-3/4 mb-4"></div>
      <div class="h-8 bg-gray-200 rounded mb-4"></div>
      <div class="h-4 bg-gray-200 rounded w-1/2"></div>
    </div>
  </div>
{/if}

<style>
  /* Custom scrollbar for game previews */
  .max-h-32::-webkit-scrollbar {
    width: 4px;
  }
  
  .max-h-32::-webkit-scrollbar-track {
    background: #f1f5f9;
    border-radius: 2px;
  }
  
  .max-h-32::-webkit-scrollbar-thumb {
    background: #cbd5e1;
    border-radius: 2px;
  }
  
  .max-h-32::-webkit-scrollbar-thumb:hover {
    background: #94a3b8;
  }
</style>