<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
</script>

<svelte:head>
  <title>Page Not Found - Nexorious</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
  <div class="max-w-md w-full text-center space-y-8">
    <div>
      <h1 class="text-6xl font-bold text-gray-900">404</h1>
      <h2 class="mt-6 text-3xl font-bold text-gray-900">Page Not Found</h2>
      <p class="mt-2 text-sm text-gray-600">
        The page you're looking for doesn't exist or has been moved.
      </p>
    </div>
    
    <div class="mt-8 space-y-6">
      <div class="space-y-4">
        <button
          onclick={() => goto('/games')}
          class="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
        >
          Go to Games
        </button>
        
        <button
          onclick={() => goto('/')}
          class="group relative w-full flex justify-center py-2 px-4 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
        >
          Go Home
        </button>
      </div>
      
      <p class="text-xs text-gray-500">
        Error {$page.status}: {$page.error?.message || 'Unknown error'}
      </p>
    </div>
  </div>
</div>