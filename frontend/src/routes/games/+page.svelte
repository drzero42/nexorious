<script lang="ts">
 import { userGames } from '$lib/stores';
 import { onMount } from 'svelte';
 import { goto } from '$app/navigation';
 import { RouteGuard, Pagination } from '$lib/components';
 import type { UserGameFilters } from '$lib/stores';

 let viewMode: 'grid' | 'list' = 'grid';
 let searchQuery = '';
 let selectedPlatform = '';
 let selectedStatus = '';
 let sortBy = 'title';

 // Local state for debounced search
 let searchTimeout: ReturnType<typeof setTimeout>;

 onMount(() => {
  // Load user games - authentication is handled by RouteGuard
  loadGames();
 });

 // Build filters based on current selections
 $: filters = {
  ...(selectedStatus && { play_status: selectedStatus }),
  ...(selectedPlatform && { platform_id: selectedPlatform }),
  ...(searchQuery && { q: searchQuery })
 } as UserGameFilters;

 // Load games with current filters and pagination
 async function loadGames() {
  try {
   await userGames.loadUserGames(
    filters,
    userGames.value.pagination.page,
    userGames.value.pagination.per_page
   );
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 // Handle search with debouncing
 function handleSearch() {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
   loadGames();
  }, 300);
 }

 // Handle filter changes
 function handleFilterChange() {
  loadGamesWithReset();
 }
 
 // Load games with page reset
 async function loadGamesWithReset() {
  try {
   await userGames.loadUserGames(filters, 1, userGames.value.pagination.per_page);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 // Watch for filter changes
 $: if (selectedStatus || selectedPlatform) {
  handleFilterChange();
 }

 // Watch for search query changes
 $: if (searchQuery !== undefined) {
  handleSearch();
 }

 function handleAddGame() {
  goto('/games/add');
 }

 function handleGameClick(gameId: string) {
  goto(`/games/${gameId}`);
 }

 // Pagination handlers
 async function handlePageChange(page: number) {
  try {
   await userGames.loadUserGames(filters, page, userGames.value.pagination.per_page);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 async function handleItemsPerPageChange(perPage: number) {
  try {
   await userGames.loadUserGames(filters, 1, perPage);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
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
 <title>My Games - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
<div>
 <!-- Header -->
 <div>
  <div>
   <h1>My Games</h1>
   <p>
    {userGames.value.pagination.total} games in your collection
   </p>
  </div>
  <div>
   <button
    on:click={handleAddGame}
   >
    +
    Add Game
   </button>
  </div>
 </div>

 <!-- Filters and Search -->
 <div>
  <div>
   <div>
    <!-- Search -->
    <div>
     <label for="search">
      Search
     </label>
     <input
      id="search"
      type="text"
      bind:value={searchQuery}
      placeholder="Search games..."
     />
    </div>

    <!-- Status Filter -->
    <div>
     <label for="status">
      Status
     </label>
     <select
      id="status"
      bind:value={selectedStatus}
     >
      <option value="">All Statuses</option>
      <option value="not_started">Not Started</option>
      <option value="in_progress">In Progress</option>
      <option value="completed">Completed</option>
      <option value="mastered">Mastered</option>
      <option value="dominated">Dominated</option>
      <option value="shelved">Shelved</option>
      <option value="dropped">Dropped</option>
      <option value="replay">Replay</option>
     </select>
    </div>

    <!-- Sort By -->
    <div>
     <label for="sortBy">
      Sort By
     </label>
     <select
      id="sortBy"
      bind:value={sortBy}
     >
      <option value="title">Title</option>
      <option value="personal_rating">Rating</option>
      <option value="play_status">Status</option>
      <option value="genre">Genre</option>
      <option value="release_date">Release Date</option>
     </select>
    </div>

    <!-- View Mode -->
    <div>
     <label for="view-mode">
      View
     </label>
     <div id="view-mode" role="radiogroup" aria-labelledby="view-mode">
      <button
       on:click={() => viewMode = 'grid'}
       role="radio"
       aria-checked={viewMode === 'grid'}
      >
       Grid
      </button>
      <button
       on:click={() => viewMode = 'list'}
       role="radio"
       aria-checked={viewMode === 'list'}
      >
       List
      </button>
     </div>
    </div>
   </div>
  </div>
 </div>

 <!-- Games Display -->
 {#if userGames.value.isLoading}
  <div>
   <div></div>
   <div>Loading games...</div>
  </div>
 {:else if userGames.value.userGames.length === 0}
  <div>
   <div>
    No Games
   </div>
   <h3>
    {userGames.value.pagination.total === 0 ? 'No games in your collection yet' : 'No games match your filters'}
   </h3>
   <p>
    {userGames.value.pagination.total === 0 ? 'Start building your game library by adding your first game.' : 'Try adjusting your search or filter criteria.'}
   </p>
   {#if userGames.value.pagination.total === 0}
    <button
     on:click={handleAddGame}
    >
     +
     Add Your First Game
    </button>
   {/if}
  </div>
 {:else}
  <!-- Grid View -->
  {#if viewMode === 'grid'}
   <div>
    {#each userGames.value.userGames as userGame (userGame.id)}
     <div
      on:click={() => handleGameClick(userGame.id)}
      on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
      tabindex="0"
      role="button"
      aria-label="View details for {userGame.game.title}"
     >
      <div>
       {#if userGame.game.cover_art_url}
        <img
         src={userGame.game.cover_art_url}
         alt="Cover art for {userGame.game.title}"
         loading="lazy"
         on:error={(e) => {
          const target = e.currentTarget as HTMLImageElement;
          const nextElement = target.nextElementSibling as HTMLElement;
          target.style.display = 'none';
          if (nextElement) {
            nextElement.style.display = 'flex';
          }
         }}
        />
        <div style="display: none;">
         No Image
        </div>
       {:else}
        <div>
         <span>No Cover</span>
        </div>
       {/if}
       
       <!-- Loved indicator -->
       {#if userGame.is_loved}
        <div>
         ♥
        </div>
       {/if}
      </div>
      
      <div>
       <h3 title="{userGame.game.title}">
        {userGame.game.title}
       </h3>
       <p title="{userGame.game.genre || 'Unknown Genre'}">
        {userGame.game.genre || 'Unknown Genre'}
       </p>
       
       <div>
        <span>
         {getStatusLabel(userGame.play_status)}
        </span>
        {#if userGame.personal_rating}
         <div>
          <span>★</span>
          <span>
           {userGame.personal_rating}
          </span>
         </div>
        {/if}
       </div>
       
       <!-- Additional info -->
       <div>
        <span>{userGame.hours_played || 0}h played</span>
        {#if userGame.game.release_date}
         <span>{new Date(userGame.game.release_date).getFullYear()}</span>
        {/if}
       </div>
      </div>
     </div>
    {/each}
   </div>
  {:else}
   <!-- List View -->
   <div>
    <div>
     <table>
      <thead>
       <tr>
        <th>
         Game
        </th>
        <th>
         Genre
        </th>
        <th>
         Status
        </th>
        <th>
         Rating
        </th>
        <th>
         Hours
        </th>
       </tr>
      </thead>
      <tbody>
       {#each userGames.value.userGames as userGame (userGame.id)}
        <tr
         on:click={() => handleGameClick(userGame.id)}
         on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
         tabindex="0"
        >
         <td>
          <div>
           <div>
            {#if userGame.game.cover_art_url}
             <img
              src={userGame.game.cover_art_url}
              alt={userGame.game.title}
             />
            {:else}
             <div>
              No Image
             </div>
            {/if}
            {#if userGame.is_loved}
             <div>
              ♥
             </div>
            {/if}
           </div>
           <div>
            <div>
             {userGame.game.title}
            </div>
            <div>
             {userGame.game.developer || 'Unknown Developer'}
            </div>
           </div>
          </div>
         </td>
         <td>
          {userGame.game.genre || 'Unknown'}
         </td>
         <td>
          <span>
           {getStatusLabel(userGame.play_status)}
          </span>
         </td>
         <td>
          {#if userGame.personal_rating}
           <div>
            <span>★</span>
            <span>{userGame.personal_rating}</span>
           </div>
          {:else}
           <span>-</span>
          {/if}
         </td>
         <td>
          {userGame.hours_played || 0}h
         </td>
        </tr>
       {/each}
      </tbody>
     </table>
    </div>
   </div>
  {/if}
  
  <!-- Pagination -->
  <Pagination 
   currentPage={userGames.value.pagination.page}
   totalPages={userGames.value.pagination.pages}
   totalItems={userGames.value.pagination.total}
   itemsPerPage={userGames.value.pagination.per_page}
   onPageChange={handlePageChange}
   onItemsPerPageChange={handleItemsPerPageChange}
  />
 {/if}
</div>
</RouteGuard>