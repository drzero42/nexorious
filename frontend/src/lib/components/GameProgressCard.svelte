<script lang="ts">
  import { ProgressBar } from '$lib/components';
  import type { UserGame, PlayStatus } from '$lib/stores/user-games.svelte';

  interface Props {
    userGame: UserGame;
    class?: string;
  }

  let { userGame, class: className = '' }: Props = $props();

  // Calculate progress based on play status and hours
  function calculateProgress(): { percentage: number; label: string; color: string } {
    const status = userGame.play_status;
    const hoursPlayed = userGame.hours_played || 0;
    const estimatedHours = userGame.game.estimated_playtime_hours || 
                          userGame.game.howlongtobeat_main || 
                          0;

    // Status-based progress
    const statusProgress: Record<PlayStatus, { min: number; max: number; color: string }> = {
      'not_started': { min: 0, max: 0, color: 'gray' },
      'in_progress': { min: 10, max: 60, color: 'blue' },
      'completed': { min: 70, max: 80, color: 'green' },
      'mastered': { min: 85, max: 95, color: 'purple' },
      'dominated': { min: 100, max: 100, color: 'yellow' },
      'shelved': { min: 20, max: 50, color: 'orange' },
      'dropped': { min: 10, max: 40, color: 'red' },
      'replay': { min: 50, max: 80, color: 'blue' }
    };

    const statusInfo = statusProgress[status];
    let percentage = statusInfo.min;
    let label = '';

    // If we have estimated hours and hours played, calculate actual progress
    if (estimatedHours > 0 && hoursPlayed > 0) {
      const actualPercentage = (hoursPlayed / estimatedHours) * 100;
      
      // For in_progress, use actual percentage within the status range
      if (status === 'in_progress') {
        percentage = Math.min(actualPercentage, statusInfo.max);
      } else if (status === 'completed' || status === 'mastered' || status === 'dominated') {
        // For completed states, show at least the minimum for that status
        percentage = Math.max(statusInfo.min, Math.min(actualPercentage, statusInfo.max));
      }
      
      label = `${hoursPlayed}h / ${estimatedHours}h`;
    } else if (hoursPlayed > 0) {
      label = `${hoursPlayed}h played`;
      // Estimate progress based on hours played for in_progress games
      if (status === 'in_progress') {
        percentage = Math.min(10 + (hoursPlayed * 2), statusInfo.max);
      }
    } else {
      // No hours data, use status defaults
      percentage = statusInfo.min;
    }

    return {
      percentage,
      label,
      color: statusInfo.color
    };
  }

  const progress = $derived(calculateProgress());

  // Get completion level description
  function getCompletionDescription(status: PlayStatus): string {
    const descriptions: Record<PlayStatus, string> = {
      'not_started': 'Ready to begin your adventure',
      'in_progress': 'Currently exploring this world',
      'completed': 'Main story completed',
      'mastered': 'All major content completed',
      'dominated': '100% completion achieved',
      'shelved': 'Taking a break from this one',
      'dropped': 'Moved on to other games',
      'replay': 'Enjoying another playthrough'
    };
    return descriptions[status];
  }

  // Format the status label
  function formatStatusLabel(status: PlayStatus): string {
    return status.split('_').map(word => 
      word.charAt(0).toUpperCase() + word.slice(1)
    ).join(' ');
  }
</script>

<div class="bg-white rounded-lg border border-gray-200 p-6 space-y-4 {className}">
  <div class="space-y-2">
    <div class="flex items-center justify-between">
      <h4 class="text-lg font-semibold text-gray-900">Progress</h4>
      <span class="text-sm font-medium text-gray-600">
        {formatStatusLabel(userGame.play_status)}
      </span>
    </div>
    <p class="text-sm text-gray-500">{getCompletionDescription(userGame.play_status)}</p>
  </div>

  <ProgressBar
    value={progress.percentage}
    max={100}
    label={progress.label}
    color={progress.color as any}
    size="lg"
    animated={true}
  />

  {#if userGame.game.howlongtobeat_main || userGame.game.howlongtobeat_extra || userGame.game.howlongtobeat_completionist}
    <div class="mt-4 pt-4 border-t border-gray-100">
      <h5 class="text-sm font-medium text-gray-700 mb-3">Estimated Completion Times</h5>
      <div class="grid grid-cols-3 gap-3 text-center">
        {#if userGame.game.howlongtobeat_main}
          <div class="space-y-1">
            <div class="text-xs text-gray-500">Main Story</div>
            <div class="text-sm font-semibold text-gray-900">{userGame.game.howlongtobeat_main}h</div>
            {#if userGame.hours_played >= userGame.game.howlongtobeat_main && userGame.play_status === 'in_progress'}
              <div class="text-xs text-green-600">✓</div>
            {/if}
          </div>
        {/if}
        {#if userGame.game.howlongtobeat_extra}
          <div class="space-y-1">
            <div class="text-xs text-gray-500">Main + Extra</div>
            <div class="text-sm font-semibold text-gray-900">{userGame.game.howlongtobeat_extra}h</div>
            {#if userGame.hours_played >= userGame.game.howlongtobeat_extra && (userGame.play_status === 'completed' || userGame.play_status === 'mastered')}
              <div class="text-xs text-green-600">✓</div>
            {/if}
          </div>
        {/if}
        {#if userGame.game.howlongtobeat_completionist}
          <div class="space-y-1">
            <div class="text-xs text-gray-500">Completionist</div>
            <div class="text-sm font-semibold text-gray-900">{userGame.game.howlongtobeat_completionist}h</div>
            {#if userGame.hours_played >= userGame.game.howlongtobeat_completionist && userGame.play_status === 'dominated'}
              <div class="text-xs text-green-600">✓</div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  {/if}

  {#if userGame.last_played}
    <div class="mt-3 pt-3 border-t border-gray-100">
      <p class="text-xs text-gray-500">
        Last played: {new Date(userGame.last_played).toLocaleDateString()}
      </p>
    </div>
  {/if}
</div>