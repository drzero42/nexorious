<script lang="ts">
  import { auth, ui } from '$lib/stores';
  import { RouteGuard } from '$lib/components';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  // Username validation state
  let newUsername = '';
  let isCheckingUsername = false;
  let usernameAvailable: boolean | null = null;
  let usernameError = '';
  
  // Password change state
  let currentPassword = '';
  let newPassword = '';
  let confirmPassword = '';
  let showCurrentPassword = false;
  let showNewPassword = false;
  let showConfirmPassword = false;
  let passwordError = '';
  let passwordSuccess = false;

  // Form loading states
  let isSubmittingUsername = false;
  let isSubmittingPassword = false;

  // Validation timeout
  let usernameTimeout: ReturnType<typeof setTimeout>;

  onMount(() => {
    // Initialize username field with current username
    if (auth.value.user) {
      newUsername = auth.value.user.username;
    }
  });

  // Password strength calculation
  $: passwordStrength = calculatePasswordStrength(newPassword);
  $: passwordsMatch = newPassword && confirmPassword && newPassword === confirmPassword;

  function calculatePasswordStrength(password: string): { score: number; label: string; color: string } {
    let score = 0;
    
    if (password.length >= 8) score += 1;
    if (password.length >= 12) score += 1;
    if (/[A-Z]/.test(password)) score += 1;
    if (/[a-z]/.test(password)) score += 1;
    if (/[0-9]/.test(password)) score += 1;
    if (/[^A-Za-z0-9]/.test(password)) score += 1;

    if (score <= 2) return { score, label: 'Weak', color: 'bg-red-500' };
    if (score <= 4) return { score, label: 'Medium', color: 'bg-yellow-500' };
    return { score, label: 'Strong', color: 'bg-green-500' };
  }

  // Debounced username availability check
  async function checkUsername() {
    if (!newUsername || newUsername === auth.value.user?.username) {
      usernameAvailable = null;
      usernameError = '';
      return;
    }

    if (newUsername.length < 3) {
      usernameAvailable = false;
      usernameError = 'Username must be at least 3 characters';
      return;
    }

    clearTimeout(usernameTimeout);
    usernameTimeout = setTimeout(async () => {
      isCheckingUsername = true;
      try {
        const result = await auth.checkUsernameAvailability(newUsername);
        usernameAvailable = result.available;
        usernameError = result.available ? '' : 'Username is already taken';
      } catch (error) {
        usernameAvailable = false;
        usernameError = 'Error checking username availability';
      } finally {
        isCheckingUsername = false;
      }
    }, 500);
  }

  // Watch username changes
  $: if (newUsername !== undefined) {
    checkUsername();
  }

  async function handleUsernameSubmit() {
    if (!newUsername || newUsername === auth.value.user?.username || !usernameAvailable) {
      return;
    }

    isSubmittingUsername = true;
    try {
      await auth.changeUsername(newUsername);
      ui.showNotification('Username updated successfully!', 'success');
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to update username';
      ui.showNotification(errorMessage, 'error');
    } finally {
      isSubmittingUsername = false;
    }
  }

  async function handlePasswordSubmit() {
    passwordError = '';
    passwordSuccess = false;

    // Validation
    if (!currentPassword || !newPassword || !confirmPassword) {
      passwordError = 'All password fields are required';
      return;
    }

    if (newPassword !== confirmPassword) {
      passwordError = 'New passwords do not match';
      return;
    }

    if (newPassword.length < 8) {
      passwordError = 'New password must be at least 8 characters';
      return;
    }

    if (currentPassword === newPassword) {
      passwordError = 'New password must be different from current password';
      return;
    }

    isSubmittingPassword = true;
    try {
      await auth.changePassword(currentPassword, newPassword);
      
      // Show success message and redirect to login
      ui.showNotification('Password changed successfully! Please log in again.', 'success');
      setTimeout(() => {
        goto('/login');
      }, 2000);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to change password';
      passwordError = errorMessage;
    } finally {
      isSubmittingPassword = false;
    }
  }

  function resetPasswordForm() {
    currentPassword = '';
    newPassword = '';
    confirmPassword = '';
    passwordError = '';
    passwordSuccess = false;
  }
</script>

<svelte:head>
  <title>Profile Settings - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div>
      <nav class="flex text-sm text-gray-500" aria-label="Breadcrumb">
        <ol class="inline-flex items-center space-x-1 md:space-x-3">
          <li>
            <span>Settings</span>
          </li>
          <li>
            <span>›</span>
          </li>
          <li>
            <span class="text-gray-900 font-medium">Profile</span>
          </li>
        </ol>
      </nav>
      
      <div class="mt-4">
        <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">
          Profile Settings
        </h1>
        <p class="mt-1 text-sm text-gray-500">
          Manage your account information and security settings
        </p>
      </div>
    </div>

    <!-- Main Content Card -->
    <div class="card max-w-2xl">
      <!-- Account Information Section -->
      <div class="border-b border-gray-200 pb-8">
        <h2 class="text-lg font-semibold text-gray-900 mb-4">Account Information</h2>
        
        <!-- Current Username Display -->
        <div class="mb-6">
          <div class="form-label">Current Username</div>
          <div class="mt-1 p-3 bg-gray-50 border border-gray-300 rounded-md text-gray-700 font-medium">
            {auth.value.user?.username}
          </div>
        </div>

        <!-- New Username Input -->
        <div class="mb-4">
          <label for="newUsername" class="form-label">New Username</label>
          <div class="mt-1 relative">
            <input
              id="newUsername"
              type="text"
              bind:value={newUsername}
              placeholder="Enter new username"
              class="form-input pr-10"
              class:border-green-500={usernameAvailable === true}
              class:border-red-500={usernameAvailable === false || usernameError}
            />
            <div class="absolute inset-y-0 right-0 flex items-center pr-3">
              {#if isCheckingUsername}
                <svg class="h-5 w-5 text-gray-400 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
              {:else if usernameAvailable === true}
                <svg class="h-5 w-5 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                </svg>
              {:else if usernameAvailable === false || usernameError}
                <svg class="h-5 w-5 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
                </svg>
              {/if}
            </div>
          </div>
          {#if usernameError}
            <p class="mt-2 text-sm text-red-600">{usernameError}</p>
          {:else if usernameAvailable === true}
            <p class="mt-2 text-sm text-green-600">Username is available</p>
          {/if}
        </div>

        <!-- Update Username Button -->
        <button
          on:click={handleUsernameSubmit}
          disabled={!newUsername || newUsername === auth.value.user?.username || usernameAvailable !== true || isSubmittingUsername}
          class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {#if isSubmittingUsername}
            <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            Updating...
          {:else}
            Update Username
          {/if}
        </button>
      </div>

      <!-- Password & Security Section -->
      <div class="pt-8">
        <h2 class="text-lg font-semibold text-gray-900 mb-4">Password & Security</h2>
        
        <!-- Current Password -->
        <div class="mb-4">
          <label for="currentPassword" class="form-label">Current Password</label>
          <div class="mt-1 relative">
            <input
              id="currentPassword"
              type={showCurrentPassword ? 'text' : 'password'}
              bind:value={currentPassword}
              placeholder="Enter current password"
              class="form-input pr-10"
            />
            <button
              type="button"
              on:click={() => showCurrentPassword = !showCurrentPassword}
              class="absolute inset-y-0 right-0 flex items-center pr-3"
            >
              {#if showCurrentPassword}
                <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                </svg>
              {:else}
                <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              {/if}
            </button>
          </div>
        </div>

        <!-- New Password -->
        <div class="mb-4">
          <label for="newPassword" class="form-label">New Password</label>
          <div class="mt-1 relative">
            <input
              id="newPassword"
              type={showNewPassword ? 'text' : 'password'}
              bind:value={newPassword}
              placeholder="Enter new password"
              class="form-input pr-10"
            />
            <button
              type="button"
              on:click={() => showNewPassword = !showNewPassword}
              class="absolute inset-y-0 right-0 flex items-center pr-3"
            >
              {#if showNewPassword}
                <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                </svg>
              {:else}
                <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              {/if}
            </button>
          </div>
          
          <!-- Password Strength Meter -->
          {#if newPassword}
            <div class="mt-2">
              <div class="flex items-center space-x-2">
                <div class="flex-1 bg-gray-200 rounded-full h-2">
                  <div 
                    class="h-2 rounded-full transition-all duration-300 {passwordStrength.color}"
                    style="width: {(passwordStrength.score / 6) * 100}%"
                  ></div>
                </div>
                <span class="text-sm text-gray-600">
                  Password strength: <span class="font-medium">{passwordStrength.label}</span>
                </span>
              </div>
            </div>
          {/if}
        </div>

        <!-- Confirm New Password -->
        <div class="mb-6">
          <label for="confirmPassword" class="form-label">Confirm New Password</label>
          <div class="mt-1 relative">
            <input
              id="confirmPassword"
              type={showConfirmPassword ? 'text' : 'password'}
              bind:value={confirmPassword}
              placeholder="Confirm new password"
              class="form-input pr-10"
              class:border-green-500={passwordsMatch}
              class:border-red-500={confirmPassword && !passwordsMatch}
            />
            <button
              type="button"
              on:click={() => showConfirmPassword = !showConfirmPassword}
              class="absolute inset-y-0 right-0 flex items-center pr-3"
            >
              {#if showConfirmPassword}
                <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                </svg>
              {:else}
                <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              {/if}
            </button>
          </div>
          {#if confirmPassword && !passwordsMatch}
            <p class="mt-2 text-sm text-red-600">Passwords do not match</p>
          {/if}
        </div>

        <!-- Error Message -->
        {#if passwordError}
          <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-md">
            <div class="flex">
              <svg class="h-5 w-5 text-red-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
              </svg>
              <p class="text-sm text-red-800">{passwordError}</p>
            </div>
          </div>
        {/if}

        <!-- Buttons -->
        <div class="flex space-x-3">
          <button
            on:click={handlePasswordSubmit}
            disabled={!currentPassword || !newPassword || !confirmPassword || !passwordsMatch || isSubmittingPassword}
            class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {#if isSubmittingPassword}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Changing Password...
            {:else}
              Change Password
            {/if}
          </button>
          
          <button
            on:click={resetPasswordForm}
            type="button"
            class="btn-secondary"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>

    <!-- Requirements Info Box (Desktop) -->
    <div class="hidden lg:block">
      <div class="card max-w-sm">
        <h3 class="text-sm font-semibold text-gray-900 mb-3">Username Requirements</h3>
        <ul class="text-xs text-gray-600 space-y-1 mb-4">
          <li>• 3-100 characters long</li>
          <li>• Letters, numbers, underscore only</li>
          <li>• No spaces or special characters</li>
          <li>• Must be unique</li>
        </ul>
        
        <h3 class="text-sm font-semibold text-gray-900 mb-3">Password Requirements</h3>
        <ul class="text-xs text-gray-600 space-y-1">
          <li>• Minimum 8 characters</li>
          <li>• At least one uppercase letter</li>
          <li>• At least one number</li>
          <li>• Special character recommended</li>
        </ul>
      </div>
    </div>
  </div>
</RouteGuard>