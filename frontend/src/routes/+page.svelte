<script lang="ts">
  import { auth } from '$lib/stores';
</script>

<div class="space-y-12">
  <!-- Hero Section -->
  <div class="text-center space-y-6">
    <div class="space-y-4">
      <h1 class="text-4xl md:text-5xl lg:text-6xl font-bold text-gradient">
        Welcome to Nexorious
      </h1>
      <p class="text-xl text-gray-600 dark:text-gray-300 max-w-2xl mx-auto">
        Your self-hosted game collection management service. Organize, track, and discover your gaming library like never before.
      </p>
    </div>
    
    {#if auth.value.user}
      <div class="space-y-6">
        <div class="flex items-center justify-center space-x-2">
          <div class="w-10 h-10 bg-gradient-gaming rounded-full flex items-center justify-center">
            <span class="text-lg font-bold text-white">
              {auth.value.user.username?.charAt(0).toUpperCase()}
            </span>
          </div>
          <div>
            <p class="text-lg font-medium text-gray-900 dark:text-white">
              Welcome back, {auth.value.user.username}!
            </p>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              Ready to manage your game collection?
            </p>
          </div>
        </div>
        
        <!-- Quick Actions -->
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 max-w-4xl mx-auto">
          <a
            href="/games"
            class="card card-hover group p-6 text-center space-y-3"
          >
            <div class="w-12 h-12 bg-primary-100 dark:bg-primary-900 rounded-lg flex items-center justify-center mx-auto group-hover:bg-primary-600 transition-colors">
              <svg class="w-6 h-6 text-primary-600 dark:text-primary-400 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
              </svg>
            </div>
            <div>
              <h3 class="font-semibold text-gray-900 dark:text-white">My Games</h3>
              <p class="text-sm text-gray-500 dark:text-gray-400">View your collection</p>
            </div>
          </a>
          
          <a
            href="/games/add"
            class="card card-hover group p-6 text-center space-y-3"
          >
            <div class="w-12 h-12 bg-success-100 dark:bg-success-900 rounded-lg flex items-center justify-center mx-auto group-hover:bg-success-600 transition-colors">
              <svg class="w-6 h-6 text-success-600 dark:text-success-400 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
              </svg>
            </div>
            <div>
              <h3 class="font-semibold text-gray-900 dark:text-white">Add Game</h3>
              <p class="text-sm text-gray-500 dark:text-gray-400">Expand your library</p>
            </div>
          </a>
          
          <a
            href="/wishlist"
            class="card card-hover group p-6 text-center space-y-3"
          >
            <div class="w-12 h-12 bg-gaming-100 dark:bg-gaming-900 rounded-lg flex items-center justify-center mx-auto group-hover:bg-gaming-600 transition-colors">
              <svg class="w-6 h-6 text-gaming-600 dark:text-gaming-400 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
              </svg>
            </div>
            <div>
              <h3 class="font-semibold text-gray-900 dark:text-white">Wishlist</h3>
              <p class="text-sm text-gray-500 dark:text-gray-400">Track wanted games</p>
            </div>
          </a>
          
          <a
            href="/dashboard"
            class="card card-hover group p-6 text-center space-y-3"
          >
            <div class="w-12 h-12 bg-secondary-100 dark:bg-secondary-800 rounded-lg flex items-center justify-center mx-auto group-hover:bg-secondary-600 transition-colors">
              <svg class="w-6 h-6 text-secondary-600 dark:text-secondary-400 group-hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
              </svg>
            </div>
            <div>
              <h3 class="font-semibold text-gray-900 dark:text-white">Dashboard</h3>
              <p class="text-sm text-gray-500 dark:text-gray-400">View statistics</p>
            </div>
          </a>
        </div>
      </div>
    {:else}
      <!-- Not logged in -->
      <div class="space-y-6">
        <p class="text-lg text-gray-600 dark:text-gray-300">
          Please log in to start managing your game collection
        </p>
        <div class="flex flex-col sm:flex-row justify-center gap-4">
          <a href="/login" class="btn btn-primary btn-lg">
            <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 16l-4-4m0 0l4-4m-4 4h14m-5 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h7a3 3 0 013 3v1" />
            </svg>
            Login
          </a>
          <a href="/register" class="btn btn-outline btn-lg">
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
      <div class="w-16 h-16 bg-primary-100 dark:bg-primary-900 rounded-full flex items-center justify-center mx-auto">
        <svg class="w-8 h-8 text-primary-600 dark:text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
        </svg>
      </div>
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
        Organize Your Library
      </h3>
      <p class="text-gray-600 dark:text-gray-400">
        Keep track of all your games across multiple platforms and storefronts in one unified collection.
      </p>
    </div>
    
    <div class="text-center space-y-3">
      <div class="w-16 h-16 bg-success-100 dark:bg-success-900 rounded-full flex items-center justify-center mx-auto">
        <svg class="w-8 h-8 text-success-600 dark:text-success-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      </div>
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
        Track Progress
      </h3>
      <p class="text-gray-600 dark:text-gray-400">
        Monitor your gaming progress with detailed completion levels from started to dominated.
      </p>
    </div>
    
    <div class="text-center space-y-3">
      <div class="w-16 h-16 bg-gaming-100 dark:bg-gaming-900 rounded-full flex items-center justify-center mx-auto">
        <svg class="w-8 h-8 text-gaming-600 dark:text-gaming-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 100 4m0-4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 100 4m0-4v2m0-6V4" />
        </svg>
      </div>
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
        Self-Hosted Privacy
      </h3>
      <p class="text-gray-600 dark:text-gray-400">
        Keep your gaming data private and secure with complete control over your self-hosted instance.
      </p>
    </div>
  </div>
</div>
