<script lang="ts">
  import { config } from '$lib/env';
  import { auth } from '$lib/stores/auth.svelte';
  import { ui } from '$lib/stores/ui.svelte';
  import type {
    DarkadiaResolutionSummaryResponse,
    DarkadiaUpdateMappingsRequest,
    DarkadiaUpdateMappingsResponse
  } from '$lib/types/darkadia';

  interface Props {
    show: boolean;
    onClose: () => void;
    onMappingsUpdated?: () => void;
  }

  let { show = false, onClose, onMappingsUpdated }: Props = $props();

  // State
  let isLoading = $state(true);
  let isSaving = $state(false);
  let resolutionData = $state<DarkadiaResolutionSummaryResponse | null>(null);
  let error = $state<string | null>(null);
  
  // Editing state - store original → new mapping pairs
  let platformMappings = $state<Map<string, string>>(new Map());
  let storefrontMappings = $state<Map<string, string>>(new Map());
  
  // Available platforms and storefronts for dropdowns
  let availablePlatforms = $state<string[]>([]);
  let availableStorefronts = $state<string[]>([]);
  
  // Reactive computed values
  const hasChanges = $derived(
    platformMappings.size > 0 || storefrontMappings.size > 0
  );

  // Load resolution summary when modal opens
  $effect(() => {
    if (show) {
      loadResolutionSummary();
      loadAvailableOptions();
    } else {
      // Reset state when modal closes
      resolutionData = null;
      platformMappings.clear();
      storefrontMappings.clear();
      error = null;
      isLoading = true;
    }
  });

  async function loadResolutionSummary() {
    try {
      isLoading = true;
      error = null;

      const response = await fetch(`${config.apiUrl}/import/sources/darkadia/resolution-summary`, {
        headers: {
          'Authorization': `Bearer ${auth.value.accessToken}`
        }
      });

      if (!response.ok) {
        if (response.status === 401) {
          await auth.refreshAuth();
          return loadResolutionSummary();
        }
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.detail || 'Failed to load resolution summary');
      }

      resolutionData = await response.json();
    } catch (err) {
      console.error('Error loading resolution summary:', err);
      error = err instanceof Error ? err.message : 'Failed to load resolution summary';
      ui.showError(error);
    } finally {
      isLoading = false;
    }
  }

  async function loadAvailableOptions() {
    try {
      // Load available platforms and storefronts from the API
      const [platformsResponse, storefrontsResponse] = await Promise.all([
        fetch(`${config.apiUrl}/platforms/simple-list`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        }),
        fetch(`${config.apiUrl}/platforms/storefronts/simple-list`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        })
      ]);

      if (platformsResponse.ok) {
        availablePlatforms = await platformsResponse.json();
      } else if (platformsResponse.status === 401) {
        await auth.refreshAuth();
        return loadAvailableOptions(); // Retry after auth refresh
      } else {
        console.warn('Failed to load platforms, using fallback list');
        availablePlatforms = ['PC (Windows)', 'PlayStation 5', 'Xbox Series X/S', 'Nintendo Switch'];
      }

      if (storefrontsResponse.ok) {
        availableStorefronts = await storefrontsResponse.json();
      } else if (storefrontsResponse.status === 401) {
        await auth.refreshAuth();
        return loadAvailableOptions(); // Retry after auth refresh
      } else {
        console.warn('Failed to load storefronts, using fallback list');
        availableStorefronts = ['Steam', 'Epic Games Store', 'PlayStation Store', 'Xbox Store', 'Nintendo eShop', 'GOG'];
      }
    } catch (err) {
      console.error('Error loading available options:', err);
      // Fallback to basic lists
      availablePlatforms = ['PC (Windows)', 'PlayStation 5', 'Xbox Series X/S', 'Nintendo Switch'];
      availableStorefronts = ['Steam', 'Epic Games Store', 'PlayStation Store', 'Xbox Store', 'Nintendo eShop', 'GOG'];
    }
  }

  function handlePlatformMappingChange(original: string, event: Event) {
    const target = event.target as HTMLSelectElement;
    const newMapped = target.value;
    if (newMapped === '') {
      platformMappings.delete(original);
    } else {
      platformMappings.set(original, newMapped);
    }
    // Trigger reactivity
    platformMappings = new Map(platformMappings);
  }

  function handleStorefrontMappingChange(original: string, event: Event) {
    const target = event.target as HTMLSelectElement;
    const newMapped = target.value;
    if (newMapped === '') {
      storefrontMappings.delete(original);
    } else {
      storefrontMappings.set(original, newMapped);
    }
    // Trigger reactivity
    storefrontMappings = new Map(storefrontMappings);
  }

  async function handleSave() {
    if (!hasChanges) return;

    try {
      isSaving = true;
      error = null;

      const mappings = [];

      // Add platform mappings
      for (const [original, newMapped] of platformMappings.entries()) {
        mappings.push({
          original_name: original,
          new_mapped_name: newMapped,
          mapping_type: 'platform' as const
        });
      }

      // Add storefront mappings
      for (const [original, newMapped] of storefrontMappings.entries()) {
        mappings.push({
          original_name: original,
          new_mapped_name: newMapped,
          mapping_type: 'storefront' as const
        });
      }

      const requestBody: DarkadiaUpdateMappingsRequest = { mappings };

      const response = await fetch(`${config.apiUrl}/import/sources/darkadia/update-mappings`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${auth.value.accessToken}`
        },
        body: JSON.stringify(requestBody)
      });

      if (!response.ok) {
        if (response.status === 401) {
          await auth.refreshAuth();
          return handleSave();
        }
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.detail || 'Failed to update mappings');
      }

      const result: DarkadiaUpdateMappingsResponse = await response.json();
      
      ui.showSuccess(`Updated ${result.updated_mappings} mappings, affecting ${result.affected_games} games`);
      
      if (result.errors.length > 0) {
        console.warn('Mapping update errors:', result.errors);
      }

      // Reset the mappings and reload data
      platformMappings.clear();
      storefrontMappings.clear();
      await loadResolutionSummary();
      
      // Notify parent component
      onMappingsUpdated?.();

    } catch (err) {
      console.error('Error updating mappings:', err);
      error = err instanceof Error ? err.message : 'Failed to update mappings';
      ui.showError(error);
    } finally {
      isSaving = false;
    }
  }

  function handleCancel() {
    platformMappings.clear();
    storefrontMappings.clear();
    onClose();
  }
</script>

{#if show}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50" role="dialog" aria-modal="true" aria-labelledby="resolution-modal-title">
    <div class="bg-white rounded-lg shadow-xl max-w-4xl w-full max-h-[90vh] overflow-hidden">
      <!-- Header -->
      <div class="p-6 border-b border-gray-200">
        <div class="flex items-center justify-between">
          <h3 id="resolution-modal-title" class="text-lg font-medium text-gray-900">
            Platform & Storefront Matches
          </h3>
          <button
            onclick={handleCancel}
            class="text-gray-400 hover:text-gray-600 transition-colors"
            aria-label="Close modal"
          >
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <p class="text-sm text-gray-500 mt-1">
          Review and modify how your CSV platform and storefront names are mapped to the system.
        </p>
      </div>

      <!-- Content -->
      <div class="p-6 overflow-y-auto max-h-[70vh]">
        {#if isLoading}
          <div class="flex items-center justify-center py-12">
            <svg class="animate-spin h-8 w-8 text-blue-600" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <span class="ml-2 text-gray-600">Loading mappings...</span>
          </div>
        {:else if error}
          <div class="text-center py-12">
            <div class="text-red-600 mb-4">
              <svg class="h-12 w-12 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
              </svg>
            </div>
            <p class="text-gray-900 font-medium mb-2">Error loading mappings</p>
            <p class="text-gray-600 mb-4">{error}</p>
            <button onclick={loadResolutionSummary} class="btn-primary">
              Try Again
            </button>
          </div>
        {:else if resolutionData}
          <div class="space-y-8">
            <!-- Platforms Section -->
            {#if resolutionData.platforms.length > 0}
              <div>
                <h4 class="text-lg font-medium text-gray-900 mb-4 flex items-center">
                  <span class="mr-2">🎮</span>
                  Platforms ({resolutionData.platforms.length})
                </h4>
                <div class="overflow-hidden shadow ring-1 ring-black ring-opacity-5 md:rounded-lg">
                  <table class="min-w-full divide-y divide-gray-300">
                    <thead class="bg-gray-50">
                      <tr>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wide">Original Name</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wide">Mapped To</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wide">Games</th>
                      </tr>
                    </thead>
                    <tbody class="bg-white divide-y divide-gray-200">
                      {#each resolutionData.platforms as platform}
                        <tr class="hover:bg-gray-50">
                          <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                            "{platform.original}"
                          </td>
                          <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                            <select 
                              class="form-select text-sm {platform.mapped === 'Unmapped' ? 'text-red-600' : ''}"
                              value={platformMappings.get(platform.original) || (platform.mapped === 'Unmapped' ? '' : platform.mapped)}
                              onchange={(e) => handlePlatformMappingChange(platform.original, e)}
                            >
                              {#if platform.mapped === 'Unmapped'}
                                <option value="" class="text-red-600">Select a platform...</option>
                              {:else}
                                <option value={platform.mapped}>{platform.mapped}</option>
                              {/if}
                              {#each availablePlatforms.filter(p => p !== platform.mapped) as availablePlatform}
                                <option value={availablePlatform}>{availablePlatform}</option>
                              {/each}
                            </select>
                          </td>
                          <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                              {platform.game_count} games
                            </span>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              </div>
            {/if}

            <!-- Storefronts Section -->
            {#if resolutionData.storefronts.length > 0}
              <div>
                <h4 class="text-lg font-medium text-gray-900 mb-4 flex items-center">
                  <span class="mr-2">🏪</span>
                  Storefronts ({resolutionData.storefronts.length})
                </h4>
                <div class="overflow-hidden shadow ring-1 ring-black ring-opacity-5 md:rounded-lg">
                  <table class="min-w-full divide-y divide-gray-300">
                    <thead class="bg-gray-50">
                      <tr>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wide">Original Name</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wide">Mapped To</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wide">Games</th>
                      </tr>
                    </thead>
                    <tbody class="bg-white divide-y divide-gray-200">
                      {#each resolutionData.storefronts as storefront}
                        <tr class="hover:bg-gray-50">
                          <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                            "{storefront.original}"
                          </td>
                          <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                            <select 
                              class="form-select text-sm {storefront.mapped === 'Unmapped' ? 'text-red-600' : ''}"
                              value={storefrontMappings.get(storefront.original) || (storefront.mapped === 'Unmapped' ? '' : storefront.mapped)}
                              onchange={(e) => handleStorefrontMappingChange(storefront.original, e)}
                            >
                              {#if storefront.mapped === 'Unmapped'}
                                <option value="" class="text-red-600">Select a storefront...</option>
                              {:else}
                                <option value={storefront.mapped}>{storefront.mapped}</option>
                              {/if}
                              {#each availableStorefronts.filter(s => s !== storefront.mapped) as availableStorefront}
                                <option value={availableStorefront}>{availableStorefront}</option>
                              {/each}
                            </select>
                          </td>
                          <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                              {storefront.game_count} games
                            </span>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              </div>
            {/if}

            <!-- No mappings message -->
            {#if resolutionData.platforms.length === 0 && resolutionData.storefronts.length === 0}
              <div class="text-center py-12">
                <div class="text-gray-400 mb-4">
                  <svg class="h-12 w-12 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                  </svg>
                </div>
                <p class="text-gray-900 font-medium mb-2">No Platform or Storefront Mappings</p>
                <p class="text-gray-600">
                  No games have been imported yet, or all platforms and storefronts are already properly mapped.
                </p>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      {#if !isLoading && !error && resolutionData && (resolutionData.platforms.length > 0 || resolutionData.storefronts.length > 0)}
        <div class="p-6 bg-gray-50 border-t border-gray-200 flex justify-between items-center">
          <div class="flex items-center text-sm text-gray-600">
            {#if hasChanges}
              <span class="inline-flex items-center">
                <svg class="h-4 w-4 mr-1 text-yellow-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                </svg>
                You have unsaved changes
              </span>
            {:else}
              <span>No changes made</span>
            {/if}
          </div>
          
          <div class="flex space-x-3">
            <button
              onclick={handleCancel}
              disabled={isSaving}
              class="btn-secondary disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              onclick={handleSave}
              disabled={!hasChanges || isSaving}
              class="btn-primary disabled:opacity-50"
            >
              {#if isSaving}
                <svg class="animate-spin h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Saving...
              {:else}
                Save Changes
              {/if}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}