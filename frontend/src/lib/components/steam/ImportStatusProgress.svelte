<script lang="ts">
  import { ProgressBar } from '$lib/components';
  import { steamImport } from '$lib/stores/steam-import.svelte';

  // Get current job data
  $: job = steamImport.value.currentJob;
  $: isProcessing = job?.status === 'processing';
  
  // Calculate estimated time remaining (simple linear estimation)
  $: {
    if (job && isProcessing && job.processed_games > 0) {
      const avgTimePerGame = (Date.now() - new Date(job.created_at).getTime()) / job.processed_games;
      const remainingGames = job.total_games - job.processed_games;
      estimatedTimeRemaining = Math.ceil((avgTimePerGame * remainingGames) / 1000 / 60); // minutes
    } else {
      estimatedTimeRemaining = null;
    }
  }
  
  let estimatedTimeRemaining: number | null = null;

  // Format time remaining
  function formatTimeRemaining(minutes: number | null): string {
    if (!minutes || minutes <= 0) return '';
    if (minutes < 1) return 'Less than 1 minute';
    if (minutes === 1) return '1 minute';
    if (minutes < 60) return `${Math.ceil(minutes)} minutes`;
    const hours = Math.floor(minutes / 60);
    const mins = Math.ceil(minutes % 60);
    if (hours === 1) return `1 hour ${mins > 0 ? `${mins} minutes` : ''}`;
    return `${hours} hours ${mins > 0 ? `${mins} minutes` : ''}`;
  }

  // Get phase description
  function getPhaseDescription(status: string): string {
    switch (status) {
      case 'pending':
        return 'Preparing to start import...';
      case 'processing':
        return 'Retrieving Steam library and matching games...';
      case 'awaiting_review':
        return 'Waiting for manual review of unmatched games';
      case 'finalizing':
        return 'Ready for final import confirmation';
      case 'completed':
        return 'Import completed successfully!';
      case 'failed':
        return 'Import failed';
      default:
        return 'Unknown status';
    }
  }

  // Get phase indicator steps
  $: phaseSteps = [
    { key: 'processing', label: 'Processing', completed: job?.status !== 'pending' },
    { key: 'review', label: 'Review', completed: job?.status && ['finalizing', 'completed'].includes(job.status) },
    { key: 'import', label: 'Import', completed: job?.status === 'completed' }
  ];
</script>

<div class="card space-y-6">
  <!-- Phase Indicator -->
  <div>
    <h2 class="text-lg font-semibold text-gray-900 mb-4">Import Progress</h2>
    
    <!-- Phase Steps -->
    <div class="flex items-center justify-between">
      {#each phaseSteps as step, index}
        <div class="flex items-center {index < phaseSteps.length - 1 ? 'flex-1' : ''}">
          <!-- Step Circle -->
          <div class="flex items-center justify-center w-8 h-8 rounded-full border-2 
                      {step.completed 
                        ? 'bg-green-500 border-green-500 text-white' 
                        : job?.status === step.key 
                          ? 'bg-blue-500 border-blue-500 text-white' 
                          : 'bg-gray-200 border-gray-300 text-gray-500'}">
            {#if step.completed}
              <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
              </svg>
            {:else if job?.status === step.key}
              <div class="w-2 h-2 bg-current rounded-full animate-pulse"></div>
            {:else}
              <span class="text-xs font-medium">{index + 1}</span>
            {/if}
          </div>
          
          <!-- Step Label -->
          <span class="ml-2 text-sm font-medium 
                       {step.completed ? 'text-green-600' : 
                         job?.status === step.key ? 'text-blue-600' : 'text-gray-500'}">
            {step.label}
          </span>
          
          <!-- Connector Line -->
          {#if index < phaseSteps.length - 1}
            <div class="flex-1 h-0.5 mx-4 
                        {step.completed ? 'bg-green-500' : 'bg-gray-300'}">
            </div>
          {/if}
        </div>
      {/each}
    </div>
  </div>

  <!-- Current Phase Description -->
  {#if job}
    <div class="bg-blue-50 rounded-lg p-4">
      <div class="flex items-center">
        {#if isProcessing}
          <svg class="animate-spin h-5 w-5 text-blue-500 mr-3" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
        {:else}
          <svg class="h-5 w-5 text-blue-500 mr-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        {/if}
        <div>
          <p class="text-sm font-medium text-blue-800">
            {getPhaseDescription(job.status)}
          </p>
          {#if estimatedTimeRemaining && isProcessing}
            <p class="text-xs text-blue-600 mt-1">
              Estimated time remaining: {formatTimeRemaining(estimatedTimeRemaining)}
            </p>
          {/if}
        </div>
      </div>
    </div>
  {/if}

  <!-- Progress Bars and Statistics -->
  {#if job && job.total_games > 0}
    <div class="space-y-4">
      <!-- Overall Progress -->
      <div>
        <div class="flex justify-between items-center mb-2">
          <span class="text-sm font-medium text-gray-700">Overall Progress</span>
          <span class="text-sm text-gray-600">
            {job.processed_games} of {job.total_games} games
          </span>
        </div>
        <ProgressBar 
          value={job.processed_games} 
          max={job.total_games} 
          color="blue"
          animated={isProcessing}
          showPercentage={true}
        />
      </div>

      <!-- Detailed Statistics -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <!-- Matched Games -->
        <div class="text-center p-3 bg-green-50 rounded-lg">
          <div class="text-xl font-bold text-green-600">{job.matched_games}</div>
          <div class="text-xs text-green-700 font-medium">Auto Matched</div>
        </div>
        
        <!-- Awaiting Review -->
        <div class="text-center p-3 bg-orange-50 rounded-lg">
          <div class="text-xl font-bold text-orange-600">{job.awaiting_review_games}</div>
          <div class="text-xs text-orange-700 font-medium">Need Review</div>
        </div>
        
        <!-- Imported -->
        <div class="text-center p-3 bg-blue-50 rounded-lg">
          <div class="text-xl font-bold text-blue-600">{job.imported_games}</div>
          <div class="text-xs text-blue-700 font-medium">New Games</div>
        </div>
        
        <!-- Platform Added -->
        <div class="text-center p-3 bg-purple-50 rounded-lg">
          <div class="text-xl font-bold text-purple-600">{job.platform_added_games}</div>
          <div class="text-xs text-purple-700 font-medium">Updated</div>
        </div>
      </div>

      <!-- Additional Stats if Available -->
      {#if job.skipped_games > 0}
        <div class="pt-2 border-t border-gray-200">
          <div class="flex justify-center">
            <div class="text-center p-2 bg-gray-50 rounded-lg">
              <div class="text-lg font-bold text-gray-600">{job.skipped_games}</div>
              <div class="text-xs text-gray-700 font-medium">Skipped Games</div>
            </div>
          </div>
        </div>
      {/if}
    </div>
  {:else if job}
    <!-- No games case -->
    <div class="text-center py-8 text-gray-500">
      <svg class="mx-auto h-12 w-12 text-gray-400 mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
      </svg>
      <p>Initializing Steam import...</p>
    </div>
  {/if}
</div>