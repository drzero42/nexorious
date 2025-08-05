<script lang="ts">
  import { steamImport } from '$lib/stores/steam-import.svelte';
  import { goto } from '$app/navigation';

  // Get current job data
  const job = $derived(steamImport.value.currentJob);

  // Categorize games by their final status
  const categorizedResults = $derived((() => {
    if (!job?.games) return { newGames: [], updatedGames: [], skippedGames: [], alreadyOwnedGames: [] };

    const newGames = job.games.filter(game => game.status === 'imported');
    const updatedGames = job.games.filter(game => game.status === 'platform_added');
    const skippedGames = job.games.filter(game => game.status === 'skipped');
    const alreadyOwnedGames = job.games.filter(game => game.status === 'already_owned');

    return { newGames, updatedGames, skippedGames, alreadyOwnedGames };
  })());

  // Calculate collection impact
  const collectionImpact = $derived((() => {
    if (!job) return null;

    const totalAdded = job.imported_games + job.platform_added_games;
    const beforeCount = totalAdded > 0 ? Math.max(0, totalAdded - job.imported_games) : 0; // Rough estimate
    const afterCount = beforeCount + totalAdded;

    return {
      before: beforeCount,
      after: afterCount,
      growth: totalAdded,
      growthPercentage: beforeCount > 0 ? Math.round((totalAdded / beforeCount) * 100) : 0
    };
  })());

  function handleViewGame(gameId: string) {
    goto(`/games/${gameId}`);
  }


  function getCategoryIcon(category: string): string {
    switch (category) {
      case 'new': return 'M12 4v16m8-8H4'; // plus
      case 'updated': return 'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15'; // refresh
      case 'skipped': return 'M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21'; // eye-slash
      case 'already_owned': return 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z'; // check-circle
      default: return 'M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z';
    }
  }

  let activeTab = $state<string>('new');
</script>

{#if job}
  <div class="space-y-6">
    <!-- Results Overview Cards -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <!-- New Games -->
      <button
        onclick={() => activeTab = 'new'}
        class="p-4 bg-green-50 border-2 rounded-lg text-center hover:bg-green-100 transition-colors
               {activeTab === 'new' ? 'border-green-500' : 'border-green-200'}"
      >
        <div class="text-2xl font-bold text-green-600">{job.imported_games}</div>
        <div class="text-sm text-green-700 font-medium">New Games</div>
        <div class="text-xs text-green-600">Imported from IGDB</div>
      </button>

      <!-- Updated Games -->
      <button
        onclick={() => activeTab = 'updated'}
        class="p-4 bg-blue-50 border-2 rounded-lg text-center hover:bg-blue-100 transition-colors
               {activeTab === 'updated' ? 'border-blue-500' : 'border-blue-200'}"
      >
        <div class="text-2xl font-bold text-blue-600">{job.platform_added_games}</div>
        <div class="text-sm text-blue-700 font-medium">Updated Games</div>
        <div class="text-xs text-blue-600">Steam platform added</div>
      </button>

      <!-- Already Owned -->
      <button
        onclick={() => activeTab = 'already_owned'}
        class="p-4 bg-purple-50 border-2 rounded-lg text-center hover:bg-purple-100 transition-colors
               {activeTab === 'already_owned' ? 'border-purple-500' : 'border-purple-200'}"
      >
        <div class="text-2xl font-bold text-purple-600">{categorizedResults.alreadyOwnedGames.length}</div>
        <div class="text-sm text-purple-700 font-medium">Already Owned</div>
        <div class="text-xs text-purple-600">No action needed</div>
      </button>

      <!-- Skipped Games -->
      <button
        onclick={() => activeTab = 'skipped'}
        class="p-4 bg-orange-50 border-2 rounded-lg text-center hover:bg-orange-100 transition-colors
               {activeTab === 'skipped' ? 'border-orange-500' : 'border-orange-200'}"
      >
        <div class="text-2xl font-bold text-orange-600">{job.skipped_games}</div>
        <div class="text-sm text-orange-700 font-medium">Skipped</div>
        <div class="text-xs text-orange-600">Not imported</div>
      </button>
    </div>

    <!-- Collection Impact -->
    {#if collectionImpact && collectionImpact.growth > 0}
      <div class="card bg-gradient-to-r from-blue-50 to-green-50 border-blue-200">
        <h3 class="text-lg font-semibold text-gray-900 mb-4 flex items-center">
          <svg class="h-5 w-5 mr-2 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-4 4" />
          </svg>
          Collection Impact
        </h3>
        
        <div class="grid md:grid-cols-3 gap-4">
          <div class="text-center p-4 bg-white rounded-lg border">
            <div class="text-2xl font-bold text-gray-600">{collectionImpact.before}</div>
            <div class="text-sm text-gray-700">Games Before</div>
          </div>
          
          <div class="text-center p-4 bg-white rounded-lg border border-green-200">
            <div class="text-2xl font-bold text-green-600">+{collectionImpact.growth}</div>
            <div class="text-sm text-green-700">Games Added</div>
          </div>
          
          <div class="text-center p-4 bg-white rounded-lg border">
            <div class="text-2xl font-bold text-blue-600">{collectionImpact.after}</div>
            <div class="text-sm text-blue-700">Games After</div>
          </div>
        </div>

        {#if collectionImpact.growthPercentage > 0}
          <div class="mt-4 text-center">
            <div class="inline-flex items-center px-3 py-1 bg-green-100 text-green-800 rounded-full text-sm">
              <svg class="h-4 w-4 mr-1" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M3.293 9.707a1 1 0 010-1.414l6-6a1 1 0 011.414 0l6 6a1 1 0 01-1.414 1.414L11 5.414V17a1 1 0 11-2 0V5.414L4.707 9.707a1 1 0 01-1.414 0z" clip-rule="evenodd" />
              </svg>
              {collectionImpact.growthPercentage}% collection growth
            </div>
          </div>
        {/if}
      </div>
    {/if}

    <!-- Game Lists by Category -->
    <div class="card">
      <div class="border-b border-gray-200 pb-4 mb-6">
        <h3 class="text-lg font-semibold text-gray-900">Import Details</h3>
      </div>

      <!-- Tab Content -->
      {#if activeTab === 'new' && categorizedResults.newGames.length > 0}
        <div>
          <div class="flex items-center mb-4">
            <svg class="h-5 w-5 mr-2 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="{getCategoryIcon('new')}" />
            </svg>
            <h4 class="text-md font-medium text-green-800">
              New Games Added ({categorizedResults.newGames.length})
            </h4>
          </div>
          
          <div class="grid gap-3 max-h-96 overflow-y-auto">
            {#each categorizedResults.newGames as game}
              <div class="flex items-center justify-between p-3 bg-green-50 border border-green-200 rounded-lg">
                <div class="flex-1">
                  <div class="font-medium text-green-800">{game.steam_name}</div>
                  <div class="text-sm text-green-600">Steam ID: {game.steam_appid}</div>
                </div>
                {#if game.matched_game_id}
                  <button
                    onclick={() => handleViewGame(game.matched_game_id!)}
                    class="text-sm text-green-600 hover:text-green-500 font-medium"
                  >
                    View Game →
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        </div>

      {:else if activeTab === 'updated' && categorizedResults.updatedGames.length > 0}
        <div>
          <div class="flex items-center mb-4">
            <svg class="h-5 w-5 mr-2 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="{getCategoryIcon('updated')}" />
            </svg>
            <h4 class="text-md font-medium text-blue-800">
              Updated Games ({categorizedResults.updatedGames.length})
            </h4>
          </div>
          
          <div class="grid gap-3 max-h-96 overflow-y-auto">
            {#each categorizedResults.updatedGames as game}
              <div class="flex items-center justify-between p-3 bg-blue-50 border border-blue-200 rounded-lg">
                <div class="flex-1">
                  <div class="font-medium text-blue-800">{game.steam_name}</div>
                  <div class="text-sm text-blue-600">Steam platform added to existing game</div>
                </div>
                {#if game.matched_game_id}
                  <button
                    onclick={() => handleViewGame(game.matched_game_id!)}
                    class="text-sm text-blue-600 hover:text-blue-500 font-medium"
                  >
                    View Game →
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        </div>

      {:else if activeTab === 'already_owned' && categorizedResults.alreadyOwnedGames.length > 0}
        <div>
          <div class="flex items-center mb-4">
            <svg class="h-5 w-5 mr-2 text-purple-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="{getCategoryIcon('already_owned')}" />
            </svg>
            <h4 class="text-md font-medium text-purple-800">
              Already Owned Games ({categorizedResults.alreadyOwnedGames.length})
            </h4>
          </div>
          
          <div class="grid gap-3 max-h-96 overflow-y-auto">
            {#each categorizedResults.alreadyOwnedGames as game}
              <div class="flex items-center justify-between p-3 bg-purple-50 border border-purple-200 rounded-lg">
                <div class="flex-1">
                  <div class="font-medium text-purple-800">{game.steam_name}</div>
                  <div class="text-sm text-purple-600">Already in collection with Steam platform</div>
                </div>
                {#if game.matched_game_id}
                  <button
                    onclick={() => handleViewGame(game.matched_game_id!)}
                    class="text-sm text-purple-600 hover:text-purple-500 font-medium"
                  >
                    View Game →
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        </div>

      {:else if activeTab === 'skipped' && categorizedResults.skippedGames.length > 0}
        <div>
          <div class="flex items-center mb-4">
            <svg class="h-5 w-5 mr-2 text-orange-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="{getCategoryIcon('skipped')}" />
            </svg>
            <h4 class="text-md font-medium text-orange-800">
              Skipped Games ({categorizedResults.skippedGames.length})
            </h4>
          </div>
          
          <div class="grid gap-3 max-h-96 overflow-y-auto">
            {#each categorizedResults.skippedGames as game}
              <div class="flex items-center justify-between p-3 bg-orange-50 border border-orange-200 rounded-lg">
                <div class="flex-1">
                  <div class="font-medium text-orange-800">{game.steam_name}</div>
                  <div class="text-sm text-orange-600">
                    {game.error_message || 'Skipped during import process'}
                  </div>
                </div>
                <a
                  href="https://store.steampowered.com/app/{game.steam_appid}/"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-sm text-orange-600 hover:text-orange-500 font-medium"
                >
                  View on Steam →
                </a>
              </div>
            {/each}
          </div>
        </div>

      {:else}
        <!-- Empty State -->
        <div class="text-center py-8 text-gray-500">
          <svg class="mx-auto h-12 w-12 text-gray-400 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
          <p>No games in this category</p>
        </div>
      {/if}
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
  /* Custom scrollbar for game lists */
  .max-h-96::-webkit-scrollbar {
    width: 6px;
  }
  
  .max-h-96::-webkit-scrollbar-track {
    background: #f1f5f9;
    border-radius: 3px;
  }
  
  .max-h-96::-webkit-scrollbar-thumb {
    background: #cbd5e1;
    border-radius: 3px;
  }
  
  .max-h-96::-webkit-scrollbar-thumb:hover {
    background: #94a3b8;
  }

  /* Smooth transitions for tab switching */
  button {
    transition: all 0.2s ease-in-out;
  }
</style>