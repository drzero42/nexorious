<script lang="ts">
  import type { DarkadiaImportJob } from '$lib/types/darkadia';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onCancel?: () => void;
    isCancelling?: boolean;
    importJob: DarkadiaImportJob | null;
  }

  let { isOpen = false, onClose, onCancel, isCancelling = false, importJob }: Props = $props();

  // Derived values from import job
  const progress = $derived(importJob?.progress || 0);
  const processedItems = $derived(importJob?.processed_items || 0);
  const totalItems = $derived(importJob?.total_items || 0);
  const successfulItems = $derived(importJob?.successful_items || 0);
  const failedItems = $derived(importJob?.failed_items || 0);
  const status = $derived(importJob?.status || 'pending');
  const errorMessage = $derived(importJob?.error_message || '');
  const isComplete = $derived(status === 'completed' || status === 'failed' || status === 'cancelled');
  const isProcessing = $derived(status === 'processing');

  // Display strings based on status
  const statusDisplay = $derived.by(() => {
    switch (status) {
      case 'pending':
        return 'Preparing import...';
      case 'processing':
        return 'Processing import...';
      case 'completed':
        return 'Import completed successfully!';
      case 'failed':
        return 'Import failed';
      case 'cancelled':
        return 'Import cancelled';
      default:
        return 'Unknown status';
    }
  });

  const statusIcon = $derived.by(() => {
    switch (status) {
      case 'pending':
        return '⏳';
      case 'processing':
        return '🔄';
      case 'completed':
        return '✅';
      case 'failed':
        return '❌';
      case 'cancelled':
        return '⏹️';
      default:
        return '❓';
    }
  });

  // Handle cancel operation
  async function handleCancel() {
    if (!importJob?.id) return;
    
    const confirmed = confirm(
      `Are you sure you want to cancel the import operation? ` +
      `${processedItems} items have already been processed.`
    );
    
    if (!confirmed) return;

    try {
      await onCancel?.();
      onClose();
    } catch (error) {
      // Error handled in parent
    }
  }

  // Simple close handler
  function handleClose() {
    onClose();
  }
</script>

{#if isOpen && importJob}
  <!-- Modal backdrop -->
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
    <div class="max-w-md w-full bg-white rounded-lg shadow-xl">
      <!-- Header -->
      <div class="px-6 py-4 border-b border-gray-200">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900 flex items-center">
            <span class="text-xl mr-2">📥</span>
            Importing Games
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
            <span>{Math.round(progress)}%</span>
          </div>
          <div class="w-full bg-gray-200 rounded-full h-2">
            <div 
              class="bg-blue-600 h-2 rounded-full transition-all duration-300 ease-out"
              class:bg-green-600={isComplete && status === 'completed'}
              class:bg-yellow-600={status === 'cancelled'}
              class:bg-red-600={status === 'failed'}
              style="width: {progress}%"
            ></div>
          </div>
        </div>

        <!-- Status Text -->
        <div class="text-center mb-4">
          {#if isComplete}
            <div class="font-medium"
                 class:text-green-600={status === 'completed'}
                 class:text-yellow-600={status === 'cancelled'}
                 class:text-red-600={status === 'failed'}>
              {statusIcon} {statusDisplay}
            </div>
          {:else if isCancelling}
            <div class="text-yellow-600 font-medium flex items-center justify-center">
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Cancelling import...
            </div>
          {:else if isProcessing}
            <div class="text-blue-600 font-medium flex items-center justify-center">
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              {statusDisplay}
            </div>
          {:else}
            <div class="text-gray-600">
              {statusDisplay}
            </div>
          {/if}
        </div>

        <!-- Progress Stats -->
        {#if totalItems > 0}
          <div class="grid grid-cols-2 gap-4 mb-4">
            <div class="text-center">
              <div class="text-2xl font-bold text-gray-900">{processedItems}</div>
              <div class="text-sm text-gray-500">of {totalItems} processed</div>
            </div>
            <div class="text-center">
              <div class="text-2xl font-bold text-gray-900">{Math.max(0, totalItems - processedItems)}</div>
              <div class="text-sm text-gray-500">remaining</div>
            </div>
          </div>
        {/if}

        <!-- Success/Failure Stats -->
        {#if processedItems > 0 && isComplete}
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

        <!-- Error Message -->
        {#if errorMessage}
          <div class="mb-4">
            <h4 class="text-sm font-medium text-red-600 mb-2">Error:</h4>
            <div class="bg-red-50 border border-red-200 rounded p-2">
              <div class="text-sm text-red-800">{errorMessage}</div>
            </div>
          </div>
        {/if}

        <!-- Completion Summary -->
        {#if isComplete}
          <div class="bg-gray-50 rounded-lg p-4 mb-4 border-l-4" 
               class:border-green-500={status === 'completed'}
               class:border-yellow-500={status === 'cancelled'}
               class:border-red-500={status === 'failed'}>
            <div class="flex items-start">
              <div class="flex-shrink-0">
                <span class="text-2xl">
                  {#if status === 'completed'}
                    ✅
                  {:else if status === 'cancelled'}
                    ⏹️
                  {:else}
                    ❌
                  {/if}
                </span>
              </div>
              <div class="ml-3 flex-1">
                <h4 class="font-medium"
                    class:text-green-800={status === 'completed'}
                    class:text-yellow-800={status === 'cancelled'}
                    class:text-red-800={status === 'failed'}>
                  {#if status === 'completed'}
                    Import Completed Successfully!
                  {:else if status === 'cancelled'}
                    Import Cancelled
                  {:else}
                    Import Failed
                  {/if}
                </h4>
                <div class="text-sm mt-1"
                     class:text-green-600={status === 'completed'}
                     class:text-yellow-600={status === 'cancelled'}
                     class:text-red-600={status === 'failed'}>
                  {#if status === 'completed'}
                    Successfully imported {successfulItems} games
                    {#if failedItems > 0}
                      ({failedItems} failed)
                    {/if}
                  {:else if status === 'cancelled'}
                    Import was cancelled after processing {processedItems} games
                  {:else if status === 'failed'}
                    Import failed after processing {processedItems} games
                  {/if}
                </div>
              </div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 bg-gray-50 rounded-b-lg">
        <div class="flex justify-between items-center">
          {#if isComplete}
            <div class="text-sm text-gray-500">
              {#if status === 'completed' && !errorMessage}
                Import completed successfully
              {:else if status === 'cancelled'}
                Import was cancelled
              {:else if status === 'failed'}
                Import failed - check the error details above
              {/if}
            </div>
            <button
              onclick={handleClose}
              class="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors font-medium"
              class:bg-green-600={status === 'completed'}
              class:hover:bg-green-700={status === 'completed'}
              class:focus:ring-green-500={status === 'completed'}
            >
              {#if status === 'completed'}
                ✓ Done
              {:else}
                Close
              {/if}
            </button>
          {:else}
            <button
              onclick={handleCancel}
              disabled={isCancelling}
              class="px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isCancelling}
                Cancelling...
              {:else}
                Cancel Import
              {/if}
            </button>
            
            <div class="text-sm text-gray-500">
              {#if isCancelling}
                <span class="flex items-center">
                  <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  Stopping import...
                </span>
              {:else}
                Import in progress...
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