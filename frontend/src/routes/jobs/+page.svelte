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
	let cancelError = $state<string | null>(null);
	let deleteError = $state<string | null>(null);

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
		cancelError = null;
		confirmCancelJob = job;
	}

	async function confirmCancel() {
		if (!confirmCancelJob) return;
		isCancelling = true;
		cancelError = null;
		try {
			await jobs.cancelJob(confirmCancelJob.id);
			confirmCancelJob = null;
		} catch (e) {
			console.error('Failed to cancel job:', e);
			cancelError = e instanceof Error ? e.message : 'Failed to cancel job';
			// Keep dialog open so user can see the error
		} finally {
			isCancelling = false;
		}
	}

	async function handleDelete(job: Job) {
		deleteError = null;
		confirmDeleteJob = job;
	}

	async function confirmDelete() {
		if (!confirmDeleteJob) return;
		isDeleting = true;
		deleteError = null;
		try {
			await jobs.deleteJob(confirmDeleteJob.id);
			confirmDeleteJob = null;
		} catch (e) {
			console.error('Failed to delete job:', e);
			deleteError = e instanceof Error ? e.message : 'Failed to delete job';
			// Keep dialog open so user can see the error
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
</RouteGuard>

<!-- Cancel Confirmation Modal - OUTSIDE RouteGuard -->
{#if confirmCancelJob}
	<!-- Modal backdrop -->
	<div
		style="position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(107, 114, 128, 0.75); z-index: 9998;"
		onclick={() => { cancelError = null; confirmCancelJob = null; }}
		onkeydown={(e) => { if (e.key === 'Escape') { cancelError = null; confirmCancelJob = null; } }}
		role="button"
		tabindex="-1"
		aria-hidden="true"
	></div>
	<!-- Modal dialog -->
	<div
		style="position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%); z-index: 9999; background: white; border-radius: 8px; max-width: 32rem; width: calc(100% - 32px); box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);"
		role="dialog"
		aria-modal="true"
		aria-labelledby="cancel-modal-title"
	>
		<div style="padding: 24px;">
			<div style="display: flex; align-items: flex-start; gap: 16px;">
				<div
					style="flex-shrink: 0; width: 40px; height: 40px; background: #fef3c7; border-radius: 50%; display: flex; align-items: center; justify-content: center;"
				>
					<svg
						style="width: 24px; height: 24px; stroke: #ca8a04;"
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
				<div style="flex: 1;">
					<h3 id="cancel-modal-title" style="font-size: 18px; font-weight: 600; color: #111827; margin: 0 0 8px 0;">
						Cancel Job
					</h3>
					<p style="font-size: 14px; color: #6b7280; margin: 0;">
						Are you sure you want to cancel this job? This action cannot be undone.
					</p>
					{#if cancelError}
						<div style="margin-top: 12px; padding: 12px; background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px;">
							<p style="font-size: 14px; color: #dc2626; margin: 0;">{cancelError}</p>
						</div>
					{/if}
				</div>
			</div>
		</div>
		<div style="padding: 12px 24px; background: #f9fafb; display: flex; justify-content: flex-end; gap: 8px; border-top: 1px solid #e5e7eb;">
			<button
				type="button"
				style="padding: 8px 16px; background: white; color: #374151; border: 1px solid #d1d5db; border-radius: 6px; font-weight: 500; cursor: pointer;"
				onclick={() => { cancelError = null; confirmCancelJob = null; }}
			>
				Close
			</button>
			<button
				type="button"
				style="padding: 8px 16px; background: #ca8a04; color: white; border: none; border-radius: 6px; font-weight: 500; cursor: pointer;"
				onclick={confirmCancel}
				disabled={isCancelling}
			>
				{isCancelling ? 'Cancelling...' : 'Cancel Job'}
			</button>
		</div>
	</div>
{/if}

<!-- Delete Confirmation Modal -->
{#if confirmDeleteJob}
	<!-- Modal backdrop -->
	<div
		style="position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(107, 114, 128, 0.75); z-index: 9998;"
		onclick={() => { deleteError = null; confirmDeleteJob = null; }}
		onkeydown={(e) => { if (e.key === 'Escape') { deleteError = null; confirmDeleteJob = null; } }}
		role="button"
		tabindex="-1"
		aria-hidden="true"
	></div>
	<!-- Modal dialog -->
	<div
		style="position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%); z-index: 9999; background: white; border-radius: 8px; max-width: 32rem; width: calc(100% - 32px); box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);"
		role="dialog"
		aria-modal="true"
		aria-labelledby="delete-modal-title"
	>
		<div style="padding: 24px;">
			<div style="display: flex; align-items: flex-start; gap: 16px;">
				<div
					style="flex-shrink: 0; width: 40px; height: 40px; background: #fee2e2; border-radius: 50%; display: flex; align-items: center; justify-content: center;"
				>
					<svg
						style="width: 24px; height: 24px; stroke: #dc2626;"
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
				<div style="flex: 1;">
					<h3 id="delete-modal-title" style="font-size: 18px; font-weight: 600; color: #111827; margin: 0 0 8px 0;">
						Delete Job
					</h3>
					<p style="font-size: 14px; color: #6b7280; margin: 0;">
						Are you sure you want to delete this job? This will also delete all associated review items. This action cannot be undone.
					</p>
					{#if deleteError}
						<div style="margin-top: 12px; padding: 12px; background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px;">
							<p style="font-size: 14px; color: #dc2626; margin: 0;">{deleteError}</p>
						</div>
					{/if}
				</div>
			</div>
		</div>
		<div style="padding: 12px 24px; background: #f9fafb; display: flex; justify-content: flex-end; gap: 8px; border-top: 1px solid #e5e7eb;">
			<button
				type="button"
				style="padding: 8px 16px; background: white; color: #374151; border: 1px solid #d1d5db; border-radius: 6px; font-weight: 500; cursor: pointer;"
				onclick={() => { deleteError = null; confirmDeleteJob = null; }}
			>
				Cancel
			</button>
			<button
				type="button"
				style="padding: 8px 16px; background: #dc2626; color: white; border: none; border-radius: 6px; font-weight: 500; cursor: pointer;"
				onclick={confirmDelete}
				disabled={isDeleting}
			>
				{isDeleting ? 'Deleting...' : 'Delete'}
			</button>
		</div>
	</div>
{/if}
