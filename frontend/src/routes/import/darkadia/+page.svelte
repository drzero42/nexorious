<script lang="ts">
	import { goto } from '$app/navigation';
	import { config } from '$lib/env';
	import { auth } from '$lib/stores';
	import { RouteGuard } from '$lib/components';

	let fileInput = $state<HTMLInputElement | null>(null);
	let selectedFile = $state<File | null>(null);
	let uploading = $state(false);
	let error = $state<string | null>(null);
	let dragOver = $state(false);

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files && input.files[0]) {
			selectFile(input.files[0]);
		}
	}

	function handleDrop(event: DragEvent) {
		event.preventDefault();
		dragOver = false;
		if (event.dataTransfer?.files && event.dataTransfer.files[0]) {
			selectFile(event.dataTransfer.files[0]);
		}
	}

	function handleDragOver(event: DragEvent) {
		event.preventDefault();
		dragOver = true;
	}

	function handleDragLeave() {
		dragOver = false;
	}

	function selectFile(file: File) {
		error = null;
		if (!file.name.toLowerCase().endsWith('.csv')) {
			error = 'Please select a CSV file';
			return;
		}
		if (file.size > 10 * 1024 * 1024) {
			error = 'File is too large. Maximum size is 10MB.';
			return;
		}
		selectedFile = file;
	}

	function clearFile() {
		selectedFile = null;
		error = null;
		if (fileInput) {
			fileInput.value = '';
		}
	}

	async function handleUpload() {
		if (!selectedFile) return;

		uploading = true;
		error = null;

		try {
			const formData = new FormData();
			formData.append('file', selectedFile);

			const response = await fetch(`${config.apiUrl}/import/darkadia`, {
				method: 'POST',
				body: formData,
				headers: {
					Authorization: `Bearer ${auth.value.accessToken}`
				}
			});

			if (!response.ok) {
				if (response.status === 401) {
					// Try to refresh token and retry
					const refreshed = await auth.refreshAuth();
					if (refreshed) {
						return handleUpload();
					}
					throw new Error('Session expired. Please log in again.');
				}
				const data = await response.json().catch(() => ({}));
				throw new Error(data.detail || 'Upload failed');
			}

			const data = await response.json();

			// Redirect to review page with job_id
			goto(`/review?job_id=${data.job_id}&source=import`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Upload failed';
			uploading = false;
		}
	}
</script>

<svelte:head>
	<title>Import from Darkadia - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-2xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
		<div class="mb-8">
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Import from Darkadia</h1>
			<p class="mt-2 text-gray-600 dark:text-gray-400">
				Upload your Darkadia CSV export to import your game collection.
			</p>
		</div>

		<!-- Upload Area -->
		<div
			class="border-2 border-dashed rounded-lg p-8 text-center transition-colors {dragOver
				? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20'
				: 'border-gray-300 dark:border-gray-600 hover:border-gray-400 dark:hover:border-gray-500'}"
			ondrop={handleDrop}
			ondragover={handleDragOver}
			ondragleave={handleDragLeave}
			role="button"
			tabindex="0"
			onkeydown={(e) => e.key === 'Enter' && fileInput?.click()}
		>
			{#if selectedFile}
				<div class="space-y-4">
					<svg
						class="mx-auto h-12 w-12 text-green-500"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
						/>
					</svg>
					<div>
						<p class="text-lg font-medium text-gray-900 dark:text-white">
							{selectedFile.name}
						</p>
						<p class="text-sm text-gray-500 dark:text-gray-400">
							{(selectedFile.size / 1024).toFixed(1)} KB
						</p>
					</div>
					<button
						type="button"
						class="text-sm text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300"
						onclick={clearFile}
					>
						Remove file
					</button>
				</div>
			{:else}
				<svg
					class="mx-auto h-12 w-12 text-gray-400"
					fill="none"
					viewBox="0 0 24 24"
					stroke="currentColor"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"
					/>
				</svg>
				<div class="mt-4">
					<button
						type="button"
						class="text-indigo-600 dark:text-indigo-400 font-medium hover:text-indigo-800 dark:hover:text-indigo-300"
						onclick={() => fileInput?.click()}
					>
						Select a file
					</button>
					<span class="text-gray-500 dark:text-gray-400"> or drag and drop</span>
				</div>
				<p class="mt-2 text-sm text-gray-500 dark:text-gray-400">CSV file up to 10MB</p>
			{/if}
		</div>

		<input
			bind:this={fileInput}
			type="file"
			accept=".csv"
			class="hidden"
			onchange={handleFileSelect}
		/>

		<!-- Error Message -->
		{#if error}
			<div class="mt-4 p-4 bg-red-50 dark:bg-red-900/20 rounded-lg">
				<p class="text-sm text-red-700 dark:text-red-300">{error}</p>
			</div>
		{/if}

		<!-- Upload Button -->
		<div class="mt-6">
			<button
				type="button"
				class="w-full flex justify-center py-3 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
				disabled={!selectedFile || uploading}
				onclick={handleUpload}
			>
				{#if uploading}
					<svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" fill="none" viewBox="0 0 24 24">
						<circle
							class="opacity-25"
							cx="12"
							cy="12"
							r="10"
							stroke="currentColor"
							stroke-width="4"
						></circle>
						<path
							class="opacity-75"
							fill="currentColor"
							d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
						></path>
					</svg>
					Processing...
				{:else}
					Upload & Process
				{/if}
			</button>
		</div>

		<!-- Instructions -->
		<div class="mt-8 p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
			<h3 class="text-sm font-medium text-gray-900 dark:text-white">How to export from Darkadia</h3>
			<ol class="mt-2 text-sm text-gray-600 dark:text-gray-400 list-decimal list-inside space-y-1">
				<li>Open Darkadia and go to your collection</li>
				<li>Click on "Export" in the menu</li>
				<li>Select CSV format and download the file</li>
				<li>Upload the CSV file here</li>
			</ol>
		</div>
	</div>
</RouteGuard>
