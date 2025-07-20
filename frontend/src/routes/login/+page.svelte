<script lang="ts">
 import { auth } from '$lib/stores';
 import { goto } from '$app/navigation';
 import { RouteGuard } from '$lib/components';

 let username = '';
 let password = '';
 let isLoading = false;
 let error = '';

 async function handleLogin() {
  if (!username || !password) {
   error = 'Please fill in all fields';
   return;
  }

  isLoading = true;
  error = '';

  try {
   await auth.login(username, password);
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
     <label for="username">
      Username
     </label>
     <input
      id="username"
      type="text"
      bind:value={username}
      on:keydown={handleKeydown}
      required
      placeholder="Enter your username"
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