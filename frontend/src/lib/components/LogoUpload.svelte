<script lang="ts">
  import { buildIconUrl } from '$lib/utils/icon-utils';
  
  interface Props {
    entityType: 'platforms' | 'storefronts';
    entityId: string;
    currentIconUrl?: string | null;
    disabled?: boolean;
    onuploaded?: (event: CustomEvent<{ theme: string; iconUrl: string; message: string }>) => void;
    ondeleted?: (event: CustomEvent<{ theme: string; message: string }>) => void;
    onerror?: (event: CustomEvent<{ message: string }>) => void;
  }
  
  let { 
    entityType, 
    entityId, 
    currentIconUrl = null, 
    disabled = false,
    onuploaded,
    ondeleted,
    onerror
  }: Props = $props();
  
  // Component state
  let dragOver = $state(false);
  let uploading = $state(false);
  let selectedTheme = $state<'light' | 'dark'>('light');
  let files = $state<FileList | null>(null);
  let fileInput: HTMLInputElement;
  let previewUrl = $state<string | null>(null);
  let error = $state<string | null>(null);
  
  // Supported file types
  const supportedTypes = ['image/svg+xml', 'image/png', 'image/jpeg', 'image/webp'];
  const maxFileSize = 2 * 1024 * 1024; // 2MB
  
  function handleDragOver(event: DragEvent) {
    event.preventDefault();
    dragOver = true;
  }
  
  function handleDragLeave(event: DragEvent) {
    event.preventDefault();
    dragOver = false;
  }
  
  function handleDrop(event: DragEvent) {
    event.preventDefault();
    dragOver = false;
    
    if (disabled) return;
    
    const droppedFiles = event.dataTransfer?.files;
    if (droppedFiles && droppedFiles.length > 0) {
      files = droppedFiles;
      handleFileSelection();
    }
  }
  
  function handleFileInputChange() {
    if (fileInput.files && fileInput.files.length > 0) {
      files = fileInput.files;
      handleFileSelection();
    }
  }
  
  function handleFileSelection() {
    if (!files || files.length === 0) return;
    
    const file = files[0];
    if (!file) return; // Additional type guard
    
    error = null;
    
    // Validate file
    if (!supportedTypes.includes(file.type)) {
      error = 'Unsupported file format. Please use SVG, PNG, JPEG, or WebP.';
      return;
    }
    
    if (file.size > maxFileSize) {
      error = 'File too large. Maximum size is 2MB.';
      return;
    }
    
    // Create preview
    const reader = new FileReader();
    reader.onload = (e) => {
      previewUrl = e.target?.result as string;
    };
    reader.readAsDataURL(file);
  }
  
  async function uploadLogo() {
    if (!files || files.length === 0) return;
    
    const file = files[0];
    if (!file) return; // Additional type guard
    
    uploading = true;
    error = null;
    
    try {
      const formData = new FormData();
      formData.append('file', file);
      
      const endpoint = entityType === 'platforms' 
        ? `/api/platforms/${entityId}/logo?theme=${selectedTheme}`
        : `/api/platforms/storefronts/${entityId}/logo?theme=${selectedTheme}`;
      const response = await fetch(endpoint, {
        method: 'POST',
        body: formData,
        headers: {
          // Let the browser set the Content-Type header for multipart/form-data
          'Authorization': `Bearer ${localStorage.getItem('token')}`
        }
      });
      
      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.detail || 'Upload failed');
      }
      
      const result = await response.json();
      
      // Clear form
      files = null;
      previewUrl = null;
      if (fileInput) fileInput.value = '';
      
      onuploaded?.(new CustomEvent('uploaded', {
        detail: {
          theme: selectedTheme,
          iconUrl: result.icon_url,
          message: result.message
        }
      }));
      
    } catch (err: any) {
      const errorMessage = (err as Error).message ?? 'Upload failed';
      error = errorMessage;
      onerror?.(new CustomEvent('error', { detail: { message: errorMessage } }));
    } finally {
      uploading = false;
    }
  }
  
  async function deleteLogo(theme?: 'light' | 'dark') {
    if (!confirm(`Are you sure you want to delete the ${theme || 'all'} logo(s)?`)) return;
    
    try {
      const themeParam = theme ? `?theme=${theme}` : '';
      const endpoint = entityType === 'platforms' 
        ? `/api/platforms/${entityId}/logo${themeParam}`
        : `/api/platforms/storefronts/${entityId}/logo${themeParam}`;
      const response = await fetch(endpoint, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`
        }
      });
      
      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.detail || 'Delete failed');
      }
      
      const result = await response.json();
      
      ondeleted?.(new CustomEvent('deleted', {
        detail: {
          theme: theme || 'all',
          message: result.message
        }
      }));
      
    } catch (err: any) {
      const errorMessage = (err as Error).message ?? 'Delete failed';
      error = errorMessage;
      onerror?.(new CustomEvent('error', { detail: { message: errorMessage } }));
    }
  }
  
  function clearPreview() {
    files = null;
    previewUrl = null;
    error = null;
    if (fileInput) fileInput.value = '';
  }
</script>

<div class="space-y-4">
  <!-- Theme Selection -->
  <fieldset class="flex items-center space-x-4">
    <legend class="block text-sm font-medium text-gray-700 mr-4">Theme:</legend>
    <div class="flex items-center space-x-3">
      <label class="flex items-center">
        <input
          type="radio"
          bind:group={selectedTheme}
          value="light"
          class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300"
          {disabled}
        />
        <span class="ml-2 text-sm text-gray-700">Light</span>
      </label>
      <label class="flex items-center">
        <input
          type="radio"
          bind:group={selectedTheme}
          value="dark"
          class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300"
          {disabled}
        />
        <span class="ml-2 text-sm text-gray-700">Dark</span>
      </label>
    </div>
  </fieldset>

  <!-- Current Logo Preview -->
  {#if currentIconUrl}
    <div class="space-y-2">
      <div class="block text-sm font-medium text-gray-700">Current Logo:</div>
      <div class="flex items-center space-x-4">
        <img 
          src={buildIconUrl(currentIconUrl)} 
          alt="Current logo" 
          class="w-8 h-8 object-contain"
          onerror={(e) => {
            const img = e.target as HTMLImageElement;
            img.style.display = 'none';
          }}
        />
        <button
          type="button"
          onclick={() => deleteLogo('light')}
          class="text-red-600 hover:text-red-900 text-sm"
          {disabled}
        >
          Remove Current Logo
        </button>
      </div>
    </div>
  {/if}

  <!-- Upload Area -->
  <div class="space-y-3">
    <label for="logo-file-input" class="block text-sm font-medium text-gray-700">
      Upload New Logo ({selectedTheme} theme):
    </label>
    
    <!-- Drag and Drop Area -->
    <div
      class={`border-2 border-dashed rounded-lg p-6 text-center transition-colors ${
        dragOver 
          ? 'border-primary-400 bg-primary-50' 
          : 'border-gray-300 hover:border-gray-400'
      } ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
      ondragover={handleDragOver}
      ondragleave={handleDragLeave}
      ondrop={handleDrop}
      onclick={() => !disabled && fileInput.click()}
      role="button"
      tabindex="0"
      onkeydown={(e) => {
        if ((e.key === 'Enter' || e.key === ' ') && !disabled) {
          fileInput.click();
        }
      }}
    >
      {#if previewUrl}
        <!-- Preview -->
        <div class="space-y-3">
          <img src={previewUrl} alt="Preview" class="mx-auto w-16 h-16 object-contain" />
          <p class="text-sm text-gray-600">
            Ready to upload as {selectedTheme} theme
          </p>
          <div class="flex justify-center space-x-2">
            <button
              type="button"
              onclick={(e) => { e.stopPropagation(); uploadLogo(); }}
              disabled={uploading || disabled}
              class="px-4 py-2 bg-primary-600 text-white rounded-md text-sm hover:bg-primary-700 disabled:opacity-50"
            >
              {uploading ? 'Uploading...' : 'Upload'}
            </button>
            <button
              type="button"
              onclick={(e) => { e.stopPropagation(); clearPreview(); }}
              {disabled}
              class="px-4 py-2 bg-gray-300 text-gray-700 rounded-md text-sm hover:bg-gray-400"
            >
              Cancel
            </button>
          </div>
        </div>
      {:else}
        <!-- Upload Prompt -->
        <div class="space-y-2">
          <div class="mx-auto w-12 h-12 text-gray-400">
            <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
            </svg>
          </div>
          <div class="text-gray-600">
            <p class="text-sm">
              <span class="font-medium">Click to upload</span> or drag and drop
            </p>
            <p class="text-xs text-gray-500 mt-1">
              SVG, PNG, JPEG or WebP (max 2MB)
            </p>
          </div>
        </div>
      {/if}
    </div>
    
    <!-- Hidden File Input -->
    <input
      id="logo-file-input"
      bind:this={fileInput}
      type="file"
      accept="image/svg+xml,image/png,image/jpeg,image/webp"
      onchange={handleFileInputChange}
      class="hidden"
      {disabled}
    />
  </div>

  <!-- Error Message -->
  {#if error}
    <div class="rounded-md bg-red-50 p-4">
      <div class="text-sm text-red-700">
        {error}
      </div>
    </div>
  {/if}

  <!-- File Format Help -->
  <div class="text-xs text-gray-500">
    <p><strong>Tip:</strong> SVG files are preferred for best quality at all sizes.</p>
    <p>PNG files work well for detailed logos with transparency.</p>
  </div>
</div>