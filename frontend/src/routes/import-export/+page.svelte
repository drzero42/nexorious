<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { RouteGuard } from '$lib/components';
	import { jobs, JobType, type Job } from '$lib/stores';
	import { websocket, WebSocketEventType } from '$lib/stores/websocket.svelte';
	import JobCard from '$lib/components/JobCard.svelte';

	// Import sources (manual file-based imports only - Steam is in /sync)
	const importSources = [
		{
			id: 'nexorious',
			title: 'Nexorious JSON',
			description:
				'Restore a previous Nexorious export with all metadata, ratings, play status, and notes intact.',
			icon: '📦',
			href: '/import/nexorious',
			features: ['Full metadata restoration', 'Preserves ratings and notes', 'Non-interactive import'],
			color: 'indigo'
		},
		{
			id: 'darkadia',
			title: 'Darkadia CSV',
			description: 'Import your game collection from Darkadia with automatic IGDB matching.',
			icon: '📊',
			href: '/import/darkadia',
			features: ['CSV file upload', 'Automatic IGDB matching', 'Review unmatched titles'],
			color: 'purple'
		}
	];

	type ColorClasses = { bg: string; border: string; hover: string; icon: string; button: string };

	const colorMap = {
		indigo: {
			bg: 'bg-indigo-50 dark:bg-indigo-900/20',
			border: 'border-indigo-200 dark:border-indigo-800',
			hover: 'hover:border-indigo-400 dark:hover:border-indigo-600 hover:shadow-md',
			icon: 'bg-indigo-100 dark:bg-indigo-900/40 text-indigo-600 dark:text-indigo-400',
			button: 'bg-indigo-600 hover:bg-indigo-700 focus:ring-indigo-500'
		},
		purple: {
			bg: 'bg-purple-50 dark:bg-purple-900/20',
			border: 'border-purple-200 dark:border-purple-800',
			hover: 'hover:border-purple-400 dark:hover:border-purple-600 hover:shadow-md',
			icon: 'bg-purple-100 dark:bg-purple-900/40 text-purple-600 dark:text-purple-400',
			button: 'bg-purple-600 hover:bg-purple-700 focus:ring-purple-500'
		},
		green: {
			bg: 'bg-green-50 dark:bg-green-900/20',
			border: 'border-green-200 dark:border-green-800',
			hover: 'hover:border-green-400 dark:hover:border-green-600 hover:shadow-md',
			icon: 'bg-green-100 dark:bg-green-900/40 text-green-600 dark:text-green-400',
			button: 'bg-green-600 hover:bg-green-700 focus:ring-green-500'
		}
	} as const satisfies Record<string, ColorClasses>;

	function getColorClasses(color: string): ColorClasses {
		if (color in colorMap) {
			return colorMap[color as keyof typeof colorMap];
		}
		return colorMap.indigo;
	}

	// Export section colors
	const exportColors = getColorClasses('green');

	// Recent import jobs (limited to 5)
	const recentJobs = $derived(
		jobs.value.jobs
			.filter((job) => job.job_type === JobType.IMPORT)
			.slice(0, 5)
	);
	const isLoadingJobs = $derived(jobs.value.isLoading);
	const jobsError = $derived(jobs.value.error);

	let unsubscribeJobCreated: (() => void) | null = null;

	onMount(async () => {
		// Load recent import jobs
		await jobs.loadJobs({ job_type: JobType.IMPORT }, 1, 5);

		// Connect to WebSocket for real-time updates
		websocket.connect();

		// Subscribe to job updates
		unsubscribeJobCreated = websocket.on(WebSocketEventType.JOB_CREATED, () => {
			// Refresh jobs list when new job is created
			jobs.loadJobs({ job_type: JobType.IMPORT }, 1, 5);
		});
	});

	onDestroy(() => {
		if (unsubscribeJobCreated) {
			unsubscribeJobCreated();
		}
	});

	function handleViewJob(job: Job) {
		goto(`/jobs/${job.id}`);
	}
</script>

<svelte:head>
	<title>Import / Export - Nexorious</title>
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
						<span class="text-gray-900 dark:text-white font-medium">Import / Export</span>
					</li>
				</ol>
			</nav>

			<div class="mt-4">
				<h1
					class="text-2xl font-bold leading-7 text-gray-900 dark:text-white sm:truncate sm:text-3xl sm:tracking-tight"
				>
					Import / Export
				</h1>
				<p class="mt-2 text-sm text-gray-500 dark:text-gray-400 max-w-2xl">
					Import your game collection from various sources or export your data for backup.
				</p>
			</div>
		</div>

		<!-- Import Section -->
		<section>
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Import Games</h2>
			<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
				{#each importSources as source}
					{@const colors = getColorClasses(source.color)}
					<div
						class="relative rounded-lg border-2 {colors.border} {colors.bg} p-6 transition-all duration-200 {colors.hover}"
					>
						<!-- Icon and Title -->
						<div class="flex items-center space-x-3 mb-4">
							<div class="{colors.icon} rounded-lg p-3">
								<span class="text-2xl">{source.icon}</span>
							</div>
							<h3 class="text-lg font-semibold text-gray-900 dark:text-white">{source.title}</h3>
						</div>

						<!-- Description -->
						<p class="text-sm text-gray-600 dark:text-gray-300 mb-4">
							{source.description}
						</p>

						<!-- Features -->
						<ul class="space-y-2 mb-6">
							{#each source.features as feature}
								<li class="flex items-center text-sm text-gray-600 dark:text-gray-300">
									<svg
										class="h-4 w-4 mr-2 text-green-500 flex-shrink-0"
										fill="none"
										viewBox="0 0 24 24"
										stroke="currentColor"
									>
										<path
											stroke-linecap="round"
											stroke-linejoin="round"
											stroke-width="2"
											d="M5 13l4 4L19 7"
										/>
									</svg>
									{feature}
								</li>
							{/each}
						</ul>

						<!-- Action Button -->
						<a
							href={source.href}
							class="w-full inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white {colors.button} focus:outline-none focus:ring-2 focus:ring-offset-2 transition-colors"
						>
							Start Import
							<svg class="ml-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path
									stroke-linecap="round"
									stroke-linejoin="round"
									stroke-width="2"
									d="M14 5l7 7m0 0l-7 7m7-7H3"
								/>
							</svg>
						</a>
					</div>
				{/each}
			</div>
		</section>

		<!-- Export Section -->
		<section>
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Export Data</h2>
			<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
				<div
					class="relative rounded-lg border-2 {exportColors.border} {exportColors.bg} p-6 transition-all duration-200"
				>
					<!-- Icon and Title -->
					<div class="flex items-center space-x-3 mb-4">
						<div class="{exportColors.icon} rounded-lg p-3">
							<span class="text-2xl">📤</span>
						</div>
						<h3 class="text-lg font-semibold text-gray-900 dark:text-white">Export Collection</h3>
					</div>

					<!-- Description -->
					<p class="text-sm text-gray-600 dark:text-gray-300 mb-4">
						Export your entire game collection to a JSON file for backup or transfer to another
						instance.
					</p>

					<!-- Features -->
					<ul class="space-y-2 mb-6">
						<li class="flex items-center text-sm text-gray-600 dark:text-gray-300">
							<svg
								class="h-4 w-4 mr-2 text-green-500 flex-shrink-0"
								fill="none"
								viewBox="0 0 24 24"
								stroke="currentColor"
							>
								<path
									stroke-linecap="round"
									stroke-linejoin="round"
									stroke-width="2"
									d="M5 13l4 4L19 7"
								/>
							</svg>
							Complete collection backup
						</li>
						<li class="flex items-center text-sm text-gray-600 dark:text-gray-300">
							<svg
								class="h-4 w-4 mr-2 text-green-500 flex-shrink-0"
								fill="none"
								viewBox="0 0 24 24"
								stroke="currentColor"
							>
								<path
									stroke-linecap="round"
									stroke-linejoin="round"
									stroke-width="2"
									d="M5 13l4 4L19 7"
								/>
							</svg>
							Includes all metadata
						</li>
						<li class="flex items-center text-sm text-gray-600 dark:text-gray-300">
							<svg
								class="h-4 w-4 mr-2 text-green-500 flex-shrink-0"
								fill="none"
								viewBox="0 0 24 24"
								stroke="currentColor"
							>
								<path
									stroke-linecap="round"
									stroke-linejoin="round"
									stroke-width="2"
									d="M5 13l4 4L19 7"
								/>
							</svg>
							Nexorious JSON format
						</li>
					</ul>

					<!-- Coming Soon Button -->
					<button
						disabled
						class="w-full inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-gray-400 dark:bg-gray-600 cursor-not-allowed"
					>
						<svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path
								stroke-linecap="round"
								stroke-linejoin="round"
								stroke-width="2"
								d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
							/>
						</svg>
						Coming Soon
					</button>
				</div>
			</div>
		</section>

		<!-- Recent Import Jobs -->
		<section>
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">Recent Import Jobs</h2>
				<a
					href="/jobs?job_type=import"
					class="text-sm text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300"
				>
					View all jobs →
				</a>
			</div>

			{#if jobsError}
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
							<p class="text-sm text-red-700 dark:text-red-300">{jobsError}</p>
						</div>
					</div>
				</div>
			{:else if isLoadingJobs}
				<div class="flex justify-center items-center py-8">
					<div
						class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 dark:border-indigo-400"
					></div>
				</div>
			{:else if recentJobs.length === 0}
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
							d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
						/>
					</svg>
					<h3 class="mt-2 text-sm font-medium text-gray-900 dark:text-white">No import jobs yet</h3>
					<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
						Start an import above to see your job history here.
					</p>
				</div>
			{:else}
				<div class="space-y-4">
					{#each recentJobs as job (job.id)}
						<JobCard {job} onView={handleViewJob} />
					{/each}
				</div>
			{/if}
		</section>

		<!-- Quick Links -->
		<div class="flex flex-wrap gap-4 text-sm pt-4 border-t border-gray-200 dark:border-gray-700">
			<a
				href="/review"
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
				Review Pending Items
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
			<a
				href="/sync"
				class="text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 inline-flex items-center"
			>
				<svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
					/>
				</svg>
				Sync Settings
			</a>
		</div>
	</div>
</RouteGuard>
