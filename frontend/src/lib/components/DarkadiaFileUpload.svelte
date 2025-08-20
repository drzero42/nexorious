<script lang="ts">
  import { darkadia } from '$lib/stores/darkadia.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import type { DarkadiaUploadResponse } from '$lib/types/darkadia';

  interface Props {
    onUploadComplete?: (result: DarkadiaUploadResponse) => void;
    onUploadStart?: () => void;
    onUploadError?: (error: string) => void;
    disabled?: boolean;
    class?: string;
  }

  let {
    onUploadComplete,
    onUploadStart,
    onUploadError,
    disabled = false,
    class: className = ''
  }: Props = $props();

  // Component state using Svelte 5 runes
  let uploadState = $state<'idle' | 'dragging' | 'uploading' | 'processing' | 'success' | 'error'>('idle');
  let selectedFile = $state<File | null>(null);
  let uploadProgress = $state(0);
  let processingProgress = $state(0);
  let errorMessage = $state<string | null>(null);
  let fileInput: HTMLInputElement;
  let dragCounter = $state(0);

  // File validation constants
  const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB

  // Derived states
  let isDragging = $derived(uploadState === 'dragging');
  let isUploading = $derived(uploadState === 'uploading');
  let isProcessing = $derived(uploadState === 'processing');
  let isSuccess = $derived(uploadState === 'success');
  let isError = $derived(uploadState === 'error');
  let isActive = $derived(isUploading || isProcessing);

  // Watch darkadia store for upload state changes
  $effect(() => {
    const state = darkadia.value.uploadState;
    
    if (state.isUploading) {
      uploadState = 'uploading';
      uploadProgress = state.uploadProgress;
    } else if (state.isImporting) {
      uploadState = 'processing';
      processingProgress = state.importProgress;
    } else if (state.uploadResult && !state.error) {
      uploadState = 'success';
      uploadProgress = 100;
      processingProgress = 100;
    } else if (state.error) {
      uploadState = 'error';
      errorMessage = state.error;
      onUploadError?.(state.error);
    }
  });

  function validateFile(file: File): string | null {
    // Check file type
    const isCSV = file.type === 'text/csv' || 
                  file.type === 'application/csv' || 
                  file.name.toLowerCase().endsWith('.csv');
    
    if (!isCSV) {
      return 'Please select a CSV file (.csv extension required)';
    }

    // Check file size
    if (file.size > MAX_FILE_SIZE) {
      return `File size must be less than ${MAX_FILE_SIZE / (1024 * 1024)}MB. Current file is ${(file.size / (1024 * 1024)).toFixed(1)}MB`;
    }

    // Check if file is empty
    if (file.size === 0) {
      return 'File cannot be empty';
    }

    return null;
  }

  function handleDragEnter(event: DragEvent) {
    event.preventDefault();
    event.stopPropagation();
    
    if (disabled || isActive) return;
    
    dragCounter++;
    if (dragCounter === 1) {
      uploadState = 'dragging';
    }
  }

  function handleDragLeave(event: DragEvent) {
    event.preventDefault();
    event.stopPropagation();
    
    if (disabled || isActive) return;
    
    dragCounter--;
    if (dragCounter === 0) {
      uploadState = selectedFile ? 'idle' : 'idle';
    }
  }

  function handleDragOver(event: DragEvent) {
    event.preventDefault();
    event.stopPropagation();
  }

  function handleDrop(event: DragEvent) {
    event.preventDefault();
    event.stopPropagation();
    
    if (disabled || isActive) return;
    
    dragCounter = 0;
    uploadState = 'idle';
    
    const files = event.dataTransfer?.files;
    if (files && files.length > 0) {
      const file = files[0];
      if (file) {
        handleFileSelection(file);
      }
    }
  }

  function handleFileInputChange() {
    if (fileInput.files && fileInput.files.length > 0) {
      const file = fileInput.files[0];
      if (file) {
        handleFileSelection(file);
      }
    }
  }

  function handleFileSelection(file: File) {
    errorMessage = null;
    
    const validationError = validateFile(file);
    if (validationError) {
      errorMessage = validationError;
      uploadState = 'error';
      onUploadError?.(validationError);
      return;
    }

    selectedFile = file;
    uploadState = 'idle';
  }

  async function uploadFile() {
    if (!selectedFile || disabled || isActive) return;

    try {
      uploadState = 'uploading';
      uploadProgress = 0;
      errorMessage = null;
      
      onUploadStart?.();
      
      const result = await darkadia.uploadCSV(selectedFile);
      
      onUploadComplete?.(result);
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : 'Upload failed';
      uploadState = 'error';
      errorMessage = errorMsg;
      onUploadError?.(errorMsg);
    }
  }

  function clearFile() {
    selectedFile = null;
    uploadState = 'idle';
    errorMessage = null;
    uploadProgress = 0;
    processingProgress = 0;
    
    if (fileInput) {
      fileInput.value = '';
    }
  }

  function retryUpload() {
    if (selectedFile) {
      uploadFile();
    } else {
      uploadState = 'idle';
      errorMessage = null;
    }
  }

  function triggerFileDialog() {
    if (!disabled && !isActive) {
      fileInput.click();
    }
  }

  // Format file size for display
  function formatFileSize(bytes: number): string {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }
</script>

<div class="w-full max-w-2xl mx-auto {className}">
  <!-- Upload Area -->
  <div
    class={`relative border-2 border-dashed rounded-xl p-8 text-center transition-all duration-200 ${
      isDragging
        ? 'border-blue-400 bg-blue-50 shadow-lg scale-102'
        : isError
        ? 'border-red-300 bg-red-50'
        : isSuccess
        ? 'border-green-300 bg-green-50'
        : 'border-gray-300 hover:border-gray-400'
    } ${
      disabled || isActive
        ? 'opacity-50 cursor-not-allowed'
        : 'cursor-pointer hover:bg-gray-50'
    }`}
    ondragenter={handleDragEnter}
    ondragleave={handleDragLeave}
    ondragover={handleDragOver}
    ondrop={handleDrop}
    onclick={triggerFileDialog}
    role="button"
    tabindex="0"
    aria-label="Upload CSV file"
    onkeydown={(e) => {
      if ((e.key === 'Enter' || e.key === ' ') && !disabled && !isActive) {
        e.preventDefault();
        triggerFileDialog();
      }
    }}
  >
    <!-- Loading Overlay -->
    {#if isActive}
      <div class="absolute inset-0 bg-white bg-opacity-80 rounded-xl flex items-center justify-center z-10">
        <div class="text-center space-y-4">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
          <p class="text-sm font-medium text-gray-700">
            {isUploading ? 'Uploading file...' : 'Processing CSV...'}
          </p>
        </div>
      </div>
    {/if}

    <!-- Success State -->
    {#if isSuccess}
      <div class="space-y-4">
        <div class="mx-auto w-16 h-16 text-green-500">
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <div>
          <h3 class="text-lg font-medium text-green-900">Upload Complete!</h3>
          <p class="text-sm text-green-700 mt-1">
            CSV file processed successfully. Games are being imported.
          </p>
        </div>
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); clearFile(); }}
          class="text-green-600 hover:text-green-800 text-sm font-medium"
        >
          Upload Another File
        </button>
      </div>

    <!-- Error State -->
    {:else if isError}
      <div class="space-y-4">
        <div class="mx-auto w-16 h-16 text-red-500">
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.962-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
          </svg>
        </div>
        <div>
          <h3 class="text-lg font-medium text-red-900">Upload Failed</h3>
          <p class="text-sm text-red-700 mt-1">
            {errorMessage}
          </p>
        </div>
        <div class="flex justify-center space-x-3">
          <button
            type="button"
            onclick={(e) => { e.stopPropagation(); retryUpload(); }}
            class="px-4 py-2 bg-red-100 text-red-700 rounded-lg text-sm font-medium hover:bg-red-200 transition-colors"
          >
            Try Again
          </button>
          <button
            type="button"
            onclick={(e) => { e.stopPropagation(); clearFile(); }}
            class="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg text-sm font-medium hover:bg-gray-200 transition-colors"
          >
            Choose Different File
          </button>
        </div>
      </div>

    <!-- File Selected State -->
    {:else if selectedFile}
      <div class="space-y-4">
        <div class="mx-auto w-16 h-16 text-blue-500">
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
        </div>
        <div>
          <h3 class="text-lg font-medium text-gray-900">{selectedFile.name}</h3>
          <p class="text-sm text-gray-600 mt-1">
            {formatFileSize(selectedFile.size)} • CSV file
          </p>
        </div>
        <div class="flex justify-center space-x-3">
          <button
            type="button"
            onclick={(e) => { e.stopPropagation(); uploadFile(); }}
            disabled={disabled}
            class="px-6 py-2 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Upload & Import
          </button>
          <button
            type="button"
            onclick={(e) => { e.stopPropagation(); clearFile(); }}
            class="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg font-medium hover:bg-gray-200 transition-colors"
          >
            Remove
          </button>
        </div>
      </div>

    <!-- Default State -->
    {:else}
      <div class="space-y-4">
        <div class="mx-auto w-16 h-16 text-gray-400">
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
          </svg>
        </div>
        <div>
          <h3 class="text-lg font-medium text-gray-900">
            {isDragging ? 'Drop CSV file here' : 'Upload Darkadia CSV'}
          </h3>
          <p class="text-sm text-gray-600 mt-1">
            {isDragging 
              ? 'Release to select this file' 
              : 'Click here or drag and drop your Darkadia export file'
            }
          </p>
        </div>
        <div class="text-xs text-gray-500 space-y-1">
          <p>• Only CSV files are accepted</p>
          <p>• Maximum file size: 10MB</p>
          <p>• File will be automatically imported after upload</p>
        </div>
      </div>
    {/if}
  </div>

  <!-- Progress Bars -->
  {#if isUploading || isProcessing}
    <div class="mt-6 space-y-4">
      {#if isUploading}
        <ProgressBar
          value={uploadProgress}
          label="Uploading file"
          color="blue"
          animated={true}
        />
      {/if}
      
      {#if isProcessing}
        <ProgressBar
          value={processingProgress}
          label="Processing CSV data"
          color="purple"
          animated={true}
        />
      {/if}
    </div>
  {/if}

  <!-- Hidden File Input -->
  <input
    bind:this={fileInput}
    type="file"
    accept=".csv,text/csv,application/csv"
    onchange={handleFileInputChange}
    class="hidden"
    disabled={disabled || isActive}
    aria-hidden="true"
  />
</div>

<style>
  .scale-102 {
    transform: scale(1.02);
  }
</style>