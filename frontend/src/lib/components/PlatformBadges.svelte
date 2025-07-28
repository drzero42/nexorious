<script lang="ts">
 import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
 import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';

 export let platforms: UserGamePlatform[] = [];
 export let compact: boolean = false;
 export let maxVisible: number = 3;

 $: groupedPlatforms = groupPlatformsByPlatform(platforms);
 $: visiblePlatforms = groupedPlatforms.slice(0, maxVisible);
 $: hiddenCount = Math.max(0, groupedPlatforms.length - maxVisible);

 // Enhanced platform-specific styling with better contrast and visual distinction
 function getPlatformColor(platformName: string): string {
  const name = platformName.toLowerCase();
  if (name.includes('playstation')) return 'bg-blue-700 text-white border-2 border-blue-800 shadow-md';
  if (name.includes('xbox')) return 'bg-green-700 text-white border-2 border-green-800 shadow-md';
  if (name.includes('nintendo') || name.includes('switch')) return 'bg-red-700 text-white border-2 border-red-800 shadow-md';
  if (name.includes('pc') || name.includes('windows')) return 'bg-gray-800 text-white border-2 border-gray-900 shadow-md';
  if (name.includes('ios')) return 'bg-gray-900 text-white border-2 border-black shadow-md';
  if (name.includes('android')) return 'bg-green-800 text-white border-2 border-green-900 shadow-md';
  return 'bg-indigo-700 text-white border-2 border-indigo-800 shadow-md'; // Default color
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
 <div class="flex flex-wrap gap-2 {compact ? 'gap-1.5' : 'gap-2.5'}">
  {#each visiblePlatforms as group (group.platform.id)}
   <div class="inline-flex items-center rounded-lg transition-all duration-200 hover:scale-105 cursor-help
               {getPlatformColor(group.platform.display_name)} 
               {compact ? 'px-2.5 py-1.5 min-h-[28px]' : 'px-4 py-2 min-h-[36px]'}" 
        role="status" 
        title="{generateTooltipText(group)}"
        aria-label="{generateTooltipText(group)}"
>
    
    <!-- Platform Name -->
    <span class="font-bold {compact ? 'text-xs' : 'text-sm'} tracking-wide">
     {group.platform.display_name}
    </span>
    
    <!-- Storefronts -->
    {#if group.storefronts.length > 0}
     <span class="mx-2 opacity-80 font-bold {compact ? 'mx-1.5 text-xs' : 'text-sm'}">•</span>
     <div class="flex items-center flex-wrap {compact ? 'gap-1' : 'gap-1.5'}">
      {#each group.storefronts as storefront, index (storefront.id)}
       {#if storefront.storefront?.display_name?.toLowerCase() === 'physical'}
        <!-- Special styling for Physical storefront -->
        <span class="inline-flex items-center bg-amber-200 text-amber-900 px-2 py-0.5 rounded-full 
                     text-xs font-bold border border-amber-400 shadow-sm
                     {compact ? 'px-1.5 py-0.5' : 'px-2 py-1'}">
         📦 {storefront.storefront.display_name}
        </span>
       {:else}
        <!-- Regular digital storefront -->
        <span class="font-semibold {compact ? 'text-xs' : 'text-sm'} 
                     bg-white bg-opacity-20 px-2 py-0.5 rounded-md
                     {compact ? 'px-1.5 py-0.5' : 'px-2 py-1'}" 
              title="{storefront.storefront?.display_name}">
         {storefront.storefront?.display_name}
        </span>
       {/if}
       {#if index < group.storefronts.length - 1 && storefront.storefront?.display_name?.toLowerCase() !== 'physical'}
        <span class="opacity-60 {compact ? 'text-xs' : 'text-sm'} mx-0.5">•</span>
       {/if}
      {/each}
     </div>
    {/if}
   </div>
  {/each}
  
  {#if hiddenCount > 0}
   <div class="inline-flex items-center rounded-lg transition-all duration-200 hover:scale-105 cursor-help
               bg-gray-600 text-white border-2 border-gray-700 shadow-md
               {compact ? 'px-2.5 py-1.5 min-h-[28px]' : 'px-4 py-2 min-h-[36px]'}"
        title="There are {hiddenCount} more platform{hiddenCount !== 1 ? 's' : ''} not shown"
        aria-label="There are {hiddenCount} more platform{hiddenCount !== 1 ? 's' : ''} not shown">
    <span class="{compact ? 'text-xs' : 'text-sm'} font-bold">+{hiddenCount} more</span>
   </div>
  {/if}
 </div>
{:else}
 <span class="text-gray-400 {compact ? 'text-xs' : 'text-sm'} italic font-medium" 
       role="status" 
       aria-label="No platforms available for this game">
  No platforms
 </span>
{/if}