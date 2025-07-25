<script lang="ts">
	import { onMount } from 'svelte';
	import type { NotificationType } from '../stores/notifications.svelte.js';

	export let id: string;
	export let type: NotificationType = 'info';
	export let message: string;
	export let duration = 5000;
	export let onRemove: (id: string) => void;

	let visible = false;
	let element: HTMLDivElement;
	let isDismissing = false;

	onMount(() => {
		// Trigger entrance animation
		setTimeout(() => {
			visible = true;
		}, 10);

		// Auto-dismiss if duration is set
		if (duration > 0) {
			setTimeout(() => {
				dismiss();
			}, duration);
		}
	});

	function dismiss() {
		if (isDismissing) return; // Prevent multiple dismiss calls
		
		isDismissing = true;
		visible = false;
		// Wait for exit animation before removing
		setTimeout(() => {
			onRemove(id);
		}, 300);
	}

	$: typeClasses = {
		success: 'bg-green-50 border-green-200 text-green-800',
		error: 'bg-red-50 border-red-200 text-red-800',
		warning: 'bg-yellow-50 border-yellow-200 text-yellow-800',
		info: 'bg-blue-50 border-blue-200 text-blue-800'
	};

	$: iconClasses = {
		success: 'text-green-400',
		error: 'text-red-400',
		warning: 'text-yellow-400',
		info: 'text-blue-400'
	};

	$: icons = {
		success: '✓',
		error: '✕',
		warning: '⚠',
		info: 'ℹ'
	};
</script>

<div
	bind:this={element}
	class="toast fixed z-50 max-w-sm w-full bg-white shadow-lg rounded-lg pointer-events-auto border transform transition-all duration-300 ease-in-out {visible
		? 'translate-x-0 opacity-100'
		: 'translate-x-full opacity-0'} {typeClasses[type]}"
	role="alert"
	aria-live="assertive"
	aria-atomic="true"
>
	<div class="p-4">
		<div class="flex items-start">
			<div class="flex-shrink-0">
				<span class="inline-block w-5 h-5 text-center text-sm font-bold {iconClasses[type]}">
					{icons[type]}
				</span>
			</div>
			<div class="ml-3 w-0 flex-1">
				<p class="text-sm font-medium">
					{message}
				</p>
			</div>
			<div class="ml-4 flex-shrink-0 flex">
				<button
					class="inline-flex text-gray-400 hover:text-gray-600 focus:outline-none focus:text-gray-600 transition ease-in-out duration-150"
					on:click={dismiss}
					type="button"
					aria-label="Close notification"
				>
					<span class="sr-only">Close</span>
					<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
							clip-rule="evenodd"
						/>
					</svg>
				</button>
			</div>
		</div>
	</div>
</div>

<style>
	.toast {
		right: 1rem;
		top: 1rem;
	}
</style>