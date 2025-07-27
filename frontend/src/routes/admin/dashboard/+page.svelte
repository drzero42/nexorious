<script lang="ts">
  import { onMount } from 'svelte';
  import { admin, auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';

  let isLoading = true;

  // Reactive statements to track admin store state
  $: adminState = admin.value;
  $: statistics = adminState.statistics;
  $: error = adminState.error;

  onMount(async () => {
    // Check if user is admin
    if (!auth.value.user?.isAdmin) {
      goto('/dashboard');
      return;
    }

    try {
      await admin.fetchStatistics();
    } catch (err) {
      console.error('Failed to fetch admin statistics:', err);
    } finally {
      isLoading = false;
    }
  });

  function formatDate(dateString: string) {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  }
</script>

<RouteGuard requireAdmin={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div class="border-b border-gray-200 pb-5">
      <h1 class="text-3xl font-bold leading-tight text-gray-900">Admin Dashboard</h1>
      <p class="mt-2 max-w-4xl text-sm text-gray-500">
        System overview and management tools
      </p>
    </div>

    {#if error}
      <div class="rounded-md bg-red-50 p-4">
        <div class="flex">
          <div class="ml-3">
            <h3 class="text-sm font-medium text-red-800">Error loading dashboard</h3>
            <div class="mt-2 text-sm text-red-700">
              <p>{error}</p>
            </div>
            <div class="mt-4">
              <button
                on:click={() => admin.clearError()}
                type="button"
                class="rounded-md bg-red-50 text-red-800 hover:bg-red-100 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 px-3 py-2 text-sm font-medium"
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      </div>
    {/if}

    {#if isLoading || adminState.isLoading}
      <div class="flex justify-center py-12">
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600" role="status" aria-label="Loading"></div>
      </div>
    {:else if statistics}
      <!-- Statistics Cards -->
      <div class="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4">
        <!-- Total Users -->
        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <div class="text-2xl">👥</div>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">Total Users</dt>
                  <dd class="text-lg font-medium text-gray-900">{statistics.totalUsers}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>

        <!-- Total Admins -->
        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <div class="text-2xl">👑</div>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">Admin Users</dt>
                  <dd class="text-lg font-medium text-gray-900">{statistics.totalAdmins}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>

        <!-- Total Games -->
        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <div class="text-2xl">🎮</div>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">Total Games</dt>
                  <dd class="text-lg font-medium text-gray-900">{statistics.totalGames || 'N/A'}</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>

        <!-- System Status -->
        <div class="bg-white overflow-hidden shadow rounded-lg">
          <div class="p-5">
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <div class="text-2xl">✅</div>
              </div>
              <div class="ml-5 w-0 flex-1">
                <dl>
                  <dt class="text-sm font-medium text-gray-500 truncate">System Status</dt>
                  <dd class="text-lg font-medium text-green-600">Healthy</dd>
                </dl>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Recent Users -->
      {#if statistics.recentUsers && statistics.recentUsers.length > 0}
        <div class="bg-white shadow rounded-lg">
          <div class="px-4 py-5 sm:p-6">
            <h3 class="text-lg leading-6 font-medium text-gray-900">Recent Users</h3>
            <div class="mt-6 flow-root">
              <ul class="-my-5 divide-y divide-gray-200">
                {#each statistics.recentUsers as user}
                  <li class="py-4">
                    <div class="flex items-center space-x-4">
                      <div class="flex-shrink-0">
                        <div class="h-8 w-8 rounded-full bg-gray-200 flex items-center justify-center">
                          <span class="text-sm font-medium text-gray-700">
                            {user.username.charAt(0).toUpperCase()}
                          </span>
                        </div>
                      </div>
                      <div class="flex-1 min-w-0">
                        <p class="text-sm font-medium text-gray-900 truncate">
                          {user.username}
                          {#if user.isAdmin}
                            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 ml-2">
                              Admin
                            </span>
                          {/if}
                          {#if !user.isActive}
                            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-800 ml-2">
                              Inactive
                            </span>
                          {/if}
                        </p>
                        <p class="text-sm text-gray-500">
                          Created {formatDate(user.createdAt)}
                        </p>
                      </div>
                      <div>
                        <a
                          href="/admin/users/{user.id}"
                          class="inline-flex items-center px-2.5 py-1.5 border border-gray-300 shadow-sm text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
                        >
                          View
                        </a>
                      </div>
                    </div>
                  </li>
                {/each}
              </ul>
            </div>
            <div class="mt-6">
              <a
                href="/admin/users"
                class="w-full flex justify-center items-center px-4 py-2 border border-gray-300 shadow-sm text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
              >
                View all users
              </a>
            </div>
          </div>
        </div>
      {/if}

      <!-- Quick Actions -->
      <div class="bg-white shadow rounded-lg">
        <div class="px-4 py-5 sm:p-6">
          <h3 class="text-lg leading-6 font-medium text-gray-900">Quick Actions</h3>
          <div class="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <a
              href="/admin/users/new"
              class="inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              <span class="mr-2">👤</span>
              Create User
            </a>
            <a
              href="/admin/users"
              class="inline-flex items-center justify-center px-4 py-2 border border-gray-300 text-sm font-medium rounded-md shadow-sm text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              <span class="mr-2">👥</span>
              Manage Users
            </a>
            <a
              href="/admin/platforms"
              class="inline-flex items-center justify-center px-4 py-2 border border-gray-300 text-sm font-medium rounded-md shadow-sm text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              <span class="mr-2">🎮</span>
              Manage Platforms
            </a>
          </div>
        </div>
      </div>
    {/if}
  </div>
</RouteGuard>