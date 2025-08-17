<script lang="ts">
 import { userGames } from '$lib/stores';
 import { onMount } from 'svelte';
 import { goto } from '$app/navigation';
 import { RouteGuard, ProgressStatistics } from '$lib/components';
 
 onMount(() => {
  // Load user games for statistics - authentication is handled by RouteGuard
  userGames.fetchUserGames();
 });

 // Calculate statistics
 const userGamesList = $derived(userGames.value.userGames);
 const totalGames = $derived(userGamesList.length);
 const completedGames = $derived(userGamesList.filter(g => g.play_status === 'completed').length);
 const masteredGames = $derived(userGamesList.filter(g => g.play_status === 'mastered').length);
 const dominatedGames = $derived(userGamesList.filter(g => g.play_status === 'dominated').length);
 const inProgressGames = $derived(userGamesList.filter(g => g.play_status === 'in_progress').length);
 const notStartedGames = $derived(userGamesList.filter(g => g.play_status === 'not_started').length);
 const droppedGames = $derived(userGamesList.filter(g => g.play_status === 'dropped').length);
 const shelvedGames = $derived(userGamesList.filter(g => g.play_status === 'shelved').length);
 const totalHours = $derived(userGamesList.reduce((sum, userGame) => sum + (userGame.hours_played || 0), 0));
 const averageRating = $derived(userGamesList.filter(g => g.personal_rating).reduce((sum, userGame) => sum + (userGame.personal_rating || 0), 0) / userGamesList.filter(g => g.personal_rating).length || 0);
 const lovedGames = $derived(userGamesList.filter(g => g.is_loved).length);
 
 // Pile of Shame (owned games not started)
 const pileOfShame = $derived(notStartedGames);
 
 // Completion rate
 const completionRate = $derived(totalGames > 0 ? ((completedGames + masteredGames + dominatedGames) / totalGames) * 100 : 0);

 // Genre breakdown
 const genreStats = $derived(userGamesList.reduce((stats: Record<string, number>, userGame) => {
  const genre = userGame.game.genre || 'Unknown';
  stats[genre] = (stats[genre] || 0) + 1;
  return stats;
 }, {}));

 // Top genres
 const topGenres = $derived(Object.entries(genreStats)
  .sort(([,a], [,b]) => (b as number) - (a as number))
  .slice(0, 5));

 // Platform breakdown (would need platform data)
 // For now, we'll mock this - accessing totalGames to avoid unused variable warning
 $effect(() => {
  // Placeholder for future platform stats implementation
  totalGames; 
 });


</script>

<svelte:head>
 <title>Dashboard - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
<div>
 <!-- Header -->
 <div>
  <h1>Dashboard</h1>
  <p>
   Your gaming statistics and insights
  </p>
 </div>

 {#if userGames.value.isLoading}
  <div>
   <div>Loading statistics...</div>
  </div>
 {:else if totalGames === 0}
  <div>
   <div>
    No games in your collection yet. Add some games to see your statistics!
   </div>
   <button
    onclick={() => goto('/games/add')}
   >
    Add Your First Game
   </button>
  </div>
 {:else}
  <!-- Progress Statistics -->
  <ProgressStatistics userGames={userGamesList} class="mb-8" />
  
  <!-- Overview Statistics -->
  <div>
   <!-- Total Games -->
   <div>
    <div>
     <div>
      <div>
      </div>
     </div>
     <div>
      <dl>
       <dt>
        Total Games
       </dt>
       <dd>
        {totalGames}
       </dd>
      </dl>
     </div>
    </div>
   </div>

   <!-- Total Hours -->
   <div>
    <div>
     <div>
      <div>
      </div>
     </div>
     <div>
      <dl>
       <dt>
        Total Hours
       </dt>
       <dd>
        {totalHours.toLocaleString()}h
       </dd>
      </dl>
     </div>
    </div>
   </div>

   <!-- Completion Rate -->
   <div>
    <div>
     <div>
      <div>
      </div>
     </div>
     <div>
      <dl>
       <dt>
        Completion Rate
       </dt>
       <dd>
        {completionRate.toFixed(1)}%
       </dd>
      </dl>
     </div>
    </div>
   </div>

   <!-- Pile of Shame -->
   <div>
    <div>
     <div>
      <div>
      </div>
     </div>
     <div>
      <dl>
       <dt>
        Pile of Shame
       </dt>
       <dd>
        {pileOfShame}
       </dd>
      </dl>
     </div>
    </div>
   </div>
  </div>

  <!-- Detailed Statistics -->
  <div>
   <!-- Play Status Breakdown -->
   <div>
    <h3>
     Play Status Breakdown
    </h3>
    <div>
     <div>
      <span>Not Started</span>
      <div>
       <span>{notStartedGames}</span>
       <div>
        <div style="width: {totalGames > 0 ? (notStartedGames / totalGames) * 100 : 0}%"></div>
       </div>
      </div>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">In Progress</span>
      <div class="flex items-center space-x-2">
       <span class="text-sm text-gray-600">{inProgressGames}</span>
       <div class="w-20 bg-gray-200 rounded-full h-2">
        <div class="bg-blue-400 h-2 rounded-full" style="width: {totalGames > 0 ? (inProgressGames / totalGames) * 100 : 0}%"></div>
       </div>
      </div>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Completed</span>
      <div class="flex items-center space-x-2">
       <span class="text-sm text-gray-600">{completedGames}</span>
       <div class="w-20 bg-gray-200 rounded-full h-2">
        <div class="bg-green-400 h-2 rounded-full" style="width: {totalGames > 0 ? (completedGames / totalGames) * 100 : 0}%"></div>
       </div>
      </div>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Mastered</span>
      <div class="flex items-center space-x-2">
       <span class="text-sm text-gray-600">{masteredGames}</span>
       <div class="w-20 bg-gray-200 rounded-full h-2">
        <div class="bg-purple-400 h-2 rounded-full" style="width: {totalGames > 0 ? (masteredGames / totalGames) * 100 : 0}%"></div>
       </div>
      </div>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Dominated</span>
      <div class="flex items-center space-x-2">
       <span class="text-sm text-gray-600">{dominatedGames}</span>
       <div class="w-20 bg-gray-200 rounded-full h-2">
        <div class="bg-yellow-400 h-2 rounded-full" style="width: {totalGames > 0 ? (dominatedGames / totalGames) * 100 : 0}%"></div>
       </div>
      </div>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Shelved</span>
      <div class="flex items-center space-x-2">
       <span class="text-sm text-gray-600">{shelvedGames}</span>
       <div class="w-20 bg-gray-200 rounded-full h-2">
        <div class="bg-orange-400 h-2 rounded-full" style="width: {totalGames > 0 ? (shelvedGames / totalGames) * 100 : 0}%"></div>
       </div>
      </div>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Dropped</span>
      <div class="flex items-center space-x-2">
       <span class="text-sm text-gray-600">{droppedGames}</span>
       <div class="w-20 bg-gray-200 rounded-full h-2">
        <div class="bg-red-400 h-2 rounded-full" style="width: {totalGames > 0 ? (droppedGames / totalGames) * 100 : 0}%"></div>
       </div>
      </div>
     </div>
    </div>
   </div>

   <!-- Top Genres -->
   <div class="bg-white rounded-lg shadow p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">
     Top Genres
    </h3>
    <div class="space-y-3">
     {#each topGenres as [genre, count]}
      <div class="flex items-center justify-between">
       <span class="text-sm font-medium text-gray-700">{genre}</span>
       <div class="flex items-center space-x-2">
        <span class="text-sm text-gray-600">{count}</span>
        <div class="w-20 bg-gray-200 rounded-full h-2">
         <div class="bg-indigo-400 h-2 rounded-full" style="width: {(count / totalGames) * 100}%"></div>
        </div>
       </div>
      </div>
     {/each}
    </div>
   </div>
  </div>

  <!-- Additional Statistics -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
   <!-- Personal Stats -->
   <div class="bg-white rounded-lg shadow p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">
     Personal Stats
    </h3>
    <div class="space-y-3">
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Average Rating</span>
      <span class="text-sm text-gray-600">
       {averageRating > 0 ? `${averageRating.toFixed(1)}/5` : 'N/A'}
      </span>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Loved Games</span>
      <span class="text-sm text-gray-600">{lovedGames}</span>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Average Hours per Game</span>
      <span class="text-sm text-gray-600">
       {totalGames > 0 ? (totalHours / totalGames).toFixed(1) : 0}h
      </span>
     </div>
    </div>
   </div>

   <!-- Game Insights -->
   <div class="bg-white rounded-lg shadow p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">
     Game Insights
    </h3>
    <div class="space-y-3">
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Pile of Shame</span>
      <span class="text-sm text-gray-600">{pileOfShame} games</span>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Completion Rate</span>
      <span class="text-sm text-gray-600">{completionRate.toFixed(1)}%</span>
     </div>
     <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">Most Played Game</span>
      <span class="text-sm text-gray-600">
       {#if userGamesList.length > 0}
        {userGamesList.reduce((max, game) => game.hours_played > max.hours_played ? game : max).game.title}
       {:else}
        None
       {/if}
      </span>
     </div>
    </div>
   </div>
  </div>
 {/if}
</div>
</RouteGuard>