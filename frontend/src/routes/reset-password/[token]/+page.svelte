<script lang="ts">
  import { auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { RouteGuard } from '$lib/components';
  import { onMount } from 'svelte';

  let token: string | undefined;
  let newPassword = '';
  let confirmPassword = '';
  let isLoading = false;
  let error = '';
  let success = false;
  let tokenValid = false;
  let validatingToken = true;

  onMount(async () => {
    token = $page.params.token;
    if (!token) {
      error = 'Invalid reset link';
      validatingToken = false;
      return;
    }

    // Validate the token
    try {
      await auth.validateResetToken(token);
      tokenValid = true;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Invalid or expired reset token';
      tokenValid = false;
    } finally {
      validatingToken = false;
    }
  });

  async function handleResetPassword() {
    if (!newPassword || !confirmPassword) {
      error = 'Please fill in all fields';
      return;
    }

    if (newPassword.length < 8) {
      error = 'Password must be at least 8 characters long';
      return;
    }

    if (newPassword !== confirmPassword) {
      error = 'Passwords do not match';
      return;
    }

    if (!token) {
      error = 'Invalid reset token';
      return;
    }

    isLoading = true;
    error = '';

    try {
      await auth.resetPassword(token, newPassword);
      success = true;
      
      // Redirect to login page after a short delay
      setTimeout(() => {
        goto('/login');
      }, 3000);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to reset password';
    } finally {
      isLoading = false;
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      handleResetPassword();
    }
  }
</script>

<svelte:head>
  <title>Reset Password - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={false}>
<div class="max-w-md mx-auto bg-white rounded-lg shadow-md p-6">
  <div class="text-center mb-8">
    <h1 class="text-2xl font-bold text-gray-900">
      Reset Password
    </h1>
    <p class="text-gray-600 mt-2">
      Enter your new password below
    </p>
  </div>

  {#if validatingToken}
    <div class="text-center py-8">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto"></div>
      <p class="mt-4 text-gray-600">Validating reset token...</p>
    </div>
  {:else if success}
    <div class="mb-6 p-4 bg-green-100 border border-green-400 text-green-700 rounded">
      <div class="flex">
        <div class="flex-shrink-0">
          <svg class="h-5 w-5 text-green-400" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/>
          </svg>
        </div>
        <div class="ml-3">
          <h3 class="text-sm font-medium text-green-800">Password reset successful!</h3>
          <p class="mt-1 text-sm text-green-700">
            Your password has been successfully reset. You will be redirected to the login page in a few seconds.
          </p>
        </div>
      </div>
    </div>
  {:else if !tokenValid}
    <div class="mb-6 p-4 bg-red-100 border border-red-400 text-red-700 rounded">
      <div class="flex">
        <div class="flex-shrink-0">
          <svg class="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd"/>
          </svg>
        </div>
        <div class="ml-3">
          <h3 class="text-sm font-medium text-red-800">Invalid Reset Link</h3>
          <p class="mt-1 text-sm text-red-700">
            {error || 'This password reset link is invalid or has expired. Please request a new one.'}
          </p>
        </div>
      </div>
    </div>
    <div class="text-center">
      <a href="/forgot-password" class="font-medium text-blue-600 hover:text-blue-500">
        Request New Reset Link
      </a>
    </div>
  {:else}
    {#if error}
      <div class="mb-4 p-3 bg-red-100 border border-red-400 text-red-700 rounded">
        {error}
      </div>
    {/if}

    <form on:submit|preventDefault={handleResetPassword} class="space-y-6">
      <div>
        <label for="newPassword" class="block text-sm font-medium text-gray-700">
          New Password
        </label>
        <input
          id="newPassword"
          type="password"
          bind:value={newPassword}
          on:keydown={handleKeydown}
          required
          minlength="8"
          class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
          placeholder="Enter your new password"
        />
        <p class="mt-1 text-xs text-gray-500">
          Password must be at least 8 characters long
        </p>
      </div>

      <div>
        <label for="confirmPassword" class="block text-sm font-medium text-gray-700">
          Confirm New Password
        </label>
        <input
          id="confirmPassword"
          type="password"
          bind:value={confirmPassword}
          on:keydown={handleKeydown}
          required
          minlength="8"
          class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
          placeholder="Confirm your new password"
        />
      </div>

      <div>
        <button
          type="submit"
          disabled={isLoading}
          class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {isLoading ? 'Resetting...' : 'Reset Password'}
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