<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { sync, SyncFrequency, SyncPlatform } from '$lib/stores';
	import { RouteGuard } from '$lib/components';
	import { getSyncFrequencyLabel } from '$lib/types/jobs';

	let isSaving = $state<Map<string, boolean>>(new Map());
	let syncingPlatforms = $state<Set<string>>(new Set());

	const configs = $derived(sync.value.configs);
	const isLoading = $derived(sync.value.isLoading);
	const error = $derived(sync.value.error);

	onMount(() => {
		sync.loadConfigs();
		sync.refreshAllStatuses();
	});

	function formatLastSync(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		const date = new Date(dateStr);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffMins = Math.floor(diffMs / 60000);
		const diffHours = Math.floor(diffMins / 60);
		const diffDays = Math.floor(diffHours / 24);

		if (diffMins < 1) return 'Just now';
		if (diffMins < 60) return `${diffMins}m ago`;
		if (diffHours < 24) return `${diffHours}h ago`;
		if (diffDays < 7) return `${diffDays}d ago`;
		return date.toLocaleDateString();
	}

	function getPlatformIcon(platform: string): string {
		const steamIcon =
			'M12 2C6.477 2 2 6.477 2 12c0 4.991 3.657 9.128 8.438 9.879V14.89h-2.54V12h2.54V9.797c0-2.506 1.492-3.89 3.777-3.89 1.094 0 2.238.195 2.238.195v2.46h-1.26c-1.243 0-1.63.771-1.63 1.562V12h2.773l-.443 2.89h-2.33v6.989C18.343 21.129 22 16.99 22 12c0-5.523-4.477-10-10-10z';
		const icons: Record<string, string> = {
			steam: steamIcon,
			epic: 'M3 3h18v18H3V3zm2 2v14h14V5H5zm3 3h2v8H8V8zm4 0h4v2h-4v2h3v2h-3v2h4v2H12V8z',
			gog: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8zm-2-8c0-1.1.9-2 2-2s2 .9 2 2-.9 2-2 2-2-.9-2-2z'
		};
		return icons[platform] ?? steamIcon;
	}

	function getPlatformColor(platform: string): string {
		const colors: Record<string, string> = {
			steam: 'text-[#1b2838]',
			epic: 'text-gray-800',
			gog: 'text-purple-700'
		};
		return colors[platform] || 'text-gray-600';
	}

	async function handleFrequencyChange(platform: SyncPlatform, frequency: SyncFrequency) {
		isSaving.set(platform, true);
		try {
			await sync.updateConfig(platform, { frequency });
		} finally {
			isSaving.delete(platform);
			isSaving = new Map(isSaving);
		}
	}

	async function handleAutoAddChange(platform: SyncPlatform, autoAdd: boolean) {
		isSaving.set(platform, true);
		try {
			await sync.updateConfig(platform, { auto_add: autoAdd });
		} finally {
			isSaving.delete(platform);
			isSaving = new Map(isSaving);
		}
	}

	async function handleEnabledChange(platform: SyncPlatform, enabled: boolean) {
		isSaving.set(platform, true);
		try {
			await sync.updateConfig(platform, { enabled });
		} finally {
			isSaving.delete(platform);
			isSaving = new Map(isSaving);
		}
	}

	async function handleTriggerSync(platform: SyncPlatform) {
		syncingPlatforms.add(platform);
		syncingPlatforms = new Set(syncingPlatforms);
		try {
			const result = await sync.triggerSync(platform);
			// Navigate to job detail page
			goto(`/jobs/${result.job_id}`);
		} catch {
			// Error is handled by the store
			syncingPlatforms.delete(platform);
			syncingPlatforms = new Set(syncingPlatforms);
		}
	}

	function isSyncingPlatform(platform: string): boolean {
		return syncingPlatforms.has(platform) || sync.isPlatformSyncing(platform);
	}
</script>

<svelte:head>
	<title>Sync Settings - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
		<!-- Back link -->
		<div class="mb-6">
			<a
				href="/dashboard"
				class="inline-flex items-center text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300"
			>
				<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M10 19l-7-7m0 0l7-7m-7 7h18"
					/>
				</svg>
				Back to Dashboard
			</a>
		</div>

		<!-- Header -->
		<div class="mb-8">
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Sync Settings</h1>
			<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
				Configure automatic synchronization with your gaming platforms
			</p>
		</div>

		<!-- Error State -->
		{#if error}
			<div class="bg-red-50 dark:bg-red-900/20 rounded-lg p-4 mb-6">
				<div class="flex">
					<div class="flex-shrink-0">
						<svg
							class="h-5 w-5 text-red-400"
							viewBox="0 0 20 20"
							fill="currentColor"
							aria-hidden="true"
						>
							<path
								fill-rule="evenodd"
								d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z"
								clip-rule="evenodd"
							/>
						</svg>
					</div>
					<div class="ml-3">
						<h3 class="text-sm font-medium text-red-800 dark:text-red-400">Error</h3>
						<p class="mt-1 text-sm text-red-700 dark:text-red-300">{error}</p>
					</div>
				</div>
			</div>
		{/if}

		<!-- Loading State -->
		{#if isLoading && configs.length === 0}
			<div class="flex justify-center items-center py-12">
				<div
					class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 dark:border-indigo-400"
				></div>
			</div>
		{:else}
			<!-- Platform Cards -->
			<div class="space-y-6">
				{#each configs as config (config.platform)}
					{@const platform = config.platform as SyncPlatform}
					{@const isSyncing = isSyncingPlatform(config.platform)}
					{@const saving = isSaving.get(config.platform)}
					<div
						class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden"
					>
						<div class="p-6">
							<!-- Platform Header -->
							<div class="flex items-center justify-between mb-6">
								<div class="flex items-center gap-3">
									<div class="w-10 h-10 rounded-lg bg-gray-100 dark:bg-gray-700 flex items-center justify-center">
										<svg
											class="w-6 h-6 {getPlatformColor(config.platform)}"
											viewBox="0 0 24 24"
											fill="currentColor"
										>
											<path d={getPlatformIcon(config.platform)} />
										</svg>
									</div>
									<div>
										<h3 class="text-lg font-medium text-gray-900 dark:text-white capitalize">
											{config.platform}
										</h3>
										<p class="text-sm text-gray-500 dark:text-gray-400">
											Last synced: {formatLastSync(config.last_synced_at)}
										</p>
									</div>
								</div>
								<div class="flex items-center gap-4">
									<!-- Enabled Toggle -->
									<label class="flex items-center cursor-pointer">
										<span class="mr-2 text-sm text-gray-600 dark:text-gray-400">
											{config.enabled ? 'Enabled' : 'Disabled'}
										</span>
										<div class="relative">
											<input
												type="checkbox"
												class="sr-only"
												checked={config.enabled}
												onchange={(e) =>
													handleEnabledChange(platform, (e.target as HTMLInputElement).checked)}
												disabled={saving}
											/>
											<div
												class="w-10 h-6 rounded-full transition-colors {config.enabled
													? 'bg-indigo-600'
													: 'bg-gray-300 dark:bg-gray-600'}"
											>
												<div
													class="absolute w-4 h-4 bg-white rounded-full top-1 transition-transform {config.enabled
														? 'translate-x-5'
														: 'translate-x-1'}"
												></div>
											</div>
										</div>
									</label>
								</div>
							</div>

							<!-- Settings Grid -->
							<div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
								<!-- Frequency -->
								<div>
									<label
										for="frequency-{config.platform}"
										class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
									>
										Sync Frequency
									</label>
									<select
										id="frequency-{config.platform}"
										class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500 disabled:opacity-50"
										value={config.frequency}
										onchange={(e) =>
											handleFrequencyChange(
												platform,
												(e.target as HTMLSelectElement).value as SyncFrequency
											)}
										disabled={!config.enabled || saving}
									>
										{#each Object.values(SyncFrequency) as freq}
											<option value={freq}>{getSyncFrequencyLabel(freq)}</option>
										{/each}
									</select>
								</div>

								<!-- Auto-add -->
								<div>
									<span class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
										Auto-add Games
									</span>
									<label class="flex items-center mt-2 cursor-pointer">
										<input
											type="checkbox"
											class="rounded border-gray-300 dark:border-gray-600 text-indigo-600 focus:ring-indigo-500 disabled:opacity-50"
											checked={config.auto_add}
											onchange={(e) =>
												handleAutoAddChange(platform, (e.target as HTMLInputElement).checked)}
											disabled={!config.enabled || saving}
										/>
										<span class="ml-2 text-sm text-gray-600 dark:text-gray-400">
											Automatically add matched games to collection
										</span>
									</label>
								</div>
							</div>

							<!-- Help text -->
							<div class="mt-4 text-sm text-gray-500 dark:text-gray-400">
								{#if config.auto_add}
									When enabled, games that match with high confidence will be automatically added to
									your collection. Games with low confidence will be queued for manual review.
								{:else}
									All new games from syncs will be queued for your review before being added to your
									collection.
								{/if}
							</div>
						</div>

						<!-- Actions -->
						<div
							class="px-6 py-4 bg-gray-50 dark:bg-gray-800/50 border-t border-gray-200 dark:border-gray-700 flex justify-between items-center"
						>
							<a
								href="/jobs?source={config.platform}&job_type=sync"
								class="text-sm text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300"
							>
								View sync history
							</a>
							<button
								class="inline-flex items-center px-4 py-2 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								onclick={() => handleTriggerSync(platform)}
								disabled={!config.enabled || isSyncing}
							>
								{#if isSyncing}
									<svg
										class="animate-spin -ml-1 mr-2 h-4 w-4 text-white"
										fill="none"
										viewBox="0 0 24 24"
									>
										<circle
											class="opacity-25"
											cx="12"
											cy="12"
											r="10"
											stroke="currentColor"
											stroke-width="4"
										></circle>
										<path
											class="opacity-75"
											fill="currentColor"
											d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
										></path>
									</svg>
									Syncing...
								{:else}
									<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path
											stroke-linecap="round"
											stroke-linejoin="round"
											stroke-width="2"
											d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
										/>
									</svg>
									Sync Now
								{/if}
							</button>
						</div>
					</div>
				{/each}
			</div>

			<!-- Info Card -->
			<div
				class="mt-8 bg-blue-50 dark:bg-blue-900/20 rounded-lg p-4 border border-blue-200 dark:border-blue-800"
			>
				<div class="flex">
					<div class="flex-shrink-0">
						<svg
							class="h-5 w-5 text-blue-400"
							viewBox="0 0 20 20"
							fill="currentColor"
							aria-hidden="true"
						>
							<path
								fill-rule="evenodd"
								d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z"
								clip-rule="evenodd"
							/>
						</svg>
					</div>
					<div class="ml-3">
						<h3 class="text-sm font-medium text-blue-800 dark:text-blue-400">
							About Platform Syncing
						</h3>
						<div class="mt-2 text-sm text-blue-700 dark:text-blue-300">
							<p>
								Platform syncing keeps your Nexorious collection in sync with your game libraries. When
								you acquire new games on Steam, Epic, or GOG, they'll automatically be detected and
								either added to your collection or queued for review.
							</p>
							<p class="mt-2">
								Syncing also detects when games are removed from your library (e.g., refunds, expired
								subscriptions) and will queue them for review so you can decide whether to keep them
								in your collection.
							</p>
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>
</RouteGuard>
