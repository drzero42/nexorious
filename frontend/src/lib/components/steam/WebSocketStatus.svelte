<script lang="ts">
  import { steamImport } from '$lib/stores/steam-import.svelte';

  // Get WebSocket connection status
  $: connectionStatus = steamImport.value.connectionStatus;
  $: isConnected = connectionStatus === 'connected';
  $: isConnecting = connectionStatus === 'connecting';
  $: isDisconnected = connectionStatus === 'disconnected';
  $: isReconnecting = connectionStatus === 'reconnecting';
  $: hasError = connectionStatus === 'error';

  // Get reconnection attempt info
  $: reconnectAttempts = steamImport.value.reconnectAttempts;
  $: maxReconnectAttempts = steamImport.value.maxReconnectAttempts;

  // Get last activity/heartbeat info
  $: lastActivity = steamImport.value.lastActivity;

  function getStatusInfo() {
    if (isConnected) {
      return {
        icon: 'connected',
        color: 'green',
        label: 'Connected',
        description: 'Real-time updates active'
      };
    } else if (isConnecting) {
      return {
        icon: 'connecting',
        color: 'blue',
        label: 'Connecting',
        description: 'Establishing connection...'
      };
    } else if (isReconnecting) {
      return {
        icon: 'reconnecting',
        color: 'orange',
        label: 'Reconnecting',
        description: `Attempt ${reconnectAttempts}/${maxReconnectAttempts}`
      };
    } else if (hasError) {
      return {
        icon: 'error',
        color: 'red',
        label: 'Connection Error',
        description: 'Unable to connect for real-time updates'
      };
    } else {
      return {
        icon: 'disconnected',
        color: 'gray',
        label: 'Disconnected',
        description: 'Real-time updates unavailable'
      };
    }
  }

  $: statusInfo = getStatusInfo();

  function handleReconnect() {
    steamImport.reconnect();
  }

  function formatLastActivity(timestamp: Date | null): string {
    if (!timestamp) return '';
    
    const now = new Date();
    const diff = now.getTime() - timestamp.getTime();
    const seconds = Math.floor(diff / 1000);
    
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    return `${hours}h ago`;
  }
</script>

<!-- WebSocket Status Indicator -->
<div class="flex items-center space-x-2">
  <!-- Status Icon -->
  <div class="relative">
    {#if statusInfo.icon === 'connected'}
      <div class="w-3 h-3 bg-green-500 rounded-full"></div>
      <div class="absolute inset-0 w-3 h-3 bg-green-500 rounded-full animate-ping opacity-75"></div>
    {:else if statusInfo.icon === 'connecting' || statusInfo.icon === 'reconnecting'}
      <div class="w-3 h-3 bg-{statusInfo.color}-500 rounded-full animate-pulse"></div>
    {:else if statusInfo.icon === 'error'}
      <div class="w-3 h-3 bg-red-500 rounded-full"></div>
    {:else}
      <div class="w-3 h-3 bg-gray-400 rounded-full"></div>
    {/if}
  </div>

  <!-- Status Text (Desktop) -->
  <div class="hidden sm:block">
    <div class="text-sm font-medium text-{statusInfo.color}-600">
      {statusInfo.label}
    </div>
    <div class="text-xs text-gray-500">
      {statusInfo.description}
    </div>
  </div>

  <!-- Detailed Status Tooltip (Mobile/Hover) -->
  <div class="relative group">
    <button 
      class="sm:hidden p-1 rounded-full hover:bg-gray-100 transition-colors"
      type="button"
      aria-label="View WebSocket connection details"
    >
      <svg class="w-4 h-4 text-gray-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
    </button>

    <!-- Tooltip -->
    <div class="absolute right-0 top-8 z-50 w-64 bg-white border border-gray-200 rounded-lg shadow-lg p-3 
                opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all duration-200
                sm:group-hover:opacity-0 sm:group-hover:invisible">
      
      <!-- Connection Status -->
      <div class="flex items-center space-x-2 mb-2">
        <div class="w-2 h-2 bg-{statusInfo.color}-500 rounded-full"></div>
        <span class="text-sm font-medium text-{statusInfo.color}-600">{statusInfo.label}</span>
      </div>
      
      <p class="text-xs text-gray-600 mb-3">
        {statusInfo.description}
      </p>

      <!-- Additional Info -->
      {#if isReconnecting}
        <div class="mb-2">
          <div class="text-xs text-gray-500">
            Reconnection attempts: {reconnectAttempts}/{maxReconnectAttempts}
          </div>
          <div class="w-full bg-gray-200 rounded-full h-1 mt-1">
            <div 
              class="bg-orange-500 h-1 rounded-full transition-all duration-300" 
              style="width: {(reconnectAttempts / maxReconnectAttempts) * 100}%"
            ></div>
          </div>
        </div>
      {/if}

      {#if lastActivity && isConnected}
        <div class="text-xs text-gray-500 mb-2">
          Last activity: {formatLastActivity(lastActivity)}
        </div>
      {/if}

      <!-- Action Buttons -->
      {#if hasError || isDisconnected}
        <button
          on:click={handleReconnect}
          class="w-full text-xs btn-primary py-1"
        >
          Reconnect
        </button>
      {/if}

      <!-- What this means -->
      <div class="mt-2 pt-2 border-t border-gray-100">
        <div class="text-xs text-gray-500">
          {#if isConnected}
            ✓ Real-time import progress updates
          {:else if isConnecting || isReconnecting}
            ⏳ Connecting for real-time updates
          {:else}
            ⚠️ Manual refresh needed for updates
          {/if}
        </div>
      </div>
    </div>
  </div>

  <!-- Reconnect Button (Desktop Error State) -->
  {#if (hasError || isDisconnected) && !isReconnecting}
    <button
      on:click={handleReconnect}
      class="hidden sm:block text-xs text-{statusInfo.color}-600 hover:text-{statusInfo.color}-500 font-medium"
    >
      Reconnect
    </button>
  {/if}
</div>

<style>
  /* Ensure tooltip appears above other elements */
  .group:hover .absolute {
    z-index: 50;
  }
  
  /* Smooth color transitions */
  .text-green-600 { color: #059669; }
  .text-blue-600 { color: #2563eb; }
  .text-orange-600 { color: #ea580c; }
  .text-red-600 { color: #dc2626; }
  .text-gray-600 { color: #4b5563; }
  
  .bg-green-500 { background-color: #10b981; }
  .bg-blue-500 { background-color: #3b82f6; }
  .bg-orange-500 { background-color: #f97316; }
  .bg-red-500 { background-color: #ef4444; }
  .bg-gray-400 { background-color: #9ca3af; }
  
  .hover\:text-green-500:hover { color: #10b981; }
  .hover\:text-blue-500:hover { color: #3b82f6; }
  .hover\:text-orange-500:hover { color: #f97316; }
  .hover\:text-red-500:hover { color: #ef4444; }
</style>