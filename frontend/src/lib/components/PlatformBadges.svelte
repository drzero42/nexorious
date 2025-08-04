<script lang="ts">
 import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
 import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';
 import { buildIconUrl, getPlatformFallbackIcon, getStorefrontFallbackIcon } from '$lib/utils/icon-utils';

 export let platforms: UserGamePlatform[] = [];
 export let compact: boolean = false;
 export let maxVisible: number = 3;
 export let showDetails: boolean = false; // For expanded detail view
 export let showStoreLinks: boolean = false; // Include store links in expanded view

 // Expandable state management - only one platform can be expanded at a time
 let expandedPlatform: string | null = null;

 $: groupedPlatforms = groupPlatformsByPlatform(platforms);
 $: visiblePlatforms = groupedPlatforms.slice(0, maxVisible);
 $: hiddenCount = Math.max(0, groupedPlatforms.length - maxVisible);

 // Unified styling for all platform badges - no platform-specific colors
 const unifiedBadgeStyle = {
  bg: 'bg-gradient-to-r from-slate-600 to-slate-700',
  border: 'border-slate-800',
  text: 'text-white'
 };


 // Generate accessible label for platform badges
 function generateAccessibleLabel(group: any): string {
  const platformName = group.platform.display_name;
  const storefrontNames = group.storefronts
   .map((sf: any) => sf.storefront?.display_name)
   .filter(Boolean)
   .join(', ');
  
  return storefrontNames 
   ? `${platformName} - Available on: ${storefrontNames}. Click to expand details.`
   : `${platformName} - No specific storefront. Click to expand details.`;
 }

 // Toggle platform expansion
 function toggleExpansion(groupId: string, event?: Event) {
   if (event) {
     event.preventDefault();
     event.stopPropagation();
   }
   
   // Toggle expansion - collapse if already expanded, expand if not
   expandedPlatform = expandedPlatform === groupId ? null : groupId;
 }

 // Handle keyboard interactions
 function handleKeydown(groupId: string, event: KeyboardEvent) {
   if (event.key === 'Enter' || event.key === ' ') {
     event.preventDefault();
     toggleExpansion(groupId);
   } else if (event.key === 'Escape') {
     expandedPlatform = null;
   }
 }
</script>

{#if groupedPlatforms.length > 0}
 <div class="space-y-3">
  <!-- Platform Badges Row -->
  <div class="flex flex-wrap {compact ? 'gap-1.5' : 'gap-2 sm:gap-2.5'}">
   {#each visiblePlatforms as group (group.platform.id)}
    {@const platformIconUrl = buildIconUrl(group.platform.icon_url)}
    {@const groupId = `platform-${group.platform.id}`}
    {@const isExpanded = expandedPlatform === groupId}
    <div 
         class="relative inline-flex items-center rounded-lg transition-all duration-200 border-2 shadow-lg
                {unifiedBadgeStyle.bg} {unifiedBadgeStyle.border} {unifiedBadgeStyle.text}
                {compact 
                  ? 'px-2.5 py-1.5 min-h-[32px]' 
                  : 'px-3 py-2 min-h-[44px] sm:px-4 sm:py-2.5 sm:min-h-[40px]'}
                cursor-pointer hover:scale-105 hover:shadow-xl active:scale-95
                {isExpanded ? 'ring-2 ring-white ring-opacity-50' : ''}" 
         role="button" 
         tabindex="0"
         title="Click to {isExpanded ? 'collapse' : 'expand'} platform details"
         aria-label="{generateAccessibleLabel(group)}"
         aria-expanded="{isExpanded}"
         on:click={(e) => toggleExpansion(groupId, e)}
         on:keydown={(e) => handleKeydown(groupId, e)}
    >
     
     <!-- Platform Icon and Name -->
     <div class="flex items-center {compact ? 'gap-1.5' : 'gap-2'}">
      {#if platformIconUrl}
       <img 
        src="{platformIconUrl}" 
        alt="{group.platform.display_name} icon" 
        class="{compact ? 'w-4 h-4' : 'w-5 h-5'} flex-shrink-0"
        loading="lazy"
        on:error={(e) => {
         const img = e.target as HTMLImageElement;
         const fallback = img.nextElementSibling as HTMLElement;
         if (img && fallback) {
          img.style.display = 'none';
          fallback.style.display = 'inline';
         }
        }}
       />
       <span class="{compact ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'} hidden" role="img" aria-hidden="true">
        {getPlatformFallbackIcon()}
       </span>
      {:else}
       <span class="{compact ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'}" role="img" aria-hidden="true">
        {getPlatformFallbackIcon()}
       </span>
      {/if}
      <span class="font-bold {compact ? 'text-xs' : 'text-sm'} tracking-wide {compact ? '' : 'truncate max-w-[80px] sm:max-w-none'}">
       {group.platform.display_name}
      </span>
     </div>
     
     <!-- Storefronts - responsive display -->
     {#if group.storefronts.length > 0 && showDetails}
      <span class="mx-2 opacity-80 font-bold {compact ? 'mx-1.5 text-xs' : 'text-sm'}">•</span>
      <div class="flex items-center flex-wrap {compact ? 'gap-1' : 'gap-1.5'}">
       {#each group.storefronts as storefront, index (storefront.id)}
        <div class="inline-flex items-center {compact ? 'gap-1' : 'gap-1.5'}">
         {#if buildIconUrl(storefront.storefront?.icon_url)}
          <img 
           src="{buildIconUrl(storefront.storefront?.icon_url)}" 
           alt="{storefront.storefront?.display_name} icon" 
           class="{compact ? 'w-3 h-3' : 'w-4 h-4'} flex-shrink-0"
           loading="lazy"
           on:error={(e) => {
            const img = e.target as HTMLImageElement;
            const fallback = img.nextElementSibling as HTMLElement;
            if (img && fallback) {
             img.style.display = 'none';
             fallback.style.display = 'inline';
            }
           }}
          />
          <span class="{compact ? 'text-xs' : 'text-sm'} hidden" role="img" aria-hidden="true">
           {getStorefrontFallbackIcon()}
          </span>
         {:else}
          <span class="{compact ? 'text-xs' : 'text-sm'}" role="img" aria-hidden="true">
           {getStorefrontFallbackIcon()}
          </span>
         {/if}
         <span class="font-medium {compact ? 'text-xs' : 'text-sm'} 
                      bg-white bg-opacity-20 px-2 py-0.5 rounded-md
                      {compact ? 'px-1.5 py-0.5' : 'px-2 py-1'}" 
               title="{storefront.storefront?.display_name}">
          {storefront.storefront?.display_name}
         </span>
        </div>
        {#if index < group.storefronts.length - 1}
         <span class="opacity-60 {compact ? 'text-xs' : 'text-sm'} mx-0.5">•</span>
        {/if}
       {/each}
      </div>
     {:else if group.storefronts.length > 0}
      <!-- Compact storefront indicators -->
      <div class="ml-2 flex items-center {compact ? 'gap-1' : 'gap-1.5'}">
       <span class="opacity-80 {compact ? 'text-xs' : 'text-sm'}">•</span>
       <!-- On mobile show just count, on desktop show icons -->
       <div class="sm:hidden">
        <span class="inline-flex items-center justify-center w-5 h-5 bg-white bg-opacity-30 rounded-full text-xs font-bold">
         {group.storefronts.length}
        </span>
       </div>
       <div class="hidden sm:flex items-center -space-x-1">
        {#each group.storefronts.slice(0, 3) as storefront}
         <span class="inline-flex items-center justify-center w-5 h-5 bg-white bg-opacity-30 rounded-full text-xs" 
               title="{storefront.storefront?.display_name}">
          {#if buildIconUrl(storefront.storefront?.icon_url)}
           <img 
            src="{buildIconUrl(storefront.storefront?.icon_url)}" 
            alt="{storefront.storefront?.display_name} icon" 
            class="w-3 h-3 flex-shrink-0"
            loading="lazy"
            on:error={(e) => {
             const img = e.target as HTMLImageElement;
             const fallback = img.nextElementSibling as HTMLElement;
             if (img && fallback) {
              img.style.display = 'none';
              fallback.style.display = 'inline';
             }
            }}
           />
           <span class="text-xs hidden" role="img" aria-hidden="true">
            {getStorefrontFallbackIcon()}
           </span>
          {:else}
           <span class="text-xs" role="img" aria-hidden="true">
            {getStorefrontFallbackIcon()}
           </span>
          {/if}
         </span>
        {/each}
        {#if group.storefronts.length > 3}
         <span class="inline-flex items-center justify-center w-5 h-5 bg-white bg-opacity-30 rounded-full text-xs font-bold">
          +{group.storefronts.length - 3}
         </span>
        {/if}
       </div>
      </div>
     {/if}
     
     <!-- Expand/Collapse Indicator -->
     <div class="ml-2 flex items-center">
      <svg 
           class="w-4 h-4 transition-transform duration-200 {isExpanded ? 'rotate-180' : ''}" 
           fill="none" 
           stroke="currentColor" 
           viewBox="0 0 24 24">
       <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
      </svg>
     </div>
    </div>
   {/each}
   
   {#if hiddenCount > 0}
    {@const hiddenGroupId = 'hidden-platforms'}
    {@const isExpanded = expandedPlatform === hiddenGroupId}
    <div class="relative inline-flex items-center rounded-lg transition-all duration-200 border-2 shadow-lg
                bg-gradient-to-r from-gray-600 to-gray-700 border-gray-800 text-white
                {compact 
                  ? 'px-2.5 py-1.5 min-h-[32px]' 
                  : 'px-3 py-2 min-h-[44px] sm:px-4 sm:py-2.5 sm:min-h-[40px]'}
                cursor-pointer hover:scale-105 hover:shadow-xl active:scale-95
                {isExpanded ? 'ring-2 ring-white ring-opacity-50' : ''}"
         role="button"
         tabindex="0"
         title="Click to {isExpanded ? 'collapse' : 'expand'} additional platforms"
         aria-label="{isExpanded ? 'Collapse' : 'Show'} {hiddenCount} additional platform{hiddenCount !== 1 ? 's' : ''}"
         aria-expanded="{isExpanded}"
         on:click={(e) => toggleExpansion(hiddenGroupId, e)}
         on:keydown={(e) => handleKeydown(hiddenGroupId, e)}>
     <div class="flex items-center {compact ? 'gap-1.5' : 'gap-2'}">
      <span class="{compact ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'}" role="img" aria-hidden="true">📦</span>
      <span class="{compact ? 'text-xs' : 'text-sm'} font-bold">+{hiddenCount} {compact ? '' : 'more'}</span>
     </div>
     
     <!-- Expand/Collapse Indicator -->
     <div class="ml-2 flex items-center">
      <svg 
           class="w-4 h-4 transition-transform duration-200 {isExpanded ? 'rotate-180' : ''}" 
           fill="none" 
           stroke="currentColor" 
           viewBox="0 0 24 24">
       <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
      </svg>
     </div>
    </div>
   {/if}
  </div>
  
  <!-- Expanded Platform Details -->
  {#if expandedPlatform}
   <div class="mt-4 p-4 bg-white border border-gray-300 rounded-lg shadow-lg transition-all duration-300">
    {#if expandedPlatform === 'hidden-platforms'}
     <!-- Hidden Platforms Details -->
     {@const hiddenPlatforms = groupedPlatforms.slice(maxVisible)}
     <div class="space-y-4">
      <div class="flex items-center gap-2 pb-2 border-b border-gray-200">
       <span class="text-lg" role="img" aria-hidden="true">📦</span>
       <h4 class="font-semibold text-gray-900 text-lg">
        Additional Platforms ({hiddenCount})
       </h4>
      </div>
      
      <div class="space-y-3 max-h-64 overflow-y-auto">
       {#each hiddenPlatforms as group}
        {@const platformIconUrl = buildIconUrl(group.platform.icon_url)}
        <div class="flex items-start gap-3 p-3 bg-gray-50 rounded-md w-full">
         {#if platformIconUrl}
          <img 
           src="{platformIconUrl}" 
           alt="{group.platform.display_name} icon" 
           class="w-6 h-6 flex-shrink-0 mt-0.5"
           loading="lazy"
           on:error={(e) => {
            const img = e.target as HTMLImageElement;
            const fallback = img.nextElementSibling as HTMLElement;
            if (img && fallback) {
             img.style.display = 'none';
             fallback.style.display = 'inline';
            }
           }}
          />
          <span class="text-lg flex-shrink-0 mt-0.5 hidden" role="img" aria-hidden="true">{getPlatformFallbackIcon()}</span>
         {:else}
          <span class="text-lg flex-shrink-0 mt-0.5" role="img" aria-hidden="true">{getPlatformFallbackIcon()}</span>
         {/if}
         <div class="flex-1 min-w-0">
          <div class="font-bold text-gray-900 text-sm mb-2">{group.platform.display_name}</div>
          {#if group.storefronts.length > 0}
           <div class="space-y-2">
            <div class="space-y-1.5">
             {#each group.storefronts as storefront}
              <div class="flex items-center justify-between gap-2 text-sm">
               <div class="flex items-center gap-2 flex-1 min-w-0">
                {#if buildIconUrl(storefront.storefront?.icon_url)}
                 <img 
                  src="{buildIconUrl(storefront.storefront?.icon_url)}" 
                  alt="{storefront.storefront?.display_name} icon" 
                  class="w-4 h-4 flex-shrink-0"
                  loading="lazy"
                  on:error={(e) => {
                   const img = e.target as HTMLImageElement;
                   const fallback = img.nextElementSibling as HTMLElement;
                   if (img && fallback) {
                    img.style.display = 'none';
                    fallback.style.display = 'inline';
                   }
                  }}
                 />
                 <span class="text-sm hidden" role="img" aria-hidden="true">
                  {getStorefrontFallbackIcon()}
                 </span>
                {:else}
                 <span class="text-sm" role="img" aria-hidden="true">
                  {getStorefrontFallbackIcon()}
                 </span>
                {/if}
                <span class="text-gray-800 font-bold text-sm">
                 {storefront.storefront?.display_name || 'Unknown Store'}
                </span>
               </div>
               {#if showStoreLinks && storefront.store_url && storefront.storefront?.name !== 'physical'}
                <a 
                     href={storefront.store_url} 
                     target="_blank" 
                     rel="noopener noreferrer"
                     class="flex-shrink-0 text-blue-600 hover:text-blue-800 transition-colors"
                     title="Open in {storefront.storefront?.display_name}"
                     aria-label="Open {storefront.storefront?.display_name} store page"
                     on:click|stopPropagation
                >
                 <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                 </svg>
                </a>
               {/if}
              </div>
             {/each}
            </div>
           </div>
          {:else}
           <div class="text-sm text-gray-500 italic">No specific storefront</div>
          {/if}
         </div>
        </div>
       {/each}
      </div>
     </div>
    {:else}
     <!-- Single Platform Details -->
     {#each visiblePlatforms as group (group.platform.id)}
      {#if expandedPlatform === `platform-${group.platform.id}`}
       <div class="space-y-4">
        <!-- Storefronts List -->
        {#if group.storefronts.length > 0}
         <div class="space-y-3">
          <div class="grid gap-3 sm:grid-cols-2">
           {#each group.storefronts as storefront}
            <div class="flex items-center justify-between gap-3 p-3 bg-gray-50 rounded-md w-full">
             <div class="flex items-center gap-3 flex-1 min-w-0">
              {#if buildIconUrl(storefront.storefront?.icon_url)}
               <img 
                src="{buildIconUrl(storefront.storefront?.icon_url)}" 
                alt="{storefront.storefront?.display_name} icon" 
                class="w-5 h-5 flex-shrink-0"
                loading="lazy"
                on:error={(e) => {
                 const img = e.target as HTMLImageElement;
                 const fallback = img.nextElementSibling as HTMLElement;
                 if (img && fallback) {
                  img.style.display = 'none';
                  fallback.style.display = 'inline';
                 }
                }}
               />
               <span class="text-lg hidden" role="img" aria-hidden="true">
                {getStorefrontFallbackIcon()}
               </span>
              {:else}
               <span class="text-lg" role="img" aria-hidden="true">
                {getStorefrontFallbackIcon()}
               </span>
              {/if}
              <span class="text-gray-800 font-bold text-sm">
               {storefront.storefront?.display_name || 'Unknown Store'}
              </span>
             </div>
             {#if showStoreLinks && storefront.store_url && storefront.storefront?.name !== 'physical'}
              <a 
                   href={storefront.store_url} 
                   target="_blank" 
                   rel="noopener noreferrer"
                   class="flex-shrink-0 text-blue-600 hover:text-blue-800 transition-colors p-1"
                   title="Open in {storefront.storefront?.display_name}"
                   aria-label="Open {storefront.storefront?.display_name} store page"
                   on:click|stopPropagation
              >
               <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
               </svg>
              </a>
             {/if}
            </div>
           {/each}
          </div>
         </div>
        {:else}
         <div class="text-sm text-gray-500 italic bg-gray-50 p-4 rounded-md">
          No specific storefront information available
         </div>
        {/if}
        
        <!-- Instructions -->
        <div class="pt-3 border-t border-gray-200 text-sm text-gray-500">
         Click the platform badge again to collapse this view
        </div>
       </div>
      {/if}
     {/each}
    {/if}
   </div>
  {/if}
 </div>
{:else}
 <div class="inline-flex items-center {compact ? 'px-2.5 py-1.5' : 'px-3 py-2 sm:px-4 sm:py-2.5'} {compact ? 'min-h-[32px]' : 'min-h-[44px] sm:min-h-[40px]'} rounded-lg bg-gray-100 border border-gray-200">
  <span class="text-gray-500 {compact ? 'text-xs' : 'text-sm'} italic font-medium flex items-center gap-2" 
        role="status" 
        aria-label="No platforms available for this game">
   <span role="img" aria-hidden="true">❌</span>
   No platforms
  </span>
 </div>
{/if}