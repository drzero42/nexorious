<script lang="ts">
  import { platforms } from '$lib/stores/platforms.svelte';
  import { ui } from '$lib/stores';
  import type { 
    PlatformResolutionUIState,
    StorefrontResolutionUIState,
    ResolutionAction
  } from '$lib/types/platform-resolution';
  import PlatformMappingRow from './PlatformMappingRow.svelte';
  import StorefrontMappingRow from './StorefrontMappingRow.svelte';

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

  // Tab state
  let activeTab = $state<'overview' | 'resolve' | 'storefronts'>('overview');
  
  // Pagination state
  let currentPage = $state(1);
  let totalPages = $state(1);
  let totalResolutions = $state(0);

  // Storefront resolution state
  let storefrontModalState = $state<StorefrontResolutionUIState>({
    isOpen: false,
    isLoading: false,
    pendingResolutions: [],
    selectedResolutions: new Set(),
    bulkOperationInProgress: false
  });

  // Storefront pagination state
  let storefrontCurrentPage = $state(1);
  let storefrontTotalPages = $state(1);
  let storefrontTotalResolutions = $state(0);

  // Derived states
  const hasResolutions = $derived(modalState.pendingResolutions.length > 0);
  const hasSelections = $derived(modalState.selectedResolutions.size > 0);
  const allSelected = $derived(
    hasResolutions && modalState.selectedResolutions.size === modalState.pendingResolutions.length
  );

  // Storefront derived states
  const hasStorefrontResolutions = $derived(storefrontModalState.pendingResolutions.length > 0);
  const hasStorefrontSelections = $derived(storefrontModalState.selectedResolutions.size > 0);
  const allStorefrontsSelected = $derived(
    hasStorefrontResolutions && storefrontModalState.selectedResolutions.size === storefrontModalState.pendingResolutions.length
  );

  // Actions queue for bulk operations
  let pendingActions = $state<ResolutionAction[]>([]);
  let storefrontPendingActions = $state<ResolutionAction[]>([]);
  
  // Track successful resolutions for proper callback
  let successfulResolutions = $state(0);
  let successfulStorefrontResolutions = $state(0);

  // Initialize modal when opened
  $effect(() => {
    if (isOpen && !modalState.isOpen) {
      console.log('🚪 [RESOLUTION-MODAL] Modal opening - will fetch fresh data from backend');
      console.log('⚠️ [RESOLUTION-MODAL] Any previously skipped platforms will reappear!');
      pendingActions = []; // Clear any stale actions when modal opens
      storefrontPendingActions = []; // Clear storefront actions
      successfulResolutions = 0; // Reset counter when modal opens
      successfulStorefrontResolutions = 0; // Reset storefront counter
      modalState.isOpen = true;
      storefrontModalState.isOpen = true;
      loadPendingResolutions();
      loadPendingStorefrontResolutions();
    } else if (!isOpen && modalState.isOpen) {
      modalState.isOpen = false;
      storefrontModalState.isOpen = false;
    }
  });

  async function loadPendingResolutions() {
    if (modalState.isLoading) return;

    console.log('🔄 [RESOLUTION-MODAL] Loading pending resolutions from backend...');
    modalState.isLoading = true;
    delete modalState.error;

    try {
      const response = await platforms.getPendingResolutions(currentPage, 20);
      
      console.log('📥 [RESOLUTION-MODAL] Loaded from backend:', {
        count: response.pending_resolutions.length,
        total: response.total,
        importIds: response.pending_resolutions.map(r => r.import_id)
      });
      
      modalState.pendingResolutions = response.pending_resolutions;
      
      totalPages = response.pages;
      totalResolutions = response.total;
      
      console.log('📊 [RESOLUTION-MODAL] Modal state updated with backend data');
      
      // Reset selections when loading new data
      modalState.selectedResolutions.clear();
      
      // If we have resolutions, switch to resolve tab
      if (hasResolutions && activeTab === 'overview') {
        activeTab = 'resolve';
      }
    } catch (error) {
      modalState.error = error instanceof Error ? error.message : 'Failed to load pending resolutions';
      ui.showError(modalState.error);
    } finally {
      modalState.isLoading = false;
    }
  }

  async function loadPendingStorefrontResolutions() {
    if (storefrontModalState.isLoading) return;

    console.log('🔄 [RESOLUTION-MODAL] Loading pending storefront resolutions from backend...');
    storefrontModalState.isLoading = true;
    delete storefrontModalState.error;

    try {
      const response = await platforms.getPendingStorefrontResolutions(storefrontCurrentPage, 20);
      
      console.log('📥 [RESOLUTION-MODAL] Loaded storefront resolutions from backend:', {
        count: response.pending_resolutions.length,
        total: response.total,
        importIds: response.pending_resolutions.map(r => r.import_id)
      });
      
      storefrontModalState.pendingResolutions = response.pending_resolutions;
      storefrontTotalPages = response.pages;
      storefrontTotalResolutions = response.total;
      
      console.log('📊 [RESOLUTION-MODAL] Storefront modal state updated with backend data');
      
      // Reset selections when loading new data
      storefrontModalState.selectedResolutions.clear();
      
      // If we have storefront resolutions, switch to storefronts tab
      if (hasStorefrontResolutions && activeTab === 'overview') {
        activeTab = 'storefronts';
      }
    } catch (error) {
      storefrontModalState.error = error instanceof Error ? error.message : 'Failed to load pending storefront resolutions';
      ui.showError(storefrontModalState.error);
    } finally {
      storefrontModalState.isLoading = false;
    }
  }

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

  function handleResolutionAction(action: ResolutionAction) {
    console.log('📥 [RESOLUTION-MODAL] Received action:', {
      type: action.type,
      import_id: action.import_id,
      currentPendingCount: pendingActions.length,
      currentResolutionsCount: modalState.pendingResolutions.length
    });
    
    // Handle skip actions by persisting to backend
    if (action.type === 'skip') {
      console.log('⏭️ [RESOLUTION-MODAL] Skip action - persisting to backend to prevent reappearance');
      
      // Call backend API to mark as skipped (resolved with null platform)
      handleSkipPersistence(action);
    } else {
      // Queue and apply resolve/create actions
      pendingActions.push(action);
      console.log('📋 [RESOLUTION-MODAL] Added to pendingActions. New length:', pendingActions.length);
      console.log('🚀 [RESOLUTION-MODAL] Applying non-skip action');
      applySingleAction(action);
    }
  }

  async function handleSkipPersistence(action: ResolutionAction) {
    try {
      console.log('📡 [RESOLUTION-MODAL] Calling backend API to persist skip for import:', action.import_id);
      
      // Mark as resolved with null platform in backend
      await platforms.resolvePlatform({
        import_id: action.import_id,
        user_notes: action.user_notes || 'Platform resolution skipped by user'
      });
      
      console.log('✅ [RESOLUTION-MODAL] Skip persisted to backend successfully');
      
      // Remove from UI after successful backend update
      modalState.pendingResolutions = modalState.pendingResolutions.filter(
        r => r.import_id !== action.import_id
      );
      
      totalResolutions -= 1;
      successfulResolutions += 1; // Count as resolved
      
      console.log('📊 [RESOLUTION-MODAL] UI updated after backend persistence - resolutions:', totalResolutions, 'successful:', successfulResolutions);
      
      ui.showInfo('Platform resolution skipped');
      
    } catch (error) {
      console.error('❌ [RESOLUTION-MODAL] Failed to persist skip to backend:', error);
      ui.showError('Failed to skip platform resolution');
    }
  }

  async function applySingleAction(action: ResolutionAction) {
    try {
      if (action.type === 'resolve' && action.platform_id) {
        await platforms.resolvePlatform({
          import_id: action.import_id,
          ...(action.platform_id && { resolved_platform_id: action.platform_id }),
          ...(action.storefront_id && { resolved_storefront_id: action.storefront_id }),
          ...(action.user_notes && { user_notes: action.user_notes })
        });
        
        ui.showSuccess('Platform resolved successfully');
        
        // Remove from pending resolutions
        modalState.pendingResolutions = modalState.pendingResolutions.filter(
          r => r.import_id !== action.import_id
        );
        
        totalResolutions -= 1;
        
      } else if (action.type === 'create' && action.platform_data) {
        const newPlatform = await platforms.createPlatformFromResolution(
          action.platform_data,
          action.import_id
        );
        
        ui.showSuccess(`Created and resolved platform: ${newPlatform.display_name}`);
        
        // Remove from pending resolutions
        modalState.pendingResolutions = modalState.pendingResolutions.filter(
          r => r.import_id !== action.import_id
        );
        
        totalResolutions -= 1;
      }
      
      // Remove from selections if it was selected
      modalState.selectedResolutions.delete(action.import_id);
      
      // Remove the successfully applied action from pendingActions
      pendingActions = pendingActions.filter(a => a.import_id !== action.import_id);
      
      // Increment successful resolutions counter
      successfulResolutions += 1;
      
      console.log('✅ [RESOLUTION-MODAL] Single platform resolved successfully. Total successful:', successfulResolutions);
      
      // Don't call callback here - let modal close handle it to avoid double counting
      
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : 'Failed to apply resolution';
      ui.showError(errorMsg);
    }
  }

  async function applyBulkActions() {
    if (pendingActions.length === 0) return;

    modalState.bulkOperationInProgress = true;
    
    try {
      const resolveActions = pendingActions.filter(a => a.type === 'resolve');
      let result: any = null;
      
      if (resolveActions.length > 0) {
        const bulkRequest = {
          resolutions: resolveActions.map(action => ({
            import_id: action.import_id,
            ...(action.platform_id && { resolved_platform_id: action.platform_id }),
            ...(action.storefront_id && { resolved_storefront_id: action.storefront_id }),
            ...(action.user_notes && { user_notes: action.user_notes })
          }))
        };
        
        result = await platforms.bulkResolvePlatforms(bulkRequest);
        
        console.log('📊 [RESOLUTION-MODAL] Bulk results:', {
          total: result.total_processed,
          successful: result.successful_resolutions,
          failed: result.failed_resolutions
        });
        
        ui.showSuccess(
          `Bulk operation completed: ${result.successful_resolutions} resolved, ${result.failed_resolutions} failed`
        );
        
        // Remove successfully resolved items
        const successfulImportIds = result.results
          .filter((r: any) => r.success)
          .map((r: any) => r.import_id);
        
        modalState.pendingResolutions = modalState.pendingResolutions.filter(
          r => !successfulImportIds.includes(r.import_id)
        );
        
        totalResolutions -= successfulImportIds.length;
        
        // Update successful resolutions counter
        successfulResolutions += result.successful_resolutions;
      }
      
      // Handle individual create actions
      const createActions = pendingActions.filter(a => a.type === 'create');
      let createSuccessCount = 0;
      for (const action of createActions) {
        if (action.platform_data) {
          try {
            await platforms.createPlatformFromResolution(
              action.platform_data,
              action.import_id
            );
            
            modalState.pendingResolutions = modalState.pendingResolutions.filter(
              r => r.import_id !== action.import_id
            );
            
            totalResolutions -= 1;
            createSuccessCount += 1;
          } catch (error) {
            console.error('Failed to create platform:', error);
          }
        }
      }
      
      // Update counter with create successes
      successfulResolutions += createSuccessCount;
      
      // Calculate total successful resolutions for this bulk operation
      const bulkSuccessCount = (result ? result.successful_resolutions : 0) + createSuccessCount;
      
      console.log('📊 [RESOLUTION-MODAL] Bulk operation summary:', {
        resolveSuccessful: result ? result.successful_resolutions : 0,
        createSuccessful: createSuccessCount,
        totalBulkSuccess: bulkSuccessCount,
        totalSessionSuccess: successfulResolutions
      });
      
      // Clear selections and actions
      modalState.selectedResolutions.clear();
      pendingActions = [];
      
      // Don't call callback here - let modal close handle it to avoid double counting
      
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : 'Failed to apply bulk actions';
      ui.showError(errorMsg);
    } finally {
      modalState.bulkOperationInProgress = false;
    }
  }

  function skipSelectedResolutions() {
    const selectedIds = Array.from(modalState.selectedResolutions);
    console.log('⏭️ [RESOLUTION-MODAL] Bulk skip clicked for', selectedIds.length, 'platforms');
    
    // Remove selected resolutions from the list (skip them)
    modalState.pendingResolutions = modalState.pendingResolutions.filter(
      r => !selectedIds.includes(r.import_id)
    );
    console.log('✅ [RESOLUTION-MODAL] Removed from list. New count:', modalState.pendingResolutions.length);
    
    totalResolutions -= selectedIds.length;
    modalState.selectedResolutions.clear();
    
    console.log('📊 [RESOLUTION-MODAL] After bulk skip - totalResolutions:', totalResolutions);
    ui.showInfo(`Skipped ${selectedIds.length} platform resolutions`);
  }

  // Storefront Resolution Handlers

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

  function handleStorefrontResolutionAction(action: ResolutionAction) {
    console.log('📥 [RESOLUTION-MODAL] Received storefront action:', {
      type: action.type,
      import_id: action.import_id,
      currentPendingCount: storefrontPendingActions.length,
      currentResolutionsCount: storefrontModalState.pendingResolutions.length
    });
    
    // Handle skip actions by persisting to backend
    if (action.type === 'skip') {
      console.log('⏭️ [RESOLUTION-MODAL] Storefront skip action - persisting to backend');
      handleStorefrontSkipPersistence(action);
    } else {
      // Queue and apply resolve/create actions
      storefrontPendingActions.push(action);
      console.log('📋 [RESOLUTION-MODAL] Added to storefrontPendingActions. New length:', storefrontPendingActions.length);
      console.log('🚀 [RESOLUTION-MODAL] Applying non-skip storefront action');
      applyStorefrontSingleAction(action);
    }
  }

  async function handleStorefrontSkipPersistence(action: ResolutionAction) {
    try {
      console.log('📡 [RESOLUTION-MODAL] Calling backend API to persist storefront skip for import:', action.import_id);
      
      // Mark as resolved with null storefront in backend
      await platforms.resolveStorefront({
        import_id: action.import_id,
        user_notes: action.user_notes || 'Storefront resolution skipped by user'
      });
      
      console.log('✅ [RESOLUTION-MODAL] Storefront skip persisted to backend successfully');
      
      // Remove from UI after successful backend update
      storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
        r => r.import_id !== action.import_id
      );
      
      storefrontTotalResolutions -= 1;
      successfulStorefrontResolutions += 1; // Count as resolved
      
      console.log('📊 [RESOLUTION-MODAL] UI updated after storefront backend persistence - resolutions:', storefrontTotalResolutions, 'successful:', successfulStorefrontResolutions);
      
      ui.showInfo('Storefront resolution skipped');
      
    } catch (error) {
      console.error('❌ [RESOLUTION-MODAL] Failed to persist storefront skip to backend:', error);
      ui.showError('Failed to skip storefront resolution');
    }
  }

  async function applyStorefrontSingleAction(action: ResolutionAction) {
    try {
      if (action.type === 'resolve' && action.storefront_id) {
        await platforms.resolveStorefront({
          import_id: action.import_id,
          resolved_storefront_id: action.storefront_id,
          user_notes: action.user_notes
        });
        
        ui.showSuccess('Storefront resolved successfully');
        
        // Remove from pending resolutions
        storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
          r => r.import_id !== action.import_id
        );
        
        storefrontTotalResolutions -= 1;
        
      } else if (action.type === 'create' && action.storefront_data) {
        const newStorefront = await platforms.createStorefrontFromResolution(
          action.storefront_data,
          action.import_id
        );
        
        ui.showSuccess(`Created and resolved storefront: ${newStorefront.display_name}`);
        
        // Remove from pending resolutions
        storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
          r => r.import_id !== action.import_id
        );
        
        storefrontTotalResolutions -= 1;
      }
      
      // Remove from selections if it was selected
      storefrontModalState.selectedResolutions.delete(action.import_id);
      
      // Remove the successfully applied action from storefrontPendingActions
      storefrontPendingActions = storefrontPendingActions.filter(a => a.import_id !== action.import_id);
      
      // Increment successful resolutions counter
      successfulStorefrontResolutions += 1;
      
      console.log('✅ [RESOLUTION-MODAL] Single storefront resolved successfully. Total successful:', successfulStorefrontResolutions);
      
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : 'Failed to apply storefront resolution';
      ui.showError(errorMsg);
    }
  }

  async function applyStorefrontBulkActions() {
    if (storefrontPendingActions.length === 0) return;

    storefrontModalState.bulkOperationInProgress = true;
    
    try {
      const resolveActions = storefrontPendingActions.filter(a => a.type === 'resolve');
      let result: any = null;
      
      if (resolveActions.length > 0) {
        const bulkRequest = {
          resolutions: resolveActions.map(action => ({
            import_id: action.import_id,
            ...(action.storefront_id && { resolved_storefront_id: action.storefront_id }),
            ...(action.user_notes && { user_notes: action.user_notes })
          }))
        };
        
        result = await platforms.bulkResolveStorefronts(bulkRequest);
        
        console.log('📊 [RESOLUTION-MODAL] Storefront bulk results:', {
          total: result.total_processed,
          successful: result.successful_resolutions,
          failed: result.failed_resolutions
        });
        
        ui.showSuccess(
          `Bulk storefront operation completed: ${result.successful_resolutions} resolved, ${result.failed_resolutions} failed`
        );
        
        // Remove successfully resolved items
        const successfulImportIds = result.results
          .filter((r: any) => r.success)
          .map((r: any) => r.import_id);
        
        storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
          r => !successfulImportIds.includes(r.import_id)
        );
        
        storefrontTotalResolutions -= successfulImportIds.length;
        
        // Update successful resolutions counter
        successfulStorefrontResolutions += result.successful_resolutions;
      }
      
      // Handle individual create actions (storefronts don't have bulk create API)
      const createActions = storefrontPendingActions.filter(a => a.type === 'create');
      let createSuccessCount = 0;
      for (const action of createActions) {
        if (action.storefront_data) {
          try {
            await platforms.createStorefrontFromResolution(
              action.storefront_data,
              action.import_id
            );
            
            storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
              r => r.import_id !== action.import_id
            );
            
            storefrontTotalResolutions -= 1;
            createSuccessCount += 1;
          } catch (error) {
            console.error('Failed to create storefront:', error);
          }
        }
      }
      
      // Update counter with create successes
      successfulStorefrontResolutions += createSuccessCount;
      
      // Clear selections and actions
      storefrontModalState.selectedResolutions.clear();
      storefrontPendingActions = [];
      
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : 'Failed to apply bulk storefront actions';
      ui.showError(errorMsg);
    } finally {
      storefrontModalState.bulkOperationInProgress = false;
    }
  }

  function skipSelectedStorefrontResolutions() {
    const selectedIds = Array.from(storefrontModalState.selectedResolutions);
    console.log('⏭️ [RESOLUTION-MODAL] Bulk storefront skip clicked for', selectedIds.length, 'storefronts');
    
    // Remove selected resolutions from the list (skip them)
    storefrontModalState.pendingResolutions = storefrontModalState.pendingResolutions.filter(
      r => !selectedIds.includes(r.import_id)
    );
    console.log('✅ [RESOLUTION-MODAL] Removed storefront from list. New count:', storefrontModalState.pendingResolutions.length);
    
    storefrontTotalResolutions -= selectedIds.length;
    storefrontModalState.selectedResolutions.clear();
    
    console.log('📊 [RESOLUTION-MODAL] After bulk storefront skip - storefrontTotalResolutions:', storefrontTotalResolutions);
    ui.showInfo(`Skipped ${selectedIds.length} storefront resolutions`);
  }

  async function handleStorefrontPageChange(page: number) {
    if (page < 1 || page > storefrontTotalPages) return;
    
    storefrontCurrentPage = page;
    await loadPendingStorefrontResolutions();
  }

  function handleClose() {
    // Check if there are unsaved actions (platforms or storefronts)
    const totalPendingActions = pendingActions.length + storefrontPendingActions.length;
    if (totalPendingActions > 0) {
      const confirmed = confirm(
        `You have ${totalPendingActions} pending resolution actions. Close without applying them?`
      );
      if (!confirmed) return;
    }
    
    // Call final callback if there were successful resolutions during this session
    const totalSuccessful = successfulResolutions + successfulStorefrontResolutions;
    if (onResolutionsComplete && totalSuccessful > 0) {
      console.log('🔒 [RESOLUTION-MODAL] Modal closing with', totalSuccessful, 'successful resolutions in session');
      console.log('⚠️ [RESOLUTION-MODAL] UI state will be lost - skipped items will reappear next time!');
      console.log('🔄 [RESOLUTION-MODAL] Calling final onResolutionsComplete with total session count:', totalSuccessful);
      onResolutionsComplete(totalSuccessful);
    }
    
    // Clean up state
    modalState.selectedResolutions.clear();
    storefrontModalState.selectedResolutions.clear();
    pendingActions = [];
    storefrontPendingActions = [];
    successfulResolutions = 0; // Reset counter
    successfulStorefrontResolutions = 0; // Reset storefront counter
    activeTab = 'overview';
    currentPage = 1;
    storefrontCurrentPage = 1;
    
    onClose();
  }

  async function handlePageChange(page: number) {
    if (page < 1 || page > totalPages) return;
    
    currentPage = page;
    await loadPendingResolutions();
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
            Platform & Storefront Resolution Required
            {#if totalResolutions > 0}
              <span class="ml-3 bg-yellow-100 text-yellow-800 py-1 px-3 rounded-full text-xs font-medium">
                {totalResolutions} platforms
              </span>
            {/if}
            {#if storefrontTotalResolutions > 0}
              <span class="ml-2 bg-blue-100 text-blue-800 py-1 px-3 rounded-full text-xs font-medium">
                {storefrontTotalResolutions} storefronts
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

        {#if totalResolutions > 0 || storefrontTotalResolutions > 0}
          <p class="text-sm text-gray-600 mt-2">
            {#if totalResolutions > 0 && storefrontTotalResolutions > 0}
              Your CSV import contains platform and storefront names that don't match our known entries.
            {:else if totalResolutions > 0}
              Your CSV import contains platform names that don't match our known platforms.
            {:else}
              Your CSV import contains storefront names that don't match our known storefronts.
            {/if}
            Review and resolve them below to continue.
          </p>
        {/if}
      </div>

      <!-- Tab Navigation -->
      <div class="border-b border-gray-200 flex-shrink-0">
        <nav class="flex">
          <button
            onclick={() => activeTab = 'overview'}
            class="px-6 py-3 text-sm font-medium border-b-2 {activeTab === 'overview' 
              ? 'border-blue-500 text-blue-600' 
              : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
          >
            Overview
          </button>
          <button
            onclick={() => activeTab = 'resolve'}
            disabled={!hasResolutions}
            class="px-6 py-3 text-sm font-medium border-b-2 {activeTab === 'resolve' 
              ? 'border-blue-500 text-blue-600' 
              : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'} 
              {!hasResolutions ? 'opacity-50 cursor-not-allowed' : ''}"
          >
            Resolve Platforms
            {#if hasResolutions}
              <span class="ml-2 bg-red-100 text-red-600 py-0.5 px-2 rounded-full text-xs font-medium">
                {modalState.pendingResolutions.length}
              </span>
            {/if}
          </button>
          <button
            onclick={() => activeTab = 'storefronts'}
            class="px-6 py-3 text-sm font-medium border-b-2 {activeTab === 'storefronts' 
              ? 'border-blue-500 text-blue-600' 
              : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'}"
          >
            Resolve Storefronts
            {#if hasStorefrontResolutions}
              <span class="ml-2 bg-blue-100 text-blue-600 py-0.5 px-2 rounded-full text-xs font-medium">
                {storefrontModalState.pendingResolutions.length}
              </span>
            {/if}
          </button>
        </nav>
      </div>

      <!-- Content Area -->
      <div class="flex-1 min-h-0 overflow-auto">
        {#if modalState.isLoading}
          <!-- Loading State -->
          <div class="flex items-center justify-center py-12">
            <div class="text-center">
              <svg class="animate-spin h-8 w-8 mx-auto text-gray-400" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              <p class="mt-2 text-sm text-gray-500">Loading platform resolutions...</p>
            </div>
          </div>
        {:else if activeTab === 'overview'}
          <!-- Overview Tab -->
          <div class="p-6">
            {#if !hasResolutions}
              <div class="text-center py-12">
                <span class="text-6xl">✅</span>
                <h3 class="mt-4 text-lg font-medium text-gray-900">All Platforms Resolved!</h3>
                <p class="mt-2 text-sm text-gray-500">
                  All platform names from your CSV import have been successfully resolved.
                </p>
                <div class="mt-6">
                  <button onclick={handleClose} class="btn-primary">
                    Continue Import
                  </button>
                </div>
              </div>
            {:else}
              <div class="space-y-6">
                <div class="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                  <div class="flex">
                    <div class="flex-shrink-0">
                      <svg class="h-5 w-5 text-yellow-400" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd"></path>
                      </svg>
                    </div>
                    <div class="ml-3">
                      <h3 class="text-sm font-medium text-yellow-800">Platform Resolution Required</h3>
                      <p class="mt-1 text-sm text-yellow-700">
                        Your CSV contains {totalResolutions} unknown platform names that need to be resolved.
                        You can either map them to existing platforms or create new ones.
                      </p>
                    </div>
                  </div>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div class="bg-blue-50 rounded-lg p-4">
                    <div class="flex items-center">
                      <div class="flex-shrink-0">
                        <span class="text-2xl">🔍</span>
                      </div>
                      <div class="ml-3">
                        <p class="text-sm font-medium text-blue-900">Auto-Suggestions</p>
                        <p class="text-xs text-blue-700">We'll suggest similar platforms</p>
                      </div>
                    </div>
                  </div>

                  <div class="bg-green-50 rounded-lg p-4">
                    <div class="flex items-center">
                      <div class="flex-shrink-0">
                        <span class="text-2xl">➕</span>
                      </div>
                      <div class="ml-3">
                        <p class="text-sm font-medium text-green-900">Create New</p>
                        <p class="text-xs text-green-700">Add missing platforms</p>
                      </div>
                    </div>
                  </div>

                  <div class="bg-gray-50 rounded-lg p-4">
                    <div class="flex items-center">
                      <div class="flex-shrink-0">
                        <span class="text-2xl">⏭️</span>
                      </div>
                      <div class="ml-3">
                        <p class="text-sm font-medium text-gray-900">Skip & Continue</p>
                        <p class="text-xs text-gray-700">Import without platforms</p>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="flex justify-center">
                  <button
                    onclick={() => activeTab = 'resolve'}
                    class="btn-primary"
                  >
                    Start Resolving Platforms
                  </button>
                </div>
              </div>
            {/if}
          </div>
        {:else if activeTab === 'resolve'}
          <!-- Resolve Tab -->
          <div class="flex flex-col min-h-0 h-full">
            <!-- Bulk Actions Bar -->
            {#if hasResolutions}
              <div class="px-6 py-3 bg-gray-50 border-b border-gray-200 flex-shrink-0">
                <div class="flex items-center justify-between">
                  <div class="flex items-center space-x-4">
                    <label class="flex items-center">
                      <input
                        type="checkbox"
                        checked={allSelected}
                        onchange={handleSelectAll}
                        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                      />
                      <span class="ml-2 text-sm text-gray-700">
                        Select all ({modalState.pendingResolutions.length})
                      </span>
                    </label>
                    
                    {#if hasSelections}
                      <span class="text-sm text-gray-600">
                        {modalState.selectedResolutions.size} selected
                      </span>
                    {/if}
                  </div>

                  <div class="flex items-center space-x-2">
                    {#if pendingActions.length > 0}
                      <button
                        onclick={applyBulkActions}
                        disabled={modalState.bulkOperationInProgress}
                        class="btn-primary disabled:opacity-50"
                      >
                        {#if modalState.bulkOperationInProgress}
                          <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                          </svg>
                          Applying...
                        {:else}
                          Apply {pendingActions.length} Actions
                        {/if}
                      </button>
                    {/if}
                    
                    {#if hasSelections}
                      <button
                        onclick={skipSelectedResolutions}
                        class="btn-secondary text-gray-600 hover:text-gray-800"
                      >
                        Skip Selected
                      </button>
                    {/if}
                  </div>
                </div>
              </div>
            {/if}

            <!-- Resolution List -->
            <div class="flex-1">
              {#if hasResolutions}
                <div class="divide-y divide-gray-200">
                  {#each modalState.pendingResolutions as resolution (resolution.import_id)}
                    <PlatformMappingRow
                      {resolution}
                      selected={modalState.selectedResolutions.has(resolution.import_id)}
                      onSelectionChange={(selected) => handleSelectionChange(resolution.import_id, selected)}
                      onResolutionAction={handleResolutionAction}
                    />
                  {/each}
                </div>

                <!-- Pagination -->
                {#if totalPages > 1}
                  <div class="px-6 py-4 border-t border-gray-200 bg-gray-50">
                    <div class="flex items-center justify-between">
                      <p class="text-sm text-gray-700">
                        Showing page {currentPage} of {totalPages} 
                        ({totalResolutions} total resolutions)
                      </p>
                      
                      <div class="flex items-center space-x-2">
                        <button
                          onclick={() => handlePageChange(currentPage - 1)}
                          disabled={currentPage <= 1}
                          class="btn-secondary disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          Previous
                        </button>
                        
                        <span class="text-sm text-gray-500">
                          {currentPage} / {totalPages}
                        </span>
                        
                        <button
                          onclick={() => handlePageChange(currentPage + 1)}
                          disabled={currentPage >= totalPages}
                          class="btn-secondary disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          Next
                        </button>
                      </div>
                    </div>
                  </div>
                {/if}
              {:else}
                <div class="text-center py-12">
                  <span class="text-6xl">🎉</span>
                  <h3 class="mt-4 text-lg font-medium text-gray-900">All Done!</h3>
                  <p class="mt-2 text-sm text-gray-500">
                    All platform resolutions have been completed.
                  </p>
                </div>
              {/if}
            </div>
          </div>
        {:else if activeTab === 'storefronts'}
          <!-- Storefronts Tab -->
          <div class="flex flex-col min-h-0 h-full">
            <!-- Bulk Actions Bar -->
            {#if hasStorefrontResolutions}
              <div class="px-6 py-3 bg-gray-50 border-b border-gray-200 flex-shrink-0">
                <div class="flex items-center justify-between">
                  <div class="flex items-center space-x-4">
                    <label class="flex items-center">
                      <input
                        type="checkbox"
                        checked={allStorefrontsSelected}
                        onchange={handleStorefrontSelectAll}
                        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                      />
                      <span class="ml-2 text-sm text-gray-700">
                        Select all ({storefrontModalState.pendingResolutions.length})
                      </span>
                    </label>
                    
                    {#if hasStorefrontSelections}
                      <span class="text-sm text-gray-600">
                        {storefrontModalState.selectedResolutions.size} selected
                      </span>
                    {/if}
                  </div>

                  <div class="flex items-center space-x-2">
                    {#if storefrontPendingActions.length > 0}
                      <button
                        onclick={applyStorefrontBulkActions}
                        disabled={storefrontModalState.bulkOperationInProgress}
                        class="btn-primary disabled:opacity-50"
                      >
                        {#if storefrontModalState.bulkOperationInProgress}
                          <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                          </svg>
                          Applying...
                        {:else}
                          Apply {storefrontPendingActions.length} Actions
                        {/if}
                      </button>
                    {/if}
                    
                    {#if hasStorefrontSelections}
                      <button
                        onclick={skipSelectedStorefrontResolutions}
                        class="btn-secondary text-gray-600 hover:text-gray-800"
                      >
                        Skip Selected
                      </button>
                    {/if}
                  </div>
                </div>
              </div>
            {/if}

            <!-- Resolution List -->
            <div class="flex-1">
              {#if hasStorefrontResolutions}
                <div class="divide-y divide-gray-200">
                  {#each storefrontModalState.pendingResolutions as resolution (resolution.import_id)}
                    <StorefrontMappingRow
                      {resolution}
                      selected={storefrontModalState.selectedResolutions.has(resolution.import_id)}
                      onSelectionChange={(selected) => handleStorefrontSelectionChange(resolution.import_id, selected)}
                      onResolutionAction={handleStorefrontResolutionAction}
                    />
                  {/each}
                </div>

                <!-- Pagination -->
                {#if storefrontTotalPages > 1}
                  <div class="px-6 py-4 border-t border-gray-200 bg-gray-50">
                    <div class="flex items-center justify-between">
                      <p class="text-sm text-gray-700">
                        Showing page {storefrontCurrentPage} of {storefrontTotalPages} 
                        ({storefrontTotalResolutions} total storefront resolutions)
                      </p>
                      
                      <div class="flex items-center space-x-2">
                        <button
                          onclick={() => handleStorefrontPageChange(storefrontCurrentPage - 1)}
                          disabled={storefrontCurrentPage <= 1}
                          class="btn-secondary disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          Previous
                        </button>
                        
                        <span class="text-sm text-gray-500">
                          {storefrontCurrentPage} / {storefrontTotalPages}
                        </span>
                        
                        <button
                          onclick={() => handleStorefrontPageChange(storefrontCurrentPage + 1)}
                          disabled={storefrontCurrentPage >= storefrontTotalPages}
                          class="btn-secondary disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          Next
                        </button>
                      </div>
                    </div>
                  </div>
                {/if}
              {:else}
                <div class="text-center py-12">
                  <span class="text-6xl">🎉</span>
                  <h3 class="mt-4 text-lg font-medium text-gray-900">All Storefronts Resolved!</h3>
                  <p class="mt-2 text-sm text-gray-500">
                    All storefront resolutions have been completed.
                  </p>
                </div>
              {/if}
            </div>
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t border-gray-200 bg-gray-50 flex-shrink-0">
        <div class="flex justify-between items-center">
          <div class="text-sm text-gray-600">
            {#if totalResolutions > 0 || storefrontTotalResolutions > 0}
              {#if totalResolutions > 0 && storefrontTotalResolutions > 0}
                {totalResolutions} platform{totalResolutions === 1 ? '' : 's'} and {storefrontTotalResolutions} storefront{storefrontTotalResolutions === 1 ? '' : 's'} need resolution
              {:else if totalResolutions > 0}
                {totalResolutions} platform{totalResolutions === 1 ? '' : 's'} need resolution
              {:else}
                {storefrontTotalResolutions} storefront{storefrontTotalResolutions === 1 ? '' : 's'} need resolution
              {/if}
            {:else}
              All platforms and storefronts resolved
            {/if}
          </div>
          
          <div class="flex space-x-3">
            <button onclick={handleClose} class="btn-secondary">
              {totalResolutions > 0 || storefrontTotalResolutions > 0 ? 'Close & Skip Remaining' : 'Close'}
            </button>
            
            {#if totalResolutions === 0 && storefrontTotalResolutions === 0}
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