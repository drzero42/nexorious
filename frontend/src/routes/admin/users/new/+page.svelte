<script lang="ts">
  import { admin } from '$lib/stores';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';

  let username = '';
  let password = '';
  let confirmPassword = '';
  let isAdmin = false;
  let isLoading = false;
  let error = '';

  async function handleCreateUser() {
    // Reset error
    error = '';

    // Validation
    if (!username.trim()) {
      error = 'Username is required';
      return;
    }

    if (!password) {
      error = 'Password is required';
      return;
    }

    if (password !== confirmPassword) {
      error = 'Passwords do not match';
      return;
    }

    if (password.length < 6) {
      error = 'Password must be at least 6 characters long';
      return;
    }

    isLoading = true;

    try {
      await admin.createUser(username.trim(), password, isAdmin);
      // Navigate back to users list on success
      goto('/admin/users');
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create user';
    } finally {
      isLoading = false;
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      handleCreateUser();
    }
  }
</script>

<svelte:head>
  <title>Create User - Admin - Nexorious</title>
</svelte:head>

<RouteGuard requireAdmin={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div class="sm:flex sm:items-center">
      <div class="sm:flex-auto">
        <nav class="flex" aria-label="Breadcrumb">
          <ol class="flex items-center space-x-4">
            <li>
              <div>
                <a href="/admin/users" class="text-sm font-medium text-gray-500 hover:text-gray-700">
                  User Management
                </a>
              </div>
            </li>
            <li>
              <div class="flex items-center">
                <svg class="h-4 w-4 flex-shrink-0 text-gray-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z" clip-rule="evenodd" />
                </svg>
                <span class="ml-4 text-sm font-medium text-gray-700">Create User</span>
              </div>
            </li>
          </ol>
        </nav>
        <h1 class="mt-2 text-3xl font-bold leading-tight text-gray-900">Create New User</h1>
        <p class="mt-2 text-sm text-gray-700">
          Create a new user account with username and password. You can also assign admin privileges.
        </p>
      </div>
    </div>

    <!-- Form -->
    <div class="bg-white shadow rounded-lg">
      <div class="px-4 py-5 sm:p-6">
        {#if error}
          <div class="mb-6 rounded-md bg-red-50 p-4">
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

        <form on:submit|preventDefault={handleCreateUser} class="space-y-6">
          <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <!-- Username -->
            <div class="sm:col-span-2">
              <label for="username" class="form-label">
                Username <span class="text-red-500">*</span>
              </label>
              <input
                id="username"
                type="text"
                bind:value={username}
                on:keydown={handleKeydown}
                required
                placeholder="Enter username"
                class="form-input"
                disabled={isLoading}
                autocomplete="username"
              />
              <p class="mt-1 text-xs text-gray-500">
                Username will be used for login and display purposes
              </p>
            </div>

            <!-- Password -->
            <div>
              <label for="password" class="form-label">
                Password <span class="text-red-500">*</span>
              </label>
              <input
                id="password"
                type="password"
                bind:value={password}
                on:keydown={handleKeydown}
                required
                placeholder="Enter password"
                class="form-input"
                disabled={isLoading}
                autocomplete="new-password"
              />
              <p class="mt-1 text-xs text-gray-500">
                Minimum 6 characters required
              </p>
            </div>

            <!-- Confirm Password -->
            <div>
              <label for="confirm-password" class="form-label">
                Confirm Password <span class="text-red-500">*</span>
              </label>
              <input
                id="confirm-password"
                type="password"
                bind:value={confirmPassword}
                on:keydown={handleKeydown}
                required
                placeholder="Confirm password"
                class="form-input"
                disabled={isLoading}
                autocomplete="new-password"
              />
            </div>

            <!-- Admin Role -->
            <div class="sm:col-span-2">
              <div class="flex items-start">
                <div class="flex items-center h-5">
                  <input
                    id="is-admin"
                    type="checkbox"
                    bind:checked={isAdmin}
                    disabled={isLoading}
                    class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                  />
                </div>
                <div class="ml-3 text-sm">
                  <label for="is-admin" class="font-medium text-gray-700">Admin User</label>
                  <p class="text-gray-500">
                    Grant administrative privileges to this user. Admin users can manage other users and system settings.
                  </p>
                </div>
              </div>
            </div>
          </div>

          <!-- Form Actions -->
          <div class="flex justify-end space-x-3 pt-6 border-t border-gray-200">
            <button
              type="button"
              on:click={() => goto('/admin/users')}
              disabled={isLoading}
              class="inline-flex justify-center py-2 px-4 border border-gray-300 shadow-sm text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isLoading}
              class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isLoading}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Creating User...
              {:else}
                Create User
              {/if}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</RouteGuard>