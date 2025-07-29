<script lang="ts">
  import { auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  let username = '';
  let password = '';
  let confirmPassword = '';
  let isLoading = false;
  let error = '';
  let needsSetup: boolean | null = null; // null means we're still checking
  let usernameInput: HTMLInputElement;

  onMount(async () => {
    // Check if setup is actually needed
    const status = await auth.checkSetupStatus();
    if (!status.needs_setup) {
      // If setup is not needed, redirect to login
      goto('/login');
    } else {
      needsSetup = true;
      // Focus the username input after setup status is confirmed
      if (usernameInput) {
        usernameInput.focus();
      }
    }
  });

  function validateForm(): boolean {
    if (!username || !password || !confirmPassword) {
      error = 'Please fill in all fields';
      return false;
    }

    if (username.length < 3) {
      error = 'Username must be at least 3 characters long';
      return false;
    }

    if (password.length < 8) {
      error = 'Password must be at least 8 characters long';
      return false;
    }

    if (password !== confirmPassword) {
      error = 'Passwords do not match';
      return false;
    }

    return true;
  }

  async function handleSetup() {
    if (!validateForm()) {
      return;
    }

    isLoading = true;
    error = '';

    try {
      await auth.createInitialAdmin(username, password);
      // Redirect to login page after successful setup
      goto('/login');
    } catch (err) {
      error = err instanceof Error ? err.message : 'Setup failed';
    } finally {
      isLoading = false;
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      handleSetup();
    }
  }
</script>

<svelte:head>
  <title>Initial Setup - Nexorious</title>
</svelte:head>

{#if needsSetup === true}
<div class="flex min-h-screen flex-col justify-center bg-gray-50 py-12 sm:px-6 lg:px-8">
  <div class="sm:mx-auto sm:w-full sm:max-w-md">
    <div class="bg-white px-4 py-8 shadow sm:rounded-lg sm:px-10">
      <!-- Header -->
      <div class="sm:mx-auto sm:w-full sm:max-w-md mb-8">
        <h1 class="text-center text-3xl font-bold tracking-tight text-gray-900">
          Welcome to Nexorious
        </h1>
        <p class="mt-2 text-center text-sm text-gray-600">
          Let's set up your admin account to get started
        </p>
      </div>

      {#if error}
        <div class="mb-4 rounded-md bg-red-50 p-4">
          <div class="flex">
            <div class="flex-shrink-0">
              <svg class="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
              </svg>
            </div>
            <div class="ml-3">
              <p class="text-sm text-red-800">
                {error}
              </p>
            </div>
          </div>
        </div>
      {/if}

      <form on:submit|preventDefault={handleSetup} class="space-y-6">
        <div>
          <label for="username" class="form-label">
            Admin Username
          </label>
          <input
            id="username"
            type="text"
            bind:value={username}
            bind:this={usernameInput}
            on:keydown={handleKeydown}
            required
            minlength="3"
            placeholder="Choose a username"
            class="form-input"
            disabled={isLoading}
          />
          <p class="mt-1 text-sm text-gray-500">
            This will be your administrator username
          </p>
        </div>

        <div>
          <label for="password" class="form-label">
            Password
          </label>
          <input
            id="password"
            type="password"
            bind:value={password}
            on:keydown={handleKeydown}
            required
            minlength="8"
            placeholder="Enter a secure password"
            class="form-input"
            disabled={isLoading}
          />
          <p class="mt-1 text-sm text-gray-500">
            Must be at least 8 characters long
          </p>
        </div>

        <div>
          <label for="confirmPassword" class="form-label">
            Confirm Password
          </label>
          <input
            id="confirmPassword"
            type="password"
            bind:value={confirmPassword}
            on:keydown={handleKeydown}
            required
            placeholder="Confirm your password"
            class="form-input"
            disabled={isLoading}
          />
        </div>

        <div class="pt-4">
          <button
            type="submit"
            disabled={isLoading}
            class="flex w-full justify-center rounded-md bg-primary-500 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-primary-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {#if isLoading}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Creating Admin Account...
            {:else}
              Create Admin Account
            {/if}
          </button>
        </div>

        <div class="mt-4 text-center">
          <p class="text-sm text-gray-500">
            This account will have full administrative privileges
          </p>
        </div>
      </form>
    </div>
  </div>
</div>
{:else}
  <!-- Show loading state while checking setup status -->
  <div class="flex min-h-screen items-center justify-center">
    <div class="text-center">
      <svg class="animate-spin h-8 w-8 text-primary-500 mx-auto" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
      <p class="mt-2 text-gray-600">Checking setup status...</p>
    </div>
  </div>
{/if}