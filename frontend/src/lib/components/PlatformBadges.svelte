<script lang="ts">
 import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
 import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';

 export let platforms: UserGamePlatform[] = [];
 export let compact: boolean = false;
 export let maxVisible: number = 3;

 $: groupedPlatforms = groupPlatformsByPlatform(platforms);
 $: visiblePlatforms = groupedPlatforms.slice(0, maxVisible);
 $: hiddenCount = Math.max(0, groupedPlatforms.length - maxVisible);
</script>

{#if groupedPlatforms.length > 0}
 <div class="flex flex-wrap gap-1 {compact ? 'text-xs' : 'text-sm'}">
  {#each visiblePlatforms as group (group.platform.id)}
   <div class="inline-flex items-center rounded-full px-2 py-1 bg-blue-100 text-blue-800 border border-blue-200">
    <span class="font-medium {compact ? 'text-xs' : 'text-sm'}">{group.platform.display_name}</span>
    {#if group.storefronts.length > 0}
     <span class="mx-1 text-blue-600">:</span>
     <span class="{compact ? 'text-xs' : 'text-sm'} truncate max-w-24" title="{group.storefronts.map(sf => sf.storefront?.display_name).filter(Boolean).join(', ')}">
      {group.storefronts.map(sf => sf.storefront?.display_name).filter(Boolean).join(', ')}
     </span>
    {/if}
   </div>
  {/each}
  
  {#if hiddenCount > 0}
   <div class="inline-flex items-center rounded-full px-2 py-1 bg-gray-100 text-gray-600 border border-gray-200">
    <span class="{compact ? 'text-xs' : 'text-sm'} font-medium">+{hiddenCount} more</span>
   </div>
  {/if}
 </div>
{:else}
 <span class="text-gray-400 {compact ? 'text-xs' : 'text-sm'}">No platforms</span>
{/if}