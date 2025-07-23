<script lang="ts">
 import { userGames } from '$lib/stores';
 import { onMount } from 'svelte';
 import { goto } from '$app/navigation';
 import { RouteGuard } from '$lib/components';
 import { resolveImageUrl } from '$lib/utils/image-url';

 onMount(() => {
  // Load user games for statistics - authentication is handled by RouteGuard
  userGames.fetchUserGames();
 });

 // Calculate statistics
 $: userGamesList = userGames.value.userGames;
 $: totalGames = userGamesList.length;
 $: completedGames = userGamesList.filter(g => g.play_status === 'completed').length;
 $: masteredGames = userGamesList.filter(g => g.play_status === 'mastered').length;
 $: dominatedGames = userGamesList.filter(g => g.play_status === 'dominated').length;
 $: inProgressGames = userGamesList.filter(g => g.play_status === 'in_progress').length;
 $: notStartedGames = userGamesList.filter(g => g.play_status === 'not_started').length;
 $: droppedGames = userGamesList.filter(g => g.play_status === 'dropped').length;
 $: shelvedGames = userGamesList.filter(g => g.play_status === 'shelved').length;
 $: totalHours = userGamesList.reduce((sum, userGame) => sum + (userGame.hours_played || 0), 0);
 $: averageRating = userGamesList.filter(g => g.personal_rating).reduce((sum, userGame) => sum + (userGame.personal_rating || 0), 0) / userGamesList.filter(g => g.personal_rating).length || 0;
 $: lovedGames = userGamesList.filter(g => g.is_loved).length;
 
 // Pile of Shame (owned games not started)
 $: pileOfShame = notStartedGames;
 
 // Completion rate
 $: completionRate = totalGames > 0 ? ((completedGames + masteredGames + dominatedGames) / totalGames) * 100 : 0;

 // Genre breakdown
 $: genreStats = userGamesList.reduce((stats: Record<string, number>, userGame) => {
  const genre = userGame.game.genre || 'Unknown';
  stats[genre] = (stats[genre] || 0) + 1;
  return stats;
 }, {});

 // Top genres
 $: topGenres = Object.entries(genreStats)
  .sort(([,a], [,b]) => (b as number) - (a as number))
  .slice(0, 5);

 // Platform breakdown (would need platform data)
 // For now, we'll mock this
 $: {
  // Placeholder for future platform stats implementation
  totalGames; // Access totalGames to avoid unused variable warning
 }

 // Recent activity (games played recently)
 $: recentGames = userGamesList
  .filter(g => g.last_played)
  .sort((a, b) => new Date(b.last_played!).getTime() - new Date(a.last_played!).getTime())
  .slice(0, 5);

 function getStatusColor(status: string) {
  const colors: Record<string, string> = {
   'not_started': 'bg-gray-100 text-gray-800',
   'in_progress': 'bg-blue-100 text-blue-800',
   'completed': 'bg-green-100 text-green-800',
   'mastered': 'bg-purple-100 text-purple-800',
   'dominated': 'bg-yellow-100 text-yellow-800',
   'shelved': 'bg-orange-100 text-orange-800',
   'dropped': 'bg-red-100 text-red-800',
   'replay': 'bg-indigo-100 text-indigo-800'
  };
  return colors[status] || 'bg-gray-100 text-gray-800';
 }

 function getStatusLabel(status: string) {
  const labels: Record<string, string> = {
   'not_started': 'Not Started',
   'in_progress': 'In Progress',
   'completed': 'Completed',
   'mastered': 'Mastered',
   'dominated': 'Dominated',
   'shelved': 'Shelved',
   'dropped': 'Dropped',
   'replay': 'Replay'
  };
  return labels[status] || status;
 }
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
    on:click={() => goto('/games/add')}
   >
    Add Your First Game
   </button>
  </div>
 {:else}
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

   <!-- Recent Activity -->
   <div class="bg-white rounded-lg shadow p-6">
    <h3 class="text-lg font-semibold text-gray-900 mb-4">
     Recent Activity
    </h3>
    {#if recentGames.length > 0}
     <div class="space-y-3">
      {#each recentGames as userGame}
       <div class="flex items-center justify-between">
        <div class="flex items-center space-x-3">
         <div class="flex-shrink-0 w-8 h-8 bg-gray-200 rounded">
          {#if userGame.game.cover_art_url}
           <img
            src={resolveImageUrl(userGame.game.cover_art_url)}
            alt={userGame.game.title}
            class="w-full h-full object-cover rounded"
           />
          {:else}
           <div class="w-full h-full bg-gray-300 rounded"></div>
          {/if}
         </div>
         <div>
          <p class="text-sm font-medium text-gray-900">{userGame.game.title}</p>
          <p class="text-xs text-gray-500">
           {new Date(userGame.last_played!).toLocaleDateString()}
          </p>
         </div>
        </div>
        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium {getStatusColor(userGame.play_status)}">
         {getStatusLabel(userGame.play_status)}
        </span>
       </div>
      {/each}
     </div>
    {:else}
     <p class="text-sm text-gray-500">
      No recent activity recorded.
     </p>
    {/if}
   </div>
  </div>
 {/if}
</div>
</RouteGuard>