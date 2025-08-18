<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { tags, type Tag } from '$lib/stores';
  import { TagBadge, TagInput } from '../core';

  interface Props {
    class?: string;
  }

  let { class: className = '' }: Props = $props();

  let showFilterDropdown = $state(false);

  // Parse active tag filters from URL
  const activeTagIds = $derived(() => {
    const tagParam = $page.url.searchParams.get('tag');
    if (!tagParam) return [];
    
    // Support both single tag and comma-separated multiple tags
    return tagParam.split(',').filter(Boolean);
  });

  // Get active tag objects
  const activeTags = $derived(() => {
    return tags.value.tags.filter(tag => activeTagIds().includes(tag.id));
  });

  // Get popular tags for quick filtering
  const popularTags = $derived(() => {
    return tags.getPopularTags(10);
  });

  const addTagFilter = (tag: Tag) => {
    if (activeTagIds().includes(tag.id)) return;
    
    const newTagIds = [...activeTagIds(), tag.id];
    updateTagFilter(newTagIds);
    showFilterDropdown = false;
  };

  const removeTagFilter = (tag: Tag) => {
    const newTagIds = activeTagIds().filter(id => id !== tag.id);
    updateTagFilter(newTagIds);
  };

  const clearAllFilters = () => {
    updateTagFilter([]);
  };

  const updateTagFilter = (tagIds: string[]) => {
    const url = new URL($page.url);
    
    if (tagIds.length > 0) {
      url.searchParams.set('tag', tagIds.join(','));
    } else {
      url.searchParams.delete('tag');
    }
    
    // Reset page to 1 when changing filters
    url.searchParams.delete('page');
    
    goto(url.toString(), { replaceState: true });
  };

  const toggleFilterDropdown = () => {
    showFilterDropdown = !showFilterDropdown;
  };

  // Close dropdown when clicking outside
  const handleDocumentClick = (event: MouseEvent) => {
    const target = event.target as HTMLElement;
    if (!target.closest('.tag-filter-dropdown')) {
      showFilterDropdown = false;
    }
  };

  $effect(() => {
    if (showFilterDropdown) {
      document.addEventListener('click', handleDocumentClick);
      return () => {
        document.removeEventListener('click', handleDocumentClick);
      };
    }
    return undefined;
  });

  // Load tags when component mounts
  $effect(() => {
    if (tags.value.tags.length === 0 && !tags.value.isLoading) {
      tags.fetchTags();
    }
  });
</script>

<!-- 
TagFilter Component
Provides tag-based filtering interface for the games collection.

Features:
- Shows active tag filters as removable badges
- Quick access to popular tags
- Search and select from all available tags
- URL state management for filters
-->

<div class="tag-filter-dropdown relative {className}">
  <!-- Active Filters Display -->
  {#if activeTags().length > 0}
    <div class="flex items-center gap-2 mb-3">
      <span class="text-sm font-medium text-gray-700">Filtered by:</span>
      <div class="flex flex-wrap gap-1">
        {#each activeTags() as tag (tag.id)}
          <TagBadge
            {tag}
            removable
            size="sm"
            clickable={false}
            onremove={removeTagFilter}
          />
        {/each}
      </div>
      <button
        type="button"
        class="text-xs text-gray-500 hover:text-gray-700 underline"
        onclick={clearAllFilters}
      >
        Clear all
      </button>
    </div>
  {/if}

  <!-- Filter Controls -->
  <div class="relative">
    <button
      type="button"
      class="
        flex items-center gap-2 px-3 py-2 bg-white border border-gray-300 rounded-md shadow-sm text-sm
        hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500
        {showFilterDropdown ? 'ring-2 ring-primary-500' : ''}
      "
      onclick={toggleFilterDropdown}
      aria-expanded={showFilterDropdown}
      aria-haspopup="true"
    >
      <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.99 1.99 0 013 12V7a4 4 0 014-4z"/>
      </svg>
      <span>Filter by Tags</span>
      <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={showFilterDropdown ? "m18 15l-6-6-6 6" : "m6 9l6 6 6-6"}/>
      </svg>
    </button>

    <!-- Dropdown Menu -->
    {#if showFilterDropdown}
      <div class="
        absolute z-50 mt-2 w-80 bg-white border border-gray-300 rounded-md shadow-lg
        max-h-96 overflow-hidden
      ">
        <div class="p-4 space-y-4">
          <!-- Tag Search Input -->
          <div>
            <TagInput
              selectedTags={[]}
              placeholder="Search tags to add filter..."
              allowCreate={false}
              maxTags={1}
              onadd={addTagFilter}
            />
          </div>

          <!-- Popular Tags Quick Filter -->
          {#if popularTags().length > 0}
            <div>
              <h4 class="text-sm font-medium text-gray-900 mb-2">Popular Tags</h4>
              <div class="flex flex-wrap gap-1">
                {#each popularTags().slice(0, 8) as tag (tag.id)}
                  {#if !activeTagIds().includes(tag.id)}
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 px-2 py-1 text-xs border border-gray-300 rounded-full hover:bg-gray-50"
                      onclick={() => addTagFilter(tag)}
                    >
                      <div 
                        class="w-2 h-2 rounded-full"
                        style="background-color: {tag.color}"
                      ></div>
                      {tag.name}
                      {#if tag.game_count}
                        <span class="text-gray-500">({tag.game_count})</span>
                      {/if}
                    </button>
                  {/if}
                {/each}
              </div>
            </div>
          {/if}

          <!-- All Tags List (scrollable) -->
          {#if tags.value.tags.length > popularTags().length}
            <div>
              <h4 class="text-sm font-medium text-gray-900 mb-2">All Tags</h4>
              <div class="max-h-40 overflow-y-auto space-y-1">
                {#each tags.value.tags as tag (tag.id)}
                  {#if !activeTagIds().includes(tag.id)}
                    <button
                      type="button"
                      class="
                        w-full flex items-center gap-2 px-2 py-1.5 text-left text-sm
                        hover:bg-gray-100 rounded
                      "
                      onclick={() => addTagFilter(tag)}
                    >
                      <div 
                        class="w-3 h-3 rounded-full flex-shrink-0"
                        style="background-color: {tag.color}"
                      ></div>
                      <div class="flex-1 min-w-0">
                        <div class="font-medium text-gray-900 truncate">{tag.name}</div>
                        {#if tag.description}
                          <div class="text-xs text-gray-500 truncate">{tag.description}</div>
                        {/if}
                      </div>
                      {#if tag.game_count}
                        <div class="text-xs text-gray-500 flex-shrink-0">
                          {tag.game_count}
                        </div>
                      {/if}
                    </button>
                  {/if}
                {/each}
              </div>
            </div>
          {/if}

          <!-- No tags message -->
          {#if tags.value.tags.length === 0 && !tags.value.isLoading}
            <div class="text-center py-4 text-gray-500">
              <svg class="w-8 h-8 mx-auto mb-2 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.99 1.99 0 013 12V7a4 4 0 014-4z"/>
              </svg>
              <p class="text-sm">No tags available</p>
              <p class="text-xs mt-1">Create tags by editing games.</p>
            </div>
          {/if}

          <!-- Loading state -->
          {#if tags.value.isLoading}
            <div class="text-center py-4">
              <div class="inline-flex items-center text-sm text-gray-500">
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Loading tags...
              </div>
            </div>
          {/if}
        </div>
      </div>
    {/if}
  </div>

  <!-- Filter Summary -->
  {#if activeTags().length > 0}
    <div class="mt-2 text-sm text-gray-600">
      Showing games tagged with: 
      <span class="font-medium">
        {activeTags().map(t => t.name).join(', ')}
      </span>
    </div>
  {/if}
</div>

<style>
  /* Ensure dropdown stays above other content */
  .tag-filter-dropdown {
    position: relative;
    z-index: 10;
  }
</style>