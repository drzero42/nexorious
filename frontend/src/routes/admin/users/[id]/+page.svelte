<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { admin, auth } from '$lib/stores';
  import { RouteGuard } from '$lib/components';
  import type { AdminUser, UserDeletionImpact } from '$lib/stores/admin.svelte';

  // Get user ID from route params
  const userId = $derived($page.params.id);

  let user = $state<AdminUser | null>(null);
  let isLoading = $state(true);
  let isSaving = $state(false);
  let error = $state<string | null>(null);
  let successMessage = $state<string | null>(null);
  let showDeleteDialog = $state(false);
  let deletionStep = $state(1); // 1: impact preview, 2: confirmation
  let deletionImpact = $state<UserDeletionImpact | null>(null);
  let isDeletionImpactLoading = $state(false);
  let usernameConfirmation = $state('');
  let showPasswordResetConfirmation = $state(false);
  let newPassword = $state('');

  // Form state
  let formData = $state({
    username: '',
    isActive: true,
    isAdmin: false
  });
  let originalData = $state({
    username: '',
    isActive: true,
    isAdmin: false
  });

  // Reactive statement to check if current user is editing themselves
  const isEditingSelf = $derived(userId ? auth.value.user?.id === userId : false);
  const hasChanges = $derived(
    formData.username !== originalData.username ||
    formData.isActive !== originalData.isActive ||
    formData.isAdmin !== originalData.isAdmin
  );

  onMount(async () => {
    // Check if user is admin
    if (!auth.value.user?.isAdmin) {
      goto('/dashboard');
      return;
    }

    await loadUser();
  });

  async function loadUser() {
    // Check if userId exists
    if (!userId) {
      error = 'User ID is required';
      isLoading = false;
      return;
    }

    try {
      isLoading = true;
      error = null;
      user = await admin.getUserById(userId);
      
      // Initialize form data
      formData = {
        username: user.username,
        isActive: user.isActive,
        isAdmin: user.isAdmin
      };
      originalData = { ...formData };
    } catch (err) {
      console.error('Failed to load user:', err);
      error = err instanceof Error ? err.message : 'Failed to load user';
    } finally {
      isLoading = false;
    }
  }

  async function handleSave() {
    if (!user || !hasChanges) return;

    try {
      isSaving = true;
      error = null;
      successMessage = null;

      // Validate form
      if (!formData.username.trim()) {
        throw new Error('Username is required');
      }

      // Check for self-modification restrictions
      if (isEditingSelf) {
        if (!formData.isActive) {
          throw new Error('You cannot deactivate your own account');
        }
        if (!formData.isAdmin) {
          throw new Error('You cannot remove your own admin privileges');
        }
      }

      const updatedUser = await admin.updateUser(user.id, {
        username: formData.username.trim(),
        isActive: formData.isActive,
        isAdmin: formData.isAdmin
      });

      // Update local state
      user = updatedUser;
      originalData = { ...formData };
      successMessage = 'User updated successfully';
      
      // Clear success message after 3 seconds
      setTimeout(() => {
        successMessage = null;
      }, 3000);
    } catch (err) {
      console.error('Failed to update user:', err);
      error = err instanceof Error ? err.message : 'Failed to update user';
    } finally {
      isSaving = false;
    }
  }

  async function handlePasswordReset() {
    if (!user || !newPassword.trim()) return;

    try {
      isSaving = true;
      error = null;
      successMessage = null;

      await admin.resetUserPassword(user.id, newPassword.trim());
      
      successMessage = 'Password reset successfully. User will need to log in again.';
      newPassword = '';
      showPasswordResetConfirmation = false;
      
      // Clear success message after 5 seconds
      setTimeout(() => {
        successMessage = null;
      }, 5000);
    } catch (err) {
      console.error('Failed to reset password:', err);
      error = err instanceof Error ? err.message : 'Failed to reset password';
    } finally {
      isSaving = false;
    }
  }

  async function startDeletion() {
    if (!user) return;

    showDeleteDialog = true;
    deletionStep = 1;
    usernameConfirmation = '';
    
    try {
      isDeletionImpactLoading = true;
      deletionImpact = await admin.getUserDeletionImpact(user.id);
    } catch (err) {
      console.error('Failed to get deletion impact:', err);
      error = err instanceof Error ? err.message : 'Failed to get deletion impact';
      showDeleteDialog = false;
    } finally {
      isDeletionImpactLoading = false;
    }
  }

  function goToConfirmation() {
    deletionStep = 2;
  }

  function cancelDeletion() {
    showDeleteDialog = false;
    deletionStep = 1;
    deletionImpact = null;
    usernameConfirmation = '';
    error = null;
  }

  async function confirmDeletion() {
    if (!user || !deletionImpact || usernameConfirmation !== user.username) return;

    try {
      isSaving = true;
      error = null;

      await admin.deleteUser(user.id);
      
      // Redirect to users list after successful deletion
      goto('/admin/users');
    } catch (err) {
      console.error('Failed to delete user:', err);
      error = err instanceof Error ? err.message : 'Failed to delete user';
      isSaving = false;
    }
  }

  function resetForm() {
    formData = { ...originalData };
    error = null;
    successMessage = null;
  }

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
</script>

<RouteGuard requireAdmin={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div class="sm:flex sm:items-center sm:justify-between">
      <div class="sm:flex-auto">
        <div class="flex items-center space-x-4">
          <a
            href="/admin/users"
            class="text-gray-400 hover:text-gray-600"
            title="Back to users"
          >
            ←
          </a>
          <div>
            <h1 class="text-3xl font-bold leading-tight text-gray-900">
              {#if isLoading}
                Loading User...
              {:else if user}
                Edit User: {user.username}
              {:else}
                User Not Found
              {/if}
            </h1>
            {#if user && !isLoading}
              <p class="mt-2 text-sm text-gray-700">
                User ID: {user.id} • Created {formatDate(user.createdAt)}
                {#if isEditingSelf}
                  <span class="ml-2 text-primary-600 font-medium">(This is you)</span>
                {/if}
              </p>
            {/if}
          </div>
        </div>
      </div>
    </div>

    {#if error}
      <div class="rounded-md bg-red-50 p-4">
        <div class="flex">
          <div class="ml-3">
            <h3 class="text-sm font-medium text-red-800">Error</h3>
            <div class="mt-2 text-sm text-red-700">
              <p>{error}</p>
            </div>
            <div class="mt-4">
              <button
                onclick={() => { error = null; }}
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

    {#if successMessage}
      <div class="rounded-md bg-green-50 p-4">
        <div class="flex">
          <div class="ml-3">
            <h3 class="text-sm font-medium text-green-800">Success</h3>
            <div class="mt-2 text-sm text-green-700">
              <p>{successMessage}</p>
            </div>
          </div>
        </div>
      </div>
    {/if}

    {#if isLoading}
      <div class="flex justify-center py-12">
        <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600" role="status" aria-label="Loading"></div>
      </div>
    {:else if user}
      <!-- User Edit Form -->
      <div class="bg-white shadow rounded-lg">
        <div class="px-4 py-5 sm:p-6">
          <h3 class="text-lg leading-6 font-medium text-gray-900 mb-6">User Information</h3>
          
          <!-- Current Status Display -->
          <div class="mb-6 p-4 bg-gray-50 rounded-lg">
            <div class="flex items-center space-x-4">
              <div class="h-12 w-12 rounded-full bg-gray-200 flex items-center justify-center">
                <span class="text-lg font-medium text-gray-700">
                  {user.username.charAt(0).toUpperCase()}
                </span>
              </div>
              <div>
                <p class="text-lg font-medium text-gray-900">{user.username}</p>
                <div class="flex flex-wrap gap-2 mt-1">
                  {#each getUserStatusBadge(user) as badge}
                    <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {badge.class}">
                      {badge.text}
                    </span>
                  {/each}
                </div>
                <p class="text-sm text-gray-500 mt-1">Last updated {formatDate(user.updatedAt)}</p>
              </div>
            </div>
          </div>

          <form onsubmit={(e) => { e.preventDefault(); handleSave(); }} class="space-y-6">
            <!-- Username -->
            <div>
              <label for="username" class="block text-sm font-medium leading-6 text-gray-900">Username</label>
              <div class="mt-2">
                <input
                  type="text"
                  name="username"
                  id="username"
                  bind:value={formData.username}
                  required
                  class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6"
                />
              </div>
            </div>

            <!-- Account Status -->
            <div class="space-y-4">
              <h4 class="text-sm font-medium text-gray-900">Account Status</h4>
              
              <!-- Active Status -->
              <div class="flex items-center space-x-3">
                <input
                  type="checkbox"
                  id="isActive"
                  bind:checked={formData.isActive}
                  disabled={isEditingSelf}
                  class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded disabled:opacity-50"
                />
                <label for="isActive" class="text-sm text-gray-700">
                  Account is active
                  {#if isEditingSelf}
                    <span class="text-gray-400">(Cannot modify your own account status)</span>
                  {/if}
                </label>
              </div>

              <!-- Admin Status -->
              <div class="flex items-center space-x-3">
                <input
                  type="checkbox"
                  id="isAdmin"
                  bind:checked={formData.isAdmin}
                  disabled={isEditingSelf}
                  class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded disabled:opacity-50"
                />
                <label for="isAdmin" class="text-sm text-gray-700">
                  Administrator privileges
                  {#if isEditingSelf}
                    <span class="text-gray-400">(Cannot modify your own admin privileges)</span>
                  {/if}
                </label>
              </div>
            </div>

            <!-- Form Actions -->
            <div class="flex justify-between pt-4">
              <div class="flex space-x-3">
                <button
                  type="submit"
                  disabled={!hasChanges || isSaving}
                  class="inline-flex justify-center rounded-md bg-primary-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-primary-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {#if isSaving}
                    Saving...
                  {:else}
                    Save Changes
                  {/if}
                </button>
                <button
                  type="button"
                  onclick={resetForm}
                  disabled={!hasChanges || isSaving}
                  class="inline-flex justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Reset
                </button>
              </div>
            </div>
          </form>
        </div>
      </div>

      <!-- Dangerous Actions -->
      <div class="bg-white shadow rounded-lg">
        <div class="px-4 py-5 sm:p-6">
          <h3 class="text-lg leading-6 font-medium text-gray-900 mb-6">Account Actions</h3>
          
          <div class="space-y-4">
            <!-- Password Reset -->
            <div class="border rounded-lg p-4">
              <h4 class="text-sm font-medium text-gray-900 mb-2">Reset Password</h4>
              <p class="text-sm text-gray-600 mb-4">
                Generate a new password for this user. They will need to log in again with the new password.
              </p>
              
              {#if showPasswordResetConfirmation}
                <div class="space-y-3">
                  <div>
                    <label for="newPassword" class="block text-sm font-medium leading-6 text-gray-900">New Password</label>
                    <input
                      type="password"
                      id="newPassword"
                      bind:value={newPassword}
                      placeholder="Enter new password"
                      class="mt-1 block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-primary-600 sm:text-sm sm:leading-6"
                    />
                  </div>
                  <div class="flex space-x-3">
                    <button
                      onclick={handlePasswordReset}
                      disabled={!newPassword.trim() || isSaving}
                      class="inline-flex justify-center rounded-md bg-yellow-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-yellow-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-yellow-600 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {#if isSaving}
                        Resetting...
                      {:else}
                        Reset Password
                      {/if}
                    </button>
                    <button
                      onclick={() => { showPasswordResetConfirmation = false; newPassword = ''; }}
                      disabled={isSaving}
                      class="inline-flex justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 disabled:opacity-50"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              {:else}
                <button
                  onclick={() => { showPasswordResetConfirmation = true; }}
                  disabled={isSaving}
                  class="inline-flex justify-center rounded-md bg-yellow-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-yellow-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-yellow-600 disabled:opacity-50"
                >
                  Reset Password
                </button>
              {/if}
            </div>

            <!-- Delete User -->
            {#if !isEditingSelf}
              <div class="border border-red-200 rounded-lg p-4">
                <h4 class="text-sm font-medium text-red-900 mb-2">Delete User</h4>
                <p class="text-sm text-red-600 mb-4">
                  Permanently delete this user account and all associated data. This action cannot be undone.
                </p>
                
                <button
                  onclick={startDeletion}
                  disabled={isSaving}
                  class="inline-flex justify-center rounded-md bg-red-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-red-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-red-600 disabled:opacity-50"
                >
                  Delete User
                </button>
              </div>
            {/if}
          </div>
        </div>
      </div>
    {:else}
      <!-- User Not Found -->
      <div class="bg-white shadow rounded-lg">
        <div class="px-4 py-5 sm:p-6">
          <div class="text-center py-12">
            <h3 class="text-lg font-medium text-gray-900 mb-2">User Not Found</h3>
            <p class="text-gray-500 mb-4">
              The user with ID "{userId || 'unknown'}" could not be found.
            </p>
            <a
              href="/admin/users"
              class="inline-flex items-center justify-center rounded-md bg-primary-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-primary-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600"
            >
              Back to Users
            </a>
          </div>
        </div>
      </div>
    {/if}
  </div>

  <!-- Delete User Dialog -->
  {#if showDeleteDialog}
    <div class="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity z-50" aria-labelledby="modal-title" role="dialog" aria-modal="true">
      <div class="fixed inset-0 overflow-y-auto">
        <div class="flex min-h-full items-end justify-center p-4 text-center sm:items-center sm:p-0">
          <div class="relative transform overflow-hidden rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-lg">
            
            {#if deletionStep === 1}
              <!-- Step 1: Data Impact Preview -->
              <div class="bg-white px-4 pb-4 pt-5 sm:p-6 sm:pb-4">
                <div class="sm:flex sm:items-start">
                  <div class="mx-auto flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full bg-red-100 sm:mx-0 sm:h-10 sm:w-10">
                    <svg class="h-6 w-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
                    </svg>
                  </div>
                  <div class="mt-3 text-center sm:ml-4 sm:mt-0 sm:text-left w-full">
                    <h3 class="text-lg font-semibold leading-6 text-gray-900" id="modal-title">
                      Delete User: {user?.username}
                    </h3>
                    
                    {#if isDeletionImpactLoading}
                      <div class="mt-4 flex justify-center">
                        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-red-600"></div>
                      </div>
                    {:else if deletionImpact}
                      <div class="mt-4">
                        <p class="text-sm text-gray-500 mb-4">
                          This action will permanently delete the following data:
                        </p>
                        
                        <div class="bg-red-50 border border-red-200 rounded-lg p-4 space-y-2">
                          <div class="flex justify-between">
                            <span class="text-sm text-gray-700">Games in collection:</span>
                            <span class="text-sm font-medium text-red-900">{deletionImpact.total_games}</span>
                          </div>
                          <div class="flex justify-between">
                            <span class="text-sm text-gray-700">User-created tags:</span>
                            <span class="text-sm font-medium text-red-900">{deletionImpact.total_tags}</span>
                          </div>
                          <div class="flex justify-between">
                            <span class="text-sm text-gray-700">Wishlist items:</span>
                            <span class="text-sm font-medium text-red-900">{deletionImpact.total_wishlist_items}</span>
                          </div>
                          <div class="flex justify-between">
                            <span class="text-sm text-gray-700">Import jobs:</span>
                            <span class="text-sm font-medium text-red-900">{deletionImpact.total_import_jobs}</span>
                          </div>
                          <div class="flex justify-between">
                            <span class="text-sm text-gray-700">Active sessions:</span>
                            <span class="text-sm font-medium text-red-900">{deletionImpact.total_sessions}</span>
                          </div>
                        </div>
                        
                        <div class="mt-4 p-3 bg-yellow-50 border border-yellow-200 rounded-lg">
                          <p class="text-sm text-yellow-800">
                            <strong>Warning:</strong> {deletionImpact.warning}
                          </p>
                        </div>
                      </div>
                    {/if}
                  </div>
                </div>
              </div>
              <div class="bg-gray-50 px-4 py-3 sm:flex sm:flex-row-reverse sm:px-6">
                <button
                  type="button"
                  onclick={goToConfirmation}
                  disabled={isDeletionImpactLoading || !deletionImpact}
                  class="inline-flex w-full justify-center rounded-md bg-red-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-red-500 sm:ml-3 sm:w-auto disabled:opacity-50"
                >
                  Continue
                </button>
                <button
                  type="button"
                  onclick={cancelDeletion}
                  class="mt-3 inline-flex w-full justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 sm:mt-0 sm:w-auto"
                >
                  Cancel
                </button>
              </div>
            
            {:else if deletionStep === 2}
              <!-- Step 2: Final Confirmation -->
              <div class="bg-white px-4 pb-4 pt-5 sm:p-6 sm:pb-4">
                <div class="sm:flex sm:items-start">
                  <div class="mx-auto flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full bg-red-100 sm:mx-0 sm:h-10 sm:w-10">
                    <svg class="h-6 w-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
                    </svg>
                  </div>
                  <div class="mt-3 text-center sm:ml-4 sm:mt-0 sm:text-left w-full">
                    <h3 class="text-lg font-semibold leading-6 text-gray-900">
                      Final Confirmation
                    </h3>
                    
                    <div class="mt-4">
                      <p class="text-sm text-gray-500 mb-4">
                        To confirm deletion, please type the username <strong>{user?.username}</strong> in the field below:
                      </p>
                      
                      <input
                        type="text"
                        bind:value={usernameConfirmation}
                        placeholder="Enter username to confirm"
                        class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-red-600 sm:text-sm sm:leading-6"
                      />
                      
                      <div class="mt-3 p-3 bg-red-50 border border-red-200 rounded-lg">
                        <p class="text-sm text-red-800">
                          This will permanently delete all user data and cannot be undone.
                        </p>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              <div class="bg-gray-50 px-4 py-3 sm:flex sm:flex-row-reverse sm:px-6">
                <button
                  type="button"
                  onclick={confirmDeletion}
                  disabled={isSaving || usernameConfirmation !== user?.username}
                  class="inline-flex w-full justify-center rounded-md bg-red-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-red-500 sm:ml-3 sm:w-auto disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {#if isSaving}
                    Deleting...
                  {:else}
                    Delete User
                  {/if}
                </button>
                <button
                  type="button"
                  onclick={() => deletionStep = 1}
                  disabled={isSaving}
                  class="mt-3 inline-flex w-full justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 sm:mt-0 sm:w-auto disabled:opacity-50"
                >
                  Back
                </button>
              </div>
            {/if}
            
          </div>
        </div>
      </div>
    </div>
  {/if}
</RouteGuard>