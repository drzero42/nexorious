<script lang="ts">
  import { auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';

  let email = '';
  let password = '';
  let isLoading = false;
  let error = '';

  async function handleLogin() {
    if (!email || !password) {
      error = 'Please fill in all fields';
      return;
    }

    isLoading = true;
    error = '';

    try {
      await auth.login(email, password);
      goto('/games');
    } catch (err) {
      error = err instanceof Error ? err.message : 'Login failed';
    } finally {
      isLoading = false;
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      handleLogin();
    }
  }
</script>

<svelte:head>
  <title>Login - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={false}>
<div class="max-w-md mx-auto bg-white dark:bg-gray-800 rounded-lg shadow-md p-6">
  <div class="text-center mb-8">
    <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
      Sign In
    </h1>
    <p class="text-gray-600 dark:text-gray-400 mt-2">
      Access your game collection
    </p>
  </div>

  {#if error}
    <div class="mb-4 p-3 bg-red-100 border border-red-400 text-red-700 rounded">
      {error}
    </div>
  {/if}

  <form on:submit|preventDefault={handleLogin} class="space-y-6">
    <div>
      <label for="email" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        Email
      </label>
      <input
        id="email"
        type="email"
        bind:value={email}
        on:keydown={handleKeydown}
        required
        class="mt-1 block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
        placeholder="Enter your email"
      />
    </div>

    <div>
      <label for="password" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        Password
      </label>
      <input
        id="password"
        type="password"
        bind:value={password}
        on:keydown={handleKeydown}
        required
        class="mt-1 block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
        placeholder="Enter your password"
      />
    </div>

    <div>
      <button
        type="submit"
        disabled={isLoading}
        class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {isLoading ? 'Signing in...' : 'Sign In'}
      </button>
    </div>

    <div class="mt-4 text-center">
      <a href="/forgot-password" class="text-sm text-blue-600 hover:text-blue-500">
        Forgot your password?
      </a>
    </div>
  </form>

  <div class="mt-6 text-center">
    <p class="text-sm text-gray-600 dark:text-gray-400">
      Don't have an account?
      <a href="/register" class="font-medium text-blue-600 hover:text-blue-500">
        Sign up
      </a>
    </p>
  </div>
</div>
</RouteGuard>