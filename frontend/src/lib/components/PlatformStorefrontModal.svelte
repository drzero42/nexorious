<script lang="ts">
  import { platforms } from '$lib/stores/platforms.svelte';
  import { darkadia } from '$lib/stores/darkadia.svelte';
  import { ui } from '$lib/stores';
  import type { 
    PlatformResolutionUIState,
    StorefrontResolutionUIState
  } from '$lib/types/platform-resolution';
  import type { 
    DarkadiaResolutionSummaryResponse,
    DarkadiaUpdateMappingsRequest 
  } from '$lib/types/darkadia';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onResolutionsComplete?: (resolvedCount: number) => void;
  }

  let { 
    isOpen = false, 
    onClose, 
    onResolutionsComplete 
  }: Props = $props();

  // Modal state
  let modalState = $state<PlatformResolutionUIState>({
    isOpen: false,
    isLoading: false,
    pendingResolutions: [],
    selectedResolutions: new Set(),
    bulkOperationInProgress: false
  });

  // Storefront resolution state
  let storefrontModalState = $state<StorefrontResolutionUIState>({
    isOpen: false,
    isLoading: false,
    pendingResolutions: [],
    selectedResolutions: new Set(),
    bulkOperationInProgress: false
  });

  // Resolution summary state (for existing mappings)
  let resolutionSummary = $state<DarkadiaResolutionSummaryResponse | null>(null);
  let isLoadingSummary = $state(false);
  let editingMappings = $state<DarkadiaUpdateMappingsRequest>({ mappings: [] });

  // UI state
  let platformsCollapsed = $state(false);
  let storefrontsCollapsed = $state(false);
  let existingMappingsCollapsed = $state(true); // Start collapsed

  // Counters
  let successfulResolutions = $state(0);
  let successfulStorefrontResolutions = $state(0);

  // Derived states
  const hasResolutions = $derived(modalState.pendingResolutions.length > 0);
  const hasStorefrontResolutions = $derived(storefrontModalState.pendingResolutions.length > 0);
  const hasAnyResolutions = $derived(hasResolutions || hasStorefrontResolutions);
  
  const hasSelections = $derived(modalState.selectedResolutions.size > 0);
  const hasStorefrontSelections = $derived(storefrontModalState.selectedResolutions.size > 0);
  
  const allSelected = $derived(
    hasResolutions && modalState.selectedResolutions.size === modalState.pendingResolutions.length
  );
  const allStorefrontsSelected = $derived(
    hasStorefrontResolutions && storefrontModalState.selectedResolutions.size === storefrontModalState.pendingResolutions.length
  );

  // Initialize modal when opened
  $effect(() => {
    if (isOpen && !modalState.isOpen) {
      console.log('🚪 [PLATFORM-STOREFRONT-MODAL] Modal opening - loading all data');
      modalState.isOpen = true;
      storefrontModalState.isOpen = true;
      successfulResolutions = 0;
      successfulStorefrontResolutions = 0;
      
      // Load all data in parallel
      Promise.all([
        loadPendingResolutions(),
        loadPendingStorefrontResolutions(),
        loadResolutionSummary()
      ]);
    } else if (!isOpen && modalState.isOpen) {
      modalState.isOpen = false;
      storefrontModalState.isOpen = false;
    }
  });

  async function loadPendingResolutions() {
    if (modalState.isLoading) return;

    console.log('🔄 [PLATFORM-STOREFRONT-MODAL] Loading pending platform resolutions...');
    modalState.isLoading = true;
    delete modalState.error;

    try {
      const response = await platforms.getPendingResolutions(1, 100); // Load all for now
      
      modalState.pendingResolutions = response.pending_resolutions;
      modalState.selectedResolutions.clear();
      
      console.log('📥 [PLATFORM-STOREFRONT-MODAL] Loaded platform resolutions:', response.pending_resolutions.length);
      
    } catch (error) {
      modalState.error = error instanceof Error ? error.message : 'Failed to load pending platform resolutions';
      ui.showError(modalState.error);
    } finally {
      modalState.isLoading = false;
    }
  }

  async function loadPendingStorefrontResolutions() {
    if (storefrontModalState.isLoading) return;

    console.log('🔄 [PLATFORM-STOREFRONT-MODAL] Loading pending storefront resolutions...');
    storefrontModalState.isLoading = true;
    delete storefrontModalState.error;

    try {
      const response = await platforms.getPendingStorefrontResolutions(1, 100); // Load all for now
      
      storefrontModalState.pendingResolutions = response.pending_resolutions;
      storefrontModalState.selectedResolutions.clear();
      
      console.log('📥 [PLATFORM-STOREFRONT-MODAL] Loaded storefront resolutions:', response.pending_resolutions.length);
      
    } catch (error) {
      storefrontModalState.error = error instanceof Error ? error.message : 'Failed to load pending storefront resolutions';
      ui.showError(storefrontModalState.error);
    } finally {
      storefrontModalState.isLoading = false;
    }
  }

  async function loadResolutionSummary() {
    if (isLoadingSummary) return;

    console.log('🔄 [PLATFORM-STOREFRONT-MODAL] Loading resolution summary...');
    isLoadingSummary = true;

    try {
      const summary = await darkadia.getResolutionSummary();
      resolutionSummary = summary;
      
      console.log('📥 [PLATFORM-STOREFRONT-MODAL] Loaded resolution summary:', summary);
      
    } catch (error) {
      console.error('❌ [PLATFORM-STOREFRONT-MODAL] Failed to load resolution summary:', error);
      // Don't show error for summary - it's optional
    } finally {
      isLoadingSummary = false;
    }
  }

  // Platform resolution handlers
  function handleSelectAll() {
    if (allSelected) {
      modalState.selectedResolutions.clear();
    } else {
      modalState.selectedResolutions = new Set(
        modalState.pendingResolutions.map(r => r.import_id)
      );
    }
  }

  function handleSelectionChange(importId: string, selected: boolean) {
    if (selected) {
      modalState.selectedResolutions.add(importId);
    } else {
      modalState.selectedResolutions.delete(importId);
    }
  }

  async function handlePlatformResolve(importId: string, platformId: string | null) {
    try {
      await platforms.resolvePlatform({
        import_id: importId,
        ...(platformId && { resolved_platform_id: platformId })
      });
      
      // Remove from pending list
      modalState.pendingResolutions = modalState.pendingResolutions.filter(
        r => r.import_id !== importId
      );
      
      successfulResolutions++;
      ui.showSuccess('Platform resolved successfully');
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to resolve platform';
      ui.showError(errorMessage);
    }
  }

  async function handlePlatformSkip(importId: string) {
    try {
      // Skip by resolving with null platform
      await platforms.resolvePlatform({
        import_id: importId,
        user_notes: 'Platform resolution skipped by user'
      });
      
      // Remove from pending list
      modalState.pendingResolutions = modalState.pendingResolutions.filter(
        r => r.import_id !== importId
      );
      
      successfulResolutions++;
      ui.showInfo('Platform resolution skipped');
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to skip platform';
      ui.showError(errorMessage);
    }
  }

  // Storefront resolution handlers
  function handleStorefrontSelectAll() {
    if (allStorefrontsSelected) {
      storefrontModalState.selectedResolutions.clear();
    } else {
      storefrontModalState.selectedResolutions = new Set(
        storefrontModalState.pendingResolutions.map(r => r.import_id)
      );
    }
  }

  function handleStorefrontSelectionChange(importId: string, selected: boolean) {
    if (selected) {
      storefrontModalState.selectedResolutions.add(importId);
    } else {
      storefrontModalState.selectedResolutions.delete(importId);
    }
  }

  async function handleStorefrontResolve(importId: string, storefrontId: string | null) {
    try {
      await platforms.resolveStorefront({
        import_id: importId,
        ...(storefrontId && { resolved_storefront_id: storefrontId })
      });
      
      // Remove from pending list
      storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
        r => r.import_id !== importId
      );
      
      successfulStorefrontResolutions++;
      ui.showSuccess('Storefront resolved successfully');
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to resolve storefront';
      ui.showError(errorMessage);
    }
  }

  async function handleStorefrontSkip(importId: string) {
    try {
      // Skip by resolving with null storefront
      await platforms.resolveStorefront({
        import_id: importId,
        user_notes: 'Storefront resolution skipped by user'
      });
      
      // Remove from pending list
      storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
        r => r.import_id !== importId
      );
      
      successfulStorefrontResolutions++;
      ui.showInfo('Storefront resolution skipped');
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to skip storefront';
      ui.showError(errorMessage);
    }
  }

  // Bulk operations
  async function handleBulkSkipPlatforms() {
    const selectedIds = Array.from(modalState.selectedResolutions);
    if (selectedIds.length === 0) return;

    try {
      modalState.bulkOperationInProgress = true;
      
      for (const importId of selectedIds) {
        await handlePlatformSkip(importId);
      }
      
      modalState.selectedResolutions.clear();
      
    } catch (error) {
      ui.showError('Some platforms failed to skip');
    } finally {
      modalState.bulkOperationInProgress = false;
    }
  }

  async function handleBulkSkipStorefronts() {
    const selectedIds = Array.from(storefrontModalState.selectedResolutions);
    if (selectedIds.length === 0) return;

    try {
      storefrontModalState.bulkOperationInProgress = true;
      
      for (const importId of selectedIds) {
        await handleStorefrontSkip(importId);
      }
      
      storefrontModalState.selectedResolutions.clear();
      
    } catch (error) {
      ui.showError('Some storefronts failed to skip');
    } finally {
      storefrontModalState.bulkOperationInProgress = false;
    }
  }

  // Existing mapping handlers
  async function handleUpdateMappings() {
    if (editingMappings.mappings.length === 0) return;

    try {
      await darkadia.updateMappings(editingMappings);
      editingMappings = { mappings: [] };
      await loadResolutionSummary(); // Reload to show updated mappings
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to update mappings';
      ui.showError(errorMessage);
    }
  }

  function handleClose() {
    const totalSuccessful = successfulResolutions + successfulStorefrontResolutions;
    
    if (onResolutionsComplete && totalSuccessful > 0) {
      console.log('🔒 [PLATFORM-STOREFRONT-MODAL] Modal closing with', totalSuccessful, 'successful resolutions');
      onResolutionsComplete(totalSuccessful);
    }
    
    // Clean up state
    modalState.selectedResolutions.clear();
    storefrontModalState.selectedResolutions.clear();
    successfulResolutions = 0;
    successfulStorefrontResolutions = 0;
    resolutionSummary = null;
    editingMappings = { mappings: [] };
    
    onClose();
  }
</script>

{#if isOpen}
  <!-- Modal backdrop -->
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
    <div class="max-w-6xl w-full bg-white rounded-lg shadow-xl max-h-[90vh] flex flex-col">
      
      <!-- Header -->
      <div class="px-6 py-4 border-b border-gray-200 flex-shrink-0">
        <div class="flex items-center justify-between">
          <h2 class="text-xl font-semibold text-gray-900 flex items-center">
            <span class="text-2xl mr-3">🔗</span>
            Platforms and Storefronts
            {#if hasResolutions}
              <span class="ml-3 bg-yellow-100 text-yellow-800 py-1 px-3 rounded-full text-xs font-medium">
                {modalState.pendingResolutions.length} platforms
              </span>
            {/if}
            {#if hasStorefrontResolutions}
              <span class="ml-2 bg-blue-100 text-blue-800 py-1 px-3 rounded-full text-xs font-medium">
                {storefrontModalState.pendingResolutions.length} storefronts
              </span>
            {/if}
          </h2>
          
          <button
            onclick={handleClose}
            class="text-gray-400 hover:text-gray-600 transition-colors"
            aria-label="Close modal"
          >
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {#if hasAnyResolutions}
          <p class="text-sm text-gray-600 mt-2">
            Resolve platform and storefront names from your CSV import to continue.
          </p>
        {:else}
          <p class="text-sm text-gray-600 mt-2">
            Review and manage your platform and storefront mappings.
          </p>
        {/if}
      </div>

      <!-- Content Area -->
      <div class="flex-1 min-h-0 overflow-auto">
        {#if modalState.isLoading || storefrontModalState.isLoading}
          <!-- Loading State -->
          <div class="flex items-center justify-center py-12">
            <div class="text-center">
              <svg class="animate-spin h-8 w-8 mx-auto text-gray-400" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              <p class="mt-2 text-sm text-gray-500">Loading platform and storefront data...</p>
            </div>
          </div>
        {:else}
          <div class="divide-y divide-gray-200">
            
            <!-- Pending Platform Resolutions Section -->
            {#if hasResolutions}
              <div class="p-6">
                <div class="flex items-center justify-between mb-4">
                  <button
                    onclick={() => platformsCollapsed = !platformsCollapsed}
                    class="flex items-center text-left hover:bg-gray-50 -mx-2 -my-1 px-2 py-1 rounded transition-colors"
                  >
                    <svg class="h-5 w-5 text-gray-400 transition-transform duration-200 {platformsCollapsed ? '' : 'rotate-90'}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                    </svg>
                    <span class="ml-2 text-lg font-medium text-gray-900">
                      ⚠️ Platforms Need Resolution ({modalState.pendingResolutions.length})
                    </span>
                  </button>

                  {#if !platformsCollapsed && hasSelections}
                    <div class="flex items-center space-x-2">
                      <button
                        onclick={handleBulkSkipPlatforms}
                        disabled={modalState.bulkOperationInProgress}
                        class="btn-secondary text-gray-600 hover:text-gray-800 disabled:opacity-50"
                      >
                        {#if modalState.bulkOperationInProgress}
                          <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                          </svg>
                          Skipping...
                        {:else}
                          Skip Selected ({modalState.selectedResolutions.size})
                        {/if}
                      </button>
                    </div>
                  {/if}
                </div>

                {#if !platformsCollapsed}
                  <!-- Bulk selection controls -->
                  {#if modalState.pendingResolutions.length > 0}
                    <div class="mb-4 p-3 bg-gray-50 rounded-lg">
                      <label class="flex items-center">
                        <input
                          type="checkbox"
                          checked={allSelected}
                          onchange={handleSelectAll}
                          class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                        />
                        <span class="ml-2 text-sm text-gray-700">
                          Select all platforms ({modalState.pendingResolutions.length})
                        </span>
                        {#if hasSelections}
                          <span class="ml-2 text-sm text-gray-600">
                            • {modalState.selectedResolutions.size} selected
                          </span>
                        {/if}
                      </label>
                    </div>
                  {/if}

                  <!-- Platform resolution list -->
                  <div class="space-y-3">
                    {#each modalState.pendingResolutions as resolution (resolution.import_id)}
                      <div class="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                        <div class="flex items-center justify-between">
                          <div class="flex items-center space-x-3">
                            <input
                              type="checkbox"
                              checked={modalState.selectedResolutions.has(resolution.import_id)}
                              onchange={(e) => handleSelectionChange(resolution.import_id, (e.target as HTMLInputElement).checked)}
                              class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                            />
                            <div>
                              <div class="font-medium text-gray-900">{resolution.original_platform_name}</div>
                              <div class="text-sm text-gray-600">
                                Found in {resolution.affected_games_count} games
                                {#if resolution.resolution_data.suggestions && resolution.resolution_data.suggestions.length > 0}
                                  • Suggested: {resolution.resolution_data.suggestions[0]?.platform_display_name}
                                {/if}
                              </div>
                            </div>
                          </div>
                          
                          <div class="flex items-center space-x-2">
                            <select 
                              class="form-select text-sm border-gray-300 rounded-md"
                              onchange={(e) => {
                                const platformId = (e.target as HTMLSelectElement).value || null;
                                if (platformId) {
                                  handlePlatformResolve(resolution.import_id, platformId);
                                }
                              }}
                            >
                              <option value="">-- Select Platform --</option>
                              {#if resolution.resolution_data.suggestions}
                                {#each resolution.resolution_data.suggestions as suggestion}
                                  <option value={suggestion.platform_id} class="font-medium">
                                    ✨ {suggestion.platform_display_name} ({Math.round(suggestion.confidence * 100)}% match)
                                  </option>
                                {/each}
                              {/if}
                              {#if platforms.value.platforms}
                                <optgroup label="All Platforms">
                                  {#each platforms.value.platforms as platform}
                                    <option value={platform.id}>{platform.display_name}</option>
                                  {/each}
                                </optgroup>
                              {/if}
                            </select>
                            
                            <button
                              onclick={() => handlePlatformSkip(resolution.import_id)}
                              class="btn-secondary text-gray-600 hover:text-gray-800 text-sm"
                              title="Skip this platform resolution"
                            >
                              Skip
                            </button>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Pending Storefront Resolutions Section -->
            {#if hasStorefrontResolutions}
              <div class="p-6">
                <div class="flex items-center justify-between mb-4">
                  <button
                    onclick={() => storefrontsCollapsed = !storefrontsCollapsed}
                    class="flex items-center text-left hover:bg-gray-50 -mx-2 -my-1 px-2 py-1 rounded transition-colors"
                  >
                    <svg class="h-5 w-5 text-gray-400 transition-transform duration-200 {storefrontsCollapsed ? '' : 'rotate-90'}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                    </svg>
                    <span class="ml-2 text-lg font-medium text-gray-900">
                      ⚠️ Storefronts Need Resolution ({storefrontModalState.pendingResolutions.length})
                    </span>
                  </button>

                  {#if !storefrontsCollapsed && hasStorefrontSelections}
                    <div class="flex items-center space-x-2">
                      <button
                        onclick={handleBulkSkipStorefronts}
                        disabled={storefrontModalState.bulkOperationInProgress}
                        class="btn-secondary text-gray-600 hover:text-gray-800 disabled:opacity-50"
                      >
                        {#if storefrontModalState.bulkOperationInProgress}
                          <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                          </svg>
                          Skipping...
                        {:else}
                          Skip Selected ({storefrontModalState.selectedResolutions.size})
                        {/if}
                      </button>
                    </div>
                  {/if}
                </div>

                {#if !storefrontsCollapsed}
                  <!-- Bulk selection controls -->
                  {#if storefrontModalState.pendingResolutions.length > 0}
                    <div class="mb-4 p-3 bg-gray-50 rounded-lg">
                      <label class="flex items-center">
                        <input
                          type="checkbox"
                          checked={allStorefrontsSelected}
                          onchange={handleStorefrontSelectAll}
                          class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                        />
                        <span class="ml-2 text-sm text-gray-700">
                          Select all storefronts ({storefrontModalState.pendingResolutions.length})
                        </span>
                        {#if hasStorefrontSelections}
                          <span class="ml-2 text-sm text-gray-600">
                            • {storefrontModalState.selectedResolutions.size} selected
                          </span>
                        {/if}
                      </label>
                    </div>
                  {/if}

                  <!-- Storefront resolution list -->
                  <div class="space-y-3">
                    {#each storefrontModalState.pendingResolutions as resolution (resolution.import_id)}
                      <div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
                        <div class="flex items-center justify-between">
                          <div class="flex items-center space-x-3">
                            <input
                              type="checkbox"
                              checked={storefrontModalState.selectedResolutions.has(resolution.import_id)}
                              onchange={(e) => handleStorefrontSelectionChange(resolution.import_id, (e.target as HTMLInputElement).checked)}
                              class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                            />
                            <div>
                              <div class="font-medium text-gray-900">{resolution.original_storefront_name}</div>
                              <div class="text-sm text-gray-600">
                                Found in {resolution.affected_games_count} games
                                {#if resolution.resolution_data.suggestions && resolution.resolution_data.suggestions.length > 0}
                                  • Suggested: {resolution.resolution_data.suggestions[0]?.storefront_display_name}
                                {/if}
                              </div>
                            </div>
                          </div>
                          
                          <div class="flex items-center space-x-2">
                            <select 
                              class="form-select text-sm border-gray-300 rounded-md"
                              onchange={(e) => {
                                const storefrontId = (e.target as HTMLSelectElement).value || null;
                                if (storefrontId) {
                                  handleStorefrontResolve(resolution.import_id, storefrontId);
                                }
                              }}
                            >
                              <option value="">-- Select Storefront --</option>
                              {#if resolution.resolution_data.suggestions}
                                {#each resolution.resolution_data.suggestions as suggestion}
                                  <option value={suggestion.storefront_id} class="font-medium">
                                    ✨ {suggestion.storefront_display_name} ({Math.round(suggestion.confidence * 100)}% match)
                                  </option>
                                {/each}
                              {/if}
                              {#if platforms.value.storefronts}
                                <optgroup label="All Storefronts">
                                  {#each platforms.value.storefronts as storefront}
                                    <option value={storefront.id}>{storefront.display_name}</option>
                                  {/each}
                                </optgroup>
                              {/if}
                            </select>
                            
                            <button
                              onclick={() => handleStorefrontSkip(resolution.import_id)}
                              class="btn-secondary text-gray-600 hover:text-gray-800 text-sm"
                              title="Skip this storefront resolution"
                            >
                              Skip
                            </button>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Existing Mappings Section -->
            {#if resolutionSummary && (resolutionSummary.platforms.length > 0 || resolutionSummary.storefronts.length > 0)}
              <div class="p-6">
                <div class="flex items-center justify-between mb-4">
                  <button
                    onclick={() => existingMappingsCollapsed = !existingMappingsCollapsed}
                    class="flex items-center text-left hover:bg-gray-50 -mx-2 -my-1 px-2 py-1 rounded transition-colors"
                  >
                    <svg class="h-5 w-5 text-gray-400 transition-transform duration-200 {existingMappingsCollapsed ? '' : 'rotate-90'}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                    </svg>
                    <span class="ml-2 text-lg font-medium text-gray-900">
                      📋 Current Mappings ({resolutionSummary.platforms.length + resolutionSummary.storefronts.length})
                    </span>
                  </button>

                  {#if !existingMappingsCollapsed && editingMappings.mappings.length > 0}
                    <button
                      onclick={handleUpdateMappings}
                      class="btn-primary"
                    >
                      Apply Changes ({editingMappings.mappings.length})
                    </button>
                  {/if}
                </div>

                {#if !existingMappingsCollapsed}
                  <div class="space-y-4">
                    {#if resolutionSummary.platforms.length > 0}
                      <div>
                        <h4 class="text-sm font-medium text-gray-900 mb-2">Platform Mappings</h4>
                        <div class="space-y-2">
                          {#each resolutionSummary.platforms as mapping}
                            <div class="bg-gray-50 border border-gray-200 rounded-lg p-3">
                              <div class="flex items-center justify-between">
                                <div class="text-sm">
                                  <span class="font-medium">{mapping.original}</span>
                                  <span class="text-gray-500">→</span>
                                  <span class="text-gray-700">{mapping.mapped}</span>
                                  <span class="text-gray-500">({mapping.game_count} games)</span>
                                </div>
                                <button class="text-blue-600 hover:text-blue-800 text-xs font-medium">
                                  Edit
                                </button>
                              </div>
                            </div>
                          {/each}
                        </div>
                      </div>
                    {/if}

                    {#if resolutionSummary.storefronts.length > 0}
                      <div>
                        <h4 class="text-sm font-medium text-gray-900 mb-2">Storefront Mappings</h4>
                        <div class="space-y-2">
                          {#each resolutionSummary.storefronts as mapping}
                            <div class="bg-gray-50 border border-gray-200 rounded-lg p-3">
                              <div class="flex items-center justify-between">
                                <div class="text-sm">
                                  <span class="font-medium">{mapping.original}</span>
                                  <span class="text-gray-500">→</span>
                                  <span class="text-gray-700">{mapping.mapped}</span>
                                  <span class="text-gray-500">({mapping.game_count} games)</span>
                                </div>
                                <button class="text-blue-600 hover:text-blue-800 text-xs font-medium">
                                  Edit
                                </button>
                              </div>
                            </div>
                          {/each}
                        </div>
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Success State -->
            {#if !hasAnyResolutions && (!resolutionSummary || (resolutionSummary.platforms.length === 0 && resolutionSummary.storefronts.length === 0))}
              <div class="text-center py-12">
                <span class="text-6xl">✅</span>
                <h3 class="mt-4 text-lg font-medium text-gray-900">All Set!</h3>
                <p class="mt-2 text-sm text-gray-500">
                  No platform or storefront resolutions needed.
                </p>
                <div class="mt-6">
                  <button onclick={handleClose} class="btn-primary">
                    Continue Import
                  </button>
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t border-gray-200 bg-gray-50 flex-shrink-0">
        <div class="flex justify-between items-center">
          <div class="text-sm text-gray-600">
            {#if hasAnyResolutions}
              {modalState.pendingResolutions.length + storefrontModalState.pendingResolutions.length} items need resolution
            {:else}
              All platforms and storefronts resolved
            {/if}
          </div>
          
          <div class="flex space-x-3">
            <button onclick={handleClose} class="btn-secondary">
              {hasAnyResolutions ? 'Close & Skip Remaining' : 'Close'}
            </button>
            
            {#if !hasAnyResolutions}
              <button onclick={handleClose} class="btn-primary">
                Continue Import
              </button>
            {/if}
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}