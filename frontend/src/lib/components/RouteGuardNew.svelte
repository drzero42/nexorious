<script lang="ts">
  import { authGuard, type AuthCheckOptions } from '$lib/services/auth-guard';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import { auth } from '$lib/stores/auth.svelte';

  interface Props {
    redirectTo?: string;
    requireAuth?: boolean;
    requireAdmin?: boolean;
    skipRefresh?: boolean;
    children?: any;
  }

  let { 
    redirectTo = '/login',
    requireAuth = true,
    requireAdmin = false,
    skipRefresh = false,
    children 
  }: Props = $props();

  let isAuthorized = $state(false);
  let isLoading = $state(true);

  async function performAuthCheck() {
    if (!browser) return;

    try {
      const options: AuthCheckOptions = {
        requireAuth,
        requireAdmin,
        skipRefresh
      };

      const result = await authGuard.checkAuthentication(options);

      if (result.shouldRedirect && result.redirectTo) {
        const targetRedirect = result.redirectTo === '/login' ? redirectTo : result.redirectTo;
        await goto(targetRedirect);
        return;
      }

      isAuthorized = result.isAuthorized;
    } catch (error) {
      console.error('Auth check failed:', error);
      if (requireAuth) {
        await goto(redirectTo);
        return;
      }
    } finally {
      isLoading = false;
    }
  }

  // Initial auth check on mount
  onMount(async () => {
    await performAuthCheck();
  });

  // Watch for auth state changes and re-check authorization
  $effect(() => {
    if (browser && !isLoading) {
      const authState = auth.value;
      
      // Re-evaluate without refresh to avoid loops
      const result = authGuard.evaluateAuthState(authState, {
        requireAuth,
        requireAdmin
      });

      if (result.shouldRedirect && result.redirectTo) {
        const targetRedirect = result.redirectTo === '/login' ? redirectTo : result.redirectTo;
        goto(targetRedirect);
      }
    }
  });
</script>

{#if isLoading}
  <div class="flex items-center justify-center min-h-screen">
    <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
  </div>
{:else if isAuthorized}
  {@render children?.()}
{:else}
  <!-- This should not normally be reached due to redirects, but just in case -->
  <div class="flex items-center justify-center min-h-screen">
    <div class="text-center">
      <h1 class="text-2xl font-bold text-gray-900 mb-2">
        Access Denied
      </h1>
      <p class="text-gray-600 mb-4">
        You don't have permission to access this page.
      </p>
      <button
        onclick={() => goto('/')}
        class="btn-primary"
      >
        Go Home
      </button>
    </div>
  </div>
{/if}