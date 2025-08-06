<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import { GameReviewCard } from '$lib/components/steam';
  import { steamImport } from '$lib/stores/steam-import.svelte';
  import { ui } from '$lib/stores';

  // Get job ID from route parameters
  const jobId = $page.params.jobId!;

  let isLoading = true;
  let error: string | null = null;
  let isSubmitting = false;
  let isCancellingImport = false;

  onMount(async () => {
    try {
      // Initialize the import job monitoring if not already polling
      if (!steamImport.value.isPolling) {
        await steamImport.connectToJob(jobId);
      }
      isLoading = false;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load import job';
      isLoading = false;
    }
  });

  onDestroy(() => {
    // Keep WebSocket connection for navigation between pages
  });

  // Handle job status changes - Svelte 5 effect
  $effect(() => {
    const job = steamImport.value.currentJob;
    
    if (job) {
      if (job.status === 'finalizing') {
        goto(`/steam/import/confirm/${jobId}`);
      } else if (job.status === 'completed') {
        goto(`/steam/import/results/${jobId}`);
      } else if (job.status === 'failed') {
        error = job.error_message || 'Import job failed';
      }
    }
  });

  // Get games awaiting review - Svelte 5 derived
  const awaitingReviewGames = $derived(steamImport.value.currentJob?.games?.filter(
    game => game.status === 'awaiting_user'
  ) || []);

  const reviewedGames = $derived(steamImport.value.userDecisions);
  const totalReviewGames = $derived(awaitingReviewGames.length);
  const completedReviews = $derived(Object.keys(reviewedGames).length);
  const allReviewsComplete = $derived(completedReviews === totalReviewGames && totalReviewGames > 0);

  // Auto-submit when all reviews are complete (optional, can be disabled for explicit user control)
  let autoSubmitEnabled = false; // Set to true to enable auto-submission
  let hasAutoSubmitted = false;

  $effect(() => {
    if (autoSubmitEnabled && allReviewsComplete && !isSubmitting && !hasAutoSubmitted) {
      hasAutoSubmitted = true;
      handleSubmitDecisions();
    } else if (!allReviewsComplete) {
      hasAutoSubmitted = false;
    }
  });

  async function handleSubmitDecisions() {
    if (!allReviewsComplete) {
      ui.showError('Please review all games before proceeding');
      return;
    }

    isSubmitting = true;
    try {
      await steamImport.submitUserDecisions(jobId, reviewedGames);
      ui.showSuccess('Game reviews submitted successfully');
      // Job status will change and trigger navigation via reactive statement
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to submit decisions';
      ui.showError(error);
    } finally {
      isSubmitting = false;
    }
  }

  async function handleCancelImport() {
    const confirmResult = confirm('Are you sure you want to cancel the Steam import? This action cannot be undone.');
    
    if (!confirmResult) {
      return;
    }

    isCancellingImport = true;
    
    try {
      await steamImport.cancelJob(jobId);
      ui.showSuccess('Steam import cancelled successfully');
      
      // Navigate back to steam settings
      goto('/settings/steam');
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to cancel import';
      ui.showError(`Failed to cancel import: ${errorMessage}`);
    } finally {
      isCancellingImport = false;
    }
  }

  async function handleSkipAll() {
    if (confirm('Are you sure you want to skip all remaining games? They will not be imported.')) {
      isSubmitting = true;
      try {
        // Skip all remaining games
        awaitingReviewGames.forEach((game) => {
          const steamAppIdStr = game.steam_appid.toString();
          
          if (!reviewedGames[steamAppIdStr]) {
            const decision = {
              action: 'skip' as const,
              notes: 'Skipped via skip all'
            };
            
            steamImport.setUserDecision(steamAppIdStr, decision);
          }
        });
        
        // Wait a tick to ensure all state updates are processed
        await new Promise(resolve => setTimeout(resolve, 0));
        
        // Auto-submit the skip decisions
        const decisionsToSubmit = steamImport.value.userDecisions;
        
        // Validate decisions object
        if (Object.keys(decisionsToSubmit).length === 0) {
          throw new Error('No decisions were set - this indicates a state synchronization issue');
        }
        
        await steamImport.submitUserDecisions(jobId, decisionsToSubmit);
        
        // Immediately check job status after submission
        try {
          await steamImport.fetchJobStatus(jobId);
        } catch (statusError) {
          console.error('Failed to fetch status after Skip All:', statusError);
        }
        
        ui.showSuccess('All remaining games skipped successfully');
        // Job status will change and trigger navigation via reactive statement
      } catch (err) {
        error = err instanceof Error ? err.message : 'Failed to skip games';
        ui.showError(error);
      } finally {
        isSubmitting = false;
      }
    }
  }

</script>

<svelte:head>
  <title>Review Steam Games - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="min-h-screen bg-gray-50 py-8">
    <div class="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
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
              <span class="text-gray-900 font-medium">Review Games</span>
            </li>
          </ol>
        </nav>
        
        <div class="flex items-center justify-between">
          <div>
            <h1 class="text-3xl font-bold text-gray-900">Review Unmatched Games</h1>
            <p class="mt-2 text-gray-600">
              Help us match these Steam games to our database or choose to skip them
            </p>
          </div>
          
          <div class="flex items-center space-x-4">
            <!-- Cancel Import Button -->
            <button
              on:click={handleCancelImport}
              disabled={isCancellingImport || isSubmitting}
              class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isCancellingImport}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Cancelling...
              {:else}
                <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
                Cancel Import
              {/if}
            </button>
            
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

        <!-- Progress Indicator -->
        <div class="mt-6 bg-white rounded-lg shadow p-6">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-gray-900">Review Progress</h2>
            <span class="text-sm text-gray-500">
              {completedReviews} of {totalReviewGames} games reviewed
            </span>
          </div>
          
          <div class="w-full bg-gray-200 rounded-full h-3">
            <div 
              class="bg-blue-500 h-3 rounded-full transition-all duration-300"
              style="width: {totalReviewGames > 0 ? (completedReviews / totalReviewGames) * 100 : 0}%"
            ></div>
          </div>
          
          {#if allReviewsComplete}
            <div class="mt-4 p-3 bg-green-50 border border-green-200 rounded-md">
              <div class="flex items-center">
                <svg class="h-5 w-5 text-green-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                </svg>
                <span class="text-sm font-medium text-green-800">All games reviewed! Ready to proceed.</span>
              </div>
            </div>
          {/if}
        </div>
      </div>

      {#if isLoading}
        <!-- Loading State -->
        <div class="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {#each Array(6) as _}
            <div class="card animate-pulse">
              <div class="h-32 bg-gray-200 rounded mb-4"></div>
              <div class="h-4 bg-gray-200 rounded w-3/4 mb-2"></div>
              <div class="h-4 bg-gray-200 rounded w-1/2"></div>
            </div>
          {/each}
        </div>
      {:else if error}
        <!-- Error State -->
        <div class="card bg-red-50 border-red-200">
          <div class="flex items-center">
            <svg class="h-6 w-6 text-red-400 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <div>
              <h3 class="text-lg font-medium text-red-800">Review Failed</h3>
              <p class="text-red-700 mt-1">{error}</p>
            </div>
          </div>
        </div>
      {:else if awaitingReviewGames.length === 0}
        <!-- No Games to Review -->
        <div class="card text-center py-12">
          <svg class="mx-auto h-12 w-12 text-gray-400 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <h3 class="text-lg font-medium text-gray-900 mb-2">All Games Matched!</h3>
          <p class="text-gray-600 mb-6">All your Steam games were automatically matched. No manual review needed.</p>
          <button
            on:click={() => goto(`/steam/import/confirm/${jobId}`)}
            class="btn-primary"
          >
            Proceed to Confirmation
          </button>
        </div>
      {:else}
        <!-- Game Review Grid -->
        <div class="space-y-6">
          <!-- Bulk Actions -->
          <div class="bg-white rounded-lg shadow p-4">
            <div class="flex items-center justify-between">
              <div class="text-sm text-gray-600">
                Bulk actions for remaining {totalReviewGames - completedReviews} games
              </div>
              <button
                on:click={handleSkipAll}
                class="btn-secondary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
                disabled={completedReviews === totalReviewGames || isSubmitting}
              >
                {#if isSubmitting}
                  <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  Skipping...
                {:else}
                  Skip All Remaining
                {/if}
              </button>
            </div>
          </div>

          <!-- Games Grid -->
          <div class="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
            {#each awaitingReviewGames as game (game.steam_appid)}
              <GameReviewCard {game} />
            {/each}
          </div>

          <!-- Action Buttons -->
          <div class="flex justify-between items-center pt-6 border-t">
            <button
              on:click={() => goto(`/steam/import/status/${jobId}`)}
              class="btn-secondary"
            >
              ← Back to Status
            </button>
            
            <button
              on:click={handleSubmitDecisions}
              disabled={!allReviewsComplete || isSubmitting}
              class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isSubmitting}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Submitting...
              {:else}
                Submit Reviews →
              {/if}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
</RouteGuard>