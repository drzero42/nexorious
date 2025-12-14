<script lang="ts">
	import type { Job } from '$lib/stores';
	import { JobStatus } from '$lib/stores';
	import {
		getJobTypeLabel,
		getJobSourceLabel,
		getJobStatusLabel,
		getJobStatusColor,
		formatDuration
	} from '$lib/types/jobs';
	import ProgressBar from './ProgressBar.svelte';

	interface Props {
		job: Job;
		compact?: boolean;
		onView?: (job: Job) => void;
		onCancel?: (job: Job) => void;
		onDelete?: (job: Job) => void;
	}

	let { job, compact = false, onView, onCancel, onDelete }: Props = $props();

	const isTerminal = $derived(job.is_terminal);
	const canCancel = $derived(
		!isTerminal && job.status !== JobStatus.PENDING && job.status !== JobStatus.AWAITING_REVIEW
	);
	const canDelete = $derived(isTerminal);
	const showProgress = $derived(
		job.status === JobStatus.PROCESSING || job.status === JobStatus.FINALIZING
	);

	function formatDate(dateStr: string | null): string {
		if (!dateStr) return '-';
		const date = new Date(dateStr);
		return date.toLocaleString();
	}

	function formatRelativeTime(dateStr: string | null): string {
		if (!dateStr) return '-';
		const date = new Date(dateStr);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffMins = Math.floor(diffMs / 60000);
		const diffHours = Math.floor(diffMins / 60);
		const diffDays = Math.floor(diffHours / 24);

		if (diffMins < 1) return 'Just now';
		if (diffMins < 60) return `${diffMins}m ago`;
		if (diffHours < 24) return `${diffHours}h ago`;
		return `${diffDays}d ago`;
	}
</script>

{#if compact}
	<!-- Compact card for table rows -->
	<div
		class="flex items-center justify-between p-3 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600 transition-colors cursor-pointer"
		onclick={() => onView?.(job)}
		onkeydown={(e) => e.key === 'Enter' && onView?.(job)}
		role="button"
		tabindex="0"
	>
		<div class="flex items-center gap-4 min-w-0">
			<span
				class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {getJobStatusColor(
					job.status
				)}"
			>
				{getJobStatusLabel(job.status)}
			</span>
			<div class="min-w-0">
				<div class="text-sm font-medium text-gray-900 dark:text-white truncate">
					{getJobTypeLabel(job.job_type)} - {getJobSourceLabel(job.source)}
				</div>
				<div class="text-xs text-gray-500 dark:text-gray-400">
					{formatRelativeTime(job.created_at)}
				</div>
			</div>
		</div>
		{#if showProgress}
			<div class="w-24 flex-shrink-0">
				<ProgressBar value={job.progress_percent} size="sm" />
			</div>
		{/if}
	</div>
{:else}
	<!-- Full card -->
	<div
		class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden"
	>
		<div class="p-4">
			<!-- Header -->
			<div class="flex items-start justify-between">
				<div>
					<h3 class="text-lg font-medium text-gray-900 dark:text-white">
						{getJobTypeLabel(job.job_type)} - {getJobSourceLabel(job.source)}
					</h3>
					<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
						Started {formatDate(job.started_at || job.created_at)}
					</p>
				</div>
				<span
					class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {getJobStatusColor(
						job.status
					)}"
				>
					{getJobStatusLabel(job.status)}
				</span>
			</div>

			<!-- Progress -->
			{#if showProgress}
				<div class="mt-4">
					<div class="flex justify-between text-sm text-gray-600 dark:text-gray-400 mb-1">
						<span>Progress</span>
						<span>{job.progress_current} / {job.progress_total}</span>
					</div>
					<ProgressBar value={job.progress_percent} />
				</div>
			{/if}

			<!-- Stats -->
			<div class="mt-4 grid grid-cols-2 gap-4 text-sm">
				<div>
					<span class="text-gray-500 dark:text-gray-400">Duration:</span>
					<span class="ml-1 text-gray-900 dark:text-white font-medium">
						{formatDuration(job.duration_seconds)}
					</span>
				</div>
				{#if job.review_item_count !== null && job.review_item_count > 0}
					<div>
						<span class="text-gray-500 dark:text-gray-400">Review items:</span>
						<span class="ml-1 text-gray-900 dark:text-white font-medium">
							{job.pending_review_count} / {job.review_item_count}
						</span>
					</div>
				{/if}
			</div>

			<!-- Error message -->
			{#if job.error_message}
				<div
					class="mt-4 p-3 bg-red-50 dark:bg-red-900/20 rounded-md text-sm text-red-700 dark:text-red-400"
				>
					{job.error_message}
				</div>
			{/if}
		</div>

		<!-- Actions -->
		<div
			class="px-4 py-3 bg-gray-50 dark:bg-gray-800/50 border-t border-gray-200 dark:border-gray-700 flex justify-end gap-2"
		>
			<button
				class="px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
				onclick={() => onView?.(job)}
			>
				View Details
			</button>
			{#if canCancel}
				<button
					class="px-3 py-1.5 text-sm font-medium text-yellow-700 dark:text-yellow-400 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md hover:bg-yellow-100 dark:hover:bg-yellow-900/40 transition-colors"
					onclick={() => onCancel?.(job)}
				>
					Cancel
				</button>
			{/if}
			{#if canDelete}
				<button
					class="px-3 py-1.5 text-sm font-medium text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md hover:bg-red-100 dark:hover:bg-red-900/40 transition-colors"
					onclick={() => onDelete?.(job)}
				>
					Delete
				</button>
			{/if}
		</div>
	</div>
{/if}
