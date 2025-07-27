<script lang="ts">
	import { notifications } from '../stores/notifications.svelte.js';
	import Toast from './Toast.svelte';
</script>

<div class="toast-container fixed top-4 right-4 z-50 space-y-2 pointer-events-none">
	{#each notifications.items as notification (notification.id)}
		<div class="pointer-events-auto">
			<Toast
				id={notification.id}
				type={notification.type}
				message={notification.message}
				duration={notification.duration || 5000}
				onRemove={notifications.remove.bind(notifications)}
			/>
		</div>
	{/each}
</div>

<style>
	.toast-container {
		max-width: 24rem;
	}
</style>