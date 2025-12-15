<script lang="ts">
  import { RouteGuard } from '$lib/components';
  import { ui } from '$lib/stores';
  import { goto } from '$app/navigation';

  // Page state
  let isDragging = $state(false);
  let selectedFile = $state<File | null>(null);
  let isUploading = $state(false);
  let uploadProgress = $state(0);
  let error = $state<string | null>(null);

  // File validation
  const MAX_FILE_SIZE = 50 * 1024 * 1024; // 50MB
  const ACCEPTED_TYPES = ['application/json', 'text/json'];

  function handleDragEnter(event: DragEvent) {
    event.preventDefault();
    isDragging = true;
  }

  function handleDragLeave(event: DragEvent) {
    event.preventDefault();
    isDragging = false;
  }

  function handleDragOver(event: DragEvent) {
    event.preventDefault();
  }

  function handleDrop(event: DragEvent) {
    event.preventDefault();
    isDragging = false;

    const files = event.dataTransfer?.files;
    const firstFile = files?.[0];
    if (firstFile) {
      handleFileSelect(firstFile);
    }
  }

  function handleFileInput(event: Event) {
    const input = event.target as HTMLInputElement;
    const firstFile = input.files?.[0];
    if (firstFile) {
      handleFileSelect(firstFile);
    }
  }

  function handleFileSelect(file: File) {
    error = null;

    // Validate file type
    if (!file.name.endsWith('.json') && !ACCEPTED_TYPES.includes(file.type)) {
      error = 'Please select a JSON file';
      return;
    }

    // Validate file size
    if (file.size > MAX_FILE_SIZE) {
      error = `File is too large. Maximum size is ${MAX_FILE_SIZE / (1024 * 1024)}MB`;
      return;
    }

    selectedFile = file;
  }

  function clearSelection() {
    selectedFile = null;
    error = null;
  }

  async function handleUpload() {
    if (!selectedFile) return;

    isUploading = true;
    uploadProgress = 0;
    error = null;

    try {
      const formData = new FormData();
      formData.append('file', selectedFile);

      const response = await fetch('/api/import/nexorious', {
        method: 'POST',
        body: formData,
        credentials: 'include'
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.detail || `Upload failed: ${response.statusText}`);
      }

      const result = await response.json();

      ui.showSuccess(`Import started! Processing ${result.total_items || 'your'} games.`);

      // Navigate to jobs page to track progress
      goto(`/jobs/${result.job_id}`);

    } catch (err) {
      error = err instanceof Error ? err.message : 'Upload failed';
      ui.showError(error);
    } finally {
      isUploading = false;
    }
  }

  // Derived states
  const canUpload = $derived(selectedFile !== null && !isUploading);
  const fileSizeFormatted = $derived(
    selectedFile
      ? selectedFile.size < 1024
        ? `${selectedFile.size} B`
        : selectedFile.size < 1024 * 1024
          ? `${(selectedFile.size / 1024).toFixed(1)} KB`
          : `${(selectedFile.size / (1024 * 1024)).toFixed(1)} MB`
      : ''
  );
</script>

<svelte:head>
  <title>Nexorious Import - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
  <div class="space-y-6">
    <!-- Header -->
    <div>
      <nav class="flex text-sm text-gray-500" aria-label="Breadcrumb">
        <ol class="inline-flex items-center space-x-1 md:space-x-3">
          <li>
            <a href="/dashboard" class="hover:text-gray-700">Dashboard</a>
          </li>
          <li>
            <span>›</span>
          </li>
          <li>
            <a href="/import" class="hover:text-gray-700">Import</a>
          </li>
          <li>
            <span>›</span>
          </li>
          <li>
            <span class="text-gray-900 font-medium">Nexorious JSON</span>
          </li>
        </ol>
      </nav>

      <div class="mt-4 flex flex-col sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">
            Nexorious JSON Import
          </h1>
          <p class="mt-1 text-sm text-gray-500">
            Restore your game collection from a Nexorious export file
          </p>
        </div>
      </div>
    </div>

    <!-- Upload Section -->
    <div class="bg-white rounded-lg shadow-lg p-8">
      <h2 class="text-xl font-semibold text-gray-900 mb-6">Upload Export File</h2>

      <!-- Drop Zone -->
      <div
        class="border-2 border-dashed rounded-lg p-8 text-center transition-colors {isDragging ? 'border-indigo-500 bg-indigo-50' : 'border-gray-300 hover:border-gray-400'}"
        ondragenter={handleDragEnter}
        ondragleave={handleDragLeave}
        ondragover={handleDragOver}
        ondrop={handleDrop}
        role="button"
        tabindex="0"
      >
        {#if selectedFile}
          <!-- File Selected -->
          <div class="space-y-4">
            <div class="flex items-center justify-center">
              <div class="bg-indigo-100 rounded-full p-4">
                <svg class="h-8 w-8 text-indigo-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
              </div>
            </div>
            <div>
              <p class="text-lg font-medium text-gray-900">{selectedFile.name}</p>
              <p class="text-sm text-gray-500">{fileSizeFormatted}</p>
            </div>
            <button
              type="button"
              onclick={clearSelection}
              class="text-sm text-red-600 hover:text-red-500"
            >
              Remove file
            </button>
          </div>
        {:else}
          <!-- No File Selected -->
          <div class="space-y-4">
            <div class="flex items-center justify-center">
              <div class="bg-gray-100 rounded-full p-4">
                <svg class="h-8 w-8 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                </svg>
              </div>
            </div>
            <div>
              <p class="text-lg font-medium text-gray-900">
                Drop your export file here
              </p>
              <p class="text-sm text-gray-500">
                or click to browse
              </p>
            </div>
            <label class="cursor-pointer">
              <span class="inline-flex items-center px-4 py-2 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50">
                Select File
              </span>
              <input
                type="file"
                accept=".json,application/json"
                class="hidden"
                onchange={handleFileInput}
              />
            </label>
            <p class="text-xs text-gray-400">
              JSON files up to 50MB
            </p>
          </div>
        {/if}
      </div>

      <!-- Error Message -->
      {#if error}
        <div class="mt-4 bg-red-50 border border-red-200 rounded-md p-4">
          <div class="flex">
            <svg class="h-5 w-5 text-red-400" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
            </svg>
            <p class="ml-3 text-sm text-red-800">{error}</p>
          </div>
        </div>
      {/if}

      <!-- Upload Progress -->
      {#if isUploading}
        <div class="mt-4">
          <div class="flex items-center justify-between text-sm text-gray-600 mb-1">
            <span>Uploading...</span>
            <span>{uploadProgress}%</span>
          </div>
          <div class="w-full bg-gray-200 rounded-full h-2">
            <div
              class="bg-indigo-600 h-2 rounded-full transition-all duration-300"
              style="width: {uploadProgress}%"
            ></div>
          </div>
        </div>
      {/if}

      <!-- Upload Button -->
      <div class="mt-6">
        <button
          type="button"
          onclick={handleUpload}
          disabled={!canUpload}
          class="w-full inline-flex items-center justify-center px-4 py-3 border border-transparent text-base font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {#if isUploading}
            <svg class="animate-spin -ml-1 mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            Uploading...
          {:else}
            <svg class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
            </svg>
            Start Import
          {/if}
        </button>
      </div>
    </div>

    <!-- Instructions -->
    <div class="bg-white rounded-lg shadow-lg p-8">
      <h2 class="text-xl font-semibold text-gray-900 mb-6">How It Works</h2>

      <div class="space-y-6">
        <div class="flex items-start">
          <div class="flex-shrink-0 w-8 h-8 bg-indigo-600 text-white rounded-full flex items-center justify-center text-sm font-semibold">
            1
          </div>
          <div class="ml-4">
            <h3 class="text-lg font-medium text-gray-900">Export from Nexorious</h3>
            <p class="text-gray-600 mt-1">
              Go to Settings → Export Data to download your collection as a JSON file.
              This includes all your games, ratings, play status, notes, and tags.
            </p>
          </div>
        </div>

        <div class="flex items-start">
          <div class="flex-shrink-0 w-8 h-8 bg-indigo-600 text-white rounded-full flex items-center justify-center text-sm font-semibold">
            2
          </div>
          <div class="ml-4">
            <h3 class="text-lg font-medium text-gray-900">Upload Your Export</h3>
            <p class="text-gray-600 mt-1">
              Drag and drop your JSON file above or click to select it.
              The file will be validated before processing.
            </p>
          </div>
        </div>

        <div class="flex items-start">
          <div class="flex-shrink-0 w-8 h-8 bg-indigo-600 text-white rounded-full flex items-center justify-center text-sm font-semibold">
            3
          </div>
          <div class="ml-4">
            <h3 class="text-lg font-medium text-gray-900">Automatic Restoration</h3>
            <p class="text-gray-600 mt-1">
              Your games will be restored with all their metadata intact.
              The import runs in the background - you can track progress on the Jobs page.
            </p>
          </div>
        </div>
      </div>

      <!-- Info Box -->
      <div class="mt-6 bg-blue-50 border border-blue-200 rounded-lg p-4">
        <div class="flex">
          <svg class="h-5 w-5 text-blue-500 mr-3 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <div class="text-sm text-blue-800">
            <p class="font-medium mb-1">Non-Interactive Import</p>
            <p>
              Nexorious exports include trusted IGDB IDs, so no manual review is needed.
              Games are imported directly with full metadata and cover art.
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</RouteGuard>
