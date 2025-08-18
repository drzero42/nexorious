<script lang="ts">
  import { tags, type Tag } from '$lib/stores';
  
  interface Props {
    selectedTagIds?: string[];
    availableTags?: Tag[];
    disabled?: boolean;
    class?: string;
    onchange?: (selectedTagIds: string[]) => void;
  }

  let {
    selectedTagIds = [],
    availableTags,
    disabled = false,
    class: className = '',
    onchange
  }: Props = $props();

  let searchQuery = $state('');
  
  // Use provided tags or fall back to store tags
  const allTags = $derived(availableTags || tags.value.tags);
  
  // Filter tags based on search query
  const filteredTags = $derived(() => {
    if (!searchQuery.trim()) return allTags;
    
    const query = searchQuery.toLowerCase();
    return allTags.filter(tag => 
      tag.name.toLowerCase().includes(query) ||
      tag.description?.toLowerCase().includes(query)
    );
  });

  // Separate selected and unselected tags
  const selectedTags = $derived(() => {
    return filteredTags().filter(tag => selectedTagIds.includes(tag.id));
  });

  const unselectedTags = $derived(() => {
    return filteredTags().filter(tag => !selectedTagIds.includes(tag.id));
  });

  // Sort tags by name
  const sortedSelectedTags = $derived(() => {
    return [...selectedTags()].sort((a, b) => a.name.localeCompare(b.name));
  });

  const sortedUnselectedTags = $derived(() => {
    return [...unselectedTags()].sort((a, b) => a.name.localeCompare(b.name));
  });

  const handleTagToggle = (tagId: string) => {
    if (disabled) return;
    
    const newSelectedTagIds = selectedTagIds.includes(tagId)
      ? selectedTagIds.filter(id => id !== tagId)
      : [...selectedTagIds, tagId];
    
    if (onchange) {
      onchange(newSelectedTagIds);
    }
  };

  const selectAll = () => {
    if (disabled) return;
    
    const allVisibleTagIds = filteredTags().map(tag => tag.id);
    if (onchange) {
      onchange(allVisibleTagIds);
    }
  };

  const selectNone = () => {
    if (disabled) return;
    
    if (onchange) {
      onchange([]);
    }
  };

  const toggleAll = () => {
    if (disabled) return;
    
    // If all visible tags are selected, deselect all; otherwise select all
    const allVisibleTagIds = filteredTags().map(tag => tag.id);
    const allVisibleSelected = allVisibleTagIds.every(id => selectedTagIds.includes(id));
    
    if (allVisibleSelected) {
      selectNone();
    } else {
      selectAll();
    }
  };
</script>

<!-- 
TagSelector Component
A multi-select interface for choosing from available tags.

Props:
- selectedTagIds: Currently selected tag IDs
- availableTags: Available tags to choose from (defaults to all tags from store)
- disabled: Whether the selector is disabled
- onchange: Callback when selection changes
-->

<div class="space-y-4 {className}">
  <!-- Search and Controls -->
  <div class="space-y-3">
    <!-- Search Input -->
    <div>
      <input
        type="text"
        class="
          block w-full rounded-md border-gray-300 shadow-sm text-sm
          focus:border-primary-500 focus:ring-primary-500
          {disabled ? 'bg-gray-50 text-gray-500 cursor-not-allowed' : ''}
        "
        placeholder="Search tags..."
        bind:value={searchQuery}
        {disabled}
      />
    </div>

    <!-- Quick Actions -->
    <div class="flex items-center justify-between">
      <div class="text-sm text-gray-700">
        {selectedTags().length} of {filteredTags().length} tags selected
      </div>
      
      <div class="flex gap-2">
        <button
          type="button"
          class="
            text-xs text-primary-600 hover:text-primary-800 font-medium
            disabled:text-gray-400 disabled:cursor-not-allowed
          "
          {disabled}
          onclick={selectAll}
        >
          Select All
        </button>
        <span class="text-gray-300">|</span>
        <button
          type="button"
          class="
            text-xs text-primary-600 hover:text-primary-800 font-medium
            disabled:text-gray-400 disabled:cursor-not-allowed
          "
          {disabled}
          onclick={selectNone}
        >
          Select None
        </button>
        <span class="text-gray-300">|</span>
        <button
          type="button"
          class="
            text-xs text-primary-600 hover:text-primary-800 font-medium
            disabled:text-gray-400 disabled:cursor-not-allowed
          "
          {disabled}
          onclick={toggleAll}
        >
          Toggle All
        </button>
      </div>
    </div>
  </div>

  <!-- No tags message -->
  {#if allTags.length === 0}
    <div class="text-center py-8 text-gray-500">
      <svg class="w-12 h-12 mx-auto mb-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.99 1.99 0 013 12V7a4 4 0 014-4z"/>
      </svg>
      <p class="text-sm">No tags available</p>
      <p class="text-xs mt-1">Create some tags first to use them here.</p>
    </div>
  {:else if filteredTags().length === 0}
    <div class="text-center py-8 text-gray-500">
      <svg class="w-8 h-8 mx-auto mb-2 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
      </svg>
      <p class="text-sm">No tags found matching "{searchQuery}"</p>
    </div>
  {:else}
    <!-- Tag Lists -->
    <div class="space-y-4 max-h-96 overflow-y-auto">
      <!-- Selected Tags -->
      {#if sortedSelectedTags().length > 0}
        <div>
          <h4 class="text-sm font-medium text-gray-900 mb-2 flex items-center gap-2">
            <svg class="w-4 h-4 text-green-600" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/>
            </svg>
            Selected ({sortedSelectedTags().length})
          </h4>
          <div class="grid grid-cols-1 gap-2">
            {#each sortedSelectedTags() as tag (tag.id)}
              <div class="
                flex items-center gap-3 p-2 bg-green-50 border border-green-200 rounded-lg
                {disabled ? 'opacity-50' : 'hover:bg-green-100 cursor-pointer'}
                transition-colors duration-200
              ">
                <input
                  type="checkbox"
                  class="h-4 w-4 text-primary-600 border-gray-300 rounded focus:ring-primary-500"
                  checked
                  {disabled}
                  onchange={() => handleTagToggle(tag.id)}
                />
                
                <div class="flex-1 flex items-center gap-2 min-w-0">
                  <div 
                    class="w-3 h-3 rounded-full border border-gray-300 flex-shrink-0"
                    style="background-color: {tag.color}"
                  ></div>
                  
                  <div class="min-w-0 flex-1">
                    <div class="font-medium text-sm text-gray-900 truncate">{tag.name}</div>
                    {#if tag.description}
                      <div class="text-xs text-gray-500 truncate">{tag.description}</div>
                    {/if}
                  </div>
                </div>
                
                {#if tag.game_count !== undefined && tag.game_count > 0}
                  <div class="text-xs text-gray-500 flex-shrink-0">
                    {tag.game_count} game{tag.game_count !== 1 ? 's' : ''}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <!-- Available Tags -->
      {#if sortedUnselectedTags().length > 0}
        <div>
          <h4 class="text-sm font-medium text-gray-900 mb-2 flex items-center gap-2">
            <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.99 1.99 0 013 12V7a4 4 0 014-4z"/>
            </svg>
            Available ({sortedUnselectedTags().length})
          </h4>
          <div class="grid grid-cols-1 gap-2">
            {#each sortedUnselectedTags() as tag (tag.id)}
              <div class="
                flex items-center gap-3 p-2 bg-white border border-gray-200 rounded-lg
                {disabled ? 'opacity-50' : 'hover:bg-gray-50 cursor-pointer'}
                transition-colors duration-200
              ">
                <input
                  type="checkbox"
                  class="h-4 w-4 text-primary-600 border-gray-300 rounded focus:ring-primary-500"
                  {disabled}
                  onchange={() => handleTagToggle(tag.id)}
                />
                
                <div class="flex-1 flex items-center gap-2 min-w-0">
                  <div 
                    class="w-3 h-3 rounded-full border border-gray-300 flex-shrink-0"
                    style="background-color: {tag.color}"
                  ></div>
                  
                  <div class="min-w-0 flex-1">
                    <div class="font-medium text-sm text-gray-900 truncate">{tag.name}</div>
                    {#if tag.description}
                      <div class="text-xs text-gray-500 truncate">{tag.description}</div>
                    {/if}
                  </div>
                </div>
                
                {#if tag.game_count !== undefined && tag.game_count > 0}
                  <div class="text-xs text-gray-500 flex-shrink-0">
                    {tag.game_count} game{tag.game_count !== 1 ? 's' : ''}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>