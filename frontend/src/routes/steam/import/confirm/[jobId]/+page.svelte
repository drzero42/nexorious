<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import { ImportSummary } from '$lib/components/steam';
  import { steamImport } from '$lib/stores/steam-import.svelte';
  import { ui } from '$lib/stores';

  // Get job ID from route parameters
  const jobId = $page.params.jobId!;

  let isLoading = true;
  let error: string | null = null;
  let isConfirming = false;

  onMount(async () => {
    try {
      // Initialize the import job monitoring if not already polling
      if (!steamImport.value.isPolling) {
        await steamImport.connectToJob(jobId);
      }
      
      // Ensure we're in the correct state
      const job = steamImport.value.currentJob;
      if (!job) {
        throw new Error('Import job not found');
      }
      
      if (job.status !== 'finalizing') {
        // Redirect to appropriate page based on status
        if (job.status === 'processing') {
          goto(`/steam/import/status/${jobId}`);
          return;
        } else if (job.status === 'awaiting_review') {
          goto(`/steam/import/review/${jobId}`);
          return;
        } else if (job.status === 'completed') {
          goto(`/steam/import/results/${jobId}`);
          return;
        }
      }
      
      isLoading = false;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load import job';
      isLoading = false;
    }
  });

  onDestroy(() => {
    // Keep polling active for navigation between pages
  });

  // Handle job status changes
  $: {
    const job = steamImport.value.currentJob;
    if (job && job.status === 'completed') {
      goto(`/steam/import/results/${jobId}`);
    } else if (job && job.status === 'failed') {
      error = job.error_message || 'Import job failed';
    }
  }

  async function handleConfirmImport() {
    isConfirming = true;
    try {
      await steamImport.confirmFinalImport(jobId);
      ui.showSuccess('Import confirmed! Processing final import...');
      // Job status will change and trigger navigation via reactive statement
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to confirm import';
      ui.showError(error);
    } finally {
      isConfirming = false;
    }
  }

  function handleGoBack() {
    // Check if there were games that needed review
    const job = steamImport.value.currentJob;
    const hadReviewGames = job?.games?.some(game => game.status === 'awaiting_user') || false;
    
    if (hadReviewGames) {
      goto(`/steam/import/review/${jobId}`);
    } else {
      goto(`/steam/import/status/${jobId}`);
    }
  }
</script>

<svelte:head>
  <title>Confirm Steam Import - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="min-h-screen bg-gray-50 py-8">
    <div class="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
      <!-- Header -->
      <div class="mb-8">
        <nav class="flex text-sm text-gray-500 mb-4" aria-label="Breadcrumb">
          <ol class="inline-flex items-center space-x-1 md:space-x-3">
            <li>
              <a href="/settings/steam" class="hover:text-gray-700">Steam Settings</a>
            </li>
            <li>
              <span>›</span>
            </li>
            <li>
              <a href={`/steam/import/status/${jobId}`} class="hover:text-gray-700">Import Status</a>
            </li>
            <li>
              <span>›</span>
            </li>
            <li>
              <span class="text-gray-900 font-medium">Confirm Import</span>
            </li>
          </ol>
        </nav>
        
        <div class="flex items-center justify-between">
          <div>
            <h1 class="text-3xl font-bold text-gray-900">Confirm Steam Import</h1>
            <p class="mt-2 text-gray-600">
              Review the final import summary and confirm to add games to your collection
            </p>
          </div>
          
          <!-- Import Status Indicator -->
          <div class="text-sm text-gray-500">
            {#if steamImport.value.lastUpdated}
              Last updated: {new Intl.DateTimeFormat('en-US', { 
                hour: '2-digit', 
                minute: '2-digit', 
                second: '2-digit' 
              }).format(steamImport.value.lastUpdated)}
            {:else}
              Syncing...
            {/if}
          </div>
        </div>
      </div>

      {#if isLoading}
        <!-- Loading State -->
        <div class="card">
          <div class="animate-pulse">
            <div class="h-4 bg-gray-200 rounded w-3/4 mb-4"></div>
            <div class="h-8 bg-gray-200 rounded mb-4"></div>
            <div class="h-4 bg-gray-200 rounded w-1/2 mb-8"></div>
            <div class="h-10 bg-gray-200 rounded w-32"></div>
          </div>
        </div>
      {:else if error}
        <!-- Error State -->
        <div class="card bg-red-50 border-red-200">
          <div class="flex items-center">
            <svg class="h-6 w-6 text-red-400 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <div>
              <h3 class="text-lg font-medium text-red-800">Confirmation Failed</h3>
              <p class="text-red-700 mt-1">{error}</p>
            </div>
          </div>
          <div class="mt-4">
            <button on:click={handleGoBack} class="btn-secondary">
              Go Back
            </button>
          </div>
        </div>
      {:else}
        <!-- Import Summary and Confirmation -->
        <div class="space-y-6">
          <!-- Summary Card -->
          <ImportSummary />
          
          <!-- Final Confirmation -->
          <div class="card bg-blue-50 border-blue-200">
            <div class="flex items-start">
              <svg class="h-6 w-6 text-blue-400 mr-3 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <div class="flex-1">
                <h3 class="text-lg font-medium text-blue-800 mb-2">Ready to Import</h3>
                <p class="text-blue-700 mb-4">
                  This will add the selected games to your collection and associate them with the Steam platform. 
                  This action cannot be undone, but you can always remove games from your collection later.
                </p>
                <div class="bg-blue-100 rounded-md p-3">
                  <p class="text-sm text-blue-800">
                    <strong>What happens next:</strong>
                  </p>
                  <ul class="text-sm text-blue-700 mt-1 list-disc list-inside space-y-1">
                    <li>New games will be imported from IGDB with full metadata</li>
                    <li>Steam platform will be added to existing games in your collection</li>
                    <li>Cover art will be downloaded and stored locally</li>
                    <li>Import statistics will be calculated and displayed</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>

          <!-- Action Buttons -->
          <div class="flex justify-between items-center pt-6 border-t">
            <button
              on:click={handleGoBack}
              class="btn-secondary"
              disabled={isConfirming}
            >
              ← Back to Review
            </button>
            
            <button
              on:click={handleConfirmImport}
              disabled={isConfirming}
              class="btn-primary bg-green-600 hover:bg-green-700 focus:ring-green-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isConfirming}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Confirming Import...
              {:else}
                <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                </svg>
                Confirm Import
              {/if}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
</RouteGuard>