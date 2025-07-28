<script lang="ts">
 import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
 import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';

 export let platforms: UserGamePlatform[] = [];
 export let compact: boolean = false;
 export let maxVisible: number = 3;

 $: groupedPlatforms = groupPlatformsByPlatform(platforms);
 $: visiblePlatforms = groupedPlatforms.slice(0, maxVisible);
 $: hiddenCount = Math.max(0, groupedPlatforms.length - maxVisible);

 // Platform-specific styling for better visual distinction
 function getPlatformColor(platformName: string): string {
  const name = platformName.toLowerCase();
  if (name.includes('playstation')) return 'bg-blue-600 text-white border-blue-700';
  if (name.includes('xbox')) return 'bg-green-600 text-white border-green-700';
  if (name.includes('nintendo') || name.includes('switch')) return 'bg-red-600 text-white border-red-700';
  if (name.includes('pc') || name.includes('windows')) return 'bg-gray-700 text-white border-gray-800';
  if (name.includes('ios')) return 'bg-gray-800 text-white border-gray-900';
  if (name.includes('android')) return 'bg-green-700 text-white border-green-800';
  return 'bg-indigo-600 text-white border-indigo-700'; // Default color
 }
</script>

{#if groupedPlatforms.length > 0}
 <div class="flex flex-wrap gap-1.5 {compact ? '' : 'gap-2'}">
  {#each visiblePlatforms as group (group.platform.id)}
   <div class="inline-flex items-center rounded-lg px-3 py-1.5 {getPlatformColor(group.platform.display_name)} shadow-sm {compact ? 'px-2 py-1' : ''}" 
        role="group" 
        aria-label="Platform: {group.platform.display_name}{group.storefronts.length > 0 ? ' - Storefronts: ' + group.storefronts.map(sf => sf.storefront?.display_name).filter(Boolean).join(', ') : ''}">
    <span class="font-semibold {compact ? 'text-xs' : 'text-sm'}">{group.platform.display_name}</span>
    {#if group.storefronts.length > 0}
     <span class="mx-1.5 opacity-75 {compact ? 'mx-1' : ''}">•</span>
     <div class="flex items-center {compact ? 'gap-1' : 'gap-1.5'}">
      {#each group.storefronts as storefront, index (storefront.id)}
       <span class="{compact ? 'text-xs' : 'text-sm'} opacity-90 font-medium" 
             title="{storefront.storefront?.display_name}">
        {storefront.storefront?.display_name}
       </span>
       {#if index < group.storefronts.length - 1}
        <span class="opacity-50 {compact ? 'text-xs' : 'text-sm'}">,</span>
       {/if}
      {/each}
     </div>
    {/if}
   </div>
  {/each}
  
  {#if hiddenCount > 0}
   <div class="inline-flex items-center rounded-lg px-3 py-1.5 bg-gray-500 text-white border border-gray-600 shadow-sm {compact ? 'px-2 py-1' : ''}">
    <span class="{compact ? 'text-xs' : 'text-sm'} font-medium">+{hiddenCount} more</span>
   </div>
  {/if}
 </div>
{:else}
 <span class="text-gray-400 {compact ? 'text-xs' : 'text-sm'} italic">No platforms</span>
{/if}