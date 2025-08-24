<script lang="ts">
  import { onMount } from 'svelte';
  import { platforms } from '$lib/stores/platforms.svelte';
  import { ui } from '$lib/stores';
  import type { 
    PendingPlatformResolution,
    PlatformMappingRowState,
    PlatformSuggestion,
    PlatformCreationFormData,
    ResolutionAction
  } from '$lib/types/platform-resolution';
  import PlatformSuggestionCard from './PlatformSuggestionCard.svelte';

  interface Props {
    resolution: PendingPlatformResolution;
    selected: boolean;
    onSelectionChange: (selected: boolean) => void;
    onResolutionAction: (action: ResolutionAction) => void;
  }

  let { 
    resolution, 
    selected = false, 
    onSelectionChange, 
    onResolutionAction 
  }: Props = $props();

  // Component state
  let rowState: PlatformMappingRowState = $state({
    isLoadingSuggestions: false,
    isResolving: false,
    showCreateForm: false,
    expanded: false
  });

  // Platform creation form data
  let createFormData: PlatformCreationFormData = $state({
    name: '',
    display_name: ''
  });

  // Reference to the create form element for auto-scrolling
  let createFormRef = $state<HTMLElement | undefined>();

  // Suggestions from resolution data
  const suggestions = $derived(resolution.resolution_data.suggestions || []);
  const hasSuggestions = $derived(suggestions.length > 0);
  const bestSuggestion = $derived(suggestions[0]); // Suggestions are sorted by confidence

  // Auto-generate platform name from display name
  $effect(() => {
    if (createFormData.display_name && !createFormData.name) {
      // Convert display name to a safe platform name
      createFormData.name = createFormData.display_name
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '');
    }
  });

  // Initialize suggestions on mount if not already loaded
  onMount(async () => {
    if (!hasSuggestions && resolution.resolution_data.status === 'pending') {
      await loadSuggestions();
    }
  });

  async function loadSuggestions() {
    if (rowState.isLoadingSuggestions) return;

    rowState.isLoadingSuggestions = true;

    try {
      const response = await platforms.getSuggestions({
        unknown_platform_name: resolution.original_platform_name,
        ...(resolution.original_storefront_name && { unknown_storefront_name: resolution.original_storefront_name }),
        min_confidence: 0.6,
        max_suggestions: 5
      });

      // Update the resolution data with suggestions
      resolution.resolution_data.suggestions = response.platform_suggestions;
      resolution.resolution_data.storefront_suggestions = response.storefront_suggestions;
      resolution.resolution_data.status = response.platform_suggestions.length > 0 ? 'suggested' : 'pending';

    } catch (error) {
      console.error('Failed to load suggestions:', error);
      ui.showError('Failed to load platform suggestions');
    } finally {
      rowState.isLoadingSuggestions = false;
    }
  }

  function handleSelectionChange() {
    onSelectionChange(!selected);
  }

  function handleExpandToggle() {
    rowState.expanded = !rowState.expanded;
    console.log('🔽 [PLATFORM-MAPPING] Row expanded:', rowState.expanded, 'for platform:', resolution.original_platform_name);
  }

  function handleUseSuggestion(suggestion: PlatformSuggestion) {
    console.log('🎯 [PLATFORM-MAPPING] Using suggestion:', {
      platform: suggestion.platform_display_name || suggestion.platform_name,
      confidence: Math.round(suggestion.confidence * 100),
      original: resolution.original_platform_name
    });
    
    const action: ResolutionAction = {
      type: 'resolve',
      import_id: resolution.import_id,
      platform_id: suggestion.platform_id,
      user_notes: `Resolved using suggestion: ${suggestion.reason} (confidence: ${Math.round(suggestion.confidence * 100)}%)`
    };
    
    rowState.selectedSuggestion = suggestion;
    onResolutionAction(action);
  }

  function handleShowCreateForm() {
    console.log('🔧 [PLATFORM-MAPPING] Showing create form for:', resolution.original_platform_name);
    
    // Auto-expand the row if it's not already expanded
    if (!rowState.expanded) {
      console.log('📂 [PLATFORM-MAPPING] Auto-expanding row to show create form');
      rowState.expanded = true;
    }
    
    rowState.showCreateForm = true;
    // Pre-fill form with original platform name
    createFormData.display_name = resolution.original_platform_name;
    createFormData.name = resolution.original_platform_name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '');

    // Auto-scroll to the form after it's rendered
    setTimeout(() => {
      if (createFormRef) {
        console.log('📜 [PLATFORM-MAPPING] Scrolling create form into view');
        createFormRef.scrollIntoView({
          behavior: 'smooth',
          block: 'nearest',
          inline: 'nearest'
        });
      } else {
        console.warn('⚠️ [PLATFORM-MAPPING] Create form ref not available for scrolling');
      }
    }, 100);
  }

  function handleCancelCreate() {
    console.log('❌ [PLATFORM-MAPPING] Canceling create form for:', resolution.original_platform_name);
    rowState.showCreateForm = false;
    createFormData = { name: '', display_name: '' };
  }

  function validateCreateForm(): string[] {
    const errors: string[] = [];
    
    if (!createFormData.name.trim()) {
      errors.push('Platform name is required');
    } else if (!/^[a-z0-9-]+$/.test(createFormData.name)) {
      errors.push('Platform name must contain only lowercase letters, numbers, and hyphens');
    }
    
    if (!createFormData.display_name.trim()) {
      errors.push('Display name is required');
    } else if (createFormData.display_name.length > 100) {
      errors.push('Display name must be 100 characters or less');
    }
    
    return errors;
  }

  function handleCreatePlatform() {
    const errors = validateCreateForm();
    if (errors.length > 0) {
      ui.showError(errors[0]!);
      return;
    }

    const action: ResolutionAction = {
      type: 'create',
      import_id: resolution.import_id,
      platform_data: { ...createFormData },
      user_notes: `Created new platform for: ${resolution.original_platform_name}`
    };
    
    onResolutionAction(action);
    handleCancelCreate();
  }

  function handleSkipResolution() {
    console.log('⏭️ [PLATFORM-MAPPING] Skip button clicked for:', resolution.original_platform_name);
    const action: ResolutionAction = {
      type: 'skip',
      import_id: resolution.import_id,
      user_notes: 'Skipped platform resolution'
    };
    console.log('📤 [PLATFORM-MAPPING] Sending skip action to modal:', action);
    
    onResolutionAction(action);
  }

  function getConfidenceColor(confidence: number): string {
    if (confidence >= 0.8) return 'text-green-600 bg-green-100';
    if (confidence >= 0.6) return 'text-yellow-600 bg-yellow-100';
    return 'text-red-600 bg-red-100';
  }

  function getConfidenceText(confidence: number): string {
    if (confidence >= 0.8) return 'High';
    if (confidence >= 0.6) return 'Medium';
    return 'Low';
  }
</script>

<div class="p-6 hover:bg-gray-50 transition-colors">
  <!-- Main Row -->
  <div class="flex items-start space-x-4">
    <!-- Selection Checkbox -->
    <div class="flex items-center pt-1">
      <input
        type="checkbox"
        checked={selected}
        onchange={handleSelectionChange}
        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
      />
    </div>

    <!-- Content Area -->
    <div class="flex-1 min-w-0">
      <!-- Header -->
      <div class="flex items-start justify-between">
        <div class="flex-1">
          <h3 class="text-lg font-medium text-gray-900 truncate">
            {resolution.original_platform_name}
          </h3>
          <p class="text-sm text-gray-600 mt-1">
            Affects {resolution.affected_games_count} game{resolution.affected_games_count === 1 ? '' : 's'}
            {#if resolution.affected_games.length > 0}
              • {resolution.affected_games.slice(0, 3).join(', ')}
              {#if resolution.affected_games.length > 3}
                and {resolution.affected_games.length - 3} more
              {/if}
            {/if}
          </p>
        </div>

        <!-- Status Badge -->
        <div class="flex items-center space-x-2">
          {#if resolution.resolution_data.status === 'suggested' && hasSuggestions}
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
              {suggestions.length} suggestion{suggestions.length === 1 ? '' : 's'}
            </span>
          {:else if resolution.resolution_data.status === 'pending'}
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800">
              Needs Resolution
            </span>
          {/if}

          <!-- Expand/Collapse Button -->
          <button
            onclick={handleExpandToggle}
            class="p-1 rounded-full hover:bg-gray-200 transition-colors"
            aria-label={rowState.expanded ? 'Collapse' : 'Expand'}
          >
            <svg 
              class="w-5 h-5 text-gray-500 transition-transform {rowState.expanded ? 'rotate-180' : ''}"
              fill="none" 
              viewBox="0 0 24 24" 
              stroke="currentColor"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
        </div>
      </div>

      <!-- Quick Actions (always visible) -->
      <div class="flex items-center space-x-2 mt-3">
        {#if bestSuggestion && !rowState.showCreateForm}
          <button
            onclick={() => handleUseSuggestion(bestSuggestion)}
            disabled={rowState.isResolving}
            title="Use {bestSuggestion.platform_display_name || bestSuggestion.platform_name} (confidence: {Math.round(bestSuggestion.confidence * 100)}%)"
            class="inline-flex items-center px-3 py-1.5 border border-transparent text-xs font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
          >
            <svg class="w-3 h-3 mr-1 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
            </svg>
            <span class="truncate max-w-[120px]">
              Use: {bestSuggestion.platform_display_name || bestSuggestion.platform_name || 'Best Match'}
            </span>
            <span class="ml-2 inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium flex-shrink-0 {getConfidenceColor(bestSuggestion.confidence)}">
              {getConfidenceText(bestSuggestion.confidence)}
            </span>
          </button>
        {/if}

        <button
          onclick={handleShowCreateForm}
          class="inline-flex items-center px-3 py-1.5 border border-gray-300 text-xs font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
        >
          <svg class="w-3 h-3 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          Create New
        </button>

        <button
          onclick={handleSkipResolution}
          class="inline-flex items-center px-3 py-1.5 border border-gray-300 text-xs font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
        >
          Skip
        </button>
      </div>

      <!-- Expanded Content -->
      {#if rowState.expanded}
        <div class="mt-4 space-y-4 border-t border-gray-200 pt-4">
          <!-- Affected Games -->
          {#if resolution.affected_games.length > 0}
            <div>
              <h4 class="text-sm font-medium text-gray-900 mb-2">Affected Games</h4>
              <div class="flex flex-wrap gap-2">
                {#each resolution.affected_games.slice(0, 10) as gameName}
                  <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
                    {gameName}
                  </span>
                {/each}
                {#if resolution.affected_games.length > 10}
                  <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-gray-200 text-gray-600">
                    +{resolution.affected_games.length - 10} more
                  </span>
                {/if}
              </div>
            </div>
          {/if}

          <!-- Suggestions -->
          {#if rowState.isLoadingSuggestions}
            <div class="flex items-center justify-center py-4">
              <svg class="animate-spin h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              <span class="ml-2 text-sm text-gray-500">Loading suggestions...</span>
            </div>
          {:else if hasSuggestions}
            <div>
              <h4 class="text-sm font-medium text-gray-900 mb-3">Platform Suggestions</h4>
              <div class="space-y-2">
                {#each suggestions as suggestion (suggestion.platform_id)}
                  <PlatformSuggestionCard
                    {suggestion}
                    onUseSuggestion={() => handleUseSuggestion(suggestion)}
                    disabled={rowState.isResolving}
                  />
                {/each}
              </div>
            </div>
          {:else if resolution.resolution_data.status !== 'pending'}
            <div class="text-center py-4">
              <p class="text-sm text-gray-500">No similar platforms found</p>
              <p class="text-xs text-gray-400 mt-1">Consider creating a new platform</p>
            </div>
          {/if}

          <!-- Create Platform Form -->
          {#if rowState.showCreateForm}
            <div bind:this={createFormRef} class="border border-gray-200 rounded-lg p-4 bg-gray-50">
              <h4 class="text-sm font-medium text-gray-900 mb-3">Create New Platform</h4>
              
              <div class="space-y-3">
                <div>
                  <label for="display-name" class="block text-sm font-medium text-gray-700">Display Name</label>
                  <input
                    id="display-name"
                    type="text"
                    bind:value={createFormData.display_name}
                    placeholder="e.g., PlayStation 6"
                    class="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 sm:text-sm"
                  />
                </div>

                <div>
                  <label for="internal-name" class="block text-sm font-medium text-gray-700">Internal Name</label>
                  <input
                    id="internal-name"
                    type="text"
                    bind:value={createFormData.name}
                    placeholder="e.g., playstation-6"
                    pattern="^[a-z0-9-]+$"
                    class="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 sm:text-sm font-mono"
                  />
                  <p class="mt-1 text-xs text-gray-500">Auto-generated from display name. Use lowercase letters, numbers, and hyphens only.</p>
                </div>

                <div>
                  <label for="icon-url" class="block text-sm font-medium text-gray-700">Icon URL (Optional)</label>
                  <input
                    id="icon-url"
                    type="url"
                    bind:value={createFormData.icon_url}
                    placeholder="https://example.com/icon.png"
                    class="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 sm:text-sm"
                  />
                </div>
              </div>

              <div class="flex justify-end space-x-2 mt-4">
                <button
                  onclick={handleCancelCreate}
                  class="px-3 py-2 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                >
                  Cancel
                </button>
                <button
                  onclick={handleCreatePlatform}
                  disabled={rowState.isResolving}
                  class="px-3 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 disabled:opacity-50"
                >
                  Create Platform
                </button>
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>