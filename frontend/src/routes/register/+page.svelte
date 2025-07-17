<script lang="ts">
  import { auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';

  let email = '';
  let username = '';
  let password = '';
  let confirmPassword = '';
  let isLoading = false;
  let error = '';
  let emailValid = false;
  let usernameChecking = false;
  let usernameAvailable = false;
  let passwordStrength = 0;

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
        password
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

  // Email validation
  function validateEmail(email: string): boolean {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
  }

  // Password strength calculation (0-5)
  function calculatePasswordStrength(password: string): number {
    let strength = 0;
    if (password.length >= 8) strength++;
    if (password.length >= 12) strength++;
    if (/[a-z]/.test(password)) strength++;
    if (/[A-Z]/.test(password)) strength++;
    if (/[0-9]/.test(password)) strength++;
    if (/[^A-Za-z0-9]/.test(password)) strength++;
    return Math.min(strength, 5);
  }

  // Username availability checking (debounced)
  let usernameCheckTimeout: NodeJS.Timeout;
  
  async function checkUsernameAvailability(username: string) {
    if (username.length < 3) {
      usernameChecking = false;
      usernameAvailable = false;
      return;
    }
    
    usernameChecking = true;
    
    // Simple timeout to simulate API call
    await new Promise(resolve => setTimeout(resolve, 500));
    
    // For demo purposes, mark username as available if it doesn't contain "admin" or "test"
    usernameAvailable = !username.toLowerCase().includes('admin') && !username.toLowerCase().includes('test');
    usernameChecking = false;
  }

  function debouncedUsernameCheck(username: string) {
    clearTimeout(usernameCheckTimeout);
    usernameCheckTimeout = setTimeout(() => {
      checkUsernameAvailability(username);
    }, 500);
  }

  // Reactive statements for validation
  $: emailValid = email.length > 0 && validateEmail(email);
  $: passwordStrength = calculatePasswordStrength(password);
  $: if (username) debouncedUsernameCheck(username);
</script>

<svelte:head>
  <title>Register - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={false}>
<div class="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900 py-12 px-4 sm:px-6 lg:px-8">
  <div class="max-w-md w-full space-y-8">
    <div class="bg-white dark:bg-gray-800 shadow-xl rounded-lg overflow-hidden">
      <!-- Header -->
      <div class="bg-gray-800 dark:bg-gray-900 px-6 py-5">
        <div class="text-center">
          <h1 class="text-xl font-bold text-white">Nexorious</h1>
          <p class="text-gray-400 text-sm mt-1">Game Collection Manager</p>
        </div>
      </div>
      
      <div class="px-6 py-8">
        <!-- Form Header -->
        <div class="mb-6">
          <h2 class="text-xl font-bold text-gray-900 dark:text-white">
            Create Account
          </h2>
          <p class="text-gray-600 dark:text-gray-400 text-sm mt-1">
            Join Nexorious to start managing your game collection
          </p>
        </div>

        {#if error}
          <div class="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 rounded text-sm">
            {error}
          </div>
        {/if}

        <form on:submit|preventDefault={handleRegister} class="space-y-5">
          <!-- Email field -->
          <div>
            <label for="email" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Email Address
            </label>
            <div class="relative">
              <input
                id="email"
                type="email"
                bind:value={email}
                on:keydown={handleKeydown}
                required
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-green-500 dark:bg-gray-700 dark:text-white"
                placeholder="Enter your email address"
              />
              {#if emailValid}
                <div class="absolute inset-y-0 right-0 pr-3 flex items-center">
                  <div class="w-4 h-4 rounded-full border-2 border-green-500 flex items-center justify-center">
                    <svg class="w-2.5 h-2.5 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
              {/if}
            </div>
          </div>

          <!-- Username field -->
          <div>
            <label for="username" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Username
            </label>
            <div class="relative">
              <input
                id="username"
                type="text"
                bind:value={username}
                on:keydown={handleKeydown}
                required
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-green-500 dark:bg-gray-700 dark:text-white"
                placeholder="Choose a unique username"
              />
              {#if usernameChecking}
                <div class="absolute inset-y-0 right-0 pr-3 flex items-center">
                  <div class="w-4 h-4 rounded-full border-2 border-yellow-500 flex items-center justify-center">
                    <span class="text-yellow-500 text-xs">?</span>
                  </div>
                </div>
              {:else if username.length >= 3 && usernameAvailable}
                <div class="absolute inset-y-0 right-0 pr-3 flex items-center">
                  <div class="w-4 h-4 rounded-full border-2 border-green-500 flex items-center justify-center">
                    <svg class="w-2.5 h-2.5 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
              {:else if username.length >= 3 && !usernameAvailable}
                <div class="absolute inset-y-0 right-0 pr-3 flex items-center">
                  <div class="w-4 h-4 rounded-full border-2 border-red-500 flex items-center justify-center">
                    <svg class="w-2.5 h-2.5 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
              {/if}
            </div>
          </div>

          <!-- Password field -->
          <div>
            <label for="password" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Password
            </label>
            <input
              id="password"
              type="password"
              bind:value={password}
              on:keydown={handleKeydown}
              required
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-green-500 dark:bg-gray-700 dark:text-white"
              placeholder="Enter a strong password"
            />
            <!-- Password strength meter -->
            <div class="mt-2">
              <div class="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-1">
                <div 
                  class="h-1 rounded-full transition-all duration-300"
                  class:bg-red-500={passwordStrength < 3}
                  class:bg-yellow-500={passwordStrength >= 3 && passwordStrength < 5}
                  class:bg-green-500={passwordStrength >= 5}
                  style="width: {(passwordStrength / 5) * 100}%"
                ></div>
              </div>
              <p class="text-xs mt-1 text-red-500">
                {#if passwordStrength < 3}
                  Weak - Add numbers and symbols
                {:else if passwordStrength < 5}
                  Medium - Add more characters
                {:else}
                  Strong password
                {/if}
              </p>
            </div>
          </div>

          <!-- Confirm Password field -->
          <div>
            <label for="confirmPassword" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Confirm Password
            </label>
            <input
              id="confirmPassword"
              type="password"
              bind:value={confirmPassword}
              on:keydown={handleKeydown}
              required
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-green-500 dark:bg-gray-700 dark:text-white"
              placeholder="Confirm your password"
            />
          </div>

          <!-- Submit button -->
          <div class="pt-2">
            <button
              type="submit"
              disabled={isLoading}
              class="w-full flex justify-center py-2.5 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {#if isLoading}
                <svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Creating account...
              {:else}
                Create Account
              {/if}
            </button>
          </div>
        </form>
      </div>
      
      <!-- Footer -->
      <div class="px-6 py-4 bg-gray-50 dark:bg-gray-700 border-t border-gray-200 dark:border-gray-600 text-center">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          Already have an account? 
          <a href="/login" class="text-blue-600 hover:text-blue-500 dark:text-blue-400 dark:hover:text-blue-300 underline">
            Sign in
          </a>
        </p>
      </div>
    </div>
  </div>
</div>
</RouteGuard>