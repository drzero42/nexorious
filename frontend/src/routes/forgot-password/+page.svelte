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
<div>
  <div>
    <h1>
      Forgot Password
    </h1>
    <p>
      Enter your email address and we'll send you a link to reset your password
    </p>
  </div>

  {#if success}
    <div>
      <div>
        <div>
          ✓
        </div>
        <div>
          <h3>Email sent successfully!</h3>
          <p>{successMessage}</p>
        </div>
      </div>
    </div>
  {/if}

  {#if error}
    <div>
      {error}
    </div>
  {/if}

  {#if !success}
    <form on:submit|preventDefault={handleForgotPassword}>
      <div>
        <label for="email">
          Email Address
        </label>
        <input
          id="email"
          type="email"
          bind:value={email}
          on:keydown={handleKeydown}
          required
          placeholder="Enter your email address"
        />
      </div>

      <div>
        <button
          type="submit"
          disabled={isLoading}
        >
          {isLoading ? 'Sending...' : 'Send Reset Link'}
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