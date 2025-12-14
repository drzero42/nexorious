<script lang="ts">
	import { appStatus } from '$lib/stores';

	const DOCS_URL = '/docs/igdb-setup.md';
</script>

{#if !appStatus.value.igdbConfigured && appStatus.value.hasFetched}
	<div
		class="bg-amber-50 border border-amber-200 rounded-lg p-4 mb-4"
		role="alert"
		aria-live="polite"
	>
		<div class="flex items-start">
			<div class="flex-shrink-0">
				<svg
					class="h-5 w-5 text-amber-400"
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
			</div>
			<div class="ml-3">
				<h3 class="text-sm font-medium text-amber-800">IGDB API Not Configured</h3>
				<div class="mt-2 text-sm text-amber-700">
					<p>
						Game search and import features are unavailable. IGDB API credentials are required
						for these features.
					</p>
				</div>
				<div class="mt-3">
					<a
						href={DOCS_URL}
						target="_blank"
						rel="noopener noreferrer"
						class="text-sm font-medium text-amber-800 hover:text-amber-900 underline"
					>
						View Setup Guide &rarr;
					</a>
				</div>
			</div>
		</div>
	</div>
{/if}
