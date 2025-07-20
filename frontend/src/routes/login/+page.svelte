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
<div>
 <div>
  <div>
   <!-- Header -->
   <div>
    <h1>
     Welcome Back
    </h1>
    <p>
     Sign in to access your game collection
    </p>
   </div>

   {#if error}
    <div>
     <div>
      {error}
     </div>
    </div>
   {/if}

   <form on:submit|preventDefault={handleLogin}>
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
      placeholder="Enter your email"
     />
    </div>

    <div>
     <label for="password">
      Password
     </label>
     <input
      id="password"
      type="password"
      bind:value={password}
      on:keydown={handleKeydown}
      required
      placeholder="Enter your password"
     />
    </div>

    <div>
     <button
      type="submit"
      disabled={isLoading}
     >
      {#if isLoading}
       Signing in...
      {:else}
       Sign In
      {/if}
     </button>
    </div>

    <div>
     <a href="/forgot-password">
      Forgot your password?
     </a>
    </div>
   </form>
  </div>
  
  <div>
   <p>
    Don't have an account?
    <a href="/register">
     Sign up
    </a>
   </p>
  </div>
 </div>
</div>
</RouteGuard>