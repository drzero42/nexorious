<script lang="ts">
  import { auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';

  let email = '';
  let username = '';
  let password = '';
  let confirmPassword = '';
  let firstName = '';
  let lastName = '';
  let isLoading = false;
  let error = '';

  async function handleRegister() {
    if (!email || !username || !password || !confirmPassword) {
      error = 'Please fill in all required fields';
      return;
    }

    if (password !== confirmPassword) {
      error = 'Passwords do not match';
      return;
    }

    if (password.length < 8) {
      error = 'Password must be at least 8 characters long';
      return;
    }

    isLoading = true;
    error = '';

    try {
      await auth.register({
        email,
        username,
        password,
        first_name: firstName,
        last_name: lastName
      });
      goto('/games');
    } catch (err) {
      error = err instanceof Error ? err.message : 'Registration failed';
    } finally {
      isLoading = false;
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      handleRegister();
    }
  }
</script>

<svelte:head>
  <title>Register - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={false}>
<div class="max-w-md mx-auto bg-white dark:bg-gray-800 rounded-lg shadow-md p-6">
  <div class="text-center mb-8">
    <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
      Create Account
    </h1>
    <p class="text-gray-600 dark:text-gray-400 mt-2">
      Join Nexorious to manage your game collection
    </p>
  </div>

  {#if error}
    <div class="mb-4 p-3 bg-red-100 border border-red-400 text-red-700 rounded">
      {error}
    </div>
  {/if}

  <form on:submit|preventDefault={handleRegister} class="space-y-6">
    <div class="grid grid-cols-2 gap-4">
      <div>
        <label for="firstName" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
          First Name
        </label>
        <input
          id="firstName"
          type="text"
          bind:value={firstName}
          on:keydown={handleKeydown}
          class="mt-1 block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
          placeholder="First name"
        />
      </div>

      <div>
        <label for="lastName" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
          Last Name
        </label>
        <input
          id="lastName"
          type="text"
          bind:value={lastName}
          on:keydown={handleKeydown}
          class="mt-1 block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
          placeholder="Last name"
        />
      </div>
    </div>

    <div>
      <label for="email" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        Email *
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
      <label for="username" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        Username *
      </label>
      <input
        id="username"
        type="text"
        bind:value={username}
        on:keydown={handleKeydown}
        required
        class="mt-1 block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
        placeholder="Choose a username"
      />
    </div>

    <div>
      <label for="password" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        Password *
      </label>
      <input
        id="password"
        type="password"
        bind:value={password}
        on:keydown={handleKeydown}
        required
        class="mt-1 block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
        placeholder="Create a password"
      />
    </div>

    <div>
      <label for="confirmPassword" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        Confirm Password *
      </label>
      <input
        id="confirmPassword"
        type="password"
        bind:value={confirmPassword}
        on:keydown={handleKeydown}
        required
        class="mt-1 block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
        placeholder="Confirm your password"
      />
    </div>

    <div>
      <button
        type="submit"
        disabled={isLoading}
        class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {isLoading ? 'Creating account...' : 'Create Account'}
      </button>
    </div>
  </form>

  <div class="mt-6 text-center">
    <p class="text-sm text-gray-600 dark:text-gray-400">
      Already have an account?
      <a href="/login" class="font-medium text-blue-600 hover:text-blue-500">
        Sign in
      </a>
    </p>
  </div>
</div>
</RouteGuard>