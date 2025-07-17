<script lang="ts">
  import { auth } from '$lib/stores';
  import { RouteGuard } from '$lib/components';

  let email = '';
  let isLoading = false;
  let error = '';
  let success = false;
  let successMessage = '';

  async function handleForgotPassword() {
    if (!email) {
      error = 'Please enter your email address';
      return;
    }

    // Basic email validation
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailRegex.test(email)) {
      error = 'Please enter a valid email address';
      return;
    }

    isLoading = true;
    error = '';
    success = false;

    try {
      await auth.forgotPassword(email);
      success = true;
      successMessage = `Password reset instructions have been sent to ${email}. Please check your email and follow the link to reset your password.`;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to send password reset email';
    } finally {
      isLoading = false;
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      handleForgotPassword();
    }
  }
</script>

<svelte:head>
  <title>Forgot Password - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={false}>
<div class="max-w-md mx-auto bg-white rounded-lg shadow-md p-6">
  <div class="text-center mb-8">
    <h1 class="text-2xl font-bold text-gray-900">
      Forgot Password
    </h1>
    <p class="text-gray-600 mt-2">
      Enter your email address and we'll send you a link to reset your password
    </p>
  </div>

  {#if success}
    <div class="mb-6 p-4 bg-green-100 border border-green-400 text-green-700 rounded">
      <div class="flex">
        <div class="flex-shrink-0">
          <svg class="h-5 w-5 text-green-400" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/>
          </svg>
        </div>
        <div class="ml-3">
          <h3 class="text-sm font-medium text-green-800">Email sent successfully!</h3>
          <p class="mt-1 text-sm text-green-700">{successMessage}</p>
        </div>
      </div>
    </div>
  {/if}

  {#if error}
    <div class="mb-4 p-3 bg-red-100 border border-red-400 text-red-700 rounded">
      {error}
    </div>
  {/if}

  {#if !success}
    <form on:submit|preventDefault={handleForgotPassword} class="space-y-6">
      <div>
        <label for="email" class="block text-sm font-medium text-gray-700">
          Email Address
        </label>
        <input
          id="email"
          type="email"
          bind:value={email}
          on:keydown={handleKeydown}
          required
          class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
          placeholder="Enter your email address"
        />
      </div>

      <div>
        <button
          type="submit"
          disabled={isLoading}
          class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {isLoading ? 'Sending...' : 'Send Reset Link'}
        </button>
      </div>
    </form>
  {/if}

  <div class="mt-6 text-center">
    <p class="text-sm text-gray-600">
      Remember your password?
      <a href="/login" class="font-medium text-blue-600 hover:text-blue-500">
        Back to Sign In
      </a>
    </p>
  </div>
</div>
</RouteGuard>