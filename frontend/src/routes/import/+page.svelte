<script lang="ts">
  import { RouteGuard } from '$lib/components';
  import { steamAvailability } from '$lib/stores/steam-availability.svelte';
  import { onMount } from 'svelte';

  // Check Steam availability on mount
  onMount(async () => {
    await steamAvailability.checkAvailability();
  });

  // Import source definitions
  const importSources = [
    {
      id: 'nexorious',
      title: 'Nexorious JSON',
      description: 'Restore a previous Nexorious export. This is the fastest way to restore your collection with all metadata, ratings, play status, and notes intact.',
      icon: '📦',
      href: '/import/nexorious',
      features: [
        'Full metadata restoration',
        'Preserves ratings and notes',
        'Restores play status and tags',
        'Non-interactive import'
      ],
      color: 'indigo',
      available: true
    },
    {
      id: 'darkadia',
      title: 'Darkadia CSV',
      description: 'Import your game collection from Darkadia. Games will be matched to IGDB entries for metadata enrichment.',
      icon: '📊',
      href: '/import/darkadia',
      features: [
        'CSV file upload',
        'Automatic IGDB matching',
        'Review unmatched titles',
        'Platform detection'
      ],
      color: 'purple',
      available: true
    },
    {
      id: 'steam',
      title: 'Steam Library',
      description: 'Import your Steam library directly. Requires Steam Web API key configuration.',
      icon: '🎮',
      href: '/import/steam',
      features: [
        'Direct Steam API integration',
        'Automatic game detection',
        'Playtime import',
        'Periodic sync support'
      ],
      color: 'blue',
      available: true,
      requiresSteam: true
    }
  ];

  // Derive which sources are actually available
  const availableSources = $derived(
    importSources.map(source => ({
      ...source,
      isDisabled: source.requiresSteam && !steamAvailability.isAvailable
    }))
  );

  type ColorClasses = { bg: string; border: string; hover: string; icon: string; button: string };

  const colorMap = {
    indigo: {
      bg: 'bg-indigo-50',
      border: 'border-indigo-200',
      hover: 'hover:border-indigo-400 hover:shadow-md',
      icon: 'bg-indigo-100 text-indigo-600',
      button: 'bg-indigo-600 hover:bg-indigo-700 focus:ring-indigo-500'
    },
    purple: {
      bg: 'bg-purple-50',
      border: 'border-purple-200',
      hover: 'hover:border-purple-400 hover:shadow-md',
      icon: 'bg-purple-100 text-purple-600',
      button: 'bg-purple-600 hover:bg-purple-700 focus:ring-purple-500'
    },
    blue: {
      bg: 'bg-blue-50',
      border: 'border-blue-200',
      hover: 'hover:border-blue-400 hover:shadow-md',
      icon: 'bg-blue-100 text-blue-600',
      button: 'bg-blue-600 hover:bg-blue-700 focus:ring-blue-500'
    }
  } as const satisfies Record<string, ColorClasses>;

  function getColorClasses(color: string): ColorClasses {
    if (color in colorMap) {
      return colorMap[color as keyof typeof colorMap];
    }
    return colorMap.indigo;
  }
</script>

<svelte:head>
  <title>Import Games - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div>
      <nav class="flex text-sm text-gray-500" aria-label="Breadcrumb">
        <ol class="inline-flex items-center space-x-1 md:space-x-3">
          <li>
            <a href="/dashboard" class="hover:text-gray-700">Dashboard</a>
          </li>
          <li>
            <span>›</span>
          </li>
          <li>
            <span class="text-gray-900 font-medium">Import</span>
          </li>
        </ol>
      </nav>

      <div class="mt-4">
        <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">
          Import Your Games
        </h1>
        <p class="mt-2 text-sm text-gray-500 max-w-2xl">
          Choose how you'd like to import your game collection. Each source has different features
          and requirements. Select the option that best fits your needs.
        </p>
      </div>
    </div>

    <!-- Import Source Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {#each availableSources as source}
        {@const colors = getColorClasses(source.color)}
        <div
          class="relative rounded-lg border-2 {colors.border} {colors.bg} p-6 transition-all duration-200 {source.isDisabled ? 'opacity-60 cursor-not-allowed' : colors.hover}"
        >
          <!-- Icon -->
          <div class="flex items-center justify-between mb-4">
            <div class="flex items-center space-x-3">
              <div class="{colors.icon} rounded-lg p-3">
                <span class="text-2xl">{source.icon}</span>
              </div>
              <h2 class="text-lg font-semibold text-gray-900">{source.title}</h2>
            </div>
            {#if source.isDisabled}
              <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-600">
                Unavailable
              </span>
            {/if}
          </div>

          <!-- Description -->
          <p class="text-sm text-gray-600 mb-4">
            {source.description}
          </p>

          <!-- Features -->
          <ul class="space-y-2 mb-6">
            {#each source.features as feature}
              <li class="flex items-center text-sm text-gray-600">
                <svg class="h-4 w-4 mr-2 text-green-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                </svg>
                {feature}
              </li>
            {/each}
          </ul>

          <!-- Action Button -->
          {#if source.isDisabled}
            <div class="space-y-2">
              <button
                disabled
                class="w-full inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-gray-400 cursor-not-allowed"
              >
                <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                </svg>
                Steam Not Available
              </button>
              <p class="text-xs text-gray-500 text-center">
                {steamAvailability.unavailableReason || 'Steam integration is not enabled'}
              </p>
            </div>
          {:else}
            <a
              href={source.href}
              class="w-full inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white {colors.button} focus:outline-none focus:ring-2 focus:ring-offset-2 transition-colors"
            >
              Start Import
              <svg class="ml-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" />
              </svg>
            </a>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Additional Information -->
    <div class="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
      <h3 class="text-lg font-semibold text-gray-900 mb-4">
        Import Tips
      </h3>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <h4 class="text-sm font-medium text-gray-900 mb-2">Before You Import</h4>
          <ul class="space-y-2 text-sm text-gray-600">
            <li class="flex items-start">
              <svg class="h-5 w-5 mr-2 text-blue-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              Imports are additive - they won't delete existing games
            </li>
            <li class="flex items-start">
              <svg class="h-5 w-5 mr-2 text-blue-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              Duplicate games are automatically detected and merged
            </li>
            <li class="flex items-start">
              <svg class="h-5 w-5 mr-2 text-blue-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              Large imports run in the background - you can track progress
            </li>
          </ul>
        </div>
        <div>
          <h4 class="text-sm font-medium text-gray-900 mb-2">Import Workflow</h4>
          <ul class="space-y-2 text-sm text-gray-600">
            <li class="flex items-start">
              <span class="inline-flex items-center justify-center h-5 w-5 rounded-full bg-blue-100 text-blue-600 text-xs font-medium mr-2 flex-shrink-0">1</span>
              Upload your file or connect your account
            </li>
            <li class="flex items-start">
              <span class="inline-flex items-center justify-center h-5 w-5 rounded-full bg-blue-100 text-blue-600 text-xs font-medium mr-2 flex-shrink-0">2</span>
              Games are matched to IGDB for metadata
            </li>
            <li class="flex items-start">
              <span class="inline-flex items-center justify-center h-5 w-5 rounded-full bg-blue-100 text-blue-600 text-xs font-medium mr-2 flex-shrink-0">3</span>
              Review any games that couldn't be auto-matched
            </li>
            <li class="flex items-start">
              <span class="inline-flex items-center justify-center h-5 w-5 rounded-full bg-blue-100 text-blue-600 text-xs font-medium mr-2 flex-shrink-0">4</span>
              Confirm and add games to your collection
            </li>
          </ul>
        </div>
      </div>
    </div>

    <!-- Quick Links -->
    <div class="flex flex-wrap gap-4 text-sm">
      <a href="/jobs" class="text-primary-600 hover:text-primary-500 inline-flex items-center">
        <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
        </svg>
        View Import Jobs
      </a>
      <a href="/review" class="text-primary-600 hover:text-primary-500 inline-flex items-center">
        <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        Review Pending Items
      </a>
      <a href="/games" class="text-primary-600 hover:text-primary-500 inline-flex items-center">
        <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
        </svg>
        View Collection
      </a>
    </div>
  </div>
</RouteGuard>
