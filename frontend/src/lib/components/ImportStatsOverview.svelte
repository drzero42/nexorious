<script lang="ts">
  import type { ImportStatCard } from '$lib/types/import';
  
  interface Props {
    stats: ImportStatCard[];
    loading?: boolean;
  }

  let {
    stats,
    loading = false
  }: Props = $props();
</script>

<div class="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-5">
  {#each stats as stat (stat.label)}
    <div class="bg-white overflow-hidden shadow rounded-lg">
      <div class="p-5">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <span class="text-2xl {stat.color || ''}">{stat.icon}</span>
          </div>
          <div class="ml-5 w-0 flex-1">
            <dl>
              <dt class="text-sm font-medium text-gray-500 truncate">{stat.label}</dt>
              <dd class="text-lg font-medium text-gray-900">
                {#if loading}
                  <div class="animate-pulse">
                    <div class="h-6 bg-gray-200 rounded w-12"></div>
                  </div>
                {:else}
                  {stat.value}
                {/if}
              </dd>
            </dl>
          </div>
        </div>
      </div>
    </div>
  {/each}
</div>