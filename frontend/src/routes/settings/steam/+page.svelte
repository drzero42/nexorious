<script lang="ts">
  import { onMount } from 'svelte';
  import { RouteGuard } from '$lib/components';
  import { steam, ui } from '$lib/stores';
  import type { SteamUserInfo } from '$lib/stores';

  // Form state
  let webApiKey = '';
  let steamId = '';
  let vanityUrl = '';
  let showApiKey = false;
  let isSubmitting = false;
  let isDeleting = false;

  // Validation state
  let apiKeyError = '';
  let steamIdError = '';
  let formError = '';

  // State for vanity URL resolution
  let showVanityResolver = false;

  onMount(async () => {
    try {
      await steam.getConfig();
      
      // If we have existing config, populate Steam ID
      if (steam.value.config?.steam_id) {
        steamId = steam.value.config.steam_id;
      }
    } catch (error) {
      // Config doesn't exist yet, that's fine
    }
  });

  // Reactive validation
  $: validateApiKey(webApiKey);
  $: validateSteamId(steamId);

  function validateApiKey(key: string) {
    apiKeyError = '';
    if (key && key.length !== 32) {
      apiKeyError = 'Steam Web API key must be exactly 32 characters';
    } else if (key && !/^[a-zA-Z0-9]+$/.test(key)) {
      apiKeyError = 'Steam Web API key must contain only alphanumeric characters';
    }
  }

  function validateSteamId(id: string) {
    steamIdError = '';
    if (id && (id.length !== 17 || !/^\d+$/.test(id))) {
      steamIdError = 'Steam ID must be exactly 17 digits';
    } else if (id && !id.startsWith('7656119')) {
      steamIdError = 'Invalid Steam ID format';
    }
  }

  async function handleVerify() {
    if (!webApiKey || apiKeyError) {
      formError = 'Please enter a valid Steam Web API key';
      return;
    }

    try {
      formError = '';
      await steam.verify(webApiKey, steamId || undefined);
      
      if (!steam.value.verificationResult?.is_valid) {
        formError = steam.value.verificationResult?.error_message || 'Verification failed';
      }
    } catch (error) {
      formError = error instanceof Error ? error.message : 'Verification failed';
    }
  }

  async function handleSave() {
    if (!webApiKey || apiKeyError || (steamId && steamIdError)) {
      formError = 'Please fix validation errors before saving';
      return;
    }

    isSubmitting = true;
    try {
      formError = '';
      await steam.setConfig(webApiKey, steamId || undefined);
      ui.showSuccess('Steam configuration saved successfully!');
      
      // Clear form
      webApiKey = '';
      showApiKey = false;
      steam.clearVerification();
    } catch (error) {
      formError = error instanceof Error ? error.message : 'Failed to save configuration';
    } finally {
      isSubmitting = false;
    }
  }

  async function handleDelete() {
    if (!confirm('Are you sure you want to delete your Steam configuration? This cannot be undone.')) {
      return;
    }

    isDeleting = true;
    try {
      await steam.deleteConfig();
      ui.showSuccess('Steam configuration deleted successfully');
      
      // Clear form
      webApiKey = '';
      steamId = '';
      showApiKey = false;
      steam.clearVerification();
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to delete configuration';
      ui.showError(errorMessage);
    } finally {
      isDeleting = false;
    }
  }

  async function handleResolveVanity() {
    if (!vanityUrl.trim()) {
      return;
    }

    try {
      const result = await steam.resolveVanityUrl(vanityUrl.trim());
      
      if (result.success && result.steam_id) {
        steamId = result.steam_id;
        vanityUrl = '';
        showVanityResolver = false;
        ui.showSuccess('Steam ID resolved successfully!');
      } else {
        ui.showError(result.error_message || 'Failed to resolve vanity URL');
      }
    } catch (error) {
      ui.showError(error instanceof Error ? error.message : 'Failed to resolve vanity URL');
    }
  }

  function clearVerification() {
    steam.clearVerification();
    formError = '';
  }

  // Get current config for display
  $: currentConfig = steam.value.config;
  $: hasConfig = currentConfig?.has_api_key;
  $: verificationResult = steam.value.verificationResult;
  $: steamUserInfo = verificationResult?.steam_user_info as SteamUserInfo | undefined;
</script>

<svelte:head>
  <title>Steam Settings - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div>
      <nav class="flex text-sm text-gray-500" aria-label="Breadcrumb">
        <ol class="inline-flex items-center space-x-1 md:space-x-3">
          <li>
            <a href="/profile" class="hover:text-gray-700">Settings</a>
          </li>
          <li>
            <span>›</span>
          </li>
          <li>
            <span class="text-gray-900 font-medium">Steam</span>
          </li>
        </ol>
      </nav>
      
      <div class="mt-4">
        <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">
          Steam Configuration
        </h1>
        <p class="mt-1 text-sm text-gray-500">
          Configure your Steam Web API key to import games from your Steam library
        </p>
      </div>
    </div>

    <!-- Current Configuration Status -->
    {#if hasConfig}
      <div class="card">
        <div class="border-b border-gray-200 pb-4 mb-4">
          <h2 class="text-lg font-semibold text-gray-900">Current Configuration</h2>
        </div>
        
        <div class="space-y-4">
          <div>
            <div class="form-label">Steam Web API Key</div>
            <div class="mt-1 p-3 bg-gray-50 border border-gray-300 rounded-md text-gray-700 font-mono text-sm">
              {currentConfig?.api_key_masked || 'Not configured'}
            </div>
          </div>

          {#if currentConfig?.steam_id}
            <div>
              <div class="form-label">Steam ID</div>
              <div class="mt-1 p-3 bg-gray-50 border border-gray-300 rounded-md text-gray-700 font-mono text-sm">
                {currentConfig.steam_id}
              </div>
            </div>
          {/if}

          <div class="flex items-center space-x-2">
            <div class="form-label">Status</div>
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {currentConfig?.is_verified ? 'bg-green-100 text-green-800' : 'bg-yellow-100 text-yellow-800'}">
              {currentConfig?.is_verified ? 'Verified' : 'Not Verified'}
            </span>
          </div>

          {#if currentConfig?.configured_at}
            <div>
              <div class="form-label">Last Updated</div>
              <div class="mt-1 text-sm text-gray-600">
                {currentConfig.configured_at.toLocaleString()}
              </div>
            </div>
          {/if}

          <div class="pt-4 flex space-x-3">
            {#if currentConfig?.is_verified && currentConfig?.steam_id}
              <a
                href="/settings/steam/import"
                class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
              >
                <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                </svg>
                Import Library
              </a>
            {/if}
            <button
              on:click={handleDelete}
              disabled={isDeleting}
              class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isDeleting}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Deleting...
              {:else}
                Delete Configuration
              {/if}
            </button>
          </div>
        </div>
      </div>
    {/if}

    <!-- Configuration Form -->
    <div class="card">
      <div class="border-b border-gray-200 pb-4 mb-6">
        <h2 class="text-lg font-semibold text-gray-900">
          {hasConfig ? 'Update' : 'Add'} Steam Configuration
        </h2>
        <p class="mt-1 text-sm text-gray-500">
          You'll need a Steam Web API key to import your Steam library. 
          <a href="https://steamcommunity.com/dev/apikey" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-500">
            Get your API key here
          </a>
        </p>
      </div>

      <!-- Steam Web API Key -->
      <div class="mb-4">
        <label for="webApiKey" class="form-label">Steam Web API Key *</label>
        <div class="mt-1 relative">
          <input
            id="webApiKey"
            type={showApiKey ? 'text' : 'password'}
            bind:value={webApiKey}
            placeholder="Enter your 32-character Steam Web API key"
            class="form-input pr-10"
            class:border-red-500={apiKeyError}
            class:border-green-500={webApiKey && !apiKeyError}
          />
          <button
            type="button"
            on:click={() => showApiKey = !showApiKey}
            class="absolute inset-y-0 right-0 flex items-center pr-3"
            aria-label={showApiKey ? 'Hide API key' : 'Show API key'}
          >
            {#if showApiKey}
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
        {#if apiKeyError}
          <p class="mt-2 text-sm text-red-600">{apiKeyError}</p>
        {/if}
      </div>

      <!-- Steam ID -->
      <div class="mb-4">
        <div class="flex items-center justify-between">
          <label for="steamId" class="form-label">Steam ID (Optional)</label>
          <button
            type="button"
            on:click={() => showVanityResolver = !showVanityResolver}
            class="text-sm text-primary-600 hover:text-primary-500"
          >
            {showVanityResolver ? 'Hide' : 'Resolve from vanity URL'}
          </button>
        </div>
        
        {#if showVanityResolver}
          <div class="mt-2 p-3 bg-gray-50 rounded-md">
            <div class="flex space-x-2">
              <input
                type="text"
                bind:value={vanityUrl}
                placeholder="Enter your Steam vanity URL or custom ID"
                class="flex-1 form-input text-sm"
              />
              <button
                type="button"
                on:click={handleResolveVanity}
                disabled={steam.value.isResolvingVanity || !vanityUrl.trim()}
                class="btn-secondary text-sm disabled:opacity-50"
              >
                {#if steam.value.isResolvingVanity}
                  <svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                {:else}
                  Resolve
                {/if}
              </button>
            </div>
            <p class="mt-1 text-xs text-gray-500">
              Example: "mynickname" from steamcommunity.com/id/mynickname
            </p>
          </div>
        {/if}

        <div class="mt-1">
          <input
            id="steamId"
            type="text"
            bind:value={steamId}
            placeholder="76561198123456789"
            class="form-input"
            class:border-red-500={steamIdError}
            class:border-green-500={steamId && !steamIdError}
          />
        </div>
        {#if steamIdError}
          <p class="mt-2 text-sm text-red-600">{steamIdError}</p>
        {:else}
          <p class="mt-2 text-sm text-gray-500">
            17-digit Steam ID for importing your library. Leave empty if you only want to verify the API key.
          </p>
        {/if}
      </div>

      <!-- Verification Section -->
      {#if webApiKey && !apiKeyError}
        <div class="mb-6 p-4 bg-blue-50 rounded-md">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-sm font-medium text-blue-900">Test Configuration</h3>
              <p class="text-sm text-blue-700">Verify your settings before saving</p>
            </div>
            <button
              type="button"
              on:click={handleVerify}
              disabled={steam.value.isVerifying}
              class="btn-primary text-sm disabled:opacity-50"
            >
              {#if steam.value.isVerifying}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Verifying...
              {:else}
                Verify Configuration
              {/if}
            </button>
          </div>

          <!-- Verification Results -->
          {#if verificationResult}
            <div class="mt-4">
              {#if verificationResult.is_valid}
                <div class="flex items-center text-green-700">
                  <svg class="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                  </svg>
                  Configuration is valid!
                </div>

                {#if steamUserInfo}
                  <div class="mt-3 p-3 bg-white rounded border">
                    <div class="flex items-center space-x-3">
                      <img 
                        src={steamUserInfo.avatar_medium} 
                        alt="Steam avatar" 
                        class="w-10 h-10 rounded"
                      />
                      <div>
                        <div class="font-medium text-gray-900">{steamUserInfo.persona_name}</div>
                        <a 
                          href={steamUserInfo.profile_url} 
                          target="_blank" 
                          rel="noopener noreferrer"
                          class="text-sm text-primary-600 hover:text-primary-500"
                        >
                          View Steam Profile →
                        </a>
                      </div>
                    </div>
                  </div>
                {/if}
              {:else}
                <div class="flex items-center text-red-700">
                  <svg class="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
                  </svg>
                  {verificationResult.error_message}
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Form Error -->
      {#if formError}
        <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-md">
          <div class="flex">
            <svg class="h-5 w-5 text-red-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
            </svg>
            <p class="text-sm text-red-800">{formError}</p>
          </div>
        </div>
      {/if}

      <!-- Action Buttons -->
      <div class="flex space-x-3">
        <button
          on:click={handleSave}
          disabled={!webApiKey || !!apiKeyError || (steamId && !!steamIdError) || isSubmitting}
          class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {#if isSubmitting}
            <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            Saving...
          {:else}
            Save Configuration
          {/if}
        </button>
        
        {#if verificationResult}
          <button
            type="button"
            on:click={clearVerification}
            class="btn-secondary"
          >
            Clear Verification
          </button>
        {/if}
      </div>
    </div>

    <!-- Help Information -->
    <div class="card max-w-2xl">
      <h3 class="text-sm font-semibold text-gray-900 mb-3">Getting Your Steam Web API Key</h3>
      <div class="text-sm text-gray-600 space-y-2">
        <p>1. Go to <a href="https://steamcommunity.com/dev/apikey" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-500">Steam Web API Key page</a></p>
        <p>2. Log in with your Steam account</p>
        <p>3. Enter any domain name (e.g., "localhost" for personal use)</p>
        <p>4. Copy the generated 32-character API key</p>
      </div>
      
      <h3 class="text-sm font-semibold text-gray-900 mb-3 mt-6">Finding Your Steam ID</h3>
      <div class="text-sm text-gray-600 space-y-2">
        <p>• Use the vanity URL resolver above if you have a custom Steam URL</p>
        <p>• Or visit <a href="https://steamid.io/" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-500">SteamID.io</a> to find your 17-digit Steam ID</p>
        <p>• Your profile must be public to import your library</p>
      </div>
    </div>
  </div>
</RouteGuard>