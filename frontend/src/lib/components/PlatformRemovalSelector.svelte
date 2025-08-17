<script lang="ts">
  import { buildIconUrl, getPlatformFallbackIcon } from '$lib/utils/icon-utils';

  export interface Props {
    availablePlatformAssociations?: Array<{
      platformId: string;
      platformName: string;
      storefrontId?: string;
      storefrontName?: string;
      associationIds: string[];
      platformIconUrl?: string;
    }>;
    selectedAssociationIds?: Set<string>;
    onselectionchange?: (event: CustomEvent<{ selectedAssociationIds: Set<string> }>) => void;
  }

  let { 
    availablePlatformAssociations = $bindable([]), 
    selectedAssociationIds = $bindable(new Set()),
    onselectionchange
  }: Props = $props();

  function toggleAssociation(associationIds: string[]) {
    const newSelection = new Set(selectedAssociationIds);
    
    // Check if all association IDs are currently selected
    const allSelected = associationIds.every(id => newSelection.has(id));
    
    if (allSelected) {
      // Remove all if they're all selected
      associationIds.forEach(id => newSelection.delete(id));
    } else {
      // Add all if not all are selected
      associationIds.forEach(id => newSelection.add(id));
    }
    
    selectedAssociationIds = newSelection;
    onselectionchange?.(new CustomEvent('selection-change', { detail: { selectedAssociationIds: newSelection } }));
  }

  function isAssociationSelected(associationIds: string[]): boolean {
    return associationIds.every(id => selectedAssociationIds.has(id));
  }

  function formatPlatformStorefrontName(platformName: string, storefrontName?: string): string {
    if (storefrontName && storefrontName !== 'No Storefront') {
      return `${platformName} (${storefrontName})`;
    }
    return platformName;
  }
</script>

<div class="space-y-3">
  <h3 class="text-lg font-medium text-gray-900 mb-2 flex items-center">
    <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
    </svg>
    Remove Platforms & Storefronts
  </h3>
  <p class="text-sm text-gray-600 mb-4">Select which platforms/storefronts to remove from the selected games.</p>
  
  {#if availablePlatformAssociations.length === 0}
    <div class="text-center py-8 bg-gray-50 border border-gray-200 rounded-lg">
      <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
      </svg>
      <p class="mt-2 text-sm text-gray-500">No platform associations found</p>
      <p class="text-xs text-gray-400">The selected games don't have any platform associations to remove.</p>
    </div>
  {:else}
    <div class="space-y-2 max-h-80 overflow-y-auto border border-gray-200 rounded-lg p-3 bg-white">
      {#each availablePlatformAssociations as association (association.platformId + '-' + (association.storefrontId || 'none'))}
        <label class="flex items-center p-3 cursor-pointer hover:bg-gray-50 transition-colors duration-200 rounded-md border border-gray-100">
          <input
            type="checkbox"
            checked={isAssociationSelected(association.associationIds)}
            onchange={() => toggleAssociation(association.associationIds)}
            class="h-4 w-4 text-red-600 focus:ring-red-500 border-gray-300 rounded"
          />
          <div class="ml-3 flex items-center gap-2 flex-1">
            {#if buildIconUrl(association.platformIconUrl)}
              <img 
                src={buildIconUrl(association.platformIconUrl)} 
                alt={association.platformName} 
                class="w-6 h-6 object-contain"
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
              <span class="text-lg hidden" role="img" aria-hidden="true">{getPlatformFallbackIcon()}</span>
            {:else}
              <span class="text-lg" role="img" aria-hidden="true">{getPlatformFallbackIcon()}</span>
            {/if}
            <span class="text-sm font-medium text-gray-900">
              {formatPlatformStorefrontName(association.platformName, association.storefrontName)}
            </span>
            <span class="text-xs text-gray-500 ml-auto">
              ({association.associationIds.length} game{association.associationIds.length === 1 ? '' : 's'})
            </span>
          </div>
          {#if isAssociationSelected(association.associationIds)}
            <svg class="h-5 w-5 text-red-500 ml-2" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
            </svg>
          {/if}
        </label>
      {/each}
    </div>
    
    <div class="mt-3 p-3 bg-red-50 border border-red-200 rounded-md">
      <div class="flex">
        <div class="flex-shrink-0">
          <svg class="h-5 w-5 text-red-600" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M8.485 3.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 3.495zM10 6a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 6zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd" />
          </svg>
        </div>
        <div class="ml-3">
          <h3 class="text-sm font-medium text-red-900">Warning</h3>
          <p class="mt-1 text-sm text-red-800">
            Selected platforms/storefronts will be permanently removed from the selected games. This action cannot be undone.
          </p>
        </div>
      </div>
    </div>
    
    {#if selectedAssociationIds.size > 0}
      <div class="mt-3 p-3 bg-blue-50 border border-blue-200 rounded-md">
        <p class="text-sm text-blue-800">
          <strong>{selectedAssociationIds.size}</strong> platform association{selectedAssociationIds.size === 1 ? '' : 's'} selected for removal
        </p>
      </div>
    {/if}
  {/if}
</div>