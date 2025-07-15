<script lang="ts">
  import { auth } from '$lib/stores';
</script>

<div class="px-4 py-6 sm:px-0">
  <div class="border-4 border-dashed border-gray-200 dark:border-gray-700 rounded-lg p-8">
    <div class="text-center">
      <h1 class="text-4xl font-bold text-gray-900 dark:text-white mb-4">
        Welcome to Nexorious
      </h1>
      <p class="text-lg text-gray-600 dark:text-gray-300 mb-8">
        Your self-hosted game collection management service
      </p>
      
      {#if auth.value.user}
        <div class="space-y-4">
          <p class="text-gray-700 dark:text-gray-300">
            Hello, {auth.value.user.username}! Ready to manage your game collection?
          </p>
          <div class="flex flex-col sm:flex-row justify-center gap-4">
            <a
              href="/games"
              class="bg-blue-500 hover:bg-blue-600 text-white px-6 py-2 rounded-lg transition-colors inline-block text-center"
            >
              My Games
            </a>
            <a
              href="/games/add"
              class="bg-green-500 hover:bg-green-600 text-white px-6 py-2 rounded-lg transition-colors inline-block text-center"
            >
              Add Game
            </a>
            <a
              href="/wishlist"
              class="bg-purple-500 hover:bg-purple-600 text-white px-6 py-2 rounded-lg transition-colors inline-block text-center"
            >
              Wishlist
            </a>
            <a
              href="/dashboard"
              class="bg-gray-500 hover:bg-gray-600 text-white px-6 py-2 rounded-lg transition-colors inline-block text-center"
            >
              Dashboard
            </a>
          </div>
        </div>
      {:else}
        <div class="space-y-4">
          <p class="text-gray-700 dark:text-gray-300">
            Please log in to start managing your game collection
          </p>
          <div class="flex justify-center space-x-4">
            <a
              href="/login"
              class="bg-blue-500 hover:bg-blue-600 text-white px-6 py-2 rounded-lg transition-colors inline-block"
            >
              Login
            </a>
            <a
              href="/register"
              class="bg-gray-500 hover:bg-gray-600 text-white px-6 py-2 rounded-lg transition-colors inline-block"
            >
              Register
            </a>
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>
