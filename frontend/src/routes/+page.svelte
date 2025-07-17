<script lang="ts">
 import { auth } from '$lib/stores';
</script>

<div class="space-y-12">
 <!-- Hero Section -->
 <div class="text-center space-y-6">
  <div class="space-y-4">
   <h1 class="text-4xl md:text-5xl lg:text-6xl font-bold bg-gradient-to-r from-indigo-500 via-purple-500 to-pink-500 bg-clip-text text-transparent">
    Welcome to Nexorious
   </h1>
   <p class="text-xl text-gray-600 max-w-2xl mx-auto">
    Your self-hosted game collection management service. Organize, track, and discover your gaming library like never before.
   </p>
  </div>
  
  {#if auth.value.user}
   <div class="space-y-6">
    <div class="flex items-center justify-center space-x-2">
     <div class="w-10 h-10 bg-gradient-to-r from-indigo-500 via-purple-500 to-pink-500 rounded-full flex items-center justify-center">
      <span class="text-lg font-bold text-white">
       {auth.value.user.username?.charAt(0).toUpperCase()}
      </span>
     </div>
     <div>
      <p class="text-lg font-medium text-gray-900">
       Welcome back, {auth.value.user.username}!
      </p>
      <p class="text-sm text-gray-500">
       Ready to manage your game collection?
      </p>
     </div>
    </div>
    
    <!-- Quick Actions -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 max-w-4xl mx-auto">
     <a
      href="/games"
      class="bg-white rounded-xl shadow-sm border border-gray-200 cursor-pointer transition-all duration-200 hover:shadow-md hover:scale-105 group p-6 text-center space-y-3"
     >
      <div class="w-12 h-12 bg-blue-100 rounded-lg flex items-center justify-center mx-auto group-hover:bg-blue-600 transition-colors">
       <svg class="w-6 h-6 text-blue-600 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
       </svg>
      </div>
      <div>
       <h3 class="font-semibold text-gray-900">My Games</h3>
       <p class="text-sm text-gray-500">View your collection</p>
      </div>
     </a>
     
     <a
      href="/games/add"
      class="bg-white rounded-xl shadow-sm border border-gray-200 cursor-pointer transition-all duration-200 hover:shadow-md hover:scale-105 group p-6 text-center space-y-3"
     >
      <div class="w-12 h-12 bg-green-100 rounded-lg flex items-center justify-center mx-auto group-hover:bg-green-600 transition-colors">
       <svg class="w-6 h-6 text-green-600 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
       </svg>
      </div>
      <div>
       <h3 class="font-semibold text-gray-900">Add Game</h3>
       <p class="text-sm text-gray-500">Expand your library</p>
      </div>
     </a>
     
     <a
      href="/wishlist"
      class="bg-white rounded-xl shadow-sm border border-gray-200 cursor-pointer transition-all duration-200 hover:shadow-md hover:scale-105 group p-6 text-center space-y-3"
     >
      <div class="w-12 h-12 bg-purple-100 rounded-lg flex items-center justify-center mx-auto group-hover:bg-purple-600 transition-colors">
       <svg class="w-6 h-6 text-purple-600 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
       </svg>
      </div>
      <div>
       <h3 class="font-semibold text-gray-900">Wishlist</h3>
       <p class="text-sm text-gray-500">Track wanted games</p>
      </div>
     </a>
     
     <a
      href="/dashboard"
      class="bg-white rounded-xl shadow-sm border border-gray-200 cursor-pointer transition-all duration-200 hover:shadow-md hover:scale-105 group p-6 text-center space-y-3"
     >
      <div class="w-12 h-12 bg-gray-100 rounded-lg flex items-center justify-center mx-auto group-hover:bg-gray-600 transition-colors">
       <svg class="w-6 h-6 text-gray-600 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
       </svg>
      </div>
      <div>
       <h3 class="font-semibold text-gray-900">Dashboard</h3>
       <p class="text-sm text-gray-500">View statistics</p>
      </div>
     </a>
    </div>
   </div>
  {:else}
   <!-- Not logged in -->
   <div class="space-y-6">
    <p class="text-lg text-gray-600">
     Please log in to start managing your game collection
    </p>
    <div class="flex flex-col sm:flex-row justify-center gap-4">
     <a href="/login" class="inline-flex items-center justify-center font-medium rounded-lg transition-all duration-200 outline-none focus:ring-2 focus:ring-blue-500/50 px-6 py-3 text-base bg-blue-600 text-white hover:bg-blue-700">
      <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
       <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 16l-4-4m0 0l4-4m-4 4h14m-5 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h7a3 3 0 013 3v1" />
      </svg>
      Login
     </a>
     <a href="/register" class="inline-flex items-center justify-center font-medium rounded-lg transition-all duration-200 outline-none focus:ring-2 focus:ring-blue-500/50 px-6 py-3 text-base border-2 border-blue-600 text-blue-600 hover:bg-blue-600 hover:text-white ">
      <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
       <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
      </svg>
      Register
     </a>
    </div>
   </div>
  {/if}
 </div>
 
 <!-- Features Section -->
 <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
  <div class="text-center space-y-3">
   <div class="w-16 h-16 bg-blue-100 rounded-full flex items-center justify-center mx-auto">
    <svg class="w-8 h-8 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
    </svg>
   </div>
   <h3 class="text-lg font-semibold text-gray-900">
    Organize Your Library
   </h3>
   <p class="text-gray-600 dark:text-gray-400">
    Keep track of all your games across multiple platforms and storefronts in one unified collection.
   </p>
  </div>
  
  <div class="text-center space-y-3">
   <div class="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mx-auto">
    <svg class="w-8 h-8 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
   </div>
   <h3 class="text-lg font-semibold text-gray-900">
    Track Progress
   </h3>
   <p class="text-gray-600 dark:text-gray-400">
    Monitor your gaming progress with detailed completion levels from started to dominated.
   </p>
  </div>
  
  <div class="text-center space-y-3">
   <div class="w-16 h-16 bg-purple-100 rounded-full flex items-center justify-center mx-auto">
    <svg class="w-8 h-8 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 100 4m0-4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 100 4m0-4v2m0-6V4" />
    </svg>
   </div>
   <h3 class="text-lg font-semibold text-gray-900">
    Self-Hosted Privacy
   </h3>
   <p class="text-gray-600 dark:text-gray-400">
    Keep your gaming data private and secure with complete control over your self-hosted instance.
   </p>
  </div>
 </div>
</div>
