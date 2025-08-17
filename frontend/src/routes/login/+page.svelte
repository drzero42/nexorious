<script lang="ts">
 import { auth } from '$lib/stores';
 import { goto } from '$app/navigation';
 import { RouteGuard } from '$lib/components';
 import { onMount } from 'svelte';

 let username = '';
 let password = '';
 let isLoading = false;
 let error = '';
 let usernameInput: HTMLInputElement;

 onMount(() => {
  if (usernameInput) {
   usernameInput.focus();
  }
 });

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
<div class="flex min-h-full flex-col justify-center py-12 sm:px-6 lg:px-8">
 <div class="sm:mx-auto sm:w-full sm:max-w-md">
  <div class="bg-white px-4 py-8 shadow sm:rounded-lg sm:px-10">
   <!-- Header -->
   <div class="sm:mx-auto sm:w-full sm:max-w-md mb-8">
    <h1 class="mt-6 text-center text-3xl font-bold tracking-tight text-gray-900">
     Welcome Back
    </h1>
    <p class="mt-2 text-center text-sm text-gray-600">
     Sign in to access your game collection
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

   <form onsubmit={(e) => { e.preventDefault(); handleLogin(); }} class="space-y-6">
    <div>
     <label for="username" class="form-label">
      Username
     </label>
     <input
      id="username"
      type="text"
      bind:value={username}
      bind:this={usernameInput}
      onkeydown={handleKeydown}
      required
      placeholder="Enter your username"
      class="form-input"
      disabled={isLoading}
     />
    </div>

    <div>
     <label for="password" class="form-label">
      Password
     </label>
     <input
      id="password"
      type="password"
      bind:value={password}
      onkeydown={handleKeydown}
      required
      placeholder="Enter your password"
      class="form-input"
      disabled={isLoading}
     />
    </div>

    <div>
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
       Signing in...
      {:else}
       Sign In
      {/if}
     </button>
    </div>

   </form>
  </div>
  
 </div>
</div>
</RouteGuard>