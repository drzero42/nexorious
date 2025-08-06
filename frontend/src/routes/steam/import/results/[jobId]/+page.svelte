<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import { ImportResults } from '$lib/components/steam';
  import { steamImport } from '$lib/stores/steam-import.svelte';
  import { ui } from '$lib/stores';

  // Get job ID from route parameters
  const jobId = $page.params.jobId!;

  let isLoading = true;
  let error: string | null = null;

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
      
      if (job.status !== 'completed') {
        // Redirect to appropriate page based on status
        if (job.status === 'processing') {
          goto(`/steam/import/status/${jobId}`);
          return;
        } else if (job.status === 'awaiting_review') {
          goto(`/steam/import/review/${jobId}`);
          return;
        } else if (job.status === 'finalizing') {
          goto(`/steam/import/confirm/${jobId}`);
          return;
        } else if (job.status === 'failed') {
          error = job.error_message || 'Import job failed';
        }
      }
      
      isLoading = false;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load import job';
      isLoading = false;
    }
  });

  onDestroy(() => {
    // Clean up polling when leaving results page since import is complete
    steamImport.disconnect();
  });

  function handleViewCollection() {
    goto('/games');
  }

  function handleNewImport() {
    goto('/settings/steam');
  }

  function handleShareResults() {
    const job = steamImport.value.currentJob;
    if (!job) return;

    const shareText = `I just imported ${job.imported_games + job.platform_added_games} games from my Steam library to Nexorious! 🎮\n\n` +
                     `📊 Import Summary:\n` +
                     `• ${job.imported_games} new games added\n` +
                     `• ${job.platform_added_games} existing games updated\n` +
                     `• ${job.skipped_games} games skipped\n` +
                     `• ${job.total_games} total games processed`;

    if (navigator.share) {
      navigator.share({
        title: 'Steam Import Results - Nexorious',
        text: shareText
      });
    } else {
      // Fallback: copy to clipboard
      navigator.clipboard.writeText(shareText).then(() => {
        ui.showSuccess('Import results copied to clipboard!');
      }).catch(() => {
        ui.showError('Failed to copy to clipboard');
      });
    }
  }
</script>

<svelte:head>
  <title>Steam Import Complete - Nexorious</title>
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
              <span class="text-gray-900 font-medium">Import Complete</span>
            </li>
          </ol>
        </nav>
        
        <div class="flex items-center justify-between">
          <div>
            <h1 class="text-3xl font-bold text-gray-900 flex items-center">
              <svg class="h-8 w-8 text-green-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              Steam Import Complete!
            </h1>
            <p class="mt-2 text-gray-600">
              Your Steam library has been successfully imported to your game collection
            </p>
          </div>
          
          <!-- Import Complete - No Status Needed -->
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
              <h3 class="text-lg font-medium text-red-800">Results Error</h3>
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
        <!-- Import Results -->
        <div class="space-y-6">
          <!-- Success Animation & Summary -->
          <div class="card bg-gradient-to-r from-green-50 to-blue-50 border-green-200">
            <div class="text-center py-8">
              <!-- Animated Success Icon -->
              <div class="mx-auto w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mb-4">
                <svg class="h-8 w-8 text-green-500 animate-bounce" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                </svg>
              </div>
              
              <h2 class="text-2xl font-bold text-gray-900 mb-2">
                Import Successful! 🎉
              </h2>
              
              <p class="text-gray-600 mb-6">
                Your Steam library has been processed and added to your game collection
              </p>

              <!-- Quick Stats -->
              <div class="grid grid-cols-2 md:grid-cols-4 gap-4 max-w-2xl mx-auto">
                {#if steamImport.value.currentJob}
                  {@const job = steamImport.value.currentJob}
                  <div class="text-center">
                    <div class="text-2xl font-bold text-blue-600">{job.imported_games}</div>
                    <div class="text-sm text-gray-600">New Games</div>
                  </div>
                  <div class="text-center">
                    <div class="text-2xl font-bold text-green-600">{job.platform_added_games}</div>
                    <div class="text-sm text-gray-600">Updated Games</div>
                  </div>
                  <div class="text-center">
                    <div class="text-2xl font-bold text-orange-600">{job.skipped_games}</div>
                    <div class="text-sm text-gray-600">Skipped</div>
                  </div>
                  <div class="text-center">
                    <div class="text-2xl font-bold text-purple-600">{job.total_games}</div>
                    <div class="text-sm text-gray-600">Total Processed</div>
                  </div>
                {/if}
              </div>
            </div>
          </div>

          <!-- Detailed Results -->
          <ImportResults />
          
          <!-- Action Buttons -->
          <div class="card">
            <div class="flex flex-col sm:flex-row gap-4 items-center justify-center">
              <button
                on:click={handleViewCollection}
                class="btn-primary flex-1 sm:flex-none"
              >
                <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                </svg>
                View My Collection
              </button>
              
              <button
                on:click={handleShareResults}
                class="btn-secondary flex-1 sm:flex-none"
              >
                <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.367 2.684 3 3 0 00-5.367-2.684z" />
                </svg>
                Share Results
              </button>
              
              <button
                on:click={handleNewImport}
                class="btn-secondary flex-1 sm:flex-none"
              >
                <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                Import Again
              </button>
            </div>
          </div>

          <!-- Import Details -->
          <div class="text-center text-sm text-gray-500">
            <p>Import Job ID: {jobId}</p>
            <p>Completed: {steamImport.value.currentJob?.completed_at 
              ? new Date(steamImport.value.currentJob.completed_at).toLocaleString() 
              : 'Unknown'}</p>
          </div>
        </div>
      {/if}
    </div>
  </div>
</RouteGuard>