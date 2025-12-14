<script lang="ts">
	import type { ReviewItem, IGDBCandidate } from '$lib/stores';
	import { ReviewItemStatus } from '$lib/stores';

	interface Props {
		item: ReviewItem;
		onMatch?: (item: ReviewItem, igdbId: number) => void;
		onSkip?: (item: ReviewItem) => void;
		onKeep?: (item: ReviewItem) => void;
		onRemove?: (item: ReviewItem) => void;
		onView?: (item: ReviewItem) => void;
		isProcessing?: boolean;
	}

	let {
		item,
		onMatch,
		onSkip,
		onKeep,
		onRemove,
		onView,
		isProcessing = false
	}: Props = $props();

	const isPending = $derived(item.status === ReviewItemStatus.PENDING);
	const isRemovalItem = $derived(
		item.source_metadata && (item.source_metadata as Record<string, unknown>).removal_detected
	);

	// Get candidates as typed array
	const candidates = $derived(
		(item.igdb_candidates || []).map((c) => {
			if ('igdb_id' in c) return c as IGDBCandidate;
			const raw = c as Record<string, unknown>;
			return {
				igdb_id: (raw.igdb_id || raw.id || 0) as number,
				name: (raw.name || '') as string,
				first_release_date: raw.first_release_date as number | null,
				cover_url: raw.cover_url as string | null,
				summary: raw.summary as string | null,
				platforms: raw.platforms as string[] | null,
				similarity_score: raw.similarity_score as number | null
			};
		})
	);

	const topCandidate = $derived(candidates[0] || null);

	function formatReleaseYear(timestamp: number | null): string {
		if (!timestamp) return '';
		const date = new Date(timestamp * 1000);
		return `(${date.getFullYear()})`;
	}

	function getStatusBadge(status: ReviewItemStatus): { class: string; label: string } {
		switch (status) {
			case ReviewItemStatus.PENDING:
				return {
					class: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300',
					label: 'Pending'
				};
			case ReviewItemStatus.MATCHED:
				return {
					class: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300',
					label: 'Matched'
				};
			case ReviewItemStatus.SKIPPED:
				return {
					class: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
					label: 'Skipped'
				};
			case ReviewItemStatus.REMOVAL:
				return {
					class: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300',
					label: 'Removed'
				};
			default:
				return { class: 'bg-gray-100 text-gray-800', label: status };
		}
	}

	const statusBadge = $derived(getStatusBadge(item.status));
</script>

<div
	class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden"
>
	<div class="p-4">
		<!-- Header -->
		<div class="flex items-start justify-between gap-4">
			<div class="min-w-0 flex-1">
				<h3 class="text-lg font-medium text-gray-900 dark:text-white truncate">
					{item.source_title}
				</h3>
				{#if item.job_type && item.job_source}
					<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
						From: {item.job_type} ({item.job_source})
					</p>
				{/if}
			</div>
			<span
				class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {statusBadge.class}"
			>
				{statusBadge.label}
			</span>
		</div>

		<!-- Removal warning -->
		{#if isRemovalItem}
			<div
				class="mt-3 p-3 bg-orange-50 dark:bg-orange-900/20 rounded-md border border-orange-200 dark:border-orange-800"
			>
				<div class="flex">
					<svg
						class="h-5 w-5 text-orange-400"
						viewBox="0 0 20 20"
						fill="currentColor"
						aria-hidden="true"
					>
						<path
							fill-rule="evenodd"
							d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z"
							clip-rule="evenodd"
						/>
					</svg>
					<div class="ml-3">
						<p class="text-sm text-orange-700 dark:text-orange-300">
							This game was detected as removed from your library during sync.
						</p>
					</div>
				</div>
			</div>
		{/if}

		<!-- Best match suggestion -->
		{#if isPending && topCandidate}
			<div
				class="mt-4 p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg border border-gray-200 dark:border-gray-600"
			>
				<div class="flex items-start gap-3">
					{#if topCandidate.cover_url}
						<img
							src={topCandidate.cover_url}
							alt={topCandidate.name}
							class="w-12 h-16 object-cover rounded"
						/>
					{:else}
						<div class="w-12 h-16 bg-gray-200 dark:bg-gray-600 rounded flex items-center justify-center">
							<svg class="w-6 h-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path
									stroke-linecap="round"
									stroke-linejoin="round"
									stroke-width="2"
									d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
								/>
							</svg>
						</div>
					{/if}
					<div class="flex-1 min-w-0">
						<p class="text-sm font-medium text-gray-900 dark:text-white">
							{topCandidate.name}
							<span class="text-gray-500 dark:text-gray-400">
								{formatReleaseYear(topCandidate.first_release_date)}
							</span>
						</p>
						{#if topCandidate.similarity_score !== null}
							<p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
								Match confidence: {Math.round(topCandidate.similarity_score * 100)}%
							</p>
						{/if}
						{#if topCandidate.summary}
							<p
								class="text-xs text-gray-600 dark:text-gray-400 mt-1 line-clamp-2"
								title={topCandidate.summary}
							>
								{topCandidate.summary}
							</p>
						{/if}
					</div>
				</div>
			</div>
		{/if}

		<!-- Resolved info -->
		{#if !isPending && item.resolved_igdb_id}
			<div class="mt-3 text-sm text-gray-600 dark:text-gray-400">
				Matched to IGDB ID: <span class="font-mono">{item.resolved_igdb_id}</span>
			</div>
		{/if}
	</div>

	<!-- Actions -->
	{#if isPending}
		<div
			class="px-4 py-3 bg-gray-50 dark:bg-gray-800/50 border-t border-gray-200 dark:border-gray-700 flex flex-wrap gap-2"
		>
			{#if isRemovalItem}
				<!-- Removal item actions -->
				<button
					class="flex-1 px-3 py-2 text-sm font-medium text-white bg-green-600 rounded-md hover:bg-green-700 transition-colors disabled:opacity-50"
					onclick={() => onKeep?.(item)}
					disabled={isProcessing}
				>
					Keep in Collection
				</button>
				<button
					class="flex-1 px-3 py-2 text-sm font-medium text-white bg-red-600 rounded-md hover:bg-red-700 transition-colors disabled:opacity-50"
					onclick={() => onRemove?.(item)}
					disabled={isProcessing}
				>
					Remove
				</button>
			{:else}
				<!-- Regular matching actions -->
				{#if topCandidate}
					<button
						class="flex-1 px-3 py-2 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 transition-colors disabled:opacity-50"
						onclick={() => onMatch?.(item, topCandidate.igdb_id)}
						disabled={isProcessing}
					>
						Match
					</button>
				{/if}
				<button
					class="flex-1 px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors disabled:opacity-50"
					onclick={() => onView?.(item)}
					disabled={isProcessing}
				>
					{candidates.length > 1 ? `View ${candidates.length} Options` : 'Search IGDB'}
				</button>
				<button
					class="px-3 py-2 text-sm font-medium text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 transition-colors disabled:opacity-50"
					onclick={() => onSkip?.(item)}
					disabled={isProcessing}
				>
					Skip
				</button>
			{/if}
		</div>
	{:else}
		<div
			class="px-4 py-3 bg-gray-50 dark:bg-gray-800/50 border-t border-gray-200 dark:border-gray-700"
		>
			<button
				class="text-sm text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300"
				onclick={() => onView?.(item)}
			>
				View Details
			</button>
		</div>
	{/if}
</div>
