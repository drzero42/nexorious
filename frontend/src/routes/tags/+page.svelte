<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { tags, ui, type Tag } from '$lib/stores';
  import { ColorPicker } from '$lib/components/tags';

  let isLoading = $state(false);
  let showCreateModal = $state(false);
  let showEditModal = $state(false);
  let selectedTag = $state<Tag | null>(null);

  // Create tag form
  let createForm = $state({
    name: '',
    color: '#6B7280',
    description: ''
  });

  // Edit tag form
  let editForm = $state({
    name: '',
    color: '#6B7280', 
    description: ''
  });

  let sortBy = $state<'name' | 'usage' | 'created'>('name');
  let sortOrder = $state<'asc' | 'desc'>('asc');
  let searchQuery = $state('');

  // Computed values
  const sortedTags = $derived.by(() => {
    let filtered = tags.value.tags;

    // Apply search filter
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter(tag => 
        tag.name.toLowerCase().includes(query) ||
        tag.description?.toLowerCase().includes(query)
      );
    }

    // Apply sorting (create new array to avoid mutation)
    return [...filtered].sort((a, b) => {
      let comparison = 0;
      
      switch (sortBy) {
        case 'name':
          comparison = a.name.localeCompare(b.name);
          break;
        case 'usage':
          const aCount = a.game_count || 0;
          const bCount = b.game_count || 0;
          comparison = aCount - bCount;
          break;
        case 'created':
          comparison = new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
          break;
      }
      
      return sortOrder === 'asc' ? comparison : -comparison;
    });
  });

  const tagStats = $derived.by(() => {
    const totalTags = tags.value.tags.length;
    const usedTags = tags.value.tags.filter(tag => (tag.game_count || 0) > 0).length;
    const unusedTags = totalTags - usedTags;
    const totalUsage = tags.value.tags.reduce((sum, tag) => sum + (tag.game_count || 0), 0);
    
    return {
      total: totalTags,
      used: usedTags,
      unused: unusedTags,
      totalUsage,
      averageUsage: totalTags > 0 ? (totalUsage / totalTags).toFixed(1) : 0
    };
  });

  onMount(async () => {
    // Load tags if not already loaded
    if (tags.value.tags.length === 0 && !tags.value.isLoading) {
      await loadTags();
    }
  });

  const loadTags = async () => {
    isLoading = true;
    try {
      await tags.fetchTags();
      
      // Try to get usage stats, but don't fail if it doesn't work
      try {
        await tags.getTagUsageStats();
      } catch (statsError) {
        // Don't fail the whole operation if usage stats fail
        console.warn('Tag usage stats failed, but tags loaded successfully');
      }
    } catch (error) {
      ui.addNotification({ title: 'Error', message: 'Failed to load tags', type: 'error' });
    } finally {
      isLoading = false;
    }
  };

  const handleTagClick = (tag: Tag) => {
    // Navigate to games page with tag filter
    goto(`/games?tag=${tag.id}`);
  };

  const openCreateModal = () => {
    createForm = {
      name: '',
      color: tags.suggestColor() || '#6B7280',
      description: ''
    };
    showCreateModal = true;
  };

  const openEditModal = (tag: Tag) => {
    selectedTag = tag;
    editForm = {
      name: tag.name,
      color: tag.color,
      description: tag.description || ''
    };
    showEditModal = true;
  };

  const closeModals = () => {
    showCreateModal = false;
    showEditModal = false;
    selectedTag = null;
  };

  const createTag = async () => {
    if (!createForm.name.trim()) {
      ui.addNotification({ title: 'Notification', message: 'Tag name is required', type: 'error' });
      return;
    }

    try {
      const tagData = {
        name: createForm.name.trim(),
        color: createForm.color,
        ...(createForm.description.trim() && { description: createForm.description.trim() })
      };
      await tags.createTag(tagData);
      
      ui.addNotification({ title: 'Notification', message: 'Tag created successfully', type: 'success' });
      closeModals();
      await loadTags(); // Refresh to get usage stats
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to create tag';
      ui.addNotification({ title: 'Error', message, type: 'error' });
    }
  };

  const updateTag = async () => {
    if (!selectedTag || !editForm.name.trim()) {
      ui.addNotification({ title: 'Notification', message: 'Tag name is required', type: 'error' });
      return;
    }

    try {
      const updateData = {
        name: editForm.name.trim(),
        color: editForm.color,
        ...(editForm.description.trim() && { description: editForm.description.trim() })
      };
      await tags.updateTag(selectedTag.id, updateData);
      
      ui.addNotification({ title: 'Notification', message: 'Tag updated successfully', type: 'success' });
      closeModals();
      await loadTags(); // Refresh to get usage stats
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update tag';
      ui.addNotification({ title: 'Error', message, type: 'error' });
    }
  };

  const deleteTag = async (tag: Tag) => {
    const confirmMessage = tag.game_count && tag.game_count > 0
      ? `Delete "${tag.name}"? This will remove it from ${tag.game_count} game${tag.game_count !== 1 ? 's' : ''}.`
      : `Delete "${tag.name}"?`;
      
    if (!confirm(confirmMessage)) return;

    try {
      await tags.deleteTag(tag.id);
      ui.addNotification({ title: 'Notification', message: 'Tag deleted successfully', type: 'success' });
      await loadTags(); // Refresh to get updated stats
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to delete tag';
      ui.addNotification({ title: 'Error', message, type: 'error' });
    }
  };

  const changeSorting = (newSortBy: typeof sortBy) => {
    if (sortBy === newSortBy) {
      sortOrder = sortOrder === 'asc' ? 'desc' : 'asc';
    } else {
      sortBy = newSortBy;
      sortOrder = 'asc';
    }
  };

</script>

<svelte:head>
  <title>Manage Tags - Nexorious</title>
</svelte:head>

<!-- Page Header -->
<div class="sm:flex sm:items-center">
  <div class="sm:flex-auto">
    <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:text-3xl sm:truncate">
      Tag Management
    </h1>
    <p class="mt-2 text-sm text-gray-700">
      Organize your games with custom tags. Click any tag to see games with that tag.
    </p>
  </div>
  <div class="mt-4 sm:mt-0 sm:ml-16 sm:flex-none">
    <button
      type="button"
      class="btn-primary"
      onclick={openCreateModal}
    >
      Create Tag
    </button>
  </div>
</div>

<!-- Stats Cards -->
<div class="mt-8 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-5">
  <div class="bg-white overflow-hidden shadow rounded-lg">
    <div class="p-5">
      <div class="flex items-center">
        <div class="flex-shrink-0">
          <svg class="h-6 w-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.99 1.99 0 013 12V7a4 4 0 014-4z"/>
          </svg>
        </div>
        <div class="ml-5 w-0 flex-1">
          <dl>
            <dt class="text-sm font-medium text-gray-500 truncate">Total Tags</dt>
            <dd class="text-lg font-medium text-gray-900">{tagStats.total}</dd>
          </dl>
        </div>
      </div>
    </div>
  </div>

  <div class="bg-white overflow-hidden shadow rounded-lg">
    <div class="p-5">
      <div class="flex items-center">
        <div class="flex-shrink-0">
          <svg class="h-6 w-6 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
          </svg>
        </div>
        <div class="ml-5 w-0 flex-1">
          <dl>
            <dt class="text-sm font-medium text-gray-500 truncate">Used Tags</dt>
            <dd class="text-lg font-medium text-gray-900">{tagStats.used}</dd>
          </dl>
        </div>
      </div>
    </div>
  </div>

  <div class="bg-white overflow-hidden shadow rounded-lg">
    <div class="p-5">
      <div class="flex items-center">
        <div class="flex-shrink-0">
          <svg class="h-6 w-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4"/>
          </svg>
        </div>
        <div class="ml-5 w-0 flex-1">
          <dl>
            <dt class="text-sm font-medium text-gray-500 truncate">Unused Tags</dt>
            <dd class="text-lg font-medium text-gray-900">{tagStats.unused}</dd>
          </dl>
        </div>
      </div>
    </div>
  </div>

  <div class="bg-white overflow-hidden shadow rounded-lg">
    <div class="p-5">
      <div class="flex items-center">
        <div class="flex-shrink-0">
          <svg class="h-6 w-6 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"/>
          </svg>
        </div>
        <div class="ml-5 w-0 flex-1">
          <dl>
            <dt class="text-sm font-medium text-gray-500 truncate">Total Usage</dt>
            <dd class="text-lg font-medium text-gray-900">{tagStats.totalUsage}</dd>
          </dl>
        </div>
      </div>
    </div>
  </div>

  <div class="bg-white overflow-hidden shadow rounded-lg">
    <div class="p-5">
      <div class="flex items-center">
        <div class="flex-shrink-0">
          <svg class="h-6 w-6 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/>
          </svg>
        </div>
        <div class="ml-5 w-0 flex-1">
          <dl>
            <dt class="text-sm font-medium text-gray-500 truncate">Avg per Tag</dt>
            <dd class="text-lg font-medium text-gray-900">{tagStats.averageUsage}</dd>
          </dl>
        </div>
      </div>
    </div>
  </div>
</div>

<!-- Search and Sort Controls -->
<div class="mt-8 bg-white shadow rounded-lg">
  <div class="px-4 py-5 sm:p-6">
    <div class="sm:flex sm:items-center sm:justify-between">
      <div class="flex-1 max-w-lg">
        <input
          type="text"
          class="block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
          placeholder="Search tags..."
          bind:value={searchQuery}
        />
      </div>
      
      <div class="mt-4 sm:mt-0 sm:ml-4 flex items-center gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm text-gray-700">Sort by:</span>
          <div class="flex rounded-md shadow-sm">
            <button
              type="button"
              class="
                relative inline-flex items-center px-3 py-2 rounded-l-md border text-xs font-medium
                {sortBy === 'name' ? 'bg-primary-50 border-primary-500 text-primary-700' : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'}
                focus:z-10 focus:outline-none focus:ring-1 focus:ring-primary-500 focus:border-primary-500
              "
              onclick={() => changeSorting('name')}
            >
              Name
              {#if sortBy === 'name'}
                <svg class="ml-1 h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                  <path d={sortOrder === 'asc' ? "M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" : "M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"}/>
                </svg>
              {/if}
            </button>
            <button
              type="button"
              class="
                relative -ml-px inline-flex items-center px-3 py-2 border text-xs font-medium
                {sortBy === 'usage' ? 'bg-primary-50 border-primary-500 text-primary-700' : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'}
                focus:z-10 focus:outline-none focus:ring-1 focus:ring-primary-500 focus:border-primary-500
              "
              onclick={() => changeSorting('usage')}
            >
              Usage
              {#if sortBy === 'usage'}
                <svg class="ml-1 h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                  <path d={sortOrder === 'asc' ? "M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" : "M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"}/>
                </svg>
              {/if}
            </button>
            <button
              type="button"
              class="
                relative -ml-px inline-flex items-center px-3 py-2 rounded-r-md border text-xs font-medium
                {sortBy === 'created' ? 'bg-primary-50 border-primary-500 text-primary-700' : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'}
                focus:z-10 focus:outline-none focus:ring-1 focus:ring-primary-500 focus:border-primary-500
              "
              onclick={() => changeSorting('created')}
            >
              Date
              {#if sortBy === 'created'}
                <svg class="ml-1 h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
                  <path d={sortOrder === 'asc' ? "M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" : "M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"}/>
                </svg>
              {/if}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</div>

<!-- Tags List -->
<div class="mt-8">
  {#if isLoading}
    <div class="text-center py-12">
      <div class="inline-flex items-center text-lg text-gray-500">
        <svg class="animate-spin -ml-1 mr-3 h-6 w-6" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
        Loading tags...
      </div>
    </div>
  {:else if sortedTags.length === 0}
    <div class="text-center py-12">
      {#if searchQuery}
        <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
        </svg>
        <h3 class="mt-2 text-sm font-medium text-gray-900">No tags found</h3>
        <p class="mt-1 text-sm text-gray-500">
          No tags match your search "{searchQuery}". Try a different search term.
        </p>
      {:else}
        <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.99 1.99 0 013 12V7a4 4 0 014-4z"/>
        </svg>
        <h3 class="mt-2 text-sm font-medium text-gray-900">No tags</h3>
        <p class="mt-1 text-sm text-gray-500">
          Get started by creating your first tag to organize your games.
        </p>
        <div class="mt-6">
          <button
            type="button"
            class="btn-primary"
            onclick={openCreateModal}
          >
            Create Your First Tag
          </button>
        </div>
      {/if}
    </div>
  {:else}
    <div class="bg-white shadow overflow-hidden sm:rounded-md">
      <ul class="divide-y divide-gray-200">
        {#each sortedTags as tag (tag.id)}
          <li class="hover:bg-gray-50 transition-colors duration-200">
            <div class="px-4 py-4 flex items-center justify-between">
              <button 
                type="button"
                class="flex items-center min-w-0 flex-1 cursor-pointer text-left bg-transparent border-none p-0 w-full" 
                onclick={() => handleTagClick(tag)}
                onkeydown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    handleTagClick(tag);
                  }
                }}
                aria-label="View games with tag {tag.name}"
              >
                <!-- Tag Color -->
                <div 
                  class="h-6 w-6 rounded-full border border-gray-300 flex-shrink-0"
                  style="background-color: {tag.color}"
                ></div>
                
                <!-- Tag Info -->
                <div class="ml-4 min-w-0 flex-1">
                  <div class="flex items-center">
                    <p class="text-sm font-medium text-gray-900 truncate">{tag.name}</p>
                    {#if tag.game_count !== undefined && tag.game_count > 0}
                      <span class="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
                        {tag.game_count} game{tag.game_count !== 1 ? 's' : ''}
                      </span>
                    {:else}
                      <span class="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-50 text-gray-500">
                        Unused
                      </span>
                    {/if}
                  </div>
                  {#if tag.description}
                    <p class="text-sm text-gray-500 truncate">{tag.description}</p>
                  {/if}
                  <p class="text-xs text-gray-400">
                    Created {new Date(tag.created_at).toLocaleDateString()}
                  </p>
                </div>
              </button>
              
              <!-- Actions -->
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  class="text-primary-600 hover:text-primary-900 text-sm font-medium"
                  onclick={() => openEditModal(tag)}
                >
                  Edit
                </button>
                <button
                  type="button"
                  class="text-red-600 hover:text-red-900 text-sm font-medium"
                  onclick={() => deleteTag(tag)}
                >
                  Delete
                </button>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</div>

<!-- Create Tag Modal -->
{#if showCreateModal}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
    <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
      <div class="mt-3">
        <h3 class="text-lg font-medium text-gray-900">Create New Tag</h3>
        <form class="mt-4 space-y-4" onsubmit={(e) => { e.preventDefault(); createTag(); }}>
          <div>
            <label for="tag-name" class="block text-sm font-medium text-gray-700">Name *</label>
            <input
              id="tag-name"
              type="text"
              class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500"
              bind:value={createForm.name}
              placeholder="Enter tag name..."
              maxlength="100"
              required
            />
          </div>
          
          <div>
            <div class="block text-sm font-medium text-gray-700 mb-2">Color</div>
            <ColorPicker
              value={createForm.color}
              onchange={(color) => createForm.color = color}
            />
          </div>
          
          <div>
            <label for="tag-description" class="block text-sm font-medium text-gray-700">Description</label>
            <textarea
              id="tag-description"
              class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500"
              bind:value={createForm.description}
              placeholder="Optional description..."
              rows="3"
              maxlength="500"
            ></textarea>
          </div>
          
          <div class="flex justify-end gap-3 pt-4">
            <button
              type="button"
              class="px-4 py-2 bg-gray-300 text-gray-700 rounded-md hover:bg-gray-400"
              onclick={closeModals}
            >
              Cancel
            </button>
            <button
              type="submit"
              class="px-4 py-2 bg-primary-600 text-white rounded-md hover:bg-primary-700"
            >
              Create Tag
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}

<!-- Edit Tag Modal -->
{#if showEditModal && selectedTag}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
    <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
      <div class="mt-3">
        <h3 class="text-lg font-medium text-gray-900">Edit Tag</h3>
        <form class="mt-4 space-y-4" onsubmit={(e) => { e.preventDefault(); updateTag(); }}>
          <div>
            <label for="edit-tag-name" class="block text-sm font-medium text-gray-700">Name *</label>
            <input
              id="edit-tag-name"
              type="text"
              class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500"
              bind:value={editForm.name}
              placeholder="Enter tag name..."
              maxlength="100"
              required
            />
          </div>
          
          <div>
            <div class="block text-sm font-medium text-gray-700 mb-2">Color</div>
            <ColorPicker
              value={editForm.color}
              onchange={(color) => editForm.color = color}
            />
          </div>
          
          <div>
            <label for="edit-tag-description" class="block text-sm font-medium text-gray-700">Description</label>
            <textarea
              id="edit-tag-description"
              class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500"
              bind:value={editForm.description}
              placeholder="Optional description..."
              rows="3"
              maxlength="500"
            ></textarea>
          </div>
          
          <div class="flex justify-end gap-3 pt-4">
            <button
              type="button"
              class="px-4 py-2 bg-gray-300 text-gray-700 rounded-md hover:bg-gray-400"
              onclick={closeModals}
            >
              Cancel
            </button>
            <button
              type="submit"
              class="px-4 py-2 bg-primary-600 text-white rounded-md hover:bg-primary-700"
            >
              Update Tag
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}