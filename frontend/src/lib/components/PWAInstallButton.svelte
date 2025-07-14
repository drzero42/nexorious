<script lang="ts">
	import { onMount } from 'svelte';
	import { showInstallPrompt, isPWAInstalled } from '$lib/pwa';

	let showInstallButton = false;
	let installing = false;

	onMount(() => {
		// Check if PWA is already installed
		if (isPWAInstalled()) {
			return;
		}

		// Listen for install available event
		const handleInstallAvailable = () => {
			showInstallButton = true;
		};

		const handleInstalled = () => {
			showInstallButton = false;
		};

		window.addEventListener('pwa-install-available', handleInstallAvailable);
		window.addEventListener('pwa-installed', handleInstalled);

		return () => {
			window.removeEventListener('pwa-install-available', handleInstallAvailable);
			window.removeEventListener('pwa-installed', handleInstalled);
		};
	});

	async function handleInstall() {
		installing = true;
		try {
			const accepted = await showInstallPrompt();
			if (accepted) {
				showInstallButton = false;
			}
		} catch (error) {
			console.error('Error showing install prompt:', error);
		} finally {
			installing = false;
		}
	}
</script>

{#if showInstallButton}
	<button
		on:click={handleInstall}
		disabled={installing}
		class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
	>
		{#if installing}
			<svg class="animate-spin -ml-1 mr-3 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
			</svg>
			Installing...
		{:else}
			<svg class="-ml-1 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
			</svg>
			Install App
		{/if}
	</button>
{/if}