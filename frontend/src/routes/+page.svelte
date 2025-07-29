<script lang="ts">
 import { auth } from '$lib/stores';
 import { goto } from '$app/navigation';
 import { onMount } from 'svelte';

 let isCheckingAuth = false;

 onMount(async () => {
   // If user is not authenticated, check setup status and redirect appropriately
   if (!auth.value.user) {
     isCheckingAuth = true;
     
     try {
       const setupStatus = await auth.checkSetupStatus();
       
       if (setupStatus.needs_setup) {
         // Redirect to initial admin setup
         goto('/setup');
       } else {
         // Redirect to login page
         goto('/login');
       }
     } catch (error) {
       console.error('Failed to check setup status:', error);
       // Default to login page on error
       goto('/login');
     } finally {
       isCheckingAuth = false;
     }
   }
 });
</script>

<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
 <!-- Hero Section -->
 <div class="py-16 sm:py-24">
  <div class="text-center">
   <h1 class="text-4xl font-bold tracking-tight text-gray-900 sm:text-6xl">
    Welcome to Nexorious
   </h1>
   <p class="mt-6 text-lg leading-8 text-gray-600 max-w-2xl mx-auto">
    Your self-hosted game collection management service. Organize, track, and discover your gaming library like never before.
   </p>
  </div>
  
  {#if auth.value.user}
   <div class="mt-12">
    <div class="flex items-center justify-center space-x-4 mb-8">
     <div class="flex h-12 w-12 items-center justify-center rounded-full bg-primary-500">
      <span class="text-lg font-medium text-white">
       {auth.value.user.username?.charAt(0).toUpperCase()}
      </span>
     </div>
     <div class="text-center">
      <p class="text-xl font-semibold text-gray-900">
       Welcome back, {auth.value.user.username}!
      </p>
      <p class="text-sm text-gray-600">
       Ready to manage your game collection?
      </p>
     </div>
    </div>
    
    <!-- Quick Actions -->
    <div class="grid grid-cols-1 gap-6 sm:grid-cols-3 max-w-3xl mx-auto">
     <a
      href="/games"
      class="group relative bg-white p-6 rounded-lg border border-gray-200 shadow-sm hover:shadow-md transition-shadow focus-within:ring-2 focus-within:ring-offset-2 focus-within:ring-primary-500"
     >
      <div class="text-center">
       <div class="flex items-center justify-center mb-4">
        <span class="text-3xl">🎮</span>
       </div>
       <h3 class="text-lg font-medium text-gray-900 group-hover:text-primary-600">My Games</h3>
       <p class="mt-2 text-sm text-gray-500">View your collection</p>
      </div>
     </a>
     
     <a
      href="/games/add"
      class="group relative bg-white p-6 rounded-lg border border-gray-200 shadow-sm hover:shadow-md transition-shadow focus-within:ring-2 focus-within:ring-offset-2 focus-within:ring-primary-500"
     >
      <div class="text-center">
       <div class="flex items-center justify-center mb-4">
        <span class="text-3xl">➕</span>
       </div>
       <h3 class="text-lg font-medium text-gray-900 group-hover:text-primary-600">Add Game</h3>
       <p class="mt-2 text-sm text-gray-500">Expand your library</p>
      </div>
     </a>
     
     <a
      href="/dashboard"
      class="group relative bg-white p-6 rounded-lg border border-gray-200 shadow-sm hover:shadow-md transition-shadow focus-within:ring-2 focus-within:ring-offset-2 focus-within:ring-primary-500"
     >
      <div class="text-center">
       <div class="flex items-center justify-center mb-4">
        <span class="text-3xl">📊</span>
       </div>
       <h3 class="text-lg font-medium text-gray-900 group-hover:text-primary-600">Dashboard</h3>
       <p class="mt-2 text-sm text-gray-500">View statistics</p>
      </div>
     </a>
    </div>
   </div>
  {:else if isCheckingAuth}
   <!-- Checking authentication status -->
   <div class="mt-12 text-center">
    <div class="flex items-center justify-center mb-4">
     <svg class="animate-spin h-8 w-8 text-primary-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
     </svg>
    </div>
    <p class="text-lg text-gray-600">
     Checking authentication status...
    </p>
   </div>
  {:else}
   <!-- Fallback - should not normally be seen due to redirects -->
   <div class="mt-12 text-center">
    <p class="text-lg text-gray-600">
     Redirecting to login...
    </p>
   </div>
  {/if}
 </div>

 <!-- Features Section -->
 <div class="py-16 bg-white rounded-lg">
  <div class="grid grid-cols-1 gap-8 sm:grid-cols-3">
   <div class="text-center">
    <div class="flex items-center justify-center mb-4">
     <span class="text-4xl">📚</span>
    </div>
    <h3 class="text-lg font-semibold text-gray-900 mb-2">
     Organize Your Library
    </h3>
    <p class="text-sm text-gray-600">
     Keep track of all your games across multiple platforms and storefronts in one unified collection.
    </p>
   </div>
   
   <div class="text-center">
    <div class="flex items-center justify-center mb-4">
     <span class="text-4xl">🎯</span>
    </div>
    <h3 class="text-lg font-semibold text-gray-900 mb-2">
     Track Progress
    </h3>
    <p class="text-sm text-gray-600">
     Monitor your gaming progress with detailed completion levels from started to dominated.
    </p>
   </div>
   
   <div class="text-center">
    <div class="flex items-center justify-center mb-4">
     <span class="text-4xl">🔐</span>
    </div>
    <h3 class="text-lg font-semibold text-gray-900 mb-2">
     Self-Hosted Privacy
    </h3>
    <p class="text-sm text-gray-600">
     Keep your gaming data private and secure with complete control over your self-hosted instance.
    </p>
   </div>
  </div>
 </div>
</div>