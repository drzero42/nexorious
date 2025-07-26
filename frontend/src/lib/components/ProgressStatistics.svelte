<script lang="ts">
  import { ProgressBar } from '$lib/components';
  import type { UserGame } from '$lib/stores/user-games.svelte';

  interface Props {
    userGames: UserGame[];
    class?: string;
  }

  let { userGames, class: className = '' }: Props = $props();

  // Calculate statistics
  const totalGames = $derived(userGames.length);
  const statusCounts = $derived({
    not_started: userGames.filter(g => g.play_status === 'not_started').length,
    in_progress: userGames.filter(g => g.play_status === 'in_progress').length,
    completed: userGames.filter(g => g.play_status === 'completed').length,
    mastered: userGames.filter(g => g.play_status === 'mastered').length,
    dominated: userGames.filter(g => g.play_status === 'dominated').length,
    shelved: userGames.filter(g => g.play_status === 'shelved').length,
    dropped: userGames.filter(g => g.play_status === 'dropped').length,
    replay: userGames.filter(g => g.play_status === 'replay').length
  });

  // Calculate completion metrics
  const completedTotal = $derived(statusCounts.completed + statusCounts.mastered + statusCounts.dominated);
  const completionRate = $derived(totalGames > 0 ? (completedTotal / totalGames) * 100 : 0);
  const activeGames = $derived(statusCounts.in_progress + statusCounts.replay);
  const abandonedGames = $derived(statusCounts.dropped + statusCounts.shelved);

  // Calculate time metrics
  const totalHoursPlayed = $derived(userGames.reduce((sum, g) => sum + (g.hours_played || 0), 0));
  const averageHoursPerGame = $derived(totalGames > 0 ? totalHoursPlayed / totalGames : 0);
  const completedGamesWithHours = $derived(userGames.filter(g => 
    (g.play_status === 'completed' || g.play_status === 'mastered' || g.play_status === 'dominated') && 
    g.hours_played > 0
  ));
  const averageCompletionTime = $derived(completedGamesWithHours.length > 0 
    ? completedGamesWithHours.reduce((sum, g) => sum + g.hours_played, 0) / completedGamesWithHours.length 
    : 0);

  // Status configuration
  const statusConfig = [
    { 
      key: 'not_started', 
      label: 'Not Started', 
      color: 'gray' as const,
      icon: '⏸️',
      description: 'Games waiting to be played'
    },
    { 
      key: 'in_progress', 
      label: 'In Progress', 
      color: 'blue' as const,
      icon: '🎮',
      description: 'Currently playing'
    },
    { 
      key: 'completed', 
      label: 'Completed', 
      color: 'green' as const,
      icon: '✅',
      description: 'Main story finished'
    },
    { 
      key: 'mastered', 
      label: 'Mastered', 
      color: 'purple' as const,
      icon: '🏆',
      description: 'All major content done'
    },
    { 
      key: 'dominated', 
      label: 'Dominated', 
      color: 'yellow' as const,
      icon: '👑',
      description: '100% completion'
    },
    { 
      key: 'shelved', 
      label: 'Shelved', 
      color: 'orange' as const,
      icon: '📚',
      description: 'On hold for later'
    },
    { 
      key: 'dropped', 
      label: 'Dropped', 
      color: 'red' as const,
      icon: '❌',
      description: 'No longer playing'
    },
    { 
      key: 'replay', 
      label: 'Replay', 
      color: 'blue' as const,
      icon: '🔄',
      description: 'Playing again'
    }
  ];
</script>

<div class="space-y-6 {className}">
  <!-- Overview Stats -->
  <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
    <div class="bg-white rounded-lg border border-gray-200 p-4">
      <div class="text-sm font-medium text-gray-500">Total Games</div>
      <div class="mt-1 text-2xl font-bold text-gray-900">{totalGames}</div>
    </div>
    <div class="bg-white rounded-lg border border-gray-200 p-4">
      <div class="text-sm font-medium text-gray-500">Completion Rate</div>
      <div class="mt-1 text-2xl font-bold text-green-600">{completionRate.toFixed(1)}%</div>
    </div>
    <div class="bg-white rounded-lg border border-gray-200 p-4">
      <div class="text-sm font-medium text-gray-500">Total Hours</div>
      <div class="mt-1 text-2xl font-bold text-blue-600">{totalHoursPlayed.toLocaleString()}</div>
    </div>
    <div class="bg-white rounded-lg border border-gray-200 p-4">
      <div class="text-sm font-medium text-gray-500">Active Games</div>
      <div class="mt-1 text-2xl font-bold text-purple-600">{activeGames}</div>
    </div>
  </div>

  <!-- Detailed Progress Breakdown -->
  <div class="bg-white rounded-lg border border-gray-200 p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">Progress Breakdown</h3>
    <div class="space-y-4">
      {#each statusConfig as status}
        {@const count = statusCounts[status.key as keyof typeof statusCounts]}
        {@const percentage = totalGames > 0 ? (count / totalGames) * 100 : 0}
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <div class="flex items-center space-x-2">
              <span class="text-lg">{status.icon}</span>
              <div>
                <span class="font-medium text-gray-900">{status.label}</span>
                <span class="text-sm text-gray-500 ml-2">({count} {count === 1 ? 'game' : 'games'})</span>
              </div>
            </div>
            <span class="text-sm text-gray-600">{percentage.toFixed(1)}%</span>
          </div>
          <ProgressBar
            value={percentage}
            max={100}
            color={status.color}
            size="sm"
            showPercentage={false}
          />
          <p class="text-xs text-gray-500">{status.description}</p>
        </div>
      {/each}
    </div>
  </div>

  <!-- Completion Journey -->
  <div class="bg-white rounded-lg border border-gray-200 p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">Completion Journey</h3>
    <div class="relative">
      <div class="absolute left-8 top-8 bottom-0 w-0.5 bg-gray-200"></div>
      <div class="space-y-6">
        <div class="flex items-center space-x-4">
          <div class="relative z-10 w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center text-2xl">
            ⏸️
          </div>
          <div class="flex-1">
            <h4 class="font-medium text-gray-900">Not Started</h4>
            <p class="text-sm text-gray-600">{statusCounts.not_started} games waiting</p>
          </div>
        </div>
        
        <div class="flex items-center space-x-4">
          <div class="relative z-10 w-16 h-16 bg-blue-100 rounded-full flex items-center justify-center text-2xl">
            🎮
          </div>
          <div class="flex-1">
            <h4 class="font-medium text-gray-900">In Progress</h4>
            <p class="text-sm text-gray-600">{statusCounts.in_progress} games active</p>
          </div>
        </div>
        
        <div class="flex items-center space-x-4">
          <div class="relative z-10 w-16 h-16 bg-green-100 rounded-full flex items-center justify-center text-2xl">
            ✅
          </div>
          <div class="flex-1">
            <h4 class="font-medium text-gray-900">Completed</h4>
            <p class="text-sm text-gray-600">{statusCounts.completed} main stories finished</p>
          </div>
        </div>
        
        <div class="flex items-center space-x-4">
          <div class="relative z-10 w-16 h-16 bg-purple-100 rounded-full flex items-center justify-center text-2xl">
            🏆
          </div>
          <div class="flex-1">
            <h4 class="font-medium text-gray-900">Mastered</h4>
            <p class="text-sm text-gray-600">{statusCounts.mastered} games fully explored</p>
          </div>
        </div>
        
        <div class="flex items-center space-x-4">
          <div class="relative z-10 w-16 h-16 bg-yellow-100 rounded-full flex items-center justify-center text-2xl">
            👑
          </div>
          <div class="flex-1">
            <h4 class="font-medium text-gray-900">Dominated</h4>
            <p class="text-sm text-gray-600">{statusCounts.dominated} games at 100%</p>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- Time Investment -->
  {#if totalHoursPlayed > 0}
    <div class="bg-white rounded-lg border border-gray-200 p-6">
      <h3 class="text-lg font-semibold text-gray-900 mb-4">Time Investment</h3>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div class="text-center">
          <div class="text-3xl font-bold text-blue-600">{totalHoursPlayed.toLocaleString()}</div>
          <div class="text-sm text-gray-500 mt-1">Total Hours Played</div>
        </div>
        <div class="text-center">
          <div class="text-3xl font-bold text-green-600">{averageHoursPerGame.toFixed(1)}</div>
          <div class="text-sm text-gray-500 mt-1">Average Hours per Game</div>
        </div>
        <div class="text-center">
          <div class="text-3xl font-bold text-purple-600">{averageCompletionTime.toFixed(1)}</div>
          <div class="text-sm text-gray-500 mt-1">Average Completion Time</div>
        </div>
      </div>
    </div>
  {/if}
</div>