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
<div class="max-w-md mx-auto">
  <div class="card animate-fade-in">
    <div class="card-body">
      <!-- Header -->
      <div class="text-center mb-8">
        <div class="w-16 h-16 bg-gradient-gaming rounded-full flex items-center justify-center mx-auto mb-4">
          <svg class="w-8 h-8 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
          </svg>
        </div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          Create Account
        </h1>
        <p class="text-gray-600 dark:text-gray-400 mt-2">
          Join Nexorious to manage your game collection
        </p>
      </div>

      {#if error}
        <div class="mb-6 p-4 bg-error-50 dark:bg-error-900/20 border border-error-200 dark:border-error-800 text-error-700 dark:text-error-400 rounded-lg">
          <div class="flex items-center">
            <svg class="w-5 h-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
            </svg>
            {error}
          </div>
        </div>
      {/if}

      <form on:submit|preventDefault={handleRegister} class="space-y-6">
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label for="firstName" class="form-label">
              First Name
            </label>
            <input
              id="firstName"
              type="text"
              bind:value={firstName}
              on:keydown={handleKeydown}
              class="form-input"
              placeholder="First name"
            />
          </div>

          <div>
            <label for="lastName" class="form-label">
              Last Name
            </label>
            <input
              id="lastName"
              type="text"
              bind:value={lastName}
              on:keydown={handleKeydown}
              class="form-input"
              placeholder="Last name"
            />
          </div>
        </div>

        <div>
          <label for="email" class="form-label">
            Email Address <span class="text-error-500">*</span>
          </label>
          <input
            id="email"
            type="email"
            bind:value={email}
            on:keydown={handleKeydown}
            required
            class="form-input"
            placeholder="Enter your email"
          />
        </div>

        <div>
          <label for="username" class="form-label">
            Username <span class="text-error-500">*</span>
          </label>
          <input
            id="username"
            type="text"
            bind:value={username}
            on:keydown={handleKeydown}
            required
            class="form-input"
            placeholder="Choose a username"
          />
        </div>

        <div>
          <label for="password" class="form-label">
            Password <span class="text-error-500">*</span>
          </label>
          <input
            id="password"
            type="password"
            bind:value={password}
            on:keydown={handleKeydown}
            required
            class="form-input"
            placeholder="Create a password"
          />
          <p class="form-help">
            Password must be at least 8 characters long
          </p>
        </div>

        <div>
          <label for="confirmPassword" class="form-label">
            Confirm Password <span class="text-error-500">*</span>
          </label>
          <input
            id="confirmPassword"
            type="password"
            bind:value={confirmPassword}
            on:keydown={handleKeydown}
            required
            class="form-input"
            placeholder="Confirm your password"
          />
        </div>

        <div>
          <button
            type="submit"
            disabled={isLoading}
            class="btn btn-primary btn-lg w-full"
          >
            {#if isLoading}
              <svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Creating account...
            {:else}
              <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
              </svg>
              Create Account
            {/if}
          </button>
        </div>
      </form>
    </div>
    
    <div class="card-footer text-center">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        Already have an account?
        <a href="/login" class="font-medium text-primary-600 hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300">
          Sign in
        </a>
      </p>
    </div>
  </div>
</div>
</RouteGuard>