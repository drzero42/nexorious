<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		review,
		ReviewItemStatus,
		type ReviewItem,
		type ReviewFilters,
		type IGDBCandidate
	} from '$lib/stores';
	import { websocket, WebSocketEventType } from '$lib/stores/websocket.svelte';
	import { RouteGuard, Pagination } from '$lib/components';
	import ReviewItemCard from '$lib/components/ReviewItemCard.svelte';

	let filters = $state<ReviewFilters>({});
	let selectedItem = $state<ReviewItem | null>(null);
	let isProcessing = $state(false);
	let processingItemId = $state<string | null>(null);

	const items = $derived(review.value.items);
	const summary = $derived(review.value.summary);
	const isLoading = $derived(review.value.isLoading);
	const error = $derived(review.value.error);
	const pagination = $derived(review.value.pagination);
	const wsStatus = $derived(websocket.value.status);

	// Parse job_id from URL query params
	const jobIdFromUrl = $derived($page.url.searchParams.get('job_id'));

	let unsubscribeReviewUpdate: (() => void) | null = null;

	onMount(() => {
		// Initialize filters from URL
		if (jobIdFromUrl) {
			filters.job_id = jobIdFromUrl;
		}
		loadItems();
		review.loadSummary();

		// Connect to WebSocket for real-time updates
		websocket.connect();

		// Subscribe to review item update events
		unsubscribeReviewUpdate = websocket.on(WebSocketEventType.REVIEW_ITEM_UPDATE, () => {
			// The websocket store already triggers review.loadSummary() on this event
			// Reload items to show new review items from running jobs
			loadItems(pagination.page);
		});
	});

	onDestroy(() => {
		// Clean up WebSocket subscription
		if (unsubscribeReviewUpdate) {
			unsubscribeReviewUpdate();
		}
	});

	async function loadItems(pageNum: number = 1) {
		await review.loadItems(filters, pageNum, pagination.per_page);
	}

	async function handlePageChange(page: number) {
		await loadItems(page);
	}

	async function handleFilterChange() {
		await loadItems(1);
	}

	async function handleMatch(item: ReviewItem, igdbId: number) {
		isProcessing = true;
		processingItemId = item.id;
		try {
			await review.matchItem(item.id, igdbId);
		} finally {
			isProcessing = false;
			processingItemId = null;
		}
	}

	async function handleSkip(item: ReviewItem) {
		isProcessing = true;
		processingItemId = item.id;
		try {
			await review.skipItem(item.id);
		} finally {
			isProcessing = false;
			processingItemId = null;
		}
	}

	async function handleKeep(item: ReviewItem) {
		isProcessing = true;
		processingItemId = item.id;
		try {
			await review.keepItem(item.id);
		} finally {
			isProcessing = false;
			processingItemId = null;
		}
	}

	async function handleRemove(item: ReviewItem) {
		isProcessing = true;
		processingItemId = item.id;
		try {
			await review.removeItem(item.id);
		} finally {
			isProcessing = false;
			processingItemId = null;
		}
	}

	function handleView(item: ReviewItem) {
		selectedItem = item;
	}

	function closeModal() {
		selectedItem = null;
	}

	async function handleModalMatch(igdbId: number) {
		if (!selectedItem) return;
		isProcessing = true;
		try {
			await review.matchItem(selectedItem.id, igdbId);
			selectedItem = null;
		} finally {
			isProcessing = false;
		}
	}

	async function handleModalSkip() {
		if (!selectedItem) return;
		isProcessing = true;
		try {
			await review.skipItem(selectedItem.id);
			selectedItem = null;
		} finally {
			isProcessing = false;
		}
	}

	function clearFilters() {
		filters = {};
		goto('/review', { replaceState: true });
		loadItems(1);
	}

	const hasFilters = $derived(
		filters.status !== undefined || filters.job_id !== undefined
	);

	// Get candidates from selected item
	const selectedCandidates = $derived(
		selectedItem
			? (selectedItem.igdb_candidates || []).map((c) => {
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
			: []
	);
</script>

<svelte:head>
	<title>Review Queue - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
		<!-- Header -->
		<div class="mb-8">
			<div class="flex items-center justify-between">
				<div>
					<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Review Queue</h1>
					<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
						Match unmatched games from your imports and syncs
					</p>
				</div>
				<div class="flex items-center gap-4">
					<!-- WebSocket Status Indicator -->
					<div class="text-sm">
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
						{/if}
					</div>
					{#if summary}
						<div class="text-right">
							<div class="text-2xl font-bold text-indigo-600 dark:text-indigo-400">
								{summary.total_pending}
							</div>
							<div class="text-sm text-gray-500 dark:text-gray-400">pending items</div>
						</div>
					{/if}
				</div>
			</div>
		</div>

		<!-- Summary Stats -->
		{#if summary}
			<div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
				<div
					class="bg-yellow-50 dark:bg-yellow-900/20 rounded-lg p-4 border border-yellow-200 dark:border-yellow-800"
				>
					<div class="text-2xl font-bold text-yellow-600 dark:text-yellow-400">
						{summary.total_pending}
					</div>
					<div class="text-sm text-yellow-700 dark:text-yellow-300">Pending</div>
				</div>
				<div
					class="bg-green-50 dark:bg-green-900/20 rounded-lg p-4 border border-green-200 dark:border-green-800"
				>
					<div class="text-2xl font-bold text-green-600 dark:text-green-400">
						{summary.total_matched}
					</div>
					<div class="text-sm text-green-700 dark:text-green-300">Matched</div>
				</div>
				<div
					class="bg-gray-50 dark:bg-gray-700 rounded-lg p-4 border border-gray-200 dark:border-gray-600"
				>
					<div class="text-2xl font-bold text-gray-600 dark:text-gray-400">
						{summary.total_skipped}
					</div>
					<div class="text-sm text-gray-700 dark:text-gray-300">Skipped</div>
				</div>
				<div
					class="bg-red-50 dark:bg-red-900/20 rounded-lg p-4 border border-red-200 dark:border-red-800"
				>
					<div class="text-2xl font-bold text-red-600 dark:text-red-400">
						{summary.total_removal}
					</div>
					<div class="text-sm text-red-700 dark:text-red-300">Removed</div>
				</div>
			</div>
		{/if}

		<!-- Filters -->
		<div
			class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-4 mb-6"
		>
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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
						{#each Object.values(ReviewItemStatus) as status}
							<option value={status}>{status.charAt(0).toUpperCase() + status.slice(1)}</option>
						{/each}
					</select>
				</div>

				<!-- Job ID Filter -->
				<div>
					<label
						for="job_id"
						class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
					>
						Job ID
					</label>
					<input
						type="text"
						id="job_id"
						class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500"
						placeholder="Filter by job ID"
						bind:value={filters.job_id}
						onchange={handleFilterChange}
					/>
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
						<h3 class="text-sm font-medium text-red-800 dark:text-red-400">Error</h3>
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
		{:else if items.length === 0}
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
						d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
					/>
				</svg>
				<h3 class="mt-2 text-sm font-medium text-gray-900 dark:text-white">No items to review</h3>
				<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
					{hasFilters
						? 'Try adjusting your filters.'
						: 'All your imports and syncs have been fully matched!'}
				</p>
			</div>
		{:else}
			<!-- Items List -->
			<div class="space-y-4">
				{#each items as item (item.id)}
					<ReviewItemCard
						{item}
						onMatch={handleMatch}
						onSkip={handleSkip}
						onKeep={handleKeep}
						onRemove={handleRemove}
						onView={handleView}
						isProcessing={isProcessing && processingItemId === item.id}
					/>
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

	<!-- IGDB Candidates Modal -->
	{#if selectedItem}
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
					onclick={closeModal}
					onkeydown={(e) => e.key === 'Escape' && closeModal()}
					role="button"
					tabindex="-1"
				></div>
				<span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true"
					>&#8203;</span
				>
				<div
					class="inline-block align-bottom bg-white dark:bg-gray-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-2xl sm:w-full"
				>
					<div class="bg-white dark:bg-gray-800 px-4 pt-5 pb-4 sm:p-6">
						<div class="mb-4">
							<h3 class="text-lg font-medium text-gray-900 dark:text-white" id="modal-title">
								Match: {selectedItem.source_title}
							</h3>
							<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
								Select the correct IGDB match for this game
							</p>
						</div>

						{#if selectedCandidates.length === 0}
							<div class="text-center py-8">
								<p class="text-gray-500 dark:text-gray-400">
									No IGDB candidates found. Try searching manually.
								</p>
							</div>
						{:else}
							<div class="space-y-3 max-h-96 overflow-y-auto">
								{#each selectedCandidates as candidate, index}
									<button
										class="w-full text-left p-3 rounded-lg border border-gray-200 dark:border-gray-700 hover:border-indigo-500 dark:hover:border-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-900/20 transition-colors disabled:opacity-50"
										onclick={() => handleModalMatch(candidate.igdb_id)}
										disabled={isProcessing}
									>
										<div class="flex items-start gap-3">
											{#if candidate.cover_url}
												<img
													src={candidate.cover_url}
													alt={candidate.name}
													class="w-16 h-20 object-cover rounded"
												/>
											{:else}
												<div
													class="w-16 h-20 bg-gray-200 dark:bg-gray-600 rounded flex items-center justify-center"
												>
													<svg
														class="w-8 h-8 text-gray-400"
														fill="none"
														viewBox="0 0 24 24"
														stroke="currentColor"
													>
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
												<p class="font-medium text-gray-900 dark:text-white">
													{candidate.name}
													{#if candidate.first_release_date}
														<span class="text-gray-500 dark:text-gray-400">
															({new Date(candidate.first_release_date * 1000).getFullYear()})
														</span>
													{/if}
												</p>
												{#if candidate.similarity_score !== null}
													<p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
														Match confidence: {Math.round(candidate.similarity_score * 100)}%
													</p>
												{/if}
												{#if candidate.summary}
													<p class="text-sm text-gray-600 dark:text-gray-400 mt-1 line-clamp-2">
														{candidate.summary}
													</p>
												{/if}
												{#if candidate.platforms && candidate.platforms.length > 0}
													<p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
														Platforms: {candidate.platforms.slice(0, 5).join(', ')}
														{#if candidate.platforms.length > 5}
															+{candidate.platforms.length - 5} more
														{/if}
													</p>
												{/if}
											</div>
											{#if index === 0}
												<span
													class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300"
												>
													Best Match
												</span>
											{/if}
										</div>
									</button>
								{/each}
							</div>
						{/if}
					</div>
					<div
						class="bg-gray-50 dark:bg-gray-800/50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse gap-2"
					>
						<button
							type="button"
							class="w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-700 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:w-auto sm:text-sm"
							onclick={handleModalSkip}
							disabled={isProcessing}
						>
							Skip
						</button>
						<button
							type="button"
							class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-700 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:mt-0 sm:w-auto sm:text-sm"
							onclick={closeModal}
						>
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}
</RouteGuard>
