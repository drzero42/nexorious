<script lang="ts">
  import { steamGames } from '$lib/stores/steam-games.svelte';
  import { darkadia } from '$lib/stores/darkadia.svelte';
  
  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onCancel?: () => void;
    isCancelling?: boolean;
    store?: 'steam' | 'darkadia';
  }

  let { isOpen = false, onClose, onCancel, isCancelling = false, store = 'steam' }: Props = $props();

  // Get batch session from the appropriate store
  const batchSession = $derived(
    store === 'darkadia' 
      ? darkadia.value.activeBatchSession 
      : steamGames.value.activeBatchSession
  );
  const isProcessing = $derived(batchSession?.isProcessing || false);

  // Reactive values for progress display
  const progressPercentage = $derived(batchSession?.progressPercentage || 0);
  const processedItems = $derived(batchSession?.processedItems || 0);
  const totalItems = $derived(batchSession?.totalItems || 0);
  const successfulItems = $derived(batchSession?.successfulItems || 0);
  const failedItems = $derived(batchSession?.failedItems || 0);
  const remainingItems = $derived(batchSession?.remainingItems || 0);
  const operationType = $derived(batchSession?.operationType || 'auto_match');
  const isComplete = $derived(batchSession?.isComplete || false);
  const status = $derived(batchSession?.status || '');
  const errors = $derived(batchSession?.errors || []);

  // Display strings based on operation type
  const operationDisplayName = $derived(
    operationType === 'auto_match' ? 'Auto-Matching' : 'Syncing'
  );
  const actionVerb = $derived(
    operationType === 'auto_match' ? 'matched' : 'synced'
  );

  // Handle cancel operation
  async function handleCancel() {
    if (!batchSession?.sessionId) return;
    
    const confirmed = confirm(
      `Are you sure you want to cancel the ${operationDisplayName.toLowerCase()} operation? ` +
      `${processedItems} items have already been processed.`
    );
    
    if (!confirmed) return;

    try {
      if (store === 'darkadia') {
        await darkadia.cancelBatchSession(batchSession.sessionId);
      } else {
        await steamGames.cancelBatchOperation(batchSession.sessionId);
      }
      onCancel?.();
      onClose();
    } catch (error) {
      // Error handled in store
    }
  }

  // Handle close when complete
  function handleClose() {
    if (isComplete) {
      if (store === 'darkadia') {
        darkadia.clearBatchSession();
      } else {
        steamGames.clearBatchSession();
      }
    }
    onClose();
  }

  // Auto-close modal when operation is cancelled or completed and no errors
  $effect(() => {
    if (isComplete && status === 'completed' && errors.length === 0) {
      // Auto-close after a short delay to show completion
      setTimeout(() => {
        if (isOpen) {
          handleClose();
        }
      }, 2000);
    }
  });
</script>

{#if isOpen && batchSession}
  <!-- Modal backdrop -->
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
    <div class="max-w-md w-full bg-white rounded-lg shadow-xl">
      <!-- Header -->
      <div class="px-6 py-4 border-b border-gray-200">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900 flex items-center">
            {#if operationType === 'auto_match'}
              <span class="text-xl mr-2">🔍</span>
            {:else}
              <span class="text-xl mr-2">🔄</span>
            {/if}
            {operationDisplayName} Games
          </h2>
          
          {#if isComplete}
            <button
              onclick={handleClose}
              class="text-gray-400 hover:text-gray-600 transition-colors"
              aria-label="Close modal"
            >
              <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          {/if}
        </div>
      </div>

      <!-- Content -->
      <div class="px-6 py-4">
        <!-- Progress Bar -->
        <div class="mb-4">
          <div class="flex justify-between text-sm text-gray-600 mb-2">
            <span>Progress</span>
            <span>{Math.round(progressPercentage)}%</span>
          </div>
          <div class="w-full bg-gray-200 rounded-full h-2">
            <div 
              class="bg-blue-600 h-2 rounded-full transition-all duration-300 ease-out"
              class:bg-green-600={isComplete && status === 'completed'}
              class:bg-yellow-600={status === 'cancelled'}
              class:bg-red-600={status === 'failed'}
              style="width: {progressPercentage}%"
            ></div>
          </div>
        </div>

        <!-- Status Text -->
        <div class="text-center mb-4">
          {#if isComplete}
            {#if status === 'completed'}
              <div class="text-green-600 font-medium">
                ✅ {operationDisplayName} completed successfully!
              </div>
            {:else if status === 'cancelled'}
              <div class="text-yellow-600 font-medium">
                ⏹️ Operation cancelled
              </div>
            {:else if status === 'failed'}
              <div class="text-red-600 font-medium">
                ❌ Operation failed
              </div>
            {/if}
          {:else if isCancelling}
            <div class="text-yellow-600 font-medium flex items-center justify-center">
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Cancelling operation...
            </div>
          {:else if isProcessing}
            <div class="text-blue-600 font-medium flex items-center justify-center">
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Processing batch...
            </div>
          {:else}
            <div class="text-gray-600">
              Ready to process next batch
            </div>
          {/if}
        </div>

        <!-- Progress Stats -->
        <div class="grid grid-cols-2 gap-4 mb-4">
          <div class="text-center">
            <div class="text-2xl font-bold text-gray-900">{processedItems}</div>
            <div class="text-sm text-gray-500">of {totalItems} processed</div>
          </div>
          <div class="text-center">
            <div class="text-2xl font-bold text-gray-900">{remainingItems}</div>
            <div class="text-sm text-gray-500">remaining</div>
          </div>
        </div>

        <!-- Success/Failure Stats -->
        {#if processedItems > 0}
          <div class="grid grid-cols-2 gap-4 mb-4">
            <div class="text-center">
              <div class="text-lg font-semibold text-green-600">{successfulItems}</div>
              <div class="text-sm text-gray-500">successful</div>
            </div>
            <div class="text-center">
              <div class="text-lg font-semibold text-red-600">{failedItems}</div>
              <div class="text-sm text-gray-500">failed</div>
            </div>
          </div>
        {/if}

        <!-- Errors -->
        {#if errors.length > 0}
          <div class="mb-4">
            <h4 class="text-sm font-medium text-red-600 mb-2">Errors ({errors.length}):</h4>
            <div class="max-h-32 overflow-y-auto bg-red-50 border border-red-200 rounded p-2">
              {#each errors as error}
                <div class="text-xs text-red-800 mb-1">{error}</div>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Status Message -->
        {#if isComplete}
          <div class="text-sm text-gray-600 text-center mb-4">
            {#if status === 'completed'}
              Successfully {actionVerb} {successfulItems} games
              {#if failedItems > 0}
                ({failedItems} failed)
              {/if}
            {:else if status === 'cancelled'}
              Operation was cancelled after processing {processedItems} games
            {:else if status === 'failed'}
              Operation failed after processing {processedItems} games
            {/if}
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 bg-gray-50 rounded-b-lg">
        <div class="flex justify-between">
          {#if isComplete}
            <div class="flex-1"></div>
            <button
              onclick={handleClose}
              class="btn-primary"
            >
              Close
            </button>
          {:else}
            <button
              onclick={handleCancel}
              disabled={isCancelling}
              class="btn-secondary disabled:opacity-50"
            >
              {#if isCancelling}
                Cancelling...
              {:else}
                Cancel
              {/if}
            </button>
            
            <div class="text-sm text-gray-500">
              {#if isCancelling}
                Stopping operation...
              {:else}
                Click cancel to stop the operation
              {/if}
            </div>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  /* Ensure modal appears above other content */
  :global(.fixed.inset-0) {
    z-index: 9999;
  }
</style>