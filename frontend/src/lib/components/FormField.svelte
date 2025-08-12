<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    label: string;
    id: string;
    error?: string | null;
    required?: boolean;
    helpText?: string;
    isDirty?: boolean;
    class?: string;
    children: Snippet;
  }

  let {
    label,
    id,
    error = null,
    required = false,
    helpText,
    isDirty = false,
    class: className = '',
    children
  }: Props = $props();

  // Derive validation state
  const hasError = $derived(Boolean(error));
  const showDirtyIndicator = $derived(isDirty && !hasError);
</script>

<div class="form-field {className}">
  <label for={id} class="form-label">
    {label}
    {#if required}
      <span class="text-red-500" aria-hidden="true">*</span>
    {/if}
    {#if showDirtyIndicator}
      <span class="text-blue-500 text-sm font-normal" title="Unsaved changes">
        (modified)
      </span>
    {/if}
  </label>
  
  <div class="form-field-input {hasError ? 'form-field-error' : ''}">
    {@render children()}
  </div>
  
  {#if error}
    <div class="form-error" role="alert" aria-describedby="{id}-error">
      <svg class="form-error-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <span id="{id}-error">{error}</span>
    </div>
  {:else if helpText}
    <div class="form-help">
      <svg class="form-help-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <span>{helpText}</span>
    </div>
  {/if}
</div>

<style>
  .form-field {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .form-field-input {
    position: relative;
  }

  .form-field-error :global(input),
  .form-field-error :global(select),
  .form-field-error :global(textarea) {
    border-color: rgb(252 165 165);
    --tw-ring-color: rgb(239 68 68);
  }

  .form-field-error :global(input:focus),
  .form-field-error :global(select:focus),
  .form-field-error :global(textarea:focus) {
    border-color: rgb(239 68 68);
    --tw-ring-color: rgb(239 68 68);
  }

  .form-error {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    font-size: 0.875rem;
    color: rgb(220 38 38);
  }

  .form-error-icon {
    height: 1rem;
    width: 1rem;
    margin-top: 0.125rem;
    flex-shrink: 0;
  }

  .form-help {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    font-size: 0.875rem;
    color: rgb(75 85 99);
  }

  .form-help-icon {
    height: 1rem;
    width: 1rem;
    margin-top: 0.125rem;
    flex-shrink: 0;
  }

  /* Global form styles that match existing patterns */
  :global(.form-label) {
    display: block;
    font-size: 0.875rem;
    font-weight: 500;
    color: rgb(55 65 81);
  }

  :global(.form-input) {
    margin-top: 0.25rem;
    display: block;
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid rgb(209 213 219);
    border-radius: 0.375rem;
    box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
    color: rgb(17 24 39);
    transition: border-color 0.2s, box-shadow 0.2s;
  }

  :global(.form-input:focus) {
    outline: none;
    border-color: rgb(59 130 246);
    box-shadow: 0 0 0 3px rgb(59 130 246 / 0.1);
  }

  :global(.form-input::placeholder) {
    color: rgb(156 163 175);
  }

  :global(.form-select) {
    margin-top: 0.25rem;
    display: block;
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid rgb(209 213 219);
    border-radius: 0.375rem;
    box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
    background-color: white;
    color: rgb(17 24 39);
    transition: border-color 0.2s, box-shadow 0.2s;
  }

  :global(.form-select:focus) {
    outline: none;
    border-color: rgb(59 130 246);
    box-shadow: 0 0 0 3px rgb(59 130 246 / 0.1);
  }

  :global(.form-textarea) {
    margin-top: 0.25rem;
    display: block;
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid rgb(209 213 219);
    border-radius: 0.375rem;
    box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
    color: rgb(17 24 39);
    transition: border-color 0.2s, box-shadow 0.2s;
    resize: vertical;
  }

  :global(.form-textarea:focus) {
    outline: none;
    border-color: rgb(59 130 246);
    box-shadow: 0 0 0 3px rgb(59 130 246 / 0.1);
  }

  :global(.form-textarea::placeholder) {
    color: rgb(156 163 175);
  }

  :global(.form-checkbox) {
    height: 1rem;
    width: 1rem;
    color: rgb(37 99 235);
    border: 1px solid rgb(209 213 219);
    border-radius: 0.25rem;
    transition: color 0.2s, border-color 0.2s;
  }

  :global(.form-checkbox:focus) {
    box-shadow: 0 0 0 3px rgb(59 130 246 / 0.1);
  }
</style>