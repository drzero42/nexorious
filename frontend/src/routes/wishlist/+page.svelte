<script lang="ts">
 import { auth, wishlist } from '$lib/stores';
 import { onMount } from 'svelte';
 import { goto } from '$app/navigation';
 import { RouteGuard } from '$lib/components';

 let searchQuery = '';
 let sortBy = 'title';
 let sortOrder: 'asc' | 'desc' = 'asc';

 onMount(() => {
  // Load wishlist - authentication is handled by RouteGuard
  wishlist.fetchWishlist();
 });

 // Filter and sort wishlist games
 $: filteredGames = wishlist.value.games
  .filter(game => {
   const matchesSearch = !searchQuery || 
    game.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
    game.genre?.toLowerCase().includes(searchQuery.toLowerCase()) ||
    game.developer?.toLowerCase().includes(searchQuery.toLowerCase());
   
   return matchesSearch;
  })
  .sort((a, b) => {
   let aValue = a[sortBy] || '';
   let bValue = b[sortBy] || '';
   
   if (sortOrder === 'asc') {
    return aValue > bValue ? 1 : -1;
   } else {
    return aValue < bValue ? 1 : -1;
   }
  });

 async function removeFromWishlist(gameId: string) {
  try {
   await wishlist.removeFromWishlist(gameId);
  } catch (error) {
   console.error('Failed to remove from wishlist:', error);
  }
 }

 async function addToCollection(gameId: string) {
  try {
   // This would move the game from wishlist to owned collection
   // For now, just remove from wishlist
   await wishlist.removeFromWishlist(gameId);
   // In a real app, this would also add to user's collection
  } catch (error) {
   console.error('Failed to add to collection:', error);
  }
 }

 function generatePriceComparisonLinks(game) {
  // Generate dynamic links for price comparison
  const gameTitle = encodeURIComponent(game.title);
  const gameSlug = game.slug || gameTitle;
  
  return {
   isThereAnyDeal: `https://isthereanydeal.com/search/?q=${gameTitle}`,
   psPrices: `https://psprices.com/region-us/search/?q=${gameTitle}`,
  };
 }

 function handleAddGame() {
  goto('/games/add');
 }
</script>

<svelte:head>
 <title>Wishlist - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
<div class="space-y-6">
 <!-- Header -->
 <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between">
  <div>
   <h1 class="text-2xl font-bold text-gray-900">Wishlist</h1>
   <p class="text-gray-600">
    {wishlist.value.games.length} games on your wishlist
   </p>
  </div>
  <div class="mt-4 sm:mt-0">
   <button
    on:click={handleAddGame}
    class="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors"
   >
    Add Game
   </button>
  </div>
 </div>

 <!-- Search and Sort -->
 <div class="bg-white rounded-lg shadow p-4">
  <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
   <!-- Search -->
   <div>
    <label for="search" class="block text-sm font-medium text-gray-700 mb-1">
     Search
    </label>
    <input
     id="search"
     type="text"
     bind:value={searchQuery}
     placeholder="Search wishlist..."
     class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
    />
   </div>

   <!-- Sort By -->
   <div>
    <label for="sortBy" class="block text-sm font-medium text-gray-700 mb-1">
     Sort By
    </label>
    <select
     id="sortBy"
     bind:value={sortBy}
     class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
    >
     <option value="title">Title</option>
     <option value="genre">Genre</option>
     <option value="developer">Developer</option>
     <option value="release_date">Release Date</option>
    </select>
   </div>

   <!-- Sort Order -->
   <div>
    <label for="sortOrder" class="block text-sm font-medium text-gray-700 mb-1">
     Order
    </label>
    <select
     id="sortOrder"
     bind:value={sortOrder}
     class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
    >
     <option value="asc">Ascending</option>
     <option value="desc">Descending</option>
    </select>
   </div>
  </div>
 </div>

 <!-- Wishlist Games -->
 {#if wishlist.value.isLoading}
  <div class="text-center py-8">
   <div class="text-gray-500">Loading wishlist...</div>
  </div>
 {:else if filteredGames.length === 0}
  <div class="text-center py-8">
   <div class="text-gray-500">
    {wishlist.value.games.length === 0 ? 'Your wishlist is empty.' : 'No games match your search.'}
   </div>
   {#if wishlist.value.games.length === 0}
    <button
     on:click={handleAddGame}
     class="mt-4 bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors"
    >
     Add Your First Game
    </button>
   {/if}
  </div>
 {:else}
  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
   {#each filteredGames as game (game.id)}
    <div class="bg-white rounded-lg shadow hover:shadow-md transition-shadow">
     <div class="p-4">
      <div class="flex">
       <!-- Cover Art -->
       <div class="flex-shrink-0 w-16 h-20 bg-gray-200 rounded">
        {#if game.cover_art_url}
         <img
          src={game.cover_art_url}
          alt={game.title}
          class="w-full h-full object-cover rounded"
         />
        {:else}
         <div class="w-full h-full flex items-center justify-center text-gray-400 text-xs">
          No Cover
         </div>
        {/if}
       </div>

       <!-- Game Info -->
       <div class="ml-4 flex-1">
        <h3 class="font-semibold text-gray-900 text-sm mb-1">
         {game.title}
        </h3>
        <p class="text-xs text-gray-600 mb-1">
         {game.developer || 'Unknown Developer'}
        </p>
        <p class="text-xs text-gray-500 mb-2">
         {game.genre || 'Unknown Genre'}
        </p>
        {#if game.release_date}
         <p class="text-xs text-gray-500">
          Released: {new Date(game.release_date).getFullYear()}
         </p>
        {/if}
       </div>
      </div>

      <!-- Price Comparison Links -->
      <div class="mt-4 border-t border-gray-200 dark:border-gray-700 pt-3">
       <p class="text-xs font-medium text-gray-700 mb-2">
        Check Prices:
       </p>
       <div class="flex flex-wrap gap-2">
        <a
         href={generatePriceComparisonLinks(game).isThereAnyDeal}
         target="_blank"
         rel="noopener noreferrer"
         class="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-blue-100 text-blue-800 hover:bg-blue-200 transition-colors"
        >
         IsThereAnyDeal
         <svg class="w-3 h-3 ml-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
         </svg>
        </a>
        <a
         href={generatePriceComparisonLinks(game).psPrices}
         target="_blank"
         rel="noopener noreferrer"
         class="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-purple-100 text-purple-800 hover:bg-purple-200 transition-colors"
        >
         PSPrices
         <svg class="w-3 h-3 ml-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
         </svg>
        </a>
       </div>
      </div>

      <!-- Actions -->
      <div class="mt-4 flex space-x-2">
       <button
        on:click={() => addToCollection(game.id)}
        class="flex-1 px-3 py-2 bg-green-600 hover:bg-green-700 text-white text-xs rounded-md transition-colors"
       >
        Add to Collection
       </button>
       <button
        on:click={() => removeFromWishlist(game.id)}
        class="px-3 py-2 bg-red-600 hover:bg-red-700 text-white text-xs rounded-md transition-colors"
       >
        Remove
       </button>
      </div>
     </div>
    </div>
   {/each}
  </div>
 {/if}
</div>
</RouteGuard>