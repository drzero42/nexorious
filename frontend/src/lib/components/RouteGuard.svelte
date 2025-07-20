<script lang="ts">
  import { auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';

  export let redirectTo: string = '/login';
  export let requireAuth: boolean = true;
  export let requireAdmin: boolean = false;

  let isAuthorized: boolean = false;
  let isLoading: boolean = true;

  onMount(async () => {
    if (!browser) return;

    // Check if user is authenticated
    const authState = auth.value;
    
    // If we have tokens but no user, try to refresh
    if (authState.accessToken && authState.refreshToken && !authState.user) {
      try {
        await auth.refreshAuth();
      } catch (error) {
        console.error('Failed to refresh auth:', error);
        if (requireAuth) {
          goto(redirectTo);
          return;
        }
      }
    }

    // Check authentication requirements
    if (requireAuth && !authState.user) {
      goto(redirectTo);
      return;
    }

    // Check admin requirements
    if (requireAdmin && (!authState.user || !authState.user.isAdmin)) {
      goto('/'); // Redirect to home if not admin
      return;
    }

    // If we don't require auth and user is authenticated, might want to redirect
    if (!requireAuth && authState.user) {
      // This is useful for login/register pages when user is already logged in
      goto('/games');
      return;
    }

    isAuthorized = true;
    isLoading = false;
  });

  // Watch for auth state changes
  $: {
    if (browser && !isLoading) {
      const authState = auth.value;
      
      if (requireAuth && !authState.user) {
        goto(redirectTo);
      } else if (requireAdmin && (!authState.user || !authState.user.isAdmin)) {
        goto('/');
      }
    }
  }
</script>

{#if isLoading}
  <div>
    <div></div>
  </div>
{:else if isAuthorized}
  <slot />
{:else}
  <!-- This should not normally be reached due to redirects, but just in case -->
  <div>
    <div>
      <h1>
        Access Denied
      </h1>
      <p>
        You don't have permission to access this page.
      </p>
      <button
        on:click={() => goto('/')}
      >
        Go Home
      </button>
    </div>
  </div>
{/if}