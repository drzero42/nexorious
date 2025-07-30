<script lang="ts">
 import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
 import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';

 export let platforms: UserGamePlatform[] = [];
 export let compact: boolean = false;
 export let maxVisible: number = 3;
 export let showDetails: boolean = false; // For expanded detail view
 export let enableHover: boolean = true; // To control hover interactions

 $: groupedPlatforms = groupPlatformsByPlatform(platforms);
 $: visiblePlatforms = groupedPlatforms.slice(0, maxVisible);
 $: hiddenCount = Math.max(0, groupedPlatforms.length - maxVisible);

 // Enhanced platform-specific styling with better visual design
 function getPlatformStyle(platformName: string): { bg: string, border: string, text: string, icon: string } {
  const name = platformName.toLowerCase();
  
  if (name.includes('playstation')) {
    return {
      bg: 'bg-gradient-to-r from-blue-600 to-blue-700',
      border: 'border-blue-800',
      text: 'text-white',
      icon: '🎮'
    };
  }
  if (name.includes('xbox')) {
    return {
      bg: 'bg-gradient-to-r from-green-600 to-green-700', 
      border: 'border-green-800',
      text: 'text-white',
      icon: '🎮'
    };
  }
  if (name.includes('nintendo') || name.includes('switch')) {
    return {
      bg: 'bg-gradient-to-r from-red-600 to-red-700',
      border: 'border-red-800', 
      text: 'text-white',
      icon: '🎮'
    };
  }
  if (name.includes('pc') || name.includes('windows')) {
    return {
      bg: 'bg-gradient-to-r from-gray-700 to-gray-800',
      border: 'border-gray-900',
      text: 'text-white',
      icon: '💻'
    };
  }
  if (name.includes('ios')) {
    return {
      bg: 'bg-gradient-to-r from-gray-800 to-gray-900',
      border: 'border-black',
      text: 'text-white',
      icon: '📱'
    };
  }
  if (name.includes('android')) {
    return {
      bg: 'bg-gradient-to-r from-green-700 to-green-800',
      border: 'border-green-900',
      text: 'text-white',
      icon: '📱'
    };
  }
  
  // Default style
  return {
    bg: 'bg-gradient-to-r from-indigo-600 to-indigo-700',
    border: 'border-indigo-800',
    text: 'text-white',
    icon: '🎯'
  };
 }

 // Get storefront-specific styling and icons
 function getStorefrontIcon(storefrontName: string): string {
  const name = storefrontName?.toLowerCase() || '';
  if (name.includes('steam')) return '🔥';
  if (name.includes('epic')) return '🎮';
  if (name.includes('gog')) return '🏪';
  if (name.includes('playstation')) return '🎮';
  if (name.includes('microsoft') || name.includes('xbox')) return '🎮';
  if (name.includes('nintendo')) return '🎮';
  if (name.includes('app store') || name.includes('apple')) return '📱';
  if (name.includes('google play')) return '🤖';
  if (name.includes('physical')) return '📦';
  if (name.includes('humble')) return '🎁';
  if (name.includes('itch')) return '🕹️';
  if (name.includes('origin') || name.includes('ea')) return '🎮';
  return '🏪'; // Default store icon
 }

 // Generate tooltip text for accessibility and hover details
 function generateTooltipText(group: any): string {
  const platformName = group.platform.display_name;
  const storefrontNames = group.storefronts
   .map((sf: any) => sf.storefront?.display_name)
   .filter(Boolean)
   .join(', ');
  
  return storefrontNames 
   ? `${platformName} - Available on: ${storefrontNames}`
   : `${platformName} - No specific storefront`;
 }
</script>

{#if groupedPlatforms.length > 0}
 <div class="flex flex-wrap {compact ? 'gap-1.5' : 'gap-2 sm:gap-2.5'}">
  {#each visiblePlatforms as group (group.platform.id)}
   {@const platformStyle = getPlatformStyle(group.platform.display_name)}
   <div class="inline-flex items-center rounded-lg transition-all duration-200 border-2 shadow-lg
               {platformStyle.bg} {platformStyle.border} {platformStyle.text}
               {compact 
                 ? 'px-2.5 py-1.5 min-h-[32px]' 
                 : 'px-3 py-2 min-h-[44px] sm:px-4 sm:py-2.5 sm:min-h-[40px]'}
               {enableHover ? 'active:scale-95 sm:hover:scale-105 sm:hover:shadow-xl cursor-help' : ''}" 
        role="status" 
        title="{generateTooltipText(group)}"
        aria-label="{generateTooltipText(group)}"
   >
    
    <!-- Platform Icon and Name -->
    <div class="flex items-center {compact ? 'gap-1.5' : 'gap-2'}">
     <span class="{compact ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'}" role="img" aria-hidden="true">
      {platformStyle.icon}
     </span>
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
        <span class="{compact ? 'text-xs' : 'text-sm'}" role="img" aria-hidden="true">
         {getStorefrontIcon(storefront.storefront?.display_name || '')}
        </span>
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
         {getStorefrontIcon(storefront.storefront?.display_name || '')}
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
   </div>
  {/each}
  
  {#if hiddenCount > 0}
   <div class="inline-flex items-center rounded-lg transition-all duration-200 border-2 shadow-lg
               bg-gradient-to-r from-gray-600 to-gray-700 border-gray-800 text-white
               {compact 
                 ? 'px-2.5 py-1.5 min-h-[32px]' 
                 : 'px-3 py-2 min-h-[44px] sm:px-4 sm:py-2.5 sm:min-h-[40px]'}
               {enableHover ? 'active:scale-95 sm:hover:scale-105 sm:hover:shadow-xl cursor-help' : ''}"
        title="There are {hiddenCount} more platform{hiddenCount !== 1 ? 's' : ''} not shown"
        aria-label="There are {hiddenCount} more platform{hiddenCount !== 1 ? 's' : ''} not shown">
    <div class="flex items-center {compact ? 'gap-1.5' : 'gap-2'}">
     <span class="{compact ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'}" role="img" aria-hidden="true">📦</span>
     <span class="{compact ? 'text-xs' : 'text-sm'} font-bold">+{hiddenCount} {compact ? '' : 'more'}</span>
    </div>
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