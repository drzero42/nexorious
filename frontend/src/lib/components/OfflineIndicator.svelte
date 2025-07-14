<script lang="ts">
	import { onMount } from 'svelte';
	import { isOnline } from '$lib/pwa';

	let online = true;

	onMount(() => {
		online = isOnline();

		const handleOnline = () => {
			online = true;
		};

		const handleOffline = () => {
			online = false;
		};

		window.addEventListener('online', handleOnline);
		window.addEventListener('offline', handleOffline);

		return () => {
			window.removeEventListener('online', handleOnline);
			window.removeEventListener('offline', handleOffline);
		};
	});
</script>

{#if !online}
	<div class="fixed top-0 left-0 right-0 bg-yellow-500 text-white text-center py-2 px-4 z-50">
		<div class="flex items-center justify-center">
			<svg class="h-5 w-5 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
			</svg>
			<span class="text-sm font-medium">You're offline. Some features may not be available.</span>
		</div>
	</div>
{/if}