<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { jobs, JobStatus } from '$lib/stores';
	import { websocket, WebSocketEventType, type JobWebSocketMessage } from '$lib/stores/websocket.svelte';
	import { RouteGuard, ProgressBar } from '$lib/components';
	import {
		getJobTypeLabel,
		getJobSourceLabel,
		getJobStatusLabel,
		getJobStatusColor,
		formatDuration
	} from '$lib/types/jobs';

	let confirmDelete = $state(false);
	let confirmCancel = $state(false);
	let isDeleting = $state(false);
	let isCancelling = $state(false);
	let isConfirming = $state(false);

	const job = $derived(jobs.value.currentJob);
	const isLoading = $derived(jobs.value.isLoading);
	const error = $derived(jobs.value.error);

	const isTerminal = $derived(job?.is_terminal ?? false);
	const canCancel = $derived(
		!isTerminal &&
			job?.status !== JobStatus.PENDING &&
			job?.status !== JobStatus.AWAITING_REVIEW
	);
	const canDelete = $derived(isTerminal);
	const canConfirm = $derived(
		job?.job_type === 'import' &&
			(job?.status === JobStatus.READY || job?.status === JobStatus.AWAITING_REVIEW) &&
			job?.pending_review_count === 0
	);
	const showProgress = $derived(
		job?.status === JobStatus.PROCESSING || job?.status === JobStatus.FINALIZING
	);
	const wsStatus = $derived(websocket.value.status);

	let unsubscribers: (() => void)[] = [];

	onMount(() => {
		const jobId = $page.params.id;
		if (jobId) {
			jobs.getJob(jobId);

			// Connect to WebSocket for real-time updates
			websocket.connect();

			// Track this job for polling fallback
			websocket.trackJob(jobId);

			// Subscribe to relevant events for this specific job
			const handleJobUpdate = (message: JobWebSocketMessage) => {
				if (message.job.id === jobId) {
					// The websocket store already updates currentJob via updateJobsStore
				}
			};

			unsubscribers.push(
				websocket.on(WebSocketEventType.JOB_PROGRESS, handleJobUpdate),
				websocket.on(WebSocketEventType.JOB_STATUS_CHANGE, handleJobUpdate),
				websocket.on(WebSocketEventType.JOB_COMPLETED, handleJobUpdate),
				websocket.on(WebSocketEventType.JOB_FAILED, handleJobUpdate)
			);
		}
	});

	onDestroy(() => {
		// Clean up WebSocket subscriptions
		unsubscribers.forEach((unsub) => unsub());

		// Stop tracking job for polling
		const jobId = $page.params.id;
		if (jobId) {
			websocket.untrackJob(jobId);
		}
	});

	function formatDate(dateStr: string | null): string {
		if (!dateStr) return '-';
		const date = new Date(dateStr);
		return date.toLocaleString();
	}

	async function handleCancel() {
		if (!job) return;
		isCancelling = true;
		try {
			await jobs.cancelJob(job.id);
			confirmCancel = false;
		} finally {
			isCancelling = false;
		}
	}

	async function handleDelete() {
		if (!job) return;
		isDeleting = true;
		try {
			await jobs.deleteJob(job.id);
			confirmDelete = false;
			goto('/jobs');
		} finally {
			isDeleting = false;
		}
	}

	async function handleConfirm() {
		if (!job) return;
		isConfirming = true;
		try {
			await jobs.confirmJob(job.id);
		} finally {
			isConfirming = false;
		}
	}

	async function handleRefresh() {
		if (!job) return;
		await jobs.getJob(job.id);
	}

	function viewReviewItems() {
		if (!job) return;
		goto(`/review?job_id=${job.id}`);
	}
</script>

<svelte:head>
	<title>{job ? `${getJobTypeLabel(job.job_type)} Job` : 'Job Details'} - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
		<!-- Back link -->
		<div class="mb-6">
			<a
				href="/jobs"
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
				Back to Jobs
			</a>
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
						<h3 class="text-sm font-medium text-red-800 dark:text-red-400">Error loading job</h3>
						<p class="mt-1 text-sm text-red-700 dark:text-red-300">{error}</p>
					</div>
				</div>
			</div>
		{/if}

		<!-- Loading State -->
		{#if isLoading && !job}
			<div class="flex justify-center items-center py-12">
				<div
					class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600 dark:border-indigo-400"
				></div>
			</div>
		{:else if job}
			<!-- Job Header -->
			<div
				class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden"
			>
				<div class="p-6">
					<div class="flex items-start justify-between">
						<div>
							<h1 class="text-2xl font-bold text-gray-900 dark:text-white">
								{getJobTypeLabel(job.job_type)} - {getJobSourceLabel(job.source)}
							</h1>
							<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
								Job ID: {job.id}
							</p>
						</div>
						<div class="flex items-center gap-2">
							<span
								class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium {getJobStatusColor(
									job.status
								)}"
							>
								{getJobStatusLabel(job.status)}
							</span>
							<!-- WebSocket Status Indicator -->
							{#if wsStatus === 'connected'}
								<span class="flex items-center gap-1.5 text-xs text-green-600 dark:text-green-400" title="Receiving live updates">
									<span class="w-1.5 h-1.5 bg-green-500 rounded-full animate-pulse"></span>
									Live
								</span>
							{:else if wsStatus === 'polling'}
								<span class="flex items-center gap-1.5 text-xs text-yellow-600 dark:text-yellow-400" title="Polling for updates">
									<span class="w-1.5 h-1.5 bg-yellow-500 rounded-full"></span>
									Polling
								</span>
							{/if}
							<button
								class="p-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 rounded-md"
								onclick={handleRefresh}
								title="Refresh"
							>
								<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
									/>
								</svg>
							</button>
						</div>
					</div>

					<!-- Progress -->
					{#if showProgress}
						<div class="mt-6">
							<div class="flex justify-between text-sm text-gray-600 dark:text-gray-400 mb-2">
								<span>Progress</span>
								<span>{job.progress_current} / {job.progress_total} ({job.progress_percent}%)</span>
							</div>
							<ProgressBar value={job.progress_percent} />
						</div>
					{/if}

					<!-- Error message -->
					{#if job.error_message}
						<div
							class="mt-6 p-4 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800"
						>
							<h3 class="text-sm font-medium text-red-800 dark:text-red-400">Error</h3>
							<p class="mt-1 text-sm text-red-700 dark:text-red-300">{job.error_message}</p>
						</div>
					{/if}
				</div>

				<!-- Job Details -->
				<div class="border-t border-gray-200 dark:border-gray-700 px-6 py-4">
					<dl class="grid grid-cols-1 sm:grid-cols-2 gap-4">
						<div>
							<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">Created</dt>
							<dd class="mt-1 text-sm text-gray-900 dark:text-white">
								{formatDate(job.created_at)}
							</dd>
						</div>
						<div>
							<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">Started</dt>
							<dd class="mt-1 text-sm text-gray-900 dark:text-white">
								{formatDate(job.started_at)}
							</dd>
						</div>
						<div>
							<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">Completed</dt>
							<dd class="mt-1 text-sm text-gray-900 dark:text-white">
								{formatDate(job.completed_at)}
							</dd>
						</div>
						<div>
							<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">Duration</dt>
							<dd class="mt-1 text-sm text-gray-900 dark:text-white">
								{formatDuration(job.duration_seconds)}
							</dd>
						</div>
						<div>
							<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">Priority</dt>
							<dd class="mt-1 text-sm text-gray-900 dark:text-white capitalize">
								{job.priority}
							</dd>
						</div>
						{#if job.taskiq_task_id}
							<div>
								<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">Task ID</dt>
								<dd
									class="mt-1 text-sm text-gray-900 dark:text-white font-mono text-xs truncate"
									title={job.taskiq_task_id}
								>
									{job.taskiq_task_id}
								</dd>
							</div>
						{/if}
					</dl>
				</div>

				<!-- Review Items Section -->
				{#if job.review_item_count !== null && job.review_item_count > 0}
					<div class="border-t border-gray-200 dark:border-gray-700 px-6 py-4">
						<div class="flex items-center justify-between">
							<div>
								<h3 class="text-sm font-medium text-gray-900 dark:text-white">Review Items</h3>
								<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
									{job.pending_review_count} pending out of {job.review_item_count} total
								</p>
							</div>
							<button
								class="px-4 py-2 text-sm font-medium text-indigo-600 dark:text-indigo-400 bg-indigo-50 dark:bg-indigo-900/20 rounded-md hover:bg-indigo-100 dark:hover:bg-indigo-900/40 transition-colors"
								onclick={viewReviewItems}
							>
								View Review Queue
							</button>
						</div>
					</div>
				{/if}

				<!-- Result Summary -->
				{#if Object.keys(job.result_summary).length > 0}
					<div class="border-t border-gray-200 dark:border-gray-700 px-6 py-4">
						<h3 class="text-sm font-medium text-gray-900 dark:text-white mb-3">Result Summary</h3>
						<dl class="grid grid-cols-2 sm:grid-cols-3 gap-4">
							{#each Object.entries(job.result_summary) as [key, value]}
								<div>
									<dt class="text-xs font-medium text-gray-500 dark:text-gray-400 capitalize">
										{key.replace(/_/g, ' ')}
									</dt>
									<dd class="mt-1 text-sm text-gray-900 dark:text-white">
										{typeof value === 'object' ? JSON.stringify(value) : String(value)}
									</dd>
								</div>
							{/each}
						</dl>
					</div>
				{/if}

				<!-- Actions -->
				<div
					class="border-t border-gray-200 dark:border-gray-700 px-6 py-4 bg-gray-50 dark:bg-gray-800/50 flex justify-end gap-3"
				>
					{#if canConfirm}
						<button
							class="px-4 py-2 text-sm font-medium text-white bg-green-600 rounded-md hover:bg-green-700 transition-colors disabled:opacity-50"
							onclick={handleConfirm}
							disabled={isConfirming}
						>
							{isConfirming ? 'Confirming...' : 'Confirm Import'}
						</button>
					{/if}
					{#if canCancel}
						<button
							class="px-4 py-2 text-sm font-medium text-yellow-700 dark:text-yellow-400 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md hover:bg-yellow-100 dark:hover:bg-yellow-900/40 transition-colors"
							onclick={() => (confirmCancel = true)}
						>
							Cancel Job
						</button>
					{/if}
					{#if canDelete}
						<button
							class="px-4 py-2 text-sm font-medium text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md hover:bg-red-100 dark:hover:bg-red-900/40 transition-colors"
							onclick={() => (confirmDelete = true)}
						>
							Delete Job
						</button>
					{/if}
				</div>
			</div>
		{:else}
			<!-- Not Found -->
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
						d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
					/>
				</svg>
				<h3 class="mt-2 text-sm font-medium text-gray-900 dark:text-white">Job not found</h3>
				<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
					The job you're looking for doesn't exist or has been deleted.
				</p>
				<div class="mt-6">
					<a
						href="/jobs"
						class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700"
					>
						Back to Jobs
					</a>
				</div>
			</div>
		{/if}
	</div>

	<!-- Cancel Confirmation Modal -->
	{#if confirmCancel}
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
					onclick={() => (confirmCancel = false)}
					onkeydown={(e) => e.key === 'Escape' && (confirmCancel = false)}
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
							onclick={handleCancel}
							disabled={isCancelling}
						>
							{isCancelling ? 'Cancelling...' : 'Cancel Job'}
						</button>
						<button
							type="button"
							class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-700 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:mt-0 sm:w-auto sm:text-sm"
							onclick={() => (confirmCancel = false)}
						>
							Close
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}

	<!-- Delete Confirmation Modal -->
	{#if confirmDelete}
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
					onclick={() => (confirmDelete = false)}
					onkeydown={(e) => e.key === 'Escape' && (confirmDelete = false)}
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
							onclick={handleDelete}
							disabled={isDeleting}
						>
							{isDeleting ? 'Deleting...' : 'Delete'}
						</button>
						<button
							type="button"
							class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-700 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:mt-0 sm:w-auto sm:text-sm"
							onclick={() => (confirmDelete = false)}
						>
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}
</RouteGuard>
