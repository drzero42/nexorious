<script lang="ts">
	import { onMount } from 'svelte';
	import { updateServiceWorker } from '$lib/pwa';

	let showUpdateNotification = false;
	let updating = false;

	onMount(() => {
		const handleUpdateAvailable = () => {
			showUpdateNotification = true;
		};

		window.addEventListener('pwa-update-available', handleUpdateAvailable);

		return () => {
			window.removeEventListener('pwa-update-available', handleUpdateAvailable);
		};
	});

	async function handleUpdate() {
		updating = true;
		try {
			updateServiceWorker();
			// The page will reload automatically when the new SW takes control
		} catch (error) {
			console.error('Error updating service worker:', error);
			updating = false;
		}
	}

	function dismissUpdate() {
		showUpdateNotification = false;
	}
</script>

{#if showUpdateNotification}
	<div class="fixed bottom-4 right-4 bg-white border border-gray-300 rounded-lg shadow-lg p-4 max-w-sm z-50">
		<div class="flex items-start">
			<div class="flex-shrink-0">
				<svg class="h-5 w-5 text-blue-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
			</div>
			<div class="ml-3 flex-1">
				<p class="text-sm font-medium text-gray-900">Update Available</p>
				<p class="text-sm text-gray-500">A new version of the app is available.</p>
				<div class="mt-3 flex space-x-2">
					<button
						on:click={handleUpdate}
						disabled={updating}
						class="inline-flex items-center px-3 py-1 border border-transparent text-xs font-medium rounded text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
					>
						{#if updating}
							<svg class="animate-spin -ml-1 mr-1 h-3 w-3 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
							</svg>
							Updating...
						{:else}
							Update
						{/if}
					</button>
					<button
						on:click={dismissUpdate}
						class="inline-flex items-center px-3 py-1 border border-gray-300 text-xs font-medium rounded text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
					>
						Later
					</button>
				</div>
			</div>
			<div class="ml-4 flex-shrink-0">
				<button
					on:click={dismissUpdate}
					aria-label="Close notification"
					class="inline-flex text-gray-400 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
				>
					<svg class="h-5 w-5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
		</div>
	</div>
{/if}