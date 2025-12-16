<script lang="ts">
	import { platforms as platformsStore } from '$lib/stores';

	interface MappingSuggestion {
		original: string;
		count: number;
		suggested_id: string | null;
		suggested_name: string | null;
	}

	interface Props {
		platformSuggestions: MappingSuggestion[];
		storefrontSuggestions: MappingSuggestion[];
		platformMappings: Record<string, string>;
		storefrontMappings: Record<string, string>;
		onPlatformMappingChange: (original: string, platformId: string) => void;
		onStorefrontMappingChange: (original: string, storefrontId: string) => void;
	}

	let {
		platformSuggestions,
		storefrontSuggestions,
		platformMappings,
		storefrontMappings,
		onPlatformMappingChange,
		onStorefrontMappingChange
	}: Props = $props();

	const platforms = $derived(platformsStore.value.platforms);
	const storefronts = $derived(platformsStore.value.storefronts);

	const unresolvedPlatformCount = $derived(
		platformSuggestions.filter((p) => !platformMappings[p.original]).length
	);
	const unresolvedStorefrontCount = $derived(
		storefrontSuggestions.filter((s) => !storefrontMappings[s.original]).length
	);
	const hasUnresolved = $derived(unresolvedPlatformCount > 0 || unresolvedStorefrontCount > 0);
</script>

{#if platformSuggestions.length > 0 || storefrontSuggestions.length > 0}
	<div class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6 mb-6">
		<div class="flex items-center justify-between mb-4">
			<h2 class="text-lg font-medium text-gray-900 dark:text-white">
				Platform & Storefront Mappings
			</h2>
			{#if hasUnresolved}
				<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300">
					{unresolvedPlatformCount + unresolvedStorefrontCount} need mapping
				</span>
			{:else}
				<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300">
					All mapped
				</span>
			{/if}
		</div>

		{#if platformSuggestions.length > 0}
			<div class="mb-6">
				<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Platforms</h3>
				<div class="space-y-3">
					{#each platformSuggestions as suggestion}
						<div class="flex items-center gap-4">
							<div class="flex-1 min-w-0">
								<span class="text-sm font-medium text-gray-900 dark:text-white">
									"{suggestion.original}"
								</span>
								<span class="text-sm text-gray-500 dark:text-gray-400 ml-2">
									({suggestion.count} game{suggestion.count !== 1 ? 's' : ''})
								</span>
							</div>
							<div class="flex items-center gap-2">
								<span class="text-gray-400">-></span>
								<select
									class="block w-48 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm"
									value={platformMappings[suggestion.original] || suggestion.suggested_id || ''}
									onchange={(e) => onPlatformMappingChange(suggestion.original, e.currentTarget.value)}
								>
									<option value="">-- Skip --</option>
									{#each platforms as platform}
										<option value={platform.id}>{platform.display_name}</option>
									{/each}
								</select>
								{#if platformMappings[suggestion.original] || suggestion.suggested_id}
									<svg class="w-5 h-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		{#if storefrontSuggestions.length > 0}
			<div>
				<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Storefronts</h3>
				<div class="space-y-3">
					{#each storefrontSuggestions as suggestion}
						<div class="flex items-center gap-4">
							<div class="flex-1 min-w-0">
								<span class="text-sm font-medium text-gray-900 dark:text-white">
									"{suggestion.original}"
								</span>
								<span class="text-sm text-gray-500 dark:text-gray-400 ml-2">
									({suggestion.count} game{suggestion.count !== 1 ? 's' : ''})
								</span>
							</div>
							<div class="flex items-center gap-2">
								<span class="text-gray-400">-></span>
								<select
									class="block w-48 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm"
									value={storefrontMappings[suggestion.original] || suggestion.suggested_id || ''}
									onchange={(e) => onStorefrontMappingChange(suggestion.original, e.currentTarget.value)}
								>
									<option value="">-- Skip --</option>
									{#each storefronts as storefront}
										<option value={storefront.id}>{storefront.display_name}</option>
									{/each}
								</select>
								{#if storefrontMappings[suggestion.original] || suggestion.suggested_id}
									<svg class="w-5 h-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
{/if}
