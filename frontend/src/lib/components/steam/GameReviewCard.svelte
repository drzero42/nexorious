<script lang="ts">
  import { IGDBSearchWidget } from '$lib/components/steam';
  import { steamImport } from '$lib/stores/steam-import.svelte';
  import type { SteamImportGameResponse } from '$lib/stores/steam-import.svelte';

  interface Props {
    game: SteamImportGameResponse;
  }

  let { game }: Props = $props();

  // Get current user decision for this game
  const userDecision = $derived(steamImport.value.userDecisions[game.steam_appid.toString()]);
  const isDecisionMade = $derived(!!userDecision);

  let showSearchWidget = $state(false);

  // Generate Steam store URL and icon URL
  const steamStoreUrl = $derived(`https://store.steampowered.com/app/${game.steam_appid}/`);
  const steamIconUrl = $derived(game.steam_appid ? 
    `https://media.steampowered.com/steamcommunity/public/images/apps/${game.steam_appid}/` : '');

  function handleSkip() {
    steamImport.setUserDecision(game.steam_appid.toString(), {
      action: 'skip',
      notes: 'Manually skipped by user'
    });
    showSearchWidget = false;
  }

  function handleSearchGame() {
    showSearchWidget = true;
  }

  function handleGameSelected(selectedGame: any) {
    steamImport.setUserDecision(game.steam_appid.toString(), {
      action: 'import',
      igdb_id: selectedGame.id,
      game_name: selectedGame.name,
      notes: `Manually matched to ${selectedGame.name}`
    });
    showSearchWidget = false;
  }

  function handleCancelSearch() {
    showSearchWidget = false;
  }

  function handleChangeDecision() {
    // Clear current decision and show search again
    steamImport.clearUserDecision(game.steam_appid.toString());
    showSearchWidget = true;
  }

  // Get decision display info
  function getDecisionInfo() {
    if (!userDecision) return null;
    
    if (userDecision.action === 'skip') {
      return {
        type: 'skip',
        label: 'Skipped',
        description: 'This game will not be imported',
        color: 'orange'
      };
    } else if (userDecision.action === 'import' && userDecision.game_name) {
      return {
        type: 'import',
        label: 'Matched',
        description: `Will import as "${userDecision.game_name}"`,
        color: 'green'
      };
    }
    
    return null;
  }

  const decisionInfo = $derived(getDecisionInfo());
</script>

<div class="card relative overflow-hidden 
     {isDecisionMade ? 
       (decisionInfo?.color === 'green' ? 'border-green-200 bg-green-50' : 'border-orange-200 bg-orange-50') 
       : 'border-gray-200 hover:border-gray-300'}
     transition-all duration-200">
  
  <!-- Decision Badge -->
  {#if decisionInfo}
    <div class="absolute top-3 right-3 z-10">
      <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium
                   {decisionInfo.color === 'green' ? 'bg-green-100 text-green-800' : 'bg-orange-100 text-orange-800'}">
        {#if decisionInfo.color === 'green'}
          <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
          </svg>
        {:else}
          <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
          </svg>
        {/if}
        {decisionInfo.label}
      </span>
    </div>
  {/if}

  <!-- Game Info Header -->
  <div class="flex items-start space-x-4 mb-4">
    <!-- Game Icon -->
    <div class="flex-shrink-0">
      <div class="w-16 h-16 bg-gray-200 rounded-lg overflow-hidden flex items-center justify-center">
        {#if steamIconUrl}
          <img 
            src="{steamIconUrl}icon.jpg" 
            alt="{game.steam_name} icon"
            class="w-full h-full object-cover"
            onerror={(event) => {
              // Fallback to generic game icon
              const target = event?.target as HTMLImageElement;
              if (target) {
                target.src = '/favicon.svg';
              }
            }}
          />
        {:else}
          <svg class="w-8 h-8 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
        {/if}
      </div>
    </div>

    <!-- Game Details -->
    <div class="flex-1 min-w-0">
      <h3 class="text-lg font-semibold text-gray-900 mb-1 break-words">
        {game.steam_name}
      </h3>
      
      <div class="space-y-1">
        <p class="text-sm text-gray-600">
          Steam App ID: {game.steam_appid}
        </p>
        
        {#if steamStoreUrl}
          <a 
            href={steamStoreUrl} 
            target="_blank" 
            rel="noopener noreferrer"
            class="inline-flex items-center text-sm text-blue-600 hover:text-blue-500"
          >
            <svg class="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
            View on Steam
          </a>
        {/if}
      </div>
    </div>
  </div>

  <!-- Decision Status -->
  {#if decisionInfo}
    <div class="mb-4 p-3 rounded-lg 
         {decisionInfo.color === 'green' ? 'bg-green-100 border border-green-200' : 'bg-orange-100 border border-orange-200'}">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm font-medium {decisionInfo.color === 'green' ? 'text-green-800' : 'text-orange-800'}">
            {decisionInfo.description}
          </p>
          {#if userDecision?.notes}
            <p class="text-xs {decisionInfo.color === 'green' ? 'text-green-600' : 'text-orange-600'} mt-1">
              {userDecision.notes}
            </p>
          {/if}
        </div>
        <button
          onclick={handleChangeDecision}
          class="text-xs {decisionInfo.color === 'green' ? 'text-green-600 hover:text-green-500' : 'text-orange-600 hover:text-orange-500'} font-medium"
        >
          Change
        </button>
      </div>
    </div>
  {/if}

  <!-- Action Buttons (when no decision made) -->
  {#if !isDecisionMade && !showSearchWidget}
    <div class="flex space-x-3">
      <button
        onclick={handleSearchGame}
        class="flex-1 btn-primary text-sm"
      >
        <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        Find Match
      </button>
      
      <button
        onclick={handleSkip}
        class="flex-1 btn-secondary text-sm text-orange-600 border-orange-300 hover:bg-orange-50"
      >
        <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-4 4" />
        </svg>
        Skip Game
      </button>
    </div>
  {/if}

  <!-- IGDB Search Widget -->
  {#if showSearchWidget}
    <div class="mt-4 border-t pt-4">
      <div class="mb-3">
        <h4 class="text-sm font-medium text-gray-900 mb-1">
          Search for "{game.steam_name}" in game database
        </h4>
        <p class="text-xs text-gray-600">
          Find the correct match or try different search terms
        </p>
      </div>
      
      <IGDBSearchWidget
        initialQuery={game.steam_name}
        onGameSelected={handleGameSelected}
        onCancel={handleCancelSearch}
      />
    </div>
  {/if}
</div>

<style>
  /* Ensure card animations are smooth */
  .card {
    transition: all 0.2s ease-in-out;
  }
  
  /* Mobile responsive adjustments */
  @media (max-width: 640px) {
    .card {
      padding: 1rem;
    }
    
    .flex.space-x-3 {
      flex-direction: column;
      gap: 0.5rem;
    }
  }
</style>