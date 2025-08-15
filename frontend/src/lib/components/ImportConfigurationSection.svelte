<script lang="ts">
  import type { ImportConfiguration } from '$lib/types/import';
  
  interface ConfigField {
    id: string;
    label: string;
    type: 'text' | 'password';
    placeholder: string;
    required?: boolean;
    helpText?: string;
    validation?: (value: string) => string | null;
  }

  interface Props {
    sourceName: string; // e.g., "Steam", "Epic Games"
    sourceIcon: string;
    configuration?: ImportConfiguration;
    fields: ConfigField[];
    onVerify?: (values: Record<string, string>) => Promise<{ isValid: boolean; errorMessage?: string; userInfo?: any }>;
    onSave: (values: Record<string, string>) => Promise<void>;
    onDelete?: () => Promise<void>;
    verificationResult?: { isValid: boolean; errorMessage?: string; userInfo?: any };
    isVerifying?: boolean;
    isSubmitting?: boolean;
    isDeleting?: boolean;
    apiKeyHelpUrl?: string;
    additionalFields?: any[]; // For custom fields like Steam ID resolver
  }

  let {
    sourceName,
    sourceIcon,
    configuration,
    fields,
    onVerify,
    onSave,
    onDelete,
    verificationResult,
    isVerifying = false,
    isSubmitting = false,
    isDeleting = false,
    apiKeyHelpUrl,
    additionalFields = []
  }: Props = $props();

  // Form state
  let fieldValues = $state<Record<string, string>>({});
  let showFields = $state<Record<string, boolean>>({});
  let fieldErrors = $state<Record<string, string>>({});
  let formError = $state('');

  // Initialize form values
  $effect(() => {
    fields.forEach(field => {
      if (!(field.id in fieldValues)) {
        fieldValues[field.id] = '';
      }
      if (!(field.id in showFields)) {
        showFields[field.id] = field.type !== 'password';
      }
    });
  });

  // Validation effects
  $effect(() => {
    fields.forEach(field => {
      if (field.validation) {
        const error = field.validation(fieldValues[field.id] || '');
        fieldErrors[field.id] = error || '';
      }
    });
  });

  function toggleFieldVisibility(fieldId: string) {
    showFields[fieldId] = !showFields[fieldId];
  }

  async function handleVerify() {
    if (!onVerify) return;
    
    // Validate required fields
    const missingFields = fields.filter(f => f.required && !fieldValues[f.id]?.trim());
    if (missingFields.length > 0) {
      formError = `Please fill in all required fields: ${missingFields.map(f => f.label).join(', ')}`;
      return;
    }

    // Check for validation errors
    const hasErrors = Object.values(fieldErrors).some(error => error);
    if (hasErrors) {
      formError = 'Please fix validation errors before verifying';
      return;
    }

    try {
      formError = '';
      await onVerify(fieldValues);
    } catch (error) {
      formError = error instanceof Error ? error.message : 'Verification failed';
    }
  }

  async function handleSave() {
    // Validate required fields
    const missingFields = fields.filter(f => f.required && !fieldValues[f.id]?.trim());
    if (missingFields.length > 0) {
      formError = `Please fill in all required fields: ${missingFields.map(f => f.label).join(', ')}`;
      return;
    }

    // Check for validation errors
    const hasErrors = Object.values(fieldErrors).some(error => error);
    if (hasErrors) {
      formError = 'Please fix validation errors before saving';
      return;
    }

    try {
      formError = '';
      await onSave(fieldValues);
      
      // Clear form after successful save
      fields.forEach(field => {
        fieldValues[field.id] = '';
        if (field.type === 'password') {
          showFields[field.id] = false;
        }
      });
    } catch (error) {
      formError = error instanceof Error ? error.message : 'Failed to save configuration';
    }
  }

  async function handleDelete() {
    if (!onDelete) return;
    
    const confirmed = confirm(`Are you sure you want to delete your ${sourceName} configuration? This cannot be undone.`);
    if (!confirmed) return;

    try {
      await onDelete();
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : `Failed to delete ${sourceName} configuration`;
      formError = errorMessage;
    }
  }

  function clearVerification() {
    // This would need to be implemented by the parent component
    // For now, just clear the form error
    formError = '';
  }
</script>

<div class="space-y-6">
  <!-- Current Configuration Status -->
  {#if configuration?.isConfigured}
    <div class="card">
      <div class="border-b border-gray-200 pb-4 mb-4">
        <h2 class="text-lg font-semibold text-gray-900 flex items-center">
          <span class="text-xl mr-2">{sourceIcon}</span>
          Current {sourceName} Configuration
        </h2>
      </div>
      
      <div class="space-y-4">
        {#if configuration.maskedApiKey}
          <div>
            <div class="form-label">API Key</div>
            <div class="mt-1 p-3 bg-gray-50 border border-gray-300 rounded-md text-gray-700 font-mono text-sm">
              {configuration.maskedApiKey}
            </div>
          </div>
        {/if}

        <div class="flex items-center space-x-2">
          <div class="form-label">Status</div>
          <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {configuration.isVerified ? 'bg-green-100 text-green-800' : 'bg-yellow-100 text-yellow-800'}">
            {configuration.isVerified ? 'Verified' : 'Not Verified'}
          </span>
        </div>

        {#if configuration.configuredAt}
          <div>
            <div class="form-label">Last Updated</div>
            <div class="mt-1 text-sm text-gray-600">
              {configuration.configuredAt.toLocaleString()}
            </div>
          </div>
        {/if}

        {#if onDelete}
          <div class="pt-4 flex space-x-3">
            <button
              onclick={handleDelete}
              disabled={isDeleting}
              class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if isDeleting}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Deleting...
              {:else}
                Delete Configuration
              {/if}
            </button>
          </div>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Configuration Form -->
  <div class="card">
    <div class="border-b border-gray-200 pb-4 mb-6">
      <h2 class="text-lg font-semibold text-gray-900 flex items-center">
        <span class="text-xl mr-2">{sourceIcon}</span>
        {configuration?.isConfigured ? 'Update' : 'Setup'} {sourceName} Configuration
      </h2>
      {#if apiKeyHelpUrl}
        <p class="mt-1 text-sm text-gray-500">
          You'll need an API key to import your {sourceName} library. 
          <a href={apiKeyHelpUrl} target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-500">
            Get your API key here
          </a>
        </p>
      {/if}
    </div>

    <!-- Form Fields -->
    <div class="space-y-4">
      {#each fields as field (field.id)}
        <div>
          <label for={field.id} class="form-label">
            {field.label}
            {#if field.required}
              <span class="text-red-500">*</span>
            {/if}
          </label>
          <div class="mt-1 relative">
            <input
              id={field.id}
              type={showFields[field.id] ? 'text' : 'password'}
              bind:value={fieldValues[field.id]}
              placeholder={field.placeholder}
              class="form-input {field.type === 'password' ? 'pr-10' : ''}"
              class:border-red-500={fieldErrors[field.id]}
              class:border-green-500={fieldValues[field.id] && !fieldErrors[field.id]}
            />
            {#if field.type === 'password'}
              <button
                type="button"
                onclick={() => toggleFieldVisibility(field.id)}
                class="absolute inset-y-0 right-0 flex items-center pr-3"
                aria-label={showFields[field.id] ? `Hide ${field.label}` : `Show ${field.label}`}
              >
                {#if showFields[field.id]}
                  <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                  </svg>
                {:else}
                  <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                  </svg>
                {/if}
              </button>
            {/if}
          </div>
          {#if fieldErrors[field.id]}
            <p class="mt-2 text-sm text-red-600">{fieldErrors[field.id]}</p>
          {:else if field.helpText}
            <p class="mt-2 text-sm text-gray-500">{field.helpText}</p>
          {/if}
        </div>
      {/each}

      <!-- Additional Fields Slot -->
      {#if additionalFields.length > 0}
        <div class="space-y-4">
          {#each additionalFields as _additionalField}
            <!-- This would render custom components based on the field type -->
            <div><!-- Custom field rendering would go here --></div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Verification Section -->
    {#if onVerify}
      <div class="mt-6 p-4 bg-blue-50 rounded-md">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-sm font-medium text-blue-900">Test Configuration</h3>
            <p class="text-sm text-blue-700">Verify your settings before saving</p>
          </div>
          <button
            type="button"
            onclick={handleVerify}
            disabled={isVerifying}
            class="btn-primary text-sm disabled:opacity-50"
          >
            {#if isVerifying}
              <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Verifying...
            {:else}
              Verify Configuration
            {/if}
          </button>
        </div>

        <!-- Verification Results -->
        {#if verificationResult}
          <div class="mt-4">
            {#if verificationResult.isValid}
              <div class="flex items-center text-green-700">
                <svg class="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                </svg>
                Configuration is valid!
              </div>

              {#if verificationResult.userInfo}
                <div class="mt-3 p-3 bg-white rounded border">
                  <div class="text-sm text-gray-600">
                    <!-- User info display would be customizable based on the source -->
                    <strong>Verified user:</strong> {verificationResult.userInfo.name || verificationResult.userInfo.persona_name || 'Unknown'}
                  </div>
                </div>
              {/if}
            {:else}
              <div class="flex items-center text-red-700">
                <svg class="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
                </svg>
                {verificationResult.errorMessage || 'Verification failed'}
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    <!-- Form Error -->
    {#if formError}
      <div class="mt-4 p-3 bg-red-50 border border-red-200 rounded-md">
        <div class="flex">
          <svg class="h-5 w-5 text-red-400 mr-2" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
          </svg>
          <p class="text-sm text-red-800">{formError}</p>
        </div>
      </div>
    {/if}

    <!-- Action Buttons -->
    <div class="mt-6 flex space-x-3">
      <button
        onclick={handleSave}
        disabled={isSubmitting}
        class="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {#if isSubmitting}
          <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          Saving...
        {:else}
          Save Configuration
        {/if}
      </button>
      
      {#if verificationResult}
        <button
          type="button"
          onclick={clearVerification}
          class="btn-secondary"
        >
          Clear Verification
        </button>
      {/if}
    </div>
  </div>
</div>