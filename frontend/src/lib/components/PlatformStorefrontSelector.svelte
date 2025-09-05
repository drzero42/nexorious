<script lang="ts">
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

  interface Props {
    platforms: Platform[];
    storefronts: Storefront[];
    currentPlatforms: PlatformCopy[];
    onPlatformChange: (copyId: string, platformId: string | null, storefrontId: string | null) => void;
    readonly?: boolean;
  }

  let {
    platforms,
    storefronts,
    currentPlatforms,
    onPlatformChange,
    readonly = false
  }: Props = $props();

  function handlePlatformSelect(copy: PlatformCopy, platformId: string) {
    const platform = platforms.find(p => p.id === platformId);
    if (!platform) return;

    // Clear storefront when platform changes
    onPlatformChange(copy.copy_identifier, platformId, null);
  }

  function handleStorefrontSelect(copy: PlatformCopy, storefrontId: string) {
    const storefront = storefronts.find(s => s.id === storefrontId);
    if (!storefront) return;

    // Keep current platform, update storefront
    onPlatformChange(copy.copy_identifier, copy.resolved_platform_id || null, storefrontId);
  }

  function clearPlatform(copy: PlatformCopy) {
    onPlatformChange(copy.copy_identifier, null, null);
  }

  function getStatusColor(resolved: boolean): string {
    return resolved ? 'text-green-600 bg-green-50 border-green-200' : 'text-amber-600 bg-amber-50 border-amber-200';
  }

  function getStatusIcon(resolved: boolean): string {
    return resolved ? 'M5 13l4 4L19 7' : 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.268 16.5c-.77.833.192 2.5 1.732 2.5z';
  }
</script>

<div class="space-y-4">
  <div class="text-sm font-medium text-gray-700 mb-3">
    Platform & Storefront Configuration
  </div>

  {#if currentPlatforms.length === 0}
    <div class="text-sm text-gray-500 text-center py-6 bg-gray-50 rounded-lg border-2 border-dashed border-gray-300">
      No platform data available for this game
    </div>
  {:else}
    <div class="space-y-3">
      {#each currentPlatforms as copy, index (copy.copy_identifier)}
        <div class="border border-gray-200 rounded-lg p-4 bg-white">
          <!-- Copy Header -->
          <div class="flex items-center justify-between mb-3">
            <div class="flex items-center space-x-2">
              <div class="text-sm font-medium text-gray-900">
                Copy {index + 1}
                {#if copy.copy_identifier !== 'default'}
                  <span class="text-xs text-gray-500 ml-1">({copy.copy_identifier})</span>
                {/if}
              </div>
              
              <!-- Status indicators -->
              <div class="flex space-x-2">
                <div class="flex items-center space-x-1">
                  <div class="w-4 h-4 rounded-full flex items-center justify-center {getStatusColor(copy.platform_resolved)}">
                    <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="{getStatusIcon(copy.platform_resolved)}" />
                    </svg>
                  </div>
                  <span class="text-xs text-gray-600">Platform</span>
                </div>
                
                {#if copy.original_storefront_name}
                  <div class="flex items-center space-x-1">
                    <div class="w-4 h-4 rounded-full flex items-center justify-center {getStatusColor(copy.storefront_resolved)}">
                      <svg class="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="{getStatusIcon(copy.storefront_resolved)}" />
                      </svg>
                    </div>
                    <span class="text-xs text-gray-600">Storefront</span>
                  </div>
                {/if}
              </div>
            </div>

            {#if !readonly}
              <button
                onclick={() => clearPlatform(copy)}
                class="text-xs text-red-600 hover:text-red-800 hover:bg-red-50 px-2 py-1 rounded"
                title="Clear platform selection"
              >
                Clear
              </button>
            {/if}
          </div>

          <!-- Original values -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-3 mb-4">
            <div class="bg-gray-50 p-3 rounded border">
              <div class="text-xs font-medium text-gray-700 mb-1">Original Platform</div>
              <div class="text-sm text-gray-900">{copy.original_platform_name || 'Unknown'}</div>
            </div>
            
            {#if copy.original_storefront_name}
              <div class="bg-gray-50 p-3 rounded border">
                <div class="text-xs font-medium text-gray-700 mb-1">Original Storefront</div>
                <div class="text-sm text-gray-900">{copy.original_storefront_name}</div>
              </div>
            {/if}
          </div>

          <!-- Platform Selection -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label for="platform-select-{copy.copy_identifier}" class="block text-xs font-medium text-gray-700 mb-2">
                Select Platform
              </label>
              {#if readonly}
                <div class="px-3 py-2 border border-gray-300 rounded-md bg-gray-50 text-sm text-gray-900">
                  {copy.resolved_platform_name || 'Not selected'}
                </div>
              {:else}
                <select
                  id="platform-select-{copy.copy_identifier}"
                  value={copy.resolved_platform_id || ''}
                  onchange={(e) => handlePlatformSelect(copy, e.currentTarget.value)}
                  class="w-full px-3 py-2 border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500 text-sm"
                >
                  <option value="">Select platform...</option>
                  {#each platforms as platform (platform.id)}
                    <option value={platform.id}>{platform.display_name}</option>
                  {/each}
                </select>
              {/if}
            </div>

            <div>
              <label for="storefront-select-{copy.copy_identifier}" class="block text-xs font-medium text-gray-700 mb-2">
                Select Storefront
                {#if !copy.original_storefront_name}
                  <span class="text-gray-500">(Optional)</span>
                {/if}
              </label>
              {#if readonly}
                <div class="px-3 py-2 border border-gray-300 rounded-md bg-gray-50 text-sm text-gray-900">
                  {copy.resolved_storefront_name || 'Not selected'}
                </div>
              {:else}
                <select
                  id="storefront-select-{copy.copy_identifier}"
                  value={copy.resolved_storefront_id || ''}
                  onchange={(e) => handleStorefrontSelect(copy, e.currentTarget.value)}
                  class="w-full px-3 py-2 border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500 text-sm"
                  disabled={!copy.resolved_platform_id}
                >
                  <option value="">Select storefront...</option>
                  {#each storefronts as storefront (storefront.id)}
                    <option value={storefront.id}>{storefront.display_name}</option>
                  {/each}
                </select>
              {/if}
            </div>
          </div>

          <!-- Current resolved values (if different from original) -->
          {#if copy.resolved_platform_name || copy.resolved_storefront_name}
            <div class="mt-3 pt-3 border-t border-gray-200">
              <div class="text-xs font-medium text-gray-700 mb-2">Currently Resolved To:</div>
              <div class="flex flex-wrap gap-2">
                {#if copy.resolved_platform_name}
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                    {copy.resolved_platform_name}
                  </span>
                {/if}
                
                {#if copy.resolved_storefront_name}
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                    {copy.resolved_storefront_name}
                  </span>
                {/if}
              </div>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if !readonly}
    <div class="text-xs text-gray-500">
      <div class="flex items-start space-x-1">
        <svg class="w-3 h-3 mt-0.5 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <div>
          Select the correct platform and storefront for each copy. Changes are applied when you save the manual match.
        </div>
      </div>
    </div>
  {/if}
</div>