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
<div>
  <div>
    <h1>
      Reset Password
    </h1>
    <p>
      Enter your new password below
    </p>
  </div>

  {#if validatingToken}
    <div>
      <div></div>
      <p>Validating reset token...</p>
    </div>
  {:else if success}
    <div>
      <div>
        <div>
          ✓
        </div>
        <div>
          <h3>Password reset successful!</h3>
          <p>
            Your password has been successfully reset. You will be redirected to the login page in a few seconds.
          </p>
        </div>
      </div>
    </div>
  {:else if !tokenValid}
    <div>
      <div>
        <div>
          ✗
        </div>
        <div>
          <h3>Invalid Reset Link</h3>
          <p>
            {error || 'This password reset link is invalid or has expired. Please request a new one.'}
          </p>
        </div>
      </div>
    </div>
    <div>
      <a href="/forgot-password">
        Request New Reset Link
      </a>
    </div>
  {:else}
    {#if error}
      <div>
        {error}
      </div>
    {/if}

    <form on:submit|preventDefault={handleResetPassword}>
      <div>
        <label for="newPassword">
          New Password
        </label>
        <input
          id="newPassword"
          type="password"
          bind:value={newPassword}
          on:keydown={handleKeydown}
          required
          minlength="8"
          placeholder="Enter your new password"
        />
        <p>
          Password must be at least 8 characters long
        </p>
      </div>

      <div>
        <label for="confirmPassword">
          Confirm New Password
        </label>
        <input
          id="confirmPassword"
          type="password"
          bind:value={confirmPassword}
          on:keydown={handleKeydown}
          required
          minlength="8"
          placeholder="Confirm your new password"
        />
      </div>

      <div>
        <button
          type="submit"
          disabled={isLoading}
        >
          {isLoading ? 'Resetting...' : 'Reset Password'}
        </button>
      </div>
    </form>
  {/if}

  <div>
    <p>
      Remember your password?
      <a href="/login">
        Back to Sign In
      </a>
    </p>
  </div>
</div>
</RouteGuard>