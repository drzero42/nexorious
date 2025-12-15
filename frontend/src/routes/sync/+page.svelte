<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { sync, SyncFrequency, SyncPlatform } from '$lib/stores';
	import { jobs, JobType, type Job } from '$lib/stores';
	import { websocket, WebSocketEventType } from '$lib/stores/websocket.svelte';
	import { RouteGuard } from '$lib/components';
	import { getSyncFrequencyLabel } from '$lib/types/jobs';
	import JobCard from '$lib/components/JobCard.svelte';

	let isSaving = $state<Map<string, boolean>>(new Map());
	let syncingPlatforms = $state<Set<string>>(new Set());

	const configs = $derived(sync.value.configs);
	const isLoading = $derived(sync.value.isLoading);
	const error = $derived(sync.value.error);

	// Recent sync jobs (limited to 5)
	const recentSyncJobs = $derived(
		jobs.value.jobs.filter((job) => job.job_type === JobType.SYNC).slice(0, 5)
	);
	const isLoadingJobs = $derived(jobs.value.isLoading);

	let unsubscribeJobCreated: (() => void) | null = null;

	onMount(async () => {
		sync.loadConfigs();
		sync.refreshAllStatuses();

		// Load recent sync jobs
		await jobs.loadJobs({ job_type: JobType.SYNC }, 1, 5);

		// Connect to WebSocket for real-time updates
		websocket.connect();

		// Subscribe to job updates
		unsubscribeJobCreated = websocket.on(WebSocketEventType.JOB_CREATED, () => {
			jobs.loadJobs({ job_type: JobType.SYNC }, 1, 5);
		});
	});

	onDestroy(() => {
		if (unsubscribeJobCreated) {
			unsubscribeJobCreated();
		}
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
			epic: 'text-gray-800 dark:text-gray-200',
			gog: 'text-purple-700 dark:text-purple-400'
		};
		return colors[platform] || 'text-gray-600 dark:text-gray-400';
	}

	function getPlatformBgColor(platform: string): string {
		const colors: Record<string, string> = {
			steam: 'bg-[#1b2838]/10 dark:bg-[#1b2838]/30',
			epic: 'bg-gray-100 dark:bg-gray-700',
			gog: 'bg-purple-100 dark:bg-purple-900/30'
		};
		return colors[platform] || 'bg-gray-100 dark:bg-gray-700';
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
			goto(`/jobs/${result.job_id}`);
		} catch {
			syncingPlatforms.delete(platform);
			syncingPlatforms = new Set(syncingPlatforms);
		}
	}

	function isSyncingPlatform(platform: string): boolean {
		return syncingPlatforms.has(platform) || sync.isPlatformSyncing(platform);
	}

	function handleViewJob(job: Job) {
		goto(`/jobs/${job.id}`);
	}
</script>

<svelte:head>
	<title>Sync - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-8">
		<!-- Header -->
		<div>
			<nav class="flex text-sm text-gray-500 dark:text-gray-400" aria-label="Breadcrumb">
				<ol class="inline-flex items-center space-x-1 md:space-x-3">
					<li>
						<a href="/dashboard" class="hover:text-gray-700 dark:hover:text-gray-300">Dashboard</a>
					</li>
					<li>
						<span class="mx-2">›</span>
					</li>
					<li>
						<span class="text-gray-900 dark:text-white font-medium">Sync</span>
					</li>
				</ol>
			</nav>

			<div class="mt-4">
				<h1
					class="text-2xl font-bold leading-7 text-gray-900 dark:text-white sm:truncate sm:text-3xl sm:tracking-tight"
				>
					Sync
				</h1>
				<p class="mt-2 text-sm text-gray-500 dark:text-gray-400 max-w-2xl">
					Connect and synchronize your game libraries from Steam, Epic, GOG, and other platforms.
				</p>
			</div>
		</div>

		<!-- Error State -->
		{#if error}
			<div class="bg-red-50 dark:bg-red-900/20 rounded-lg p-4">
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

		<!-- Connected Services Section -->
		<section>
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Connected Services</h2>

			{#if isLoading && configs.length === 0}
				<div class="flex justify-center items-center py-12">
					<div
						class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 dark:border-indigo-400"
					></div>
				</div>
			{:else}
				<div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
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
										<div
											class="w-12 h-12 rounded-lg {getPlatformBgColor(config.platform)} flex items-center justify-center"
										>
											<svg
												class="w-7 h-7 {getPlatformColor(config.platform)}"
												viewBox="0 0 24 24"
												fill="currentColor"
											>
												<path d={getPlatformIcon(config.platform)} />
											</svg>
										</div>
										<div>
											<h3 class="text-lg font-semibold text-gray-900 dark:text-white capitalize">
												{config.platform}
											</h3>
											<p class="text-sm text-gray-500 dark:text-gray-400">
												Last synced: {formatLastSync(config.last_synced_at)}
											</p>
										</div>
									</div>
									<!-- Status Badge -->
									<div class="flex items-center gap-2">
										{#if config.enabled}
											<span
												class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-400"
											>
												Connected
											</span>
										{:else}
											<span
												class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400"
											>
												Disconnected
											</span>
										{/if}
									</div>
								</div>

								<!-- Settings -->
								<div class="space-y-4">
									<!-- Enabled Toggle -->
									<div class="flex items-center justify-between">
										<span class="text-sm text-gray-700 dark:text-gray-300">Enable sync</span>
										<label class="flex items-center cursor-pointer">
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

									<!-- Frequency -->
									<div class="flex items-center justify-between">
										<span class="text-sm text-gray-700 dark:text-gray-300">Sync frequency</span>
										<select
											class="rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm disabled:opacity-50"
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
									<div class="flex items-center justify-between">
										<span class="text-sm text-gray-700 dark:text-gray-300">Auto-add games</span>
										<label class="flex items-center cursor-pointer">
											<input
												type="checkbox"
												class="rounded border-gray-300 dark:border-gray-600 text-indigo-600 focus:ring-indigo-500 disabled:opacity-50"
												checked={config.auto_add}
												onchange={(e) =>
													handleAutoAddChange(platform, (e.target as HTMLInputElement).checked)}
												disabled={!config.enabled || saving}
											/>
										</label>
									</div>
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
									View history
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
			{/if}
		</section>

		<!-- Recent Sync Jobs -->
		<section>
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">Recent Sync Jobs</h2>
				<a
					href="/jobs?job_type=sync"
					class="text-sm text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300"
				>
					View all jobs →
				</a>
			</div>

			{#if isLoadingJobs && recentSyncJobs.length === 0}
				<div class="flex justify-center items-center py-8">
					<div
						class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 dark:border-indigo-400"
					></div>
				</div>
			{:else if recentSyncJobs.length === 0}
				<div
					class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-8 text-center"
				>
					<svg
						class="mx-auto h-12 w-12 text-gray-400"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
						aria-hidden="true"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
						/>
					</svg>
					<h3 class="mt-2 text-sm font-medium text-gray-900 dark:text-white">No sync jobs yet</h3>
					<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
						Enable a service above and trigger a sync to see your job history here.
					</p>
				</div>
			{:else}
				<div class="space-y-4">
					{#each recentSyncJobs as job (job.id)}
						<JobCard {job} onView={handleViewJob} />
					{/each}
				</div>
			{/if}
		</section>

		<!-- Info Card -->
		<div
			class="bg-blue-50 dark:bg-blue-900/20 rounded-lg p-4 border border-blue-200 dark:border-blue-800"
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
					</div>
				</div>
			</div>
		</div>

		<!-- Quick Links -->
		<div class="flex flex-wrap gap-4 text-sm pt-4 border-t border-gray-200 dark:border-gray-700">
			<a
				href="/review?source=sync"
				class="text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 inline-flex items-center"
			>
				<svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
					/>
				</svg>
				Review Sync Items
			</a>
			<a
				href="/import-export"
				class="text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 inline-flex items-center"
			>
				<svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"
					/>
				</svg>
				Import / Export
			</a>
			<a
				href="/games"
				class="text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 inline-flex items-center"
			>
				<svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"
					/>
				</svg>
				View Collection
			</a>
		</div>
	</div>
</RouteGuard>
