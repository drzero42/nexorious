<script lang="ts">
  import { onMount } from 'svelte';
  import { platforms, auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import type { Platform, Storefront, PlatformCreateRequest, StorefrontCreateRequest, PlatformUpdateRequest, StorefrontUpdateRequest } from '$lib/stores/platforms.svelte';

  let isLoading = true;
  let activeTab: 'platforms' | 'storefronts' | 'associations' = 'platforms';
  let searchQuery = '';
  let statusFilter: 'all' | 'active' | 'inactive' = 'all';
  
  // Association management state
  let associationsLoading = false;
  let platformAssociations: Map<string, Set<string>> = new Map();
  
  // Platform form state
  let showCreatePlatformForm = false;
  let editingPlatform: Platform | null = null;
  let platformForm: PlatformCreateRequest = {
    name: '',
    display_name: '',
    icon_url: '',
    is_active: true,
    default_storefront_id: ''
  };

  // Storefront form state
  let showCreateStorefrontForm = false;
  let editingStorefront: Storefront | null = null;
  let storefrontForm: StorefrontCreateRequest = {
    name: '',
    display_name: '',
    icon_url: '',
    base_url: '',
    is_active: true
  };

  // Confirmation dialog state
  let showDeleteConfirm = false;
  let deleteTarget: { type: 'platform' | 'storefront'; id: string; name: string } | null = null;


  // Reactive statements to track platform store state
  $: platformsList = $platforms.platforms;
  $: storefrontsList = $platforms.storefronts;
  $: error = $platforms.error;
  $: isStoreLoading = $platforms.isLoading;

  // Filtered platforms based on search and filter criteria
  $: filteredPlatforms = platformsList.filter(platform => {
    const matchesSearch = platform.name.toLowerCase().includes(searchQuery.toLowerCase()) || 
                          platform.display_name.toLowerCase().includes(searchQuery.toLowerCase());
    
    switch (statusFilter) {
      case 'active':
        return matchesSearch && platform.is_active;
      case 'inactive':
        return matchesSearch && !platform.is_active;
      default:
        return matchesSearch;
    }
  });

  // Filtered storefronts based on search and filter criteria
  $: filteredStorefronts = storefrontsList.filter(storefront => {
    const matchesSearch = storefront.name.toLowerCase().includes(searchQuery.toLowerCase()) || 
                          storefront.display_name.toLowerCase().includes(searchQuery.toLowerCase());
    
    switch (statusFilter) {
      case 'active':
        return matchesSearch && storefront.is_active;
      case 'inactive':
        return matchesSearch && !storefront.is_active;
      default:
        return matchesSearch;
    }
  });

  onMount(async () => {
    // Check if user is admin
    if (!auth.value.user?.isAdmin) {
      goto('/dashboard');
      return;
    }

    try {
      // Load ALL platforms and storefronts (both active and inactive) for admin management
      await platforms.fetchAll();
    } catch (err) {
      console.error('Failed to load platforms and storefronts:', err);
    } finally {
      isLoading = false;
    }
  });

  function resetPlatformForm() {
    platformForm = {
      name: '',
      display_name: '',
      icon_url: '',
      is_active: true,
      default_storefront_id: ''
    };
    editingPlatform = null;
    showCreatePlatformForm = false;
  }

  function resetStorefrontForm() {
    storefrontForm = {
      name: '',
      display_name: '',
      icon_url: '',
      base_url: '',
      is_active: true
    };
    editingStorefront = null;
    showCreateStorefrontForm = false;
  }

  function editPlatform(platform: Platform) {
    platformForm = {
      name: platform.name,
      display_name: platform.display_name,
      icon_url: platform.icon_url || '',
      is_active: platform.is_active,
      default_storefront_id: platform.default_storefront_id || ''
    };
    editingPlatform = platform;
    showCreatePlatformForm = true;
  }

  function editStorefront(storefront: Storefront) {
    storefrontForm = {
      name: storefront.name,
      display_name: storefront.display_name,
      icon_url: storefront.icon_url || '',
      base_url: storefront.base_url || '',
      is_active: storefront.is_active
    };
    editingStorefront = storefront;
    showCreateStorefrontForm = true;
  }

  async function savePlatform() {
    try {
      if (editingPlatform) {
        // Update existing platform
        const updateData: PlatformUpdateRequest = {
          display_name: platformForm.display_name
        };
        if (platformForm.icon_url && platformForm.icon_url.trim()) {
          updateData.icon_url = platformForm.icon_url;
        }
        // Handle default storefront - empty string means no default
        if (platformForm.default_storefront_id && platformForm.default_storefront_id.trim()) {
          updateData.default_storefront_id = platformForm.default_storefront_id;
        } else {
          updateData.default_storefront_id = null;
        }
        await platforms.updatePlatform(editingPlatform.id, updateData);
      } else {
        // Create new platform - include default storefront
        const createData = { ...platformForm };
        if (!createData.default_storefront_id || !createData.default_storefront_id.trim()) {
          delete createData.default_storefront_id;
        }
        await platforms.createPlatform(createData);
      }
      resetPlatformForm();
    } catch (err) {
      console.error('Failed to save platform:', err);
    }
  }

  async function saveStorefront() {
    try {
      if (editingStorefront) {
        // Update existing storefront
        const updateData: StorefrontUpdateRequest = {
          display_name: storefrontForm.display_name
        };
        if (storefrontForm.icon_url && storefrontForm.icon_url.trim()) {
          updateData.icon_url = storefrontForm.icon_url;
        }
        if (storefrontForm.base_url && storefrontForm.base_url.trim()) {
          updateData.base_url = storefrontForm.base_url;
        }
        await platforms.updateStorefront(editingStorefront.id, updateData);
      } else {
        // Create new storefront
        await platforms.createStorefront(storefrontForm);
      }
      resetStorefrontForm();
    } catch (err) {
      console.error('Failed to save storefront:', err);
    }
  }

  async function togglePlatformStatus(platform: Platform) {
    try {
      await platforms.updatePlatform(platform.id, { is_active: !platform.is_active });
    } catch (err) {
      console.error('Failed to toggle platform status:', err);
    }
  }

  async function toggleStorefrontStatus(storefront: Storefront) {
    try {
      await platforms.updateStorefront(storefront.id, { is_active: !storefront.is_active });
    } catch (err) {
      console.error('Failed to toggle storefront status:', err);
    }
  }

  function confirmDelete(type: 'platform' | 'storefront', id: string, name: string) {
    deleteTarget = { type, id, name };
    showDeleteConfirm = true;
  }

  async function executeDelete() {
    if (!deleteTarget) return;

    try {
      if (deleteTarget.type === 'platform') {
        await platforms.deletePlatform(deleteTarget.id);
      } else {
        await platforms.deleteStorefront(deleteTarget.id);
      }
      showDeleteConfirm = false;
      deleteTarget = null;
    } catch (err) {
      console.error(`Failed to delete ${deleteTarget?.type}:`, err);
    }
  }

  function cancelDelete() {
    showDeleteConfirm = false;
    deleteTarget = null;
  }

  function formatDate(dateString: string) {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  }

  // Load platform-storefront associations
  async function loadAssociations() {
    associationsLoading = true;
    
    try {
      // Clear existing associations
      platformAssociations.clear();
      
      // Load associations for each platform
      for (const platform of filteredPlatforms) {
        const storefronts = await platforms.getPlatformStorefronts(platform.id);
        const storefrontIds = new Set<string>(storefronts.map((s: Storefront) => s.id));
        platformAssociations.set(platform.id, storefrontIds);
      }
      
      // Trigger reactivity
      platformAssociations = new Map(platformAssociations);
    } catch (err) {
      console.error('Failed to load associations:', err);
    } finally {
      associationsLoading = false;
    }
  }

  // Handle association checkbox change
  async function handleAssociationChange(platformId: string, storefrontId: string, isChecked: boolean) {
    try {
      if (isChecked) {
        await platforms.createPlatformStorefrontAssociation(platformId, storefrontId);
        
        // Update local state
        const associations = platformAssociations.get(platformId) || new Set();
        associations.add(storefrontId);
        platformAssociations.set(platformId, associations);
      } else {
        await platforms.deletePlatformStorefrontAssociation(platformId, storefrontId);
        
        // Update local state
        const associations = platformAssociations.get(platformId) || new Set();
        associations.delete(storefrontId);
        platformAssociations.set(platformId, associations);
      }
      
      // Trigger reactivity
      platformAssociations = new Map(platformAssociations);
    } catch (err) {
      console.error('Failed to update association:', err);
      // Reload associations to ensure UI is in sync
      await loadAssociations();
    }
  }

  // Check if platform has storefront association
  function hasAssociation(platformId: string, storefrontId: string): boolean {
    return platformAssociations.get(platformId)?.has(storefrontId) || false;
  }

</script>

<RouteGuard requireAdmin={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div class="border-b border-gray-200 pb-5">
      <h1 class="text-3xl font-bold leading-tight text-gray-900">Platform & Storefront Management</h1>
      <p class="mt-2 max-w-4xl text-sm text-gray-500">
        Manage available platforms and storefronts for the application
      </p>
    </div>

    <!-- Tab Navigation -->
    <div class="border-b border-gray-200">
      <nav class="-mb-px flex space-x-8">
        <button
          on:click={() => activeTab = 'platforms'}
          class={`py-2 px-1 border-b-2 font-medium text-sm ${
            activeTab === 'platforms'
              ? 'border-primary-500 text-primary-600'
              : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
          }`}
        >
          <span class="mr-2">🎮</span>
          Platforms
        </button>
        <button
          on:click={() => activeTab = 'storefronts'}
          class={`py-2 px-1 border-b-2 font-medium text-sm ${
            activeTab === 'storefronts'
              ? 'border-primary-500 text-primary-600'
              : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
          }`}
        >
          <span class="mr-2">🏪</span>
          Storefronts
        </button>
        <button
          on:click={() => { activeTab = 'associations'; loadAssociations(); }}
          class={`py-2 px-1 border-b-2 font-medium text-sm ${
            activeTab === 'associations'
              ? 'border-primary-500 text-primary-600'
              : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
          }`}
        >
          <span class="mr-2">🔗</span>
          Associations
        </button>
      </nav>
    </div>

    {#if error}
      <div class="rounded-md bg-red-50 p-4">
        <div class="flex">
          <div class="ml-3">
            <h3 class="text-sm font-medium text-red-800">Error</h3>
            <div class="mt-2 text-sm text-red-700">
              <p>{error}</p>
            </div>
            <div class="mt-4">
              <button
                on:click={() => platforms.clearError()}
                type="button"
                class="rounded-md bg-red-50 text-red-800 hover:bg-red-100 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 px-3 py-2 text-sm font-medium"
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      </div>
    {/if}

    {#if isLoading || isStoreLoading}
      <div class="flex justify-center py-12">
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600" role="status" aria-label="Loading"></div>
      </div>
    {:else}
      <!-- Search and Filter Controls -->
      <div class="bg-white shadow rounded-lg p-6">
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between space-y-4 sm:space-y-0 sm:space-x-4">
          <div class="flex-1">
            <input
              type="text"
              bind:value={searchQuery}
              placeholder={`Search ${activeTab}...`}
              class="block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
            />
          </div>
          <div>
            <select
              bind:value={statusFilter}
              class="block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
            >
              <option value="all">All Status</option>
              <option value="active">Active Only</option>
              <option value="inactive">Inactive Only</option>
            </select>
          </div>
          {#if activeTab !== 'associations'}
            <div>
              <button
                on:click={() => activeTab === 'platforms' ? (showCreatePlatformForm = true) : (showCreateStorefrontForm = true)}
                class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
              >
                <span class="mr-2">+</span>
                Add {activeTab === 'platforms' ? 'Platform' : 'Storefront'}
              </button>
            </div>
          {/if}
        </div>
      </div>

      {#if activeTab === 'platforms'}
        <!-- Platforms Management -->
        <div class="bg-white shadow rounded-lg">
          <div class="px-4 py-5 sm:p-6">
            <h3 class="text-lg leading-6 font-medium text-gray-900 mb-6">Platforms</h3>
            
            {#if filteredPlatforms.length === 0}
              <div class="text-center py-12">
                <div class="text-gray-400 text-lg mb-2">🎮</div>
                <p class="text-gray-500">No platforms found</p>
              </div>
            {:else}
              <div class="overflow-hidden shadow ring-1 ring-black ring-opacity-5 rounded-lg">
                <table class="min-w-full divide-y divide-gray-300">
                  <thead class="bg-gray-50">
                    <tr>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Name</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Display Name</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Source</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Created</th>
                      <th class="relative px-6 py-3"><span class="sr-only">Actions</span></th>
                    </tr>
                  </thead>
                  <tbody class="bg-white divide-y divide-gray-200">
                    {#each filteredPlatforms as platform}
                      <tr>
                        <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                          {platform.name}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          {platform.display_name}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap">
                          <button
                            on:click={() => togglePlatformStatus(platform)}
                            class={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                              platform.is_active
                                ? 'bg-green-100 text-green-800'
                                : 'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {platform.is_active ? 'Active' : 'Inactive'}
                          </button>
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          <span class={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                            platform.source === 'official' ? 'bg-blue-100 text-blue-800' : 'bg-purple-100 text-purple-800'
                          }`}>
                            {platform.source}
                          </span>
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          {formatDate(platform.created_at)}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                          <button
                            on:click={() => editPlatform(platform)}
                            class="text-primary-600 hover:text-primary-900 mr-4"
                          >
                            Edit
                          </button>
                          <button
                            on:click={() => confirmDelete('platform', platform.id, platform.display_name)}
                            class="text-red-600 hover:text-red-900"
                          >
                            Delete
                          </button>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}
          </div>
        </div>
      {:else if activeTab === 'storefronts'}
        <!-- Storefronts Management -->
        <div class="bg-white shadow rounded-lg">
          <div class="px-4 py-5 sm:p-6">
            <h3 class="text-lg leading-6 font-medium text-gray-900 mb-6">Storefronts</h3>
            
            {#if filteredStorefronts.length === 0}
              <div class="text-center py-12">
                <div class="text-gray-400 text-lg mb-2">🏪</div>
                <p class="text-gray-500">No storefronts found</p>
              </div>
            {:else}
              <div class="overflow-hidden shadow ring-1 ring-black ring-opacity-5 rounded-lg">
                <table class="min-w-full divide-y divide-gray-300">
                  <thead class="bg-gray-50">
                    <tr>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Name</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Display Name</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Base URL</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Source</th>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Created</th>
                      <th class="relative px-6 py-3"><span class="sr-only">Actions</span></th>
                    </tr>
                  </thead>
                  <tbody class="bg-white divide-y divide-gray-200">
                    {#each filteredStorefronts as storefront}
                      <tr>
                        <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                          {storefront.name}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          {storefront.display_name}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          {#if storefront.base_url}
                            <a href={storefront.base_url} target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-900">
                              {storefront.base_url}
                            </a>
                          {:else}
                            <span class="text-gray-400">-</span>
                          {/if}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap">
                          <button
                            on:click={() => toggleStorefrontStatus(storefront)}
                            class={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                              storefront.is_active
                                ? 'bg-green-100 text-green-800'
                                : 'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {storefront.is_active ? 'Active' : 'Inactive'}
                          </button>
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          <span class={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                            storefront.source === 'official' ? 'bg-blue-100 text-blue-800' : 'bg-purple-100 text-purple-800'
                          }`}>
                            {storefront.source}
                          </span>
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          {formatDate(storefront.created_at)}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                          <button
                            on:click={() => editStorefront(storefront)}
                            class="text-primary-600 hover:text-primary-900 mr-4"
                          >
                            Edit
                          </button>
                          <button
                            on:click={() => confirmDelete('storefront', storefront.id, storefront.display_name)}
                            class="text-red-600 hover:text-red-900"
                          >
                            Delete
                          </button>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}
          </div>
        </div>
      {:else if activeTab === 'associations'}
        <!-- Platform-Storefront Associations Management -->
        <div class="bg-white shadow rounded-lg">
          <div class="px-4 py-5 sm:p-6">
            <div class="flex items-center justify-between mb-6">
              <h3 class="text-lg leading-6 font-medium text-gray-900">Platform-Storefront Associations</h3>
              <button
                on:click={loadAssociations}
                disabled={associationsLoading}
                class="inline-flex items-center px-3 py-2 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50"
              >
                {#if associationsLoading}
                  <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-600 mr-2"></div>
                {/if}
                Refresh
              </button>
            </div>
            
            {#if associationsLoading}
              <div class="flex justify-center py-12">
                <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600" role="status" aria-label="Loading"></div>
              </div>
            {:else if filteredPlatforms.length === 0 || filteredStorefronts.length === 0}
              <div class="text-center py-12">
                <div class="text-gray-400 text-lg mb-2">🔗</div>
                <p class="text-gray-500">No platforms or storefronts available</p>
              </div>
            {:else}
              <div class="overflow-x-auto">
                <table class="min-w-full divide-y divide-gray-300">
                  <thead class="bg-gray-50">
                    <tr>
                      <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider sticky left-0 bg-gray-50">
                        Platform
                      </th>
                      {#each filteredStorefronts as storefront}
                        <th class="px-3 py-3 text-center text-xs font-medium text-gray-500 uppercase tracking-wider min-w-[120px]">
                          <div class="flex flex-col items-center">
                            <span class="mb-1">{storefront.display_name}</span>
                            <span class={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${
                              storefront.is_active ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'
                            }`}>
                              {storefront.is_active ? 'Active' : 'Inactive'}
                            </span>
                          </div>
                        </th>
                      {/each}
                    </tr>
                  </thead>
                  <tbody class="bg-white divide-y divide-gray-200">
                    {#each filteredPlatforms as platform}
                      <tr class="hover:bg-gray-50">
                        <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 sticky left-0 bg-white">
                          <div class="flex flex-col">
                            <span>{platform.display_name}</span>
                            <span class={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium mt-1 ${
                              platform.is_active ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'
                            }`}>
                              {platform.is_active ? 'Active' : 'Inactive'}
                            </span>
                          </div>
                        </td>
                        {#each filteredStorefronts as storefront}
                          <td class="px-3 py-4 whitespace-nowrap text-center">
                            <input
                              type="checkbox"
                              checked={hasAssociation(platform.id, storefront.id)}
                              on:change={(e) => handleAssociationChange(platform.id, storefront.id, e.currentTarget.checked)}
                              class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded disabled:opacity-50"
                              disabled={!platform.is_active || !storefront.is_active}
                            />
                          </td>
                        {/each}
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
              
              <div class="mt-4 text-sm text-gray-500">
                <p><strong>Note:</strong> Checkboxes are disabled for inactive platforms or storefronts. Only active items can have associations.</p>
              </div>
            {/if}
          </div>
        </div>
      {/if}
    {/if}
  </div>

  <!-- Platform Create/Edit Modal -->
  {#if showCreatePlatformForm}
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50" role="dialog" aria-modal="true" tabindex="-1" on:click={resetPlatformForm} on:keydown={(e) => e.key === 'Escape' && resetPlatformForm()}>
      <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white" on:click|stopPropagation>
        <div class="mt-3">
          <h3 class="text-lg font-medium text-gray-900 mb-4">
            {editingPlatform ? 'Edit Platform' : 'Create New Platform'}
          </h3>
          
          <form on:submit|preventDefault={savePlatform} class="space-y-4">
            <div>
              <label for="platform-name" class="block text-sm font-medium text-gray-700">Platform Name</label>
              <input
                id="platform-name"
                type="text"
                bind:value={platformForm.name}
                disabled={!!editingPlatform}
                required
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm disabled:bg-gray-50 disabled:text-gray-500"
                placeholder="e.g., nintendo_switch"
              />
            </div>
            
            <div>
              <label for="platform-display-name" class="block text-sm font-medium text-gray-700">Display Name</label>
              <input
                id="platform-display-name"
                type="text"
                bind:value={platformForm.display_name}
                required
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
                placeholder="e.g., Nintendo Switch"
              />
            </div>
            
            <div>
              <label for="platform-icon-url" class="block text-sm font-medium text-gray-700">Icon URL (Optional)</label>
              <input
                id="platform-icon-url"
                type="url"
                bind:value={platformForm.icon_url}
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
                placeholder="https://example.com/icon.png"
              />
            </div>
            
            <div>
              <label for="platform-default-storefront" class="block text-sm font-medium text-gray-700">Default Storefront (Optional)</label>
              <select
                id="platform-default-storefront"
                bind:value={platformForm.default_storefront_id}
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
              >
                <option value="">No Default</option>
                {#each storefrontsList.filter(s => s.is_active) as storefront}
                  <option value={storefront.id}>{storefront.display_name}</option>
                {/each}
              </select>
              <p class="mt-1 text-sm text-gray-500">The storefront that will be automatically selected when users add games for this platform.</p>
            </div>
            
            <div class="flex justify-end space-x-3 pt-4">
              <button
                type="button"
                on:click={resetPlatformForm}
                class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
              >
                Cancel
              </button>
              <button
                type="submit"
                class="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md shadow-sm hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
              >
                {editingPlatform ? 'Update' : 'Create'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  {/if}

  <!-- Storefront Create/Edit Modal -->
  {#if showCreateStorefrontForm}
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50" role="dialog" aria-modal="true" tabindex="-1" on:click={resetStorefrontForm} on:keydown={(e) => e.key === 'Escape' && resetStorefrontForm()}>
      <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white" on:click|stopPropagation>
        <div class="mt-3">
          <h3 class="text-lg font-medium text-gray-900 mb-4">
            {editingStorefront ? 'Edit Storefront' : 'Create New Storefront'}
          </h3>
          
          <form on:submit|preventDefault={saveStorefront} class="space-y-4">
            <div>
              <label for="storefront-name" class="block text-sm font-medium text-gray-700">Storefront Name</label>
              <input
                id="storefront-name"
                type="text"
                bind:value={storefrontForm.name}
                disabled={!!editingStorefront}
                required
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm disabled:bg-gray-50 disabled:text-gray-500"
                placeholder="e.g., epic_games"
              />
            </div>
            
            <div>
              <label for="storefront-display-name" class="block text-sm font-medium text-gray-700">Display Name</label>
              <input
                id="storefront-display-name"
                type="text"
                bind:value={storefrontForm.display_name}
                required
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
                placeholder="e.g., Epic Games Store"
              />
            </div>
            
            <div>
              <label for="storefront-base-url" class="block text-sm font-medium text-gray-700">Base URL (Optional)</label>
              <input
                id="storefront-base-url"
                type="url"
                bind:value={storefrontForm.base_url}
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
                placeholder="https://store.epicgames.com"
              />
            </div>
            
            <div>
              <label for="storefront-icon-url" class="block text-sm font-medium text-gray-700">Icon URL (Optional)</label>
              <input
                id="storefront-icon-url"
                type="url"
                bind:value={storefrontForm.icon_url}
                class="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
                placeholder="https://example.com/icon.png"
              />
            </div>
            
            <div class="flex justify-end space-x-3 pt-4">
              <button
                type="button"
                on:click={resetStorefrontForm}
                class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
              >
                Cancel
              </button>
              <button
                type="submit"
                class="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md shadow-sm hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
              >
                {editingStorefront ? 'Update' : 'Create'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  {/if}

  <!-- Delete Confirmation Modal -->
  {#if showDeleteConfirm && deleteTarget}
    <!-- svelte-ignore a11y-click-events-have-key-events -->
    <!-- svelte-ignore a11y-no-static-element-interactions -->
    <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50" role="dialog" aria-modal="true" tabindex="-1" on:click={cancelDelete} on:keydown={(e) => e.key === 'Escape' && cancelDelete()}>
      <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white" on:click|stopPropagation>
        <div class="mt-3">
          <h3 class="text-lg font-medium text-gray-900 mb-4">Confirm Deletion</h3>
          <p class="text-sm text-gray-500 mb-4">
            Are you sure you want to delete the {deleteTarget.type} "{deleteTarget.name}"?
          </p>
          <div class="bg-red-50 border border-red-200 rounded-md p-4 mb-4">
            <p class="text-sm text-red-700">
              <strong>Warning:</strong> This action cannot be undone. The {deleteTarget.type} will be removed from all user games.
            </p>
          </div>
          
          <div class="flex justify-end space-x-3">
            <button
              type="button"
              on:click={cancelDelete}
              class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md shadow-sm hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              Cancel
            </button>
            <button
              type="button"
              on:click={executeDelete}
              class="px-4 py-2 text-sm font-medium text-white bg-red-600 border border-transparent rounded-md shadow-sm hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500"
            >
              Delete
            </button>
          </div>
        </div>
      </div>
    </div>
  {/if}
</RouteGuard>