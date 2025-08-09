<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { notifications } from '$lib/stores/notifications.svelte';

  interface Props {
    fallback?: boolean;
    showNotification?: boolean;
    errorTitle?: string;
    children?: any;
  }

  let { 
    fallback = true, 
    showNotification = true,
    errorTitle = 'An error occurred',
    children 
  }: Props = $props();

  let hasError = $state(false);
  let errorMessage = $state<string>('');
  let errorStack = $state<string>('');

  // Error handler for unhandled promise rejections
  function handleUnhandledRejection(event: PromiseRejectionEvent) {
    console.error('Unhandled promise rejection:', event.reason);
    
    const error = event.reason;
    const message = error instanceof Error ? error.message : String(error);
    
    if (showNotification) {
      notifications.showError(`${errorTitle}: ${message}`);
    }
    
    if (fallback) {
      hasError = true;
      errorMessage = message;
      errorStack = error instanceof Error ? error.stack || '' : '';
    }

    // Prevent default browser handling
    event.preventDefault();
  }

  // Error handler for uncaught errors
  function handleError(event: ErrorEvent) {
    console.error('Uncaught error:', event.error);
    
    const message = event.error instanceof Error ? event.error.message : event.message;
    
    if (showNotification) {
      notifications.showError(`${errorTitle}: ${message}`);
    }
    
    if (fallback) {
      hasError = true;
      errorMessage = message;
      errorStack = event.error instanceof Error ? event.error.stack || '' : '';
    }

    // Prevent default browser handling
    event.preventDefault();
  }

  onMount(() => {
    window.addEventListener('unhandledrejection', handleUnhandledRejection);
    window.addEventListener('error', handleError);
  });

  onDestroy(() => {
    window.removeEventListener('unhandledrejection', handleUnhandledRejection);
    window.removeEventListener('error', handleError);
  });

  function retry() {
    hasError = false;
    errorMessage = '';
    errorStack = '';
  }

  function reportError() {
    // In a real app, this would send error reports to a service
    console.log('Error report:', { errorMessage, errorStack, userAgent: navigator.userAgent });
    notifications.showInfo('Error report sent. Thank you for your feedback.');
  }
</script>

{#if hasError && fallback}
  <div class="min-h-screen bg-gray-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8">
    <div class="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
      <div class="bg-white py-8 px-4 shadow sm:rounded-lg sm:px-10">
        <div class="text-center">
          <!-- Error Icon -->
          <svg class="mx-auto h-12 w-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.694-.833-2.464 0L4.35 16.5c-.77.833.192 2.5 1.732 2.5z" />
          </svg>
          
          <h1 class="mt-4 text-lg font-medium text-gray-900">
            {errorTitle}
          </h1>
          
          <p class="mt-2 text-sm text-gray-600">
            Something went wrong. Please try refreshing the page or contact support if the problem persists.
          </p>
          
          {#if errorMessage}
            <div class="mt-4 p-4 bg-gray-100 rounded-md text-left">
              <p class="text-sm text-gray-800 font-mono">
                {errorMessage}
              </p>
            </div>
          {/if}
          
          <div class="mt-6 flex flex-col space-y-3">
            <button
              onclick={retry}
              class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              Try Again
            </button>
            
            <button
              onclick={() => window.location.reload()}
              class="w-full flex justify-center py-2 px-4 border border-gray-300 rounded-md shadow-sm bg-white text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              Refresh Page
            </button>
            
            <button
              onclick={reportError}
              class="w-full flex justify-center py-2 px-4 border border-gray-300 rounded-md shadow-sm bg-white text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
            >
              Report Error
            </button>
          </div>
          
          <div class="mt-4">
            <a 
              href="/" 
              class="text-sm text-primary-600 hover:text-primary-500"
            >
              ← Back to Home
            </a>
          </div>
        </div>
      </div>
    </div>
  </div>
{:else}
  {@render children?.()}
{/if}