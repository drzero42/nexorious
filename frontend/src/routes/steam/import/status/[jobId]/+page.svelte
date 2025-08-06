<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import { ImportStatusProgress } from '$lib/components/steam';
  import { steamImport } from '$lib/stores/steam-import.svelte';

  // Get job ID from route parameters
  const jobId = $page.params.jobId!;

  let isLoading = true;
  let error: string | null = null;
  let processingTimeout: NodeJS.Timeout | null = null;

  onMount(async () => {
    try {
      // Initialize the import job monitoring
      await steamImport.connectToJob(jobId);
      isLoading = false;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load import job';
      isLoading = false;
    }
  });

  onDestroy(() => {
    // Clean up WebSocket connection and timeout
    steamImport.disconnect();
    if (processingTimeout) {
      clearTimeout(processingTimeout);
      processingTimeout = null;
    }
  });

  // Handle job status changes and processing completion
  $: {
    const job = steamImport.value.currentJob;
    if (job) {
      // Handle status-based navigation (existing logic)
      if (job.status === 'awaiting_review') {
        // Navigate to review page when job reaches review phase
        goto(`/steam/import/review/${jobId}`);
      } else if (job.status === 'finalizing') {
        // Navigate to confirmation page when ready for final import
        goto(`/steam/import/confirm/${jobId}`);
      } else if (job.status === 'completed') {
        // Navigate to results page when completed
        goto(`/steam/import/results/${jobId}`);
      } else if (job.status === 'failed') {
        error = job.error_message || 'Import job failed';
      }
      
      // Handle processing complete but status not updated yet
      else if (job.status === 'processing' && 
               job.total_games > 0 && 
               job.processed_games === job.total_games) {
        console.log('Processing complete, determining next navigation...', {
          awaiting_review_games: job.awaiting_review_games,
          matched_games: job.matched_games
        });
        
        // Clear any existing timeout
        if (processingTimeout) {
          clearTimeout(processingTimeout);
        }
        
        // Set timeout for fallback status refresh if navigation doesn't happen
        processingTimeout = setTimeout(async () => {
          console.log('Timeout reached, refreshing job status as fallback...');
          try {
            await steamImport.fetchJobStatus(jobId);
          } catch (error) {
            console.error('Error refreshing job status:', error);
          }
        }, 3000); // 3 second timeout
        
        // All games processed, navigate based on results
        if (job.awaiting_review_games > 0) {
          // Has games needing review, navigate to review
          console.log('Navigating to review page due to games awaiting review');
          // Clear timeout since we're navigating
          if (processingTimeout) {
            clearTimeout(processingTimeout);
            processingTimeout = null;
          }
          goto(`/steam/import/review/${jobId}`);
        } else if (job.matched_games > 0) {
          // All matched, navigate to confirm
          console.log('Navigating to confirm page due to all games matched');
          // Clear timeout since we're navigating
          if (processingTimeout) {
            clearTimeout(processingTimeout);
            processingTimeout = null;
          }
          goto(`/steam/import/confirm/${jobId}`);
        }
      }
      
      // Clear timeout if status is no longer processing
      else if (job.status !== 'processing' && processingTimeout) {
        clearTimeout(processingTimeout);
        processingTimeout = null;
      }
    }
  }

  // Manual navigation function
  function handleGoToReview() {
    goto(`/steam/import/review/${jobId}`);
  }

  // Check if review button should be shown
  $: showReviewButton = (() => {
    const job = steamImport.value.currentJob;
    if (!job) return false;
    
    // Show button if status is awaiting_review
    if (job.status === 'awaiting_review') return true;
    
    // Show button if processing is complete and there are games awaiting review
    if (job.status === 'processing' && 
        job.total_games > 0 && 
        job.processed_games === job.total_games && 
        job.awaiting_review_games > 0) {
      return true;
    }
    
    return false;
  })();

</script>

<svelte:head>
  <title>Steam Import Status - Nexorious</title>
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
              <span class="text-gray-900 font-medium">Import Status</span>
            </li>
          </ol>
        </nav>
        
        <div class="flex items-center justify-between">
          <div>
            <h1 class="text-3xl font-bold text-gray-900">Steam Library Import</h1>
            <p class="mt-2 text-gray-600">Importing your Steam library in the background</p>
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
              Loading...
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
            <div class="h-4 bg-gray-200 rounded w-1/2"></div>
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
              <h3 class="text-lg font-medium text-red-800">Import Failed</h3>
              <p class="text-red-700 mt-1">{error}</p>
            </div>
          </div>
          <div class="mt-4">
            <a href="/settings/steam" class="btn-secondary">
              Back to Steam Settings
            </a>
          </div>
        </div>
      {:else}
        <!-- Import Progress -->
        <div class="space-y-6">
          <!-- Main Progress Card -->
          <ImportStatusProgress />
          
          <!-- Action Buttons -->
          <div class="flex justify-between items-center">
            {#if showReviewButton}
              <button
                on:click={handleGoToReview}
                class="btn-primary"
              >
                <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
                Continue to Review
              </button>
            {:else}
              <div></div>
            {/if}
            
            <div class="text-sm text-gray-500">
              Job ID: {jobId}
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
</RouteGuard>