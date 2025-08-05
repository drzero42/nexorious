<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import { steam, ui } from '$lib/stores';
  import type { SteamLibraryImportRequest } from '$lib/stores/steam.svelte';

  // Import configuration state
  let fuzzyThreshold = 0.8;
  let mergeStrategy = 'skip';

  // Form state
  let showAdvancedSettings = false;
  let isLoadingLibrary = false;
  let hasLoadedLibrary = false;

  // Component state
  let currentStep: 'config' | 'preview' | 'importing' | 'results' = 'config';

  onMount(async () => {
    // Check if Steam is configured
    try {
      await steam.getConfig();
      
      if (!steam.value.config?.has_api_key || !steam.value.config?.steam_id) {
        ui.showError('Please configure your Steam settings first');
        goto('/settings/steam');
        return;
      }
    } catch (error) {
      ui.showError('Please configure your Steam settings first');
      goto('/settings/steam');
      return;
    }
  });

  async function handleLoadLibrary() {
    if (!steam.value.config?.has_api_key || !steam.value.config?.steam_id) {
      ui.showError('Steam configuration is missing');
      return;
    }

    isLoadingLibrary = true;
    try {
      await steam.getLibrary();
      hasLoadedLibrary = true;
      currentStep = 'preview';
      ui.showSuccess(`Loaded ${steam.value.library?.total_games || 0} games from your Steam library`);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to load Steam library';
      ui.showError(errorMessage);
    } finally {
      isLoadingLibrary = false;
    }
  }

  async function handleImportLibrary() {
    if (!steam.value.library) {
      ui.showError('Please load your Steam library first');
      return;
    }

    const importConfig: SteamLibraryImportRequest = {
      fuzzy_threshold: fuzzyThreshold,
      merge_strategy: mergeStrategy
    };

    currentStep = 'importing';
    
    try {
      await steam.importLibrary(importConfig);
      currentStep = 'results';
      ui.showSuccess('Steam library import completed!');
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to import Steam library';
      ui.showError(errorMessage);
      currentStep = 'preview';
    }
  }

  function handleStartOver() {
    steam.clearLibrary();
    hasLoadedLibrary = false;
    currentStep = 'config';
  }


  // Reactive values
  $: currentConfig = steam.value.config;
  $: library = steam.value.library;
  $: importResults = steam.value.importResults;
  $: libraryError = steam.value.libraryError;
  $: isImporting = steam.value.isImporting;
  $: isLoadingLibraryState = steam.value.isLoadingLibrary;
</script>

<svelte:head>
  <title>Steam Library Import - Nexorious</title>
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
            <a href="/settings/steam" class="hover:text-gray-700">Steam</a>
          </li>
          <li>
            <span>›</span>
          </li>
          <li>
            <span class="text-gray-900 font-medium">Import Library</span>
          </li>
        </ol>
      </nav>
      
      <div class="mt-4">
        <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">
          Import Steam Library
        </h1>
        <p class="mt-1 text-sm text-gray-500">
          Import games from your Steam library into your collection
        </p>
      </div>
    </div>

    <!-- Progress Steps -->
    <div class="bg-white border border-gray-200 rounded-lg p-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center space-x-4">
          <!-- Step 1: Configuration -->
          <div class="flex items-center">
            <div class="flex items-center justify-center w-8 h-8 rounded-full {currentStep === 'config' ? 'bg-primary-600 text-white' : (hasLoadedLibrary ? 'bg-green-500 text-white' : 'bg-gray-300 text-gray-600')}">
              {#if hasLoadedLibrary}
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              {:else}
                1
              {/if}
            </div>
            <span class="ml-2 text-sm font-medium text-gray-900">Configure</span>
          </div>

          <div class="w-16 h-0.5 bg-gray-300"></div>

          <!-- Step 2: Preview -->
          <div class="flex items-center">
            <div class="flex items-center justify-center w-8 h-8 rounded-full {currentStep === 'preview' ? 'bg-primary-600 text-white' : (currentStep === 'importing' || currentStep === 'results' ? 'bg-green-500 text-white' : 'bg-gray-300 text-gray-600')}">
              {#if currentStep === 'importing' || currentStep === 'results'}
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              {:else}
                2
              {/if}
            </div>
            <span class="ml-2 text-sm font-medium text-gray-900">Preview</span>
          </div>

          <div class="w-16 h-0.5 bg-gray-300"></div>

          <!-- Step 3: Import -->
          <div class="flex items-center">
            <div class="flex items-center justify-center w-8 h-8 rounded-full {currentStep === 'importing' ? 'bg-primary-600 text-white' : (currentStep === 'results' ? 'bg-green-500 text-white' : 'bg-gray-300 text-gray-600')}">
              {#if currentStep === 'results'}
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              {:else}
                3
              {/if}
            </div>
            <span class="ml-2 text-sm font-medium text-gray-900">Import</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Step 1: Configuration -->
    {#if currentStep === 'config'}
      <div class="card">
        <div class="border-b border-gray-200 pb-4 mb-6">
          <h2 class="text-lg font-semibold text-gray-900">Import Configuration</h2>
          <p class="mt-1 text-sm text-gray-500">
            Configure how your Steam library will be imported
          </p>
        </div>

        <!-- Steam Account Info -->
        {#if currentConfig}
          <div class="mb-6 p-4 bg-blue-50 rounded-md">
            <h3 class="text-sm font-medium text-blue-900 mb-2">Steam Account</h3>
            <div class="space-y-1 text-sm text-blue-700">
              <div>API Key: {currentConfig.api_key_masked}</div>
              {#if currentConfig.steam_id}
                <div>Steam ID: {currentConfig.steam_id}</div>
              {/if}
              <div class="flex items-center">
                Status: 
                <span class="ml-1 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {currentConfig.is_verified ? 'bg-green-100 text-green-800' : 'bg-yellow-100 text-yellow-800'}">
                  {currentConfig.is_verified ? 'Verified' : 'Not Verified'}
                </span>
              </div>
            </div>
          </div>
        {/if}

        <!-- Basic Settings -->
        <div class="space-y-4">
          <div>
            <label for="mergeStrategy" class="form-label">Merge Strategy</label>
            <select id="mergeStrategy" bind:value={mergeStrategy} class="form-input">
              <option value="skip">Skip games already in collection</option>
              <option value="add_platforms">Add missing platforms to existing games</option>
            </select>
            <p class="mt-1 text-sm text-gray-500">
              How to handle games that are already in your collection
            </p>
          </div>

          <!-- Advanced Settings Toggle -->
          <div class="pt-4">
            <button
              type="button"
              on:click={() => showAdvancedSettings = !showAdvancedSettings}
              class="flex items-center text-sm text-primary-600 hover:text-primary-500"
            >
              <svg class="w-4 h-4 mr-1 transform transition-transform {showAdvancedSettings ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
              </svg>
              Advanced Settings
            </button>
          </div>

          <!-- Advanced Settings -->
          {#if showAdvancedSettings}
            <div class="space-y-4 pl-6 border-l-2 border-gray-100">
              <div>
                <label for="fuzzyThreshold" class="form-label">
                  Fuzzy Matching Threshold: {fuzzyThreshold}
                </label>
                <input
                  id="fuzzyThreshold"
                  type="range"
                  min="0.5"
                  max="1.0"
                  step="0.05"
                  bind:value={fuzzyThreshold}
                  class="w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer"
                />
                <div class="flex justify-between text-xs text-gray-500 mt-1">
                  <span>Loose (0.5)</span>
                  <span>Strict (1.0)</span>
                </div>
                <p class="mt-1 text-sm text-gray-500">
                  How strict to be when matching Steam games to IGDB games. Lower values find more matches but may include incorrect ones.
                </p>
              </div>

            </div>
          {/if}
        </div>

        <!-- Load Library Button -->
        <div class="pt-6">
          <button
            on:click={handleLoadLibrary}
            disabled={isLoadingLibrary || isLoadingLibraryState}
            class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {#if isLoadingLibrary || isLoadingLibraryState}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Loading Library...
            {:else}
              Load Steam Library
            {/if}
          </button>
        </div>
      </div>
    {/if}

    <!-- Step 2: Library Preview -->
    {#if currentStep === 'preview' && library}
      <div class="card">
        <div class="border-b border-gray-200 pb-4 mb-6">
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900">Library Preview</h2>
              <p class="mt-1 text-sm text-gray-500">
                {library.total_games} games found in your Steam library
              </p>
            </div>
            <button
              on:click={handleStartOver}
              class="btn-secondary"
            >
              Change Settings
            </button>
          </div>
        </div>

        <!-- Library Stats -->
        <div class="grid grid-cols-1 gap-4 mb-6">
          <div class="bg-blue-50 rounded-lg p-4 text-center">
            <div class="text-2xl font-bold text-blue-900">{library.total_games}</div>
            <div class="text-sm text-blue-700">Total Games</div>
          </div>
        </div>

        <!-- Games List (First 10) -->
        <div class="mb-6">
          <h3 class="text-sm font-medium text-gray-900 mb-3">Preview (showing first 10 games)</h3>
          <div class="space-y-2">
            {#each library.games.slice(0, 10) as game}
              <div class="flex items-center justify-between py-2 px-3 bg-gray-50 rounded-md">
                <div class="flex-1">
                  <div class="font-medium text-gray-900">{game.name}</div>
                  <div class="text-sm text-gray-500">
                    Platform: PC (Windows)
                  </div>
                </div>
                {#if game.img_icon_url}
                  <img 
                    src="https://media.steampowered.com/steamcommunity/public/images/apps/{game.appid}/{game.img_icon_url}.jpg" 
                    alt="{game.name} icon"
                    class="w-8 h-8 rounded"
                  />
                {/if}
              </div>
            {/each}
          </div>
          {#if library.games.length > 10}
            <p class="text-sm text-gray-500 mt-2">
              ... and {library.games.length - 10} more games
            </p>
          {/if}
        </div>

        <!-- Import Button -->
        <div class="flex space-x-3">
          <button
            on:click={handleImportLibrary}
            disabled={isImporting}
            class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {#if isImporting}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Importing...
            {:else}
              Import {library.total_games} Games
            {/if}
          </button>
        </div>
      </div>
    {/if}

    <!-- Step 3: Importing Progress -->
    {#if currentStep === 'importing'}
      <div class="card">
        <div class="text-center py-12">
          <svg class="animate-spin mx-auto h-12 w-12 text-primary-600" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <h2 class="mt-4 text-lg font-semibold text-gray-900">Importing Steam Library</h2>
          <p class="mt-2 text-sm text-gray-500">
            This may take a few moments. Please don't close this page.
          </p>
        </div>
      </div>
    {/if}

    <!-- Step 4: Import Results -->
    {#if currentStep === 'results' && importResults}
      <div class="card">
        <div class="border-b border-gray-200 pb-4 mb-6">
          <h2 class="text-lg font-semibold text-gray-900">Import Complete</h2>
          <p class="mt-1 text-sm text-gray-500">
            {importResults.import_summary}
          </p>
        </div>

        <!-- Results Summary -->
        <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
          <div class="bg-green-50 rounded-lg p-4 text-center">
            <div class="text-2xl font-bold text-green-900">{importResults.imported_count}</div>
            <div class="text-sm text-green-700">Imported</div>
          </div>
          <div class="bg-yellow-50 rounded-lg p-4 text-center">
            <div class="text-2xl font-bold text-yellow-900">{importResults.skipped_count}</div>
            <div class="text-sm text-yellow-700">Skipped</div>
          </div>
          <div class="bg-red-50 rounded-lg p-4 text-center">
            <div class="text-2xl font-bold text-red-900">{importResults.failed_count}</div>
            <div class="text-sm text-red-700">Failed</div>
          </div>
          <div class="bg-gray-50 rounded-lg p-4 text-center">
            <div class="text-2xl font-bold text-gray-900">{importResults.no_match_count}</div>
            <div class="text-sm text-gray-700">No Match</div>
          </div>
        </div>

        <!-- Platform Breakdown -->
        {#if Object.keys(importResults.platform_breakdown).length > 0}
          <div class="mb-6">
            <h3 class="text-sm font-medium text-gray-900 mb-3">Platform Distribution</h3>
            <div class="space-y-2">
              {#each Object.entries(importResults.platform_breakdown) as [platform, count]}
                <div class="flex items-center justify-between py-1">
                  <span class="text-sm text-gray-700 capitalize">{platform.replace('-', ' ')}</span>
                  <span class="text-sm font-medium text-gray-900">{count} games</span>
                </div>
              {/each}
            </div>
          </div>
        {/if}

        <!-- Action Buttons -->
        <div class="flex space-x-3">
          <a href="/games" class="btn-primary">
            View Collection
          </a>
          <button
            on:click={handleStartOver}
            class="btn-secondary"
          >
            Import Again
          </button>
        </div>
      </div>
    {/if}

    <!-- Error Display -->
    {#if libraryError}
      <div class="card">
        <div class="p-4 bg-red-50 border border-red-200 rounded-md">
          <div class="flex">
            <svg class="h-5 w-5 text-red-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
            </svg>
            <div>
              <h3 class="text-sm font-medium text-red-800">Import Error</h3>
              <p class="text-sm text-red-700 mt-1">{libraryError}</p>
            </div>
          </div>
          <div class="mt-4">
            <button
              on:click={() => steam.clearLibraryError()}
              class="btn-secondary text-sm"
            >
              Dismiss
            </button>
          </div>
        </div>
      </div>
    {/if}
  </div>
</RouteGuard>