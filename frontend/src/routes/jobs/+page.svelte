<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { jobs, JobType, JobSource, JobStatus, type Job, type JobFilters } from '$lib/stores';
	import { websocket, WebSocketEventType } from '$lib/stores/websocket.svelte';
	import { RouteGuard, Pagination } from '$lib/components';
	import JobCard from '$lib/components/JobCard.svelte';
	import { getJobTypeLabel, getJobSourceLabel, getJobStatusLabel } from '$lib/types/jobs';

	let filters = $state<JobFilters>({});
	let confirmDeleteJob = $state<Job | null>(null);
	let confirmCancelJob = $state<Job | null>(null);
	let isDeleting = $state(false);
	let isCancelling = $state(false);

	const jobsList = $derived(jobs.value.jobs);
	const isLoading = $derived(jobs.value.isLoading);
	const error = $derived(jobs.value.error);
	const pagination = $derived(jobs.value.pagination);
	const wsStatus = $derived(websocket.value.status);

	let unsubscribeJobCreated: (() => void) | null = null;

	onMount(() => {
		loadJobs();

		// Connect to WebSocket for real-time updates
		websocket.connect();

		// Subscribe to job_created events to auto-refresh the list
		unsubscribeJobCreated = websocket.on(WebSocketEventType.JOB_CREATED, () => {
			// The websocket store already updates jobs.value.jobs,
			// so we don't need to manually reload
		});
	});

	onDestroy(() => {
		// Clean up WebSocket subscription
		if (unsubscribeJobCreated) {
			unsubscribeJobCreated();
		}
	});

	async function loadJobs(page: number = 1) {
		await jobs.loadJobs(filters, page, pagination.per_page);
	}

	async function handlePageChange(page: number) {
		await loadJobs(page);
	}

	async function handleFilterChange() {
		await loadJobs(1);
	}

	function handleView(job: Job) {
		goto(`/jobs/${job.id}`);
	}

	async function handleCancel(job: Job) {
		confirmCancelJob = job;
	}

	async function confirmCancel() {
		if (!confirmCancelJob) return;
		isCancelling = true;
		try {
			await jobs.cancelJob(confirmCancelJob.id);
			confirmCancelJob = null;
		} finally {
			isCancelling = false;
		}
	}

	async function handleDelete(job: Job) {
		confirmDeleteJob = job;
	}

	async function confirmDelete() {
		if (!confirmDeleteJob) return;
		isDeleting = true;
		try {
			await jobs.deleteJob(confirmDeleteJob.id);
			confirmDeleteJob = null;
		} finally {
			isDeleting = false;
		}
	}

	function clearFilters() {
		filters = {};
		loadJobs(1);
	}

	const hasFilters = $derived(
		filters.job_type !== undefined ||
			filters.source !== undefined ||
			filters.status !== undefined
	);
</script>

<svelte:head>
	<title>Jobs - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
		<!-- Header -->
		<div class="mb-8">
			<div class="flex items-center justify-between">
				<div>
					<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Background Jobs</h1>
					<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
						View and manage your sync, import, and export tasks
					</p>
				</div>
				<!-- WebSocket Status Indicator -->
				<div class="flex items-center gap-2 text-sm">
					{#if wsStatus === 'connected'}
						<span class="flex items-center gap-1.5 text-green-600 dark:text-green-400">
							<span class="w-2 h-2 bg-green-500 rounded-full animate-pulse"></span>
							Live updates
						</span>
					{:else if wsStatus === 'polling'}
						<span class="flex items-center gap-1.5 text-yellow-600 dark:text-yellow-400">
							<span class="w-2 h-2 bg-yellow-500 rounded-full"></span>
							Polling mode
						</span>
					{:else if wsStatus === 'connecting' || wsStatus === 'reconnecting'}
						<span class="flex items-center gap-1.5 text-gray-500 dark:text-gray-400">
							<span class="w-2 h-2 bg-gray-400 rounded-full animate-pulse"></span>
							Connecting...
						</span>
					{:else}
						<span class="flex items-center gap-1.5 text-gray-400 dark:text-gray-500">
							<span class="w-2 h-2 bg-gray-400 rounded-full"></span>
							Offline
						</span>
					{/if}
				</div>
			</div>
		</div>

		<!-- Filters -->
		<div
			class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-4 mb-6"
		>
			<div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
				<!-- Job Type Filter -->
				<div>
					<label
						for="job-type"
						class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
					>
						Type
					</label>
					<select
						id="job-type"
						class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
						bind:value={filters.job_type}
						onchange={handleFilterChange}
					>
						<option value={undefined}>All Types</option>
						{#each Object.values(JobType) as type}
							<option value={type}>{getJobTypeLabel(type)}</option>
						{/each}
					</select>
				</div>

				<!-- Source Filter -->
				<div>
					<label
						for="source"
						class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
					>
						Source
					</label>
					<select
						id="source"
						class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
						bind:value={filters.source}
						onchange={handleFilterChange}
					>
						<option value={undefined}>All Sources</option>
						{#each Object.values(JobSource) as source}
							<option value={source}>{getJobSourceLabel(source)}</option>
						{/each}
					</select>
				</div>

				<!-- Status Filter -->
				<div>
					<label
						for="status"
						class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
					>
						Status
					</label>
					<select
						id="status"
						class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
						bind:value={filters.status}
						onchange={handleFilterChange}
					>
						<option value={undefined}>All Statuses</option>
						{#each Object.values(JobStatus) as status}
							<option value={status}>{getJobStatusLabel(status)}</option>
						{/each}
					</select>
				</div>
			</div>

			{#if hasFilters}
				<div class="mt-4 flex justify-end">
					<button
						class="text-sm text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300"
						onclick={clearFilters}
					>
						Clear filters
					</button>
				</div>
			{/if}
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
						<h3 class="text-sm font-medium text-red-800 dark:text-red-400">Error loading jobs</h3>
						<p class="mt-1 text-sm text-red-700 dark:text-red-300">{error}</p>
					</div>
				</div>
			</div>
		{/if}

		<!-- Loading State -->
		{#if isLoading}
			<div class="flex justify-center items-center py-12">
				<div
					class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 dark:border-indigo-400"
				></div>
			</div>
		{:else if jobsList.length === 0}
			<!-- Empty State -->
			<div class="text-center py-12">
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
				<h3 class="mt-2 text-sm font-medium text-gray-900 dark:text-white">No jobs found</h3>
				<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
					{hasFilters ? 'Try adjusting your filters.' : 'Jobs will appear here when you sync, import, or export data.'}
				</p>
			</div>
		{:else}
			<!-- Jobs List -->
			<div class="space-y-4">
				{#each jobsList as job (job.id)}
					<JobCard {job} onView={handleView} onCancel={handleCancel} onDelete={handleDelete} />
				{/each}
			</div>

			<!-- Pagination -->
			{#if pagination.pages > 1}
				<div class="mt-6">
					<Pagination
						currentPage={pagination.page}
						totalPages={pagination.pages}
						totalItems={pagination.total}
						itemsPerPage={pagination.per_page}
						onPageChange={handlePageChange}
					/>
				</div>
			{/if}
		{/if}
	</div>

	<!-- Cancel Confirmation Modal -->
	{#if confirmCancelJob}
		<div
			class="fixed inset-0 z-50 overflow-y-auto"
			aria-labelledby="modal-title"
			role="dialog"
			aria-modal="true"
		>
			<div
				class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0"
			>
				<div
					class="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity"
					aria-hidden="true"
					onclick={() => (confirmCancelJob = null)}
					onkeydown={(e) => e.key === 'Escape' && (confirmCancelJob = null)}
					role="button"
					tabindex="-1"
				></div>
				<span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true"
					>&#8203;</span
				>
				<div
					class="inline-block align-bottom bg-white dark:bg-gray-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full"
				>
					<div class="bg-white dark:bg-gray-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
						<div class="sm:flex sm:items-start">
							<div
								class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-yellow-100 dark:bg-yellow-900/20 sm:mx-0 sm:h-10 sm:w-10"
							>
								<svg
									class="h-6 w-6 text-yellow-600 dark:text-yellow-400"
									fill="none"
									viewBox="0 0 24 24"
									stroke="currentColor"
								>
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
									/>
								</svg>
							</div>
							<div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
								<h3
									class="text-lg leading-6 font-medium text-gray-900 dark:text-white"
									id="modal-title"
								>
									Cancel Job
								</h3>
								<div class="mt-2">
									<p class="text-sm text-gray-500 dark:text-gray-400">
										Are you sure you want to cancel this job? This action cannot be undone.
									</p>
								</div>
							</div>
						</div>
					</div>
					<div
						class="bg-gray-50 dark:bg-gray-800/50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse gap-2"
					>
						<button
							type="button"
							class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-yellow-600 text-base font-medium text-white hover:bg-yellow-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-yellow-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50"
							onclick={confirmCancel}
							disabled={isCancelling}
						>
							{isCancelling ? 'Cancelling...' : 'Cancel Job'}
						</button>
						<button
							type="button"
							class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-700 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:mt-0 sm:w-auto sm:text-sm"
							onclick={() => (confirmCancelJob = null)}
						>
							Close
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}

	<!-- Delete Confirmation Modal -->
	{#if confirmDeleteJob}
		<div
			class="fixed inset-0 z-50 overflow-y-auto"
			aria-labelledby="modal-title"
			role="dialog"
			aria-modal="true"
		>
			<div
				class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0"
			>
				<div
					class="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity"
					aria-hidden="true"
					onclick={() => (confirmDeleteJob = null)}
					onkeydown={(e) => e.key === 'Escape' && (confirmDeleteJob = null)}
					role="button"
					tabindex="-1"
				></div>
				<span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true"
					>&#8203;</span
				>
				<div
					class="inline-block align-bottom bg-white dark:bg-gray-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full"
				>
					<div class="bg-white dark:bg-gray-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
						<div class="sm:flex sm:items-start">
							<div
								class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-red-100 dark:bg-red-900/20 sm:mx-0 sm:h-10 sm:w-10"
							>
								<svg
									class="h-6 w-6 text-red-600 dark:text-red-400"
									fill="none"
									viewBox="0 0 24 24"
									stroke="currentColor"
								>
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
									/>
								</svg>
							</div>
							<div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
								<h3
									class="text-lg leading-6 font-medium text-gray-900 dark:text-white"
									id="modal-title"
								>
									Delete Job
								</h3>
								<div class="mt-2">
									<p class="text-sm text-gray-500 dark:text-gray-400">
										Are you sure you want to delete this job? This will also delete all associated
										review items. This action cannot be undone.
									</p>
								</div>
							</div>
						</div>
					</div>
					<div
						class="bg-gray-50 dark:bg-gray-800/50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse gap-2"
					>
						<button
							type="button"
							class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50"
							onclick={confirmDelete}
							disabled={isDeleting}
						>
							{isDeleting ? 'Deleting...' : 'Delete'}
						</button>
						<button
							type="button"
							class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-700 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:mt-0 sm:w-auto sm:text-sm"
							onclick={() => (confirmDeleteJob = null)}
						>
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}
</RouteGuard>
