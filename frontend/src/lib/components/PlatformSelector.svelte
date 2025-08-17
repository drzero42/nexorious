<script lang="ts">
  import { platforms } from '$lib/stores/platforms.svelte';
  import { buildIconUrl, getPlatformFallbackIcon } from '$lib/utils/icon-utils';

  export interface Props {
    selectedPlatforms: Set<string>;
    platformStorefronts: Map<string, Set<string>>;
    platformStoreUrls: Map<string, string>;
    igdbPlatformNames?: string[];
    onplatformtoggle?: (event: CustomEvent<{ platformId: string }>) => void;
    onstorefronttoggle?: (event: CustomEvent<{ platformId: string; storefrontId: string }>) => void;
    onstoreurlchange?: (event: CustomEvent<{ platformId: string; url: string }>) => void;
  }
  
  let { 
    selectedPlatforms = $bindable(), 
    platformStorefronts = $bindable(), 
    platformStoreUrls = $bindable(), 
    igdbPlatformNames = [],
    onplatformtoggle,
    onstorefronttoggle,
    onstoreurlchange
  }: Props = $props();

  // Local state for collapsible sections
  let showOtherPlatforms = $state(false);
  let showOtherStorefronts = $state(new Map<string, boolean>());

  // Reactive statements for active platforms and storefronts
  const activePlatforms = $derived(platforms.value?.platforms?.filter(platform => platform.is_active) || []);
  const activeStorefronts = $derived(platforms.value?.storefronts?.filter(storefront => storefront.is_active) || []);

  // IGDB platform filtering helpers
  function isPlatformInIGDB(platform: any, igdbPlatforms: string[]): boolean {
    if (!igdbPlatforms || igdbPlatforms.length === 0) return false;
    
    return igdbPlatforms.some(igdbPlatform => 
      igdbPlatform.toLowerCase() === platform.display_name.toLowerCase() ||
      igdbPlatform.toLowerCase() === platform.name.toLowerCase()
    );
  }

  function getIGDBPlatforms(platforms: any[], igdbPlatforms: string[]): any[] {
    if (!igdbPlatforms || igdbPlatforms.length === 0) return [];
    return platforms.filter(platform => isPlatformInIGDB(platform, igdbPlatforms));
  }

  function getOtherPlatforms(platforms: any[], igdbPlatforms: string[]): any[] {
    if (!igdbPlatforms || igdbPlatforms.length === 0) return platforms;
    return platforms.filter(platform => !isPlatformInIGDB(platform, igdbPlatforms));
  }

  // Reactive statements for filtered platforms
  const igdbPlatforms = $derived(getIGDBPlatforms(activePlatforms, igdbPlatformNames));
  const otherPlatforms = $derived(getOtherPlatforms(activePlatforms, igdbPlatformNames));

  // Platform and storefront management
  function togglePlatform(platformId: string) {
    onplatformtoggle?.(new CustomEvent('platform-toggle', { detail: { platformId } }));
  }

  function toggleStorefrontForPlatform(platformId: string, storefrontId: string) {
    onstorefronttoggle?.(new CustomEvent('storefront-toggle', { detail: { platformId, storefrontId } }));
  }

  function setStoreUrlForPlatform(platformId: string, url: string) {
    onstoreurlchange?.(new CustomEvent('store-url-change', { detail: { platformId, url } }));
  }

  function isStorefrontSelectedForPlatform(platformId: string, storefrontId: string): boolean {
    const storefronts = platformStorefronts.get(platformId);
    return storefronts ? storefronts.has(storefrontId) : false;
  }

  function getPrimaryStorefrontsForPlatform(platformId: string): any[] {
    const platform = activePlatforms.find(p => p.id === platformId);
    if (!platform) return [];
    
    // Get associated storefront IDs for this platform
    const associatedStorefrontIds = new Set(platform.storefronts?.map((s: any) => s.id) || []);
    
    // Return active storefronts that ARE associated with this platform
    return activeStorefronts.filter(storefront => 
      associatedStorefrontIds.has(storefront.id)
    );
  }

  function getOtherStorefrontsForPlatform(platformId: string): any[] {
    const platform = activePlatforms.find(p => p.id === platformId);
    if (!platform) return activeStorefronts; // If platform not found, show all storefronts
    
    // Get associated storefront IDs for this platform
    const associatedStorefrontIds = new Set(platform.storefronts?.map((s: any) => s.id) || []);
    
    // Return active storefronts that are NOT associated with this platform
    return activeStorefronts.filter(storefront => 
      !associatedStorefrontIds.has(storefront.id)
    );
  }

  function toggleOtherStorefronts(platformId: string) {
    const current = showOtherStorefronts.get(platformId) || false;
    showOtherStorefronts.set(platformId, !current);
    showOtherStorefronts = new Map(showOtherStorefronts); // Trigger reactivity
  }
</script>

<div class="space-y-4">
  <h3 class="text-lg font-medium text-gray-900 mb-2 flex items-center">
    <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
    </svg>
    Platforms & Storefronts
  </h3>
  <p class="text-sm text-gray-600 mb-4">Select where you own this game and optionally add store details.</p>
  
  {#if platforms.value?.isLoading}
    <div class="text-center py-8">
      <svg class="animate-spin h-8 w-8 text-gray-400 mx-auto" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
      <p class="mt-2 text-sm text-gray-500">Loading platforms...</p>
    </div>
  {:else if platforms.value?.error}
    <div class="rounded-lg bg-red-50 border border-red-200 p-4">
      <div class="flex">
        <div class="flex-shrink-0">
          <svg class="h-5 w-5 text-red-600" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
          </svg>
        </div>
        <div class="ml-3">
          <h3 class="text-sm font-medium text-red-900">Platform Loading Error</h3>
          <p class="mt-1 text-sm text-red-800">{platforms.value?.error}</p>
        </div>
      </div>
    </div>
  {:else}
    <div class="space-y-3">
      <!-- IGDB Platforms Section -->
      {#if igdbPlatforms.length > 0}
        <div class="mb-4">
          <h4 class="text-sm font-medium text-gray-700 mb-3 flex items-center">
            <svg class="h-4 w-4 text-primary-500 mr-2" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
            </svg>
            Available on these platforms
          </h4>
          <div class="space-y-3">
            {#each igdbPlatforms as platform (platform.id)}
              <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                <!-- Platform Header -->
                <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                  <input
                    id="platform-{platform.id}"
                    type="checkbox"
                    checked={selectedPlatforms.has(platform.id)}
                    onchange={() => togglePlatform(platform.id)}
                    class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                  />
                  <div class="ml-3 flex items-center gap-2 flex-1">
                    {#if buildIconUrl(platform.icon_url)}
                      <img 
                        src={buildIconUrl(platform.icon_url)} 
                        alt={platform.display_name} 
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
                    <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                  </div>
                  {#if selectedPlatforms.has(platform.id)}
                    <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                    </svg>
                  {/if}
                </label>

                <!-- Platform Details (shown when selected) -->
                {#if selectedPlatforms.has(platform.id)}
                  <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                      <!-- Storefront Selection -->
                      <fieldset>
                        <legend class="block text-xs font-medium text-gray-700 mb-2">
                          Storefronts (optional)
                        </legend>
                        <div class="space-y-3 max-h-32 overflow-y-auto border border-gray-200 rounded-md p-2 bg-white">
                          <!-- Primary storefronts (associated with platform) -->
                          {#if getPrimaryStorefrontsForPlatform(platform.id).length > 0}
                            <div>
                              <div class="flex items-center mb-2">
                                <svg class="h-3 w-3 text-primary-500 mr-1" fill="currentColor" viewBox="0 0 20 20">
                                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.236 4.53L8.107 10.5a.75.75 0 00-1.214 1.029l2.5 3.5a.75.75 0 001.214 0l4-5.5z" clip-rule="evenodd" />
                                </svg>
                                <span class="text-xs font-medium text-primary-700">Recommended</span>
                              </div>
                              <div class="space-y-2 bg-primary-50 border border-primary-200 rounded-md p-2">
                                {#each getPrimaryStorefrontsForPlatform(platform.id) as storefront (storefront.id)}
                                  <label class="flex items-center cursor-pointer">
                                    <input
                                      type="checkbox"
                                      checked={isStorefrontSelectedForPlatform(platform.id, storefront.id)}
                                      onchange={() => toggleStorefrontForPlatform(platform.id, storefront.id)}
                                      class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                    />
                                    <span class="ml-2 text-sm font-medium text-gray-900">{storefront.display_name}</span>
                                  </label>
                                {/each}
                              </div>
                            </div>
                          {/if}
                          
                          <!-- Other storefronts (collapsed by default) -->
                          {#if getOtherStorefrontsForPlatform(platform.id).length > 0}
                            <div class="{getPrimaryStorefrontsForPlatform(platform.id).length > 0 ? 'border-t border-gray-200 pt-3' : ''}">
                              <button
                                type="button"
                                onclick={() => toggleOtherStorefronts(platform.id)}
                                class="flex items-center justify-between w-full p-2 bg-gray-50 border border-gray-200 rounded-md text-xs text-gray-600 hover:text-gray-800 hover:bg-gray-100 transition-colors duration-200"
                              >
                                <span class="flex items-center">
                                  <svg class="h-3 w-3 text-gray-400 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14-7l-7 7-7-7m14 14l-7-7-7 7" />
                                  </svg>
                                  Other storefronts ({getOtherStorefrontsForPlatform(platform.id).length})
                                </span>
                                <svg class="h-3 w-3 text-gray-400 transition-transform duration-200 {showOtherStorefronts.get(platform.id) ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                                </svg>
                              </button>
                              
                              {#if showOtherStorefronts.get(platform.id)}
                                <div class="mt-2 space-y-2 bg-gray-50 border border-gray-200 rounded-md p-2">
                                  {#each getOtherStorefrontsForPlatform(platform.id) as storefront (storefront.id)}
                                    <label class="flex items-center cursor-pointer">
                                      <input
                                        type="checkbox"
                                        checked={isStorefrontSelectedForPlatform(platform.id, storefront.id)}
                                        onchange={() => toggleStorefrontForPlatform(platform.id, storefront.id)}
                                        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                      />
                                      <span class="ml-2 text-sm text-gray-500 italic">{storefront.display_name}</span>
                                    </label>
                                  {/each}
                                </div>
                              {/if}
                            </div>
                          {/if}
                          
                          {#if getPrimaryStorefrontsForPlatform(platform.id).length === 0 && getOtherStorefrontsForPlatform(platform.id).length === 0}
                            <p class="text-xs text-gray-500 italic">No storefronts available</p>
                          {/if}
                        </div>
                      </fieldset>

                      <!-- Store URL -->
                      <div>
                        <label for="store-url-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                          Store URL (optional)
                        </label>
                        <input
                          id="store-url-{platform.id}"
                          type="url"
                          value={platformStoreUrls.get(platform.id) || ''}
                          oninput={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                          placeholder="https://store.example.com/game"
                          class="form-input text-sm py-1.5"
                        />
                      </div>
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Others Section -->
      {#if otherPlatforms.length > 0}
        <div>
          <button
            type="button"
            onclick={() => showOtherPlatforms = !showOtherPlatforms}
            class="w-full flex items-center justify-between p-3 bg-gray-50 border border-gray-200 rounded-lg hover:bg-gray-100 transition-colors duration-200"
          >
            <span class="text-sm font-medium text-gray-700 flex items-center">
              <svg class="h-4 w-4 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14-7l-7 7-7-7m14 14l-7-7-7 7" />
              </svg>
              Other platforms ({otherPlatforms.length})
            </span>
            <svg class="h-4 w-4 text-gray-400 transition-transform duration-200 {showOtherPlatforms ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          
          {#if showOtherPlatforms}
            <div class="mt-3 space-y-3">
              {#each otherPlatforms as platform (platform.id)}
                <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                  <!-- Platform Header -->
                  <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                    <input
                      id="platform-other-{platform.id}"
                      type="checkbox"
                      checked={selectedPlatforms.has(platform.id)}
                      onchange={() => togglePlatform(platform.id)}
                      class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                    />
                    <div class="ml-3 flex items-center gap-2 flex-1">
                      {#if buildIconUrl(platform.icon_url)}
                        <img 
                          src={buildIconUrl(platform.icon_url)} 
                          alt={platform.display_name} 
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
                      <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                    </div>
                    {#if selectedPlatforms.has(platform.id)}
                      <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                      </svg>
                    {/if}
                  </label>

                  <!-- Platform Details (shown when selected) -->
                  {#if selectedPlatforms.has(platform.id)}
                    <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                        <!-- Storefront Selection -->
                        <fieldset>
                          <legend class="block text-xs font-medium text-gray-700 mb-2">
                            Storefronts (optional)
                          </legend>
                          <div class="space-y-3 max-h-32 overflow-y-auto border border-gray-200 rounded-md p-2 bg-white">
                            <!-- Primary storefronts (associated with platform) -->
                            {#if getPrimaryStorefrontsForPlatform(platform.id).length > 0}
                              <div>
                                <div class="flex items-center mb-2">
                                  <svg class="h-3 w-3 text-primary-500 mr-1" fill="currentColor" viewBox="0 0 20 20">
                                    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.236 4.53L8.107 10.5a.75.75 0 00-1.214 1.029l2.5 3.5a.75.75 0 001.214 0l4-5.5z" clip-rule="evenodd" />
                                  </svg>
                                  <span class="text-xs font-medium text-primary-700">Recommended</span>
                                </div>
                                <div class="space-y-2 bg-primary-50 border border-primary-200 rounded-md p-2">
                                  {#each getPrimaryStorefrontsForPlatform(platform.id) as storefront (storefront.id)}
                                    <label class="flex items-center cursor-pointer">
                                      <input
                                        type="checkbox"
                                        checked={isStorefrontSelectedForPlatform(platform.id, storefront.id)}
                                        onchange={() => toggleStorefrontForPlatform(platform.id, storefront.id)}
                                        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                      />
                                      <span class="ml-2 text-sm font-medium text-gray-900">{storefront.display_name}</span>
                                    </label>
                                  {/each}
                                </div>
                              </div>
                            {/if}
                            
                            <!-- Other storefronts (collapsed by default) -->
                            {#if getOtherStorefrontsForPlatform(platform.id).length > 0}
                              <div class="{getPrimaryStorefrontsForPlatform(platform.id).length > 0 ? 'border-t border-gray-200 pt-3' : ''}">
                                <button
                                  type="button"
                                  onclick={() => toggleOtherStorefronts(platform.id)}
                                  class="flex items-center justify-between w-full p-2 bg-gray-50 border border-gray-200 rounded-md text-xs text-gray-600 hover:text-gray-800 hover:bg-gray-100 transition-colors duration-200"
                                >
                                  <span class="flex items-center">
                                    <svg class="h-3 w-3 text-gray-400 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14-7l-7 7-7-7m14 14l-7-7-7 7" />
                                    </svg>
                                    Other storefronts ({getOtherStorefrontsForPlatform(platform.id).length})
                                  </span>
                                  <svg class="h-3 w-3 text-gray-400 transition-transform duration-200 {showOtherStorefronts.get(platform.id) ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                                  </svg>
                                </button>
                                
                                {#if showOtherStorefronts.get(platform.id)}
                                  <div class="mt-2 space-y-2 bg-gray-50 border border-gray-200 rounded-md p-2">
                                    {#each getOtherStorefrontsForPlatform(platform.id) as storefront (storefront.id)}
                                      <label class="flex items-center cursor-pointer">
                                        <input
                                          type="checkbox"
                                          checked={isStorefrontSelectedForPlatform(platform.id, storefront.id)}
                                          onchange={() => toggleStorefrontForPlatform(platform.id, storefront.id)}
                                          class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                        />
                                        <span class="ml-2 text-sm text-gray-500 italic">{storefront.display_name}</span>
                                      </label>
                                    {/each}
                                  </div>
                                {/if}
                              </div>
                            {/if}
                            
                            {#if getPrimaryStorefrontsForPlatform(platform.id).length === 0 && getOtherStorefrontsForPlatform(platform.id).length === 0}
                              <p class="text-xs text-gray-500 italic">No storefronts available</p>
                            {/if}
                          </div>
                        </fieldset>

                        <!-- Store URL -->
                        <div>
                          <label for="store-url-other-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                            Store URL (optional)
                          </label>
                          <input
                            id="store-url-other-{platform.id}"
                            type="url"
                            value={platformStoreUrls.get(platform.id) || ''}
                            oninput={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                            placeholder="https://store.example.com/game"
                            class="form-input text-sm py-1.5"
                          />
                        </div>
                      </div>
                    </div>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Fallback: Show all platforms if no IGDB data -->
      {#if igdbPlatforms.length === 0 && otherPlatforms.length === 0}
        {#each activePlatforms as platform (platform.id)}
          <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
            <!-- Platform Header -->
            <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
              <input
                id="platform-{platform.id}"
                type="checkbox"
                checked={selectedPlatforms.has(platform.id)}
                onchange={() => togglePlatform(platform.id)}
                class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
              />
              <div class="ml-3 flex items-center gap-2 flex-1">
                {#if buildIconUrl(platform.icon_url)}
                  <img 
                    src={buildIconUrl(platform.icon_url)} 
                    alt={platform.display_name} 
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
                <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
              </div>
              {#if selectedPlatforms.has(platform.id)}
                <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                </svg>
              {/if}
            </label>

            <!-- Platform Details (shown when selected) -->
            {#if selectedPlatforms.has(platform.id)}
              <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                  <!-- Storefront Selection -->
                  <fieldset>
                    <legend class="block text-xs font-medium text-gray-700 mb-2">
                      Storefronts (optional)
                    </legend>
                    <div class="space-y-3 max-h-32 overflow-y-auto border border-gray-200 rounded-md p-2 bg-white">
                      <!-- Primary storefronts (associated with platform) -->
                      {#if getPrimaryStorefrontsForPlatform(platform.id).length > 0}
                        <div>
                          <div class="flex items-center mb-2">
                            <svg class="h-3 w-3 text-primary-500 mr-1" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.236 4.53L8.107 10.5a.75.75 0 00-1.214 1.029l2.5 3.5a.75.75 0 001.214 0l4-5.5z" clip-rule="evenodd" />
                            </svg>
                            <span class="text-xs font-medium text-primary-700">Recommended</span>
                          </div>
                          <div class="space-y-2 bg-primary-50 border border-primary-200 rounded-md p-2">
                            {#each getPrimaryStorefrontsForPlatform(platform.id) as storefront (storefront.id)}
                              <label class="flex items-center cursor-pointer">
                                <input
                                  type="checkbox"
                                  checked={isStorefrontSelectedForPlatform(platform.id, storefront.id)}
                                  onchange={() => toggleStorefrontForPlatform(platform.id, storefront.id)}
                                  class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                />
                                <span class="ml-2 text-sm font-medium text-gray-900">{storefront.display_name}</span>
                              </label>
                            {/each}
                          </div>
                        </div>
                      {/if}
                      
                      <!-- Other storefronts (collapsed by default) -->
                      {#if getOtherStorefrontsForPlatform(platform.id).length > 0}
                        <div class="{getPrimaryStorefrontsForPlatform(platform.id).length > 0 ? 'border-t border-gray-200 pt-3' : ''}">
                          <button
                            type="button"
                            onclick={() => toggleOtherStorefronts(platform.id)}
                            class="flex items-center justify-between w-full p-2 bg-gray-50 border border-gray-200 rounded-md text-xs text-gray-600 hover:text-gray-800 hover:bg-gray-100 transition-colors duration-200"
                          >
                            <span class="flex items-center">
                              <svg class="h-3 w-3 text-gray-400 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14-7l-7 7-7-7m14 14l-7-7-7 7" />
                              </svg>
                              Other storefronts ({getOtherStorefrontsForPlatform(platform.id).length})
                            </span>
                            <svg class="h-3 w-3 text-gray-400 transition-transform duration-200 {showOtherStorefronts.get(platform.id) ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {#if showOtherStorefronts.get(platform.id)}
                            <div class="mt-2 space-y-2 bg-gray-50 border border-gray-200 rounded-md p-2">
                              {#each getOtherStorefrontsForPlatform(platform.id) as storefront (storefront.id)}
                                <label class="flex items-center cursor-pointer">
                                  <input
                                    type="checkbox"
                                    checked={isStorefrontSelectedForPlatform(platform.id, storefront.id)}
                                    onchange={() => toggleStorefrontForPlatform(platform.id, storefront.id)}
                                    class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                                  />
                                  <span class="ml-2 text-sm text-gray-500 italic">{storefront.display_name}</span>
                                </label>
                              {/each}
                            </div>
                          {/if}
                        </div>
                      {/if}
                      
                      {#if getPrimaryStorefrontsForPlatform(platform.id).length === 0 && getOtherStorefrontsForPlatform(platform.id).length === 0}
                        <p class="text-xs text-gray-500 italic">No storefronts available</p>
                      {/if}
                    </div>
                  </fieldset>

                  <!-- Store URL -->
                  <div>
                    <label for="store-url-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                      Store URL (optional)
                    </label>
                    <input
                      id="store-url-{platform.id}"
                      type="url"
                      value={platformStoreUrls.get(platform.id) || ''}
                      oninput={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                      placeholder="https://store.example.com/game"
                      class="form-input text-sm py-1.5"
                    />
                  </div>
                </div>
              </div>
            {/if}
          </div>
        {/each}
      {/if}
      
      {#if activePlatforms.length === 0}
        <div class="text-center py-8">
          <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
          </svg>
          <p class="mt-2 text-sm text-gray-500">No platforms available</p>
          <p class="text-xs text-gray-400">Contact an administrator to add platforms.</p>
        </div>
      {/if}
    </div>
  {/if}
</div>