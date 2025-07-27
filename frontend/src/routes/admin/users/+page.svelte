<script lang="ts">
  import { onMount } from 'svelte';
  import { admin, auth } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import type { AdminUser } from '$lib/stores/admin.svelte';

  let isLoading = true;
  let searchQuery = '';
  let statusFilter: 'all' | 'active' | 'inactive' | 'admin' = 'all';

  // Reactive statements to track admin store state
  $: adminState = admin.value;
  $: users = adminState.users;
  $: error = adminState.error;

  // Filtered users based on search and filter criteria
  $: filteredUsers = users.filter(user => {
    const matchesSearch = user.username.toLowerCase().includes(searchQuery.toLowerCase());
    
    switch (statusFilter) {
      case 'active':
        return matchesSearch && user.isActive;
      case 'inactive':
        return matchesSearch && !user.isActive;
      case 'admin':
        return matchesSearch && user.isAdmin;
      default:
        return matchesSearch;
    }
  });

  onMount(async () => {
    // Check if user is admin
    if (!auth.value.user?.isAdmin) {
      goto('/dashboard');
      return;
    }

    try {
      await admin.fetchUsers();
    } catch (err) {
      console.error('Failed to fetch users:', err);
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

  function getUserStatusBadge(user: AdminUser) {
    const badges = [];
    
    if (user.isAdmin) {
      badges.push({ text: 'Admin', class: 'bg-purple-100 text-purple-800' });
    }
    
    if (!user.isActive) {
      badges.push({ text: 'Inactive', class: 'bg-red-100 text-red-800' });
    } else if (!user.isAdmin) {
      badges.push({ text: 'User', class: 'bg-green-100 text-green-800' });
    }
    
    return badges;
  }

  async function handleToggleUserStatus(user: AdminUser) {
    try {
      await admin.updateUser(user.id, { isActive: !user.isActive });
    } catch (error) {
      console.error('Failed to toggle user status:', error);
    }
  }
</script>

<RouteGuard requireAdmin={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div class="sm:flex sm:items-center">
      <div class="sm:flex-auto">
        <h1 class="text-3xl font-bold leading-tight text-gray-900">User Management</h1>
        <p class="mt-2 text-sm text-gray-700">
          Manage all users in the system. Create, edit, activate, and deactivate user accounts.
        </p>
      </div>
      <div class="mt-4 sm:ml-16 sm:mt-0 sm:flex-none">
        <a
          href="/admin/users/new"
          class="inline-flex items-center justify-center rounded-md bg-primary-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-primary-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600"
        >
          <span class="mr-2">+</span>
          Create User
        </a>
      </div>
    </div>

    {#if error}
      <div class="rounded-md bg-red-50 p-4">
        <div class="flex">
          <div class="ml-3">
            <h3 class="text-sm font-medium text-red-800">Error loading users</h3>
            <div class="mt-2 text-sm text-red-700">
              <p>{error}</p>
            </div>
            <div class="mt-4">
              <button
                on:click={() => admin.clearError()}
                type="button"
                class="rounded-md bg-red-50 px-3 py-2 text-sm font-medium text-red-800 hover:bg-red-100 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2"
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      </div>
    {/if}

    <!-- Search and Filters -->
    <div class="bg-white shadow rounded-lg">
      <div class="px-4 py-5 sm:p-6">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <!-- Search -->
          <div>
            <label for="search" class="block text-sm font-medium leading-6 text-gray-900">Search Users</label>
            <div class="mt-2">
              <input
                type="text"
                name="search"
                id="search"
                bind:value={searchQuery}
                placeholder="Search by username..."
                class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6"
              />
            </div>
          </div>

          <!-- Status Filter -->
          <div>
            <label for="status-filter" class="block text-sm font-medium leading-6 text-gray-900">Filter by Status</label>
            <div class="mt-2">
              <select
                id="status-filter"
                name="status-filter"
                bind:value={statusFilter}
                class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6"
              >
                <option value="all">All Users</option>
                <option value="active">Active Only</option>
                <option value="inactive">Inactive Only</option>
                <option value="admin">Admins Only</option>
              </select>
            </div>
          </div>

          <!-- Results Count -->
          <div class="flex items-end">
            <div class="text-sm text-gray-500">
              Showing {filteredUsers.length} of {users.length} users
            </div>
          </div>
        </div>
      </div>
    </div>

    {#if isLoading || adminState.isLoading}
      <div class="flex justify-center py-12">
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600" role="status" aria-label="Loading"></div>
      </div>
    {:else}
      <!-- Users Table -->
      <div class="bg-white shadow rounded-lg overflow-hidden">
        <div class="px-4 py-5 sm:p-6">
          <h3 class="text-lg leading-6 font-medium text-gray-900 mb-4">
            Users ({filteredUsers.length})
          </h3>
          
          {#if filteredUsers.length === 0}
            <div class="text-center py-12">
              <div class="text-gray-500">
                {#if searchQuery || statusFilter !== 'all'}
                  No users match your search criteria.
                {:else}
                  No users found.
                {/if}
              </div>
              {#if searchQuery || statusFilter !== 'all'}
                <button
                  on:click={() => { searchQuery = ''; statusFilter = 'all'; }}
                  class="mt-4 text-primary-600 hover:text-primary-500 text-sm font-medium"
                >
                  Clear filters
                </button>
              {/if}
            </div>
          {:else}
            <!-- Mobile Card View -->
            <div class="block sm:hidden space-y-4">
              {#each filteredUsers as user}
                <div class="bg-gray-50 rounded-lg p-4">
                  <div class="flex items-center justify-between">
                    <div class="flex items-center space-x-3">
                      <div class="h-10 w-10 rounded-full bg-gray-200 flex items-center justify-center">
                        <span class="text-sm font-medium text-gray-700">
                          {user.username.charAt(0).toUpperCase()}
                        </span>
                      </div>
                      <div>
                        <p class="text-sm font-medium text-gray-900">{user.username}</p>
                        <div class="flex flex-wrap gap-1 mt-1">
                          {#each getUserStatusBadge(user) as badge}
                            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {badge.class}">
                              {badge.text}
                            </span>
                          {/each}
                        </div>
                      </div>
                    </div>
                    <div class="flex space-x-2">
                      <a
                        href="/admin/users/{user.id}"
                        class="text-primary-600 hover:text-primary-900 text-sm font-medium"
                      >
                        View
                      </a>
                    </div>
                  </div>
                  <div class="mt-3 text-xs text-gray-500">
                    Created {formatDate(user.createdAt)}
                  </div>
                </div>
              {/each}
            </div>

            <!-- Desktop Table View -->
            <div class="hidden sm:block">
              <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50">
                  <tr>
                    <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      User
                    </th>
                    <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Status
                    </th>
                    <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Created
                    </th>
                    <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Last Updated
                    </th>
                    <th scope="col" class="relative px-6 py-3">
                      <span class="sr-only">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody class="bg-white divide-y divide-gray-200">
                  {#each filteredUsers as user}
                    <tr class="hover:bg-gray-50">
                      <td class="px-6 py-4 whitespace-nowrap">
                        <div class="flex items-center">
                          <div class="h-10 w-10 rounded-full bg-gray-200 flex items-center justify-center">
                            <span class="text-sm font-medium text-gray-700">
                              {user.username.charAt(0).toUpperCase()}
                            </span>
                          </div>
                          <div class="ml-4">
                            <div class="text-sm font-medium text-gray-900">{user.username}</div>
                            <div class="text-sm text-gray-500">ID: {user.id.substring(0, 8)}...</div>
                          </div>
                        </div>
                      </td>
                      <td class="px-6 py-4 whitespace-nowrap">
                        <div class="flex flex-wrap gap-1">
                          {#each getUserStatusBadge(user) as badge}
                            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {badge.class}">
                              {badge.text}
                            </span>
                          {/each}
                        </div>
                      </td>
                      <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                        {formatDate(user.createdAt)}
                      </td>
                      <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                        {formatDate(user.updatedAt)}
                      </td>
                      <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                        <div class="flex justify-end space-x-2">
                          <a
                            href="/admin/users/{user.id}"
                            class="text-primary-600 hover:text-primary-900"
                          >
                            View
                          </a>
                          <button
                            on:click={() => handleToggleUserStatus(user)}
                            class="text-gray-600 hover:text-gray-900"
                            title={user.isActive ? 'Deactivate user' : 'Activate user'}
                          >
                            {user.isActive ? 'Deactivate' : 'Activate'}
                          </button>
                        </div>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</RouteGuard>