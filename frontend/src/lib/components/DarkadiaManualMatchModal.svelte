<script lang="ts">
  import { onMount } from 'svelte';
  import { config } from '$lib/env';
  import { auth } from '$lib/stores/auth.svelte';
  import { ui } from '$lib/stores/ui.svelte';
  import IGDBSearchWidget from './IGDBSearchWidget.svelte';
  import PlatformStorefrontSelector from './PlatformStorefrontSelector.svelte';
  import type { DarkadiaGameResponse } from '$lib/types/darkadia';

  interface IGDBGame {
    igdb_id: string;
    igdb_slug?: string;
    title: string;
    release_date?: string;
    cover_art_url?: string;
    description?: string;
    platforms: string[];
    howlongtobeat_main?: number;
    howlongtobeat_extra?: number;
    howlongtobeat_completionist?: number;
  }

  interface Platform {
    id: string;
    name: string;
    display_name: string;
  }

  interface Storefront {
    id: string;
    name: string;
    display_name: string;
  }

  interface PlatformCopy {
    copy_identifier: string;
    original_platform_name?: string;
    original_storefront_name?: string;
    resolved_platform_id?: string;
    resolved_storefront_id?: string;
    resolved_platform_name?: string;
    resolved_storefront_name?: string;
    platform_resolved: boolean;
    storefront_resolved: boolean;
  }

  interface GamePlatformOptions {
    game: {
      id: string;
      name: string;
      igdb_id?: string;
      igdb_title?: string;
    };
    available_platforms: Platform[];
    available_storefronts: Storefront[];
    current_platforms: PlatformCopy[];
  }

  interface Props {
    game: DarkadiaGameResponse;
    onComplete: (result: { igdb_game?: IGDBGame; platform_changes?: any[] }) => void;
    onCancel: () => void;
  }

  let { game, onComplete, onCancel }: Props = $props();

  // Modal state
  let currentStep = $state<'igdb' | 'platforms'>('igdb');
  let selectedIGDBGame = $state<IGDBGame | null>(null);
  let platformOptions = $state<GamePlatformOptions | null>(null);
  let platformChanges = $state<Map<string, { platformId: string | null; storefrontId: string | null }>>(new Map());
  
  // Loading states
  let isLoadingPlatforms = $state(false);
  let isSaving = $state(false);
  let platformError = $state<string | null>(null);

  // Initialize
  onMount(async () => {
    await loadPlatformOptions();
  });

  async function loadPlatformOptions() {
    if (!auth.value.accessToken) return;
    
    isLoadingPlatforms = true;
    platformError = null;
    
    try {
      const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/${game.id}/platforms`, {
        headers: {
          'Authorization': `Bearer ${auth.value.accessToken}`
        }
      });

      if (!response.ok) {
        if (response.status === 401) {
          await auth.refreshAuth();
          return loadPlatformOptions();
        }
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.detail || 'Failed to load platform options');
      }

      platformOptions = await response.json();
    } catch (error) {
      platformError = error instanceof Error ? error.message : 'Failed to load platform options';
      console.error('Error loading platform options:', error);
    } finally {
      isLoadingPlatforms = false;
    }
  }

  function handleIGDBGameSelected(igdbGame: IGDBGame) {
    selectedIGDBGame = igdbGame;
    // If we have platform data, move to platform selection
    if (platformOptions?.current_platforms.length) {
      currentStep = 'platforms';
    }
    // Don't auto-complete - always let user review and confirm their selection
    // The buttons will handle the completion based on what data is available
  }

  function handleIGDBCancel() {
    selectedIGDBGame = null;
    onCancel();
  }

  function handlePlatformChange(copyId: string, platformId: string | null, storefrontId: string | null) {
    platformChanges.set(copyId, { platformId, storefrontId });
    // Trigger reactivity
    platformChanges = new Map(platformChanges);
  }

  function goBackToIGDB() {
    currentStep = 'igdb';
  }

  async function handleComplete() {
    if (isSaving) return;
    
    isSaving = true;
    
    try {
      const result: { igdb_game?: IGDBGame; platform_changes?: any[] } = {};
      
      // Include IGDB game if selected
      if (selectedIGDBGame) {
        result.igdb_game = selectedIGDBGame;
      }
      
      // Include platform changes if any
      if (platformChanges.size > 0) {
        result.platform_changes = Array.from(platformChanges.entries()).map(([copyId, changes]) => ({
          copy_identifier: copyId,
          platform_id: changes.platformId,
          storefront_id: changes.storefrontId
        }));
      }
      
      onComplete(result);
    } catch (error) {
      console.error('Error completing manual match:', error);
      ui.showError('Failed to save manual match');
    } finally {
      isSaving = false;
    }
  }

  function handleKeyDown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !isSaving) {
      onCancel();
    }
  }

</script>

<svelte:window onkeydown={handleKeyDown} />

<!-- Modal overlay -->
<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-50">
  <div class="bg-white rounded-lg shadow-xl max-w-4xl w-full max-h-[90vh] flex flex-col">
    <!-- Header -->
    <div class="px-6 py-4 border-b border-gray-200">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-xl font-semibold text-gray-900">Manual Game Matching</h2>
          <p class="text-sm text-gray-600 mt-1">
            Match "{game.name}" to IGDB and configure platforms
          </p>
        </div>
        
        <!-- Step indicator -->
        <div class="flex items-center space-x-3">
          <div class="flex items-center space-x-2">
            <div class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium
                        {currentStep === 'igdb' ? 'bg-blue-500 text-white' : 'bg-green-500 text-white'}">
              1
            </div>
            <span class="text-sm {currentStep === 'igdb' ? 'text-blue-600 font-medium' : 'text-green-600'}">
              IGDB Match
            </span>
          </div>
          
          <div class="w-8 h-0.5 bg-gray-300"></div>
          
          <div class="flex items-center space-x-2">
            <div class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium
                        {currentStep === 'platforms' ? 'bg-blue-500 text-white' : 
                         selectedIGDBGame ? 'bg-gray-200 text-gray-600' : 'bg-gray-200 text-gray-400'}">
              2
            </div>
            <span class="text-sm {currentStep === 'platforms' ? 'text-blue-600 font-medium' : 
                                   selectedIGDBGame ? 'text-gray-600' : 'text-gray-400'}">
              Platforms
            </span>
          </div>
        </div>

        <button
          onclick={onCancel}
          class="text-gray-400 hover:text-gray-600 transition-colors"
          disabled={isSaving}
          aria-label="Close modal"
        >
          <svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-hidden flex flex-col">
      {#if currentStep === 'igdb'}
        <!-- IGDB Selection Step -->
        <div class="flex-1 p-6">
          <div class="mb-4">
            <h3 class="text-lg font-medium text-gray-900 mb-2">Search for IGDB Game</h3>
            <p class="text-sm text-gray-600">
              Find the correct game in the IGDB database. This will provide metadata and artwork for your game.
            </p>
          </div>
          
          <IGDBSearchWidget
            initialQuery={game.name}
            onGameSelected={handleIGDBGameSelected}
            onCancel={handleIGDBCancel}
          />
          
          {#if selectedIGDBGame}
            <div class="mt-4 p-3 bg-green-50 border border-green-200 rounded-lg">
              <div class="flex items-center space-x-2">
                <svg class="w-5 h-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                </svg>
                <span class="text-sm font-medium text-green-800">
                  Selected: {selectedIGDBGame.title}
                </span>
              </div>
            </div>
          {/if}
        </div>
      {:else if currentStep === 'platforms'}
        <!-- Platform Selection Step -->
        <div class="flex-1 overflow-y-auto p-6">
          <div class="mb-4">
            <div class="flex items-center justify-between mb-2">
              <h3 class="text-lg font-medium text-gray-900">Configure Platforms & Storefronts</h3>
              <button
                onclick={goBackToIGDB}
                class="text-sm text-blue-600 hover:text-blue-800"
                disabled={isSaving}
              >
                ← Back to IGDB
              </button>
            </div>
            <p class="text-sm text-gray-600">
              Configure which platforms and storefronts each copy of this game should be associated with.
            </p>
          </div>

          {#if isLoadingPlatforms}
            <div class="flex items-center justify-center py-12">
              <div class="flex items-center space-x-3">
                <svg class="animate-spin h-5 w-5 text-blue-500" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                <span class="text-sm text-gray-600">Loading platform options...</span>
              </div>
            </div>
          {:else if platformError}
            <div class="bg-red-50 border border-red-200 rounded-lg p-4">
              <div class="flex items-center space-x-2">
                <svg class="w-5 h-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <span class="text-sm font-medium text-red-800">Error loading platform options</span>
              </div>
              <p class="text-sm text-red-700 mt-1">{platformError}</p>
              <button
                onclick={loadPlatformOptions}
                class="mt-2 text-sm text-red-600 hover:text-red-800 underline"
              >
                Retry
              </button>
            </div>
          {:else if platformOptions}
            <PlatformStorefrontSelector
              platforms={platformOptions.available_platforms}
              storefronts={platformOptions.available_storefronts}
              currentPlatforms={platformOptions.current_platforms}
              onPlatformChange={handlePlatformChange}
              readonly={false}
            />
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-6 py-4 border-t border-gray-200 bg-gray-50">
      <div class="flex items-center justify-between">
        <div class="text-sm text-gray-600">
          {#if currentStep === 'igdb'}
            Step 1 of 2: Select IGDB game for metadata
          {:else}
            Step 2 of 2: Configure platform assignments
          {/if}
        </div>
        
        <div class="flex items-center space-x-3">
          <button
            onclick={onCancel}
            disabled={isSaving}
            class="btn-secondary"
          >
            Cancel
          </button>
          
          {#if currentStep === 'igdb'}
            {#if selectedIGDBGame && !platformOptions?.current_platforms.length}
              <!-- Complete directly if no platform data -->
              <button
                onclick={handleComplete}
                disabled={isSaving}
                class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Complete Match
              </button>
            {:else}
              <!-- Go to platforms step if platform data exists -->
              <button
                onclick={() => currentStep = 'platforms'}
                disabled={!platformOptions?.current_platforms?.length}
                class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Configure Platforms →
              </button>
            {/if}
          {:else}
            <button
              onclick={handleComplete}
              disabled={isSaving}
              class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isSaving}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Saving...
              {:else}
                Complete Manual Match
              {/if}
            </button>
          {/if}
        </div>
      </div>
    </div>
  </div>
</div>