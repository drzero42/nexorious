<script lang="ts">
  import { tags, userGames, type Tag } from '$lib/stores';
  import { TagInput, TagBadge } from '../core';
  
  interface Props {
    userGameId: string;
    currentTags?: Tag[];
    disabled?: boolean;
    class?: string;
  }

  let {
    userGameId,
    currentTags = [],
    disabled = false,
    class: className = ''
  }: Props = $props();

  let isLoading = $state(false);
  let error = $state<string | null>(null);

  // Track changes for optimistic updates
  let pendingChanges = $state<{
    toAdd: Tag[];
    toRemove: Tag[];
  }>({
    toAdd: [],
    toRemove: []
  });

  const hasChanges = $derived(
    pendingChanges.toAdd.length > 0 || pendingChanges.toRemove.length > 0
  );

  // Determine which tags to show (current + pending additions - pending removals)
  const displayTags = $derived(() => {
    const currentTagIds = new Set(currentTags.map(t => t.id));
    const toRemoveIds = new Set(pendingChanges.toRemove.map(t => t.id));

    return [
      // Keep current tags that aren't being removed
      ...currentTags.filter(tag => !toRemoveIds.has(tag.id)),
      // Add new tags that aren't already current
      ...pendingChanges.toAdd.filter(tag => !currentTagIds.has(tag.id))
    ];
  });

  const handleTagAdd = (tag: Tag) => {
    const isCurrentTag = currentTags.some(t => t.id === tag.id);
    const isAlreadyPendingRemoval = pendingChanges.toRemove.some(t => t.id === tag.id);
    const isAlreadyPendingAdd = pendingChanges.toAdd.some(t => t.id === tag.id);

    if (isCurrentTag && isAlreadyPendingRemoval) {
      // If it was marked for removal, un-mark it
      pendingChanges.toRemove = pendingChanges.toRemove.filter(t => t.id !== tag.id);
    } else if (!isCurrentTag && !isAlreadyPendingAdd) {
      // Add to pending additions
      pendingChanges.toAdd = [...pendingChanges.toAdd, tag];
    }
  };

  const handleTagRemove = (tag: Tag) => {
    const isCurrentTag = currentTags.some(t => t.id === tag.id);
    const isAlreadyPendingAdd = pendingChanges.toAdd.some(t => t.id === tag.id);

    if (isCurrentTag) {
      // Mark current tag for removal
      pendingChanges.toRemove = [...pendingChanges.toRemove, tag];
    } else if (isAlreadyPendingAdd) {
      // Remove from pending additions
      pendingChanges.toAdd = pendingChanges.toAdd.filter(t => t.id !== tag.id);
    }
  };

  const handleTagChange = (_newSelectedTags: Tag[]) => {
    // This function is not currently used but kept for potential future use
  };

  const saveChanges = async () => {
    if (!hasChanges || isLoading) return;

    isLoading = true;
    error = null;

    try {
      // Apply removals first
      if (pendingChanges.toRemove.length > 0) {
        const tagIdsToRemove = pendingChanges.toRemove.map(t => t.id);
        await tags.removeTagsFromGame(userGameId, tagIdsToRemove);
      }

      // Apply additions
      if (pendingChanges.toAdd.length > 0) {
        const tagIdsToAdd = pendingChanges.toAdd.map(t => t.id);
        await tags.assignTagsToGame(userGameId, tagIdsToAdd);
      }

      // Update current tags to reflect the changes
      currentTags = displayTags();
      
      // Clear pending changes
      pendingChanges = { toAdd: [], toRemove: [] };

      // Refresh user game data to get updated tag associations
      // userGameId is a UserGameId (UUID), so use getUserGameById
      await userGames.getUserGameById(userGameId as any);

    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save tag changes';
      console.error('Error saving tag changes:', err);
    } finally {
      isLoading = false;
    }
  };

  const cancelChanges = () => {
    pendingChanges = { toAdd: [], toRemove: [] };
    error = null;
  };

  // Auto-save functionality (optional)
  const autoSave = $state(false);
  
  $effect(() => {
    if (autoSave && hasChanges && !isLoading) {
      // Debounce auto-save
      const timeoutId = setTimeout(() => {
        saveChanges();
      }, 1000);
      
      return () => clearTimeout(timeoutId);
    }
    return undefined;
  });
</script>

<!-- 
GameTagEditor Component
Allows editing tags for a specific game with inline creation and optimistic updates.

Props:
- userGameId: ID of the user game to edit tags for
- currentTags: Currently assigned tags
- disabled: Whether the editor is disabled
-->

<div class="space-y-4 {className}">
  <!-- Error Display -->
  {#if error}
    <div class="bg-red-50 border border-red-200 rounded-md p-3">
      <div class="flex">
        <svg class="h-5 w-5 text-red-400" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd"/>
        </svg>
        <div class="ml-3">
          <h3 class="text-sm font-medium text-red-800">Error saving tags</h3>
          <div class="mt-1 text-sm text-red-700">{error}</div>
        </div>
        <div class="ml-auto pl-3">
          <button
            type="button"
            class="text-red-400 hover:text-red-600"
            onclick={() => error = null}
          >
            <span class="sr-only">Dismiss</span>
            <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd"/>
            </svg>
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Current Tags Display -->
  <div class="space-y-2">
    <div class="block text-sm font-medium text-gray-700 mb-2">
      Tags
      {#if displayTags().length > 0}
        <span class="text-gray-500">({displayTags().length})</span>
      {/if}
    </div>
    
    {#if displayTags().length > 0}
      <div class="flex flex-wrap gap-1">
        {#each displayTags() as tag (tag.id)}
          {@const isPendingRemoval = pendingChanges.toRemove.some(t => t.id === tag.id)}
          {@const isPendingAdd = pendingChanges.toAdd.some(t => t.id === tag.id)}
          
          <TagBadge
            {tag}
            removable={!disabled}
            class={`
              transition-all duration-200
              ${isPendingRemoval ? 'opacity-50 line-through' : ''}
              ${isPendingAdd ? 'ring-2 ring-green-500 ring-opacity-50' : ''}
            `}
            onremove={handleTagRemove}
          />
        {/each}
      </div>
    {:else}
      <div class="text-sm text-gray-500 italic">
        No tags assigned. Use the input below to add tags.
      </div>
    {/if}
  </div>

  <!-- Tag Input -->
  <TagInput
    selectedTags={[]}
    placeholder="Add tags..."
    allowCreate={true}
    {disabled}
    onadd={handleTagAdd}
    onchange={handleTagChange}
  />

  <!-- Pending Changes Summary -->
  {#if hasChanges}
    <div class="bg-blue-50 border border-blue-200 rounded-md p-3">
      <div class="text-sm text-blue-800">
        <div class="font-medium mb-1">Pending Changes:</div>
        <ul class="list-disc list-inside space-y-1">
          {#if pendingChanges.toAdd.length > 0}
            <li>
              Add {pendingChanges.toAdd.length} tag{pendingChanges.toAdd.length !== 1 ? 's' : ''}:
              <span class="font-medium">
                {pendingChanges.toAdd.map(t => t.name).join(', ')}
              </span>
            </li>
          {/if}
          {#if pendingChanges.toRemove.length > 0}
            <li>
              Remove {pendingChanges.toRemove.length} tag{pendingChanges.toRemove.length !== 1 ? 's' : ''}:
              <span class="font-medium">
                {pendingChanges.toRemove.map(t => t.name).join(', ')}
              </span>
            </li>
          {/if}
        </ul>
      </div>
    </div>
  {/if}

  <!-- Action Buttons -->
  {#if hasChanges}
    <div class="flex items-center gap-3 pt-2 border-t border-gray-200">
      <button
        type="button"
        class="
          px-4 py-2 bg-primary-600 text-white text-sm font-medium rounded-md
          hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500
          disabled:opacity-50 disabled:cursor-not-allowed
        "
        disabled={isLoading || disabled}
        onclick={saveChanges}
      >
        {#if isLoading}
          <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white inline" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          Saving...
        {:else}
          Save Changes
        {/if}
      </button>
      
      <button
        type="button"
        class="
          px-4 py-2 bg-gray-300 text-gray-700 text-sm font-medium rounded-md
          hover:bg-gray-400 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500
          disabled:opacity-50 disabled:cursor-not-allowed
        "
        disabled={isLoading || disabled}
        onclick={cancelChanges}
      >
        Cancel
      </button>
    </div>
  {/if}

  <!-- Auto-save Toggle (optional feature) -->
  <!-- Uncomment if you want auto-save functionality -->
  <!--
  <div class="flex items-center">
    <input
      type="checkbox"
      id="autosave-{userGameId}"
      class="h-4 w-4 text-primary-600 border-gray-300 rounded focus:ring-primary-500"
      bind:checked={autoSave}
      {disabled}
    />
    <label for="autosave-{userGameId}" class="ml-2 text-sm text-gray-700">
      Auto-save changes
    </label>
  </div>
  -->
</div>