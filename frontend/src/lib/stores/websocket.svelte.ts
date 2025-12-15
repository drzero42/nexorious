/**
 * WebSocket store for real-time job updates.
 *
 * Provides connection management with auto-reconnect using exponential backoff.
 * Handles all job event types from the backend WebSocket endpoint.
 */

import { browser } from '$app/environment';
import { config } from '$lib/env';
import { auth } from './auth.svelte';
import { jobs } from './jobs.svelte';
import { review } from './review.svelte';
import { notifications } from './notifications.svelte';
import { JobStatus, type Job } from '$lib/types/jobs';
import { getJobTypeLabel, getJobSourceLabel } from '$lib/types/jobs';

// ============================================================================
// Types
// ============================================================================

export type WebSocketStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'polling';

export enum WebSocketEventType {
  CONNECTED = 'connected',
  ERROR = 'error',
  JOB_CREATED = 'job_created',
  JOB_PROGRESS = 'job_progress',
  JOB_STATUS_CHANGE = 'job_status_change',
  JOB_COMPLETED = 'job_completed',
  JOB_FAILED = 'job_failed',
  REVIEW_ITEM_UPDATE = 'review_item_update'
}

export interface WebSocketMessage {
  event: WebSocketEventType;
  timestamp: string;
}

export interface ConnectionMessage extends WebSocketMessage {
  user_id?: string;
  message?: string;
}

export interface JobWebSocketMessage extends WebSocketMessage {
  job: Job;
}

export interface WebSocketState {
  connection: WebSocket | null;
  status: WebSocketStatus;
  lastError: string | null;
  reconnectAttempts: number;
  lastConnectedAt: Date | null;
  lastMessageAt: Date | null;
  /** Whether currently using polling fallback mode */
  isPolling: boolean;
  /** Job IDs being actively polled */
  polledJobIds: Set<string>;
}

export type WebSocketEventCallback = (message: JobWebSocketMessage) => void;

// ============================================================================
// Configuration
// ============================================================================

const INITIAL_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;
const MAX_RECONNECT_ATTEMPTS = 10;
const RECONNECT_BACKOFF_MULTIPLIER = 2;
const POLLING_INTERVAL_MS = 3000;

// ============================================================================
// Store Implementation
// ============================================================================

const initialState: WebSocketState = {
  connection: null,
  status: 'disconnected',
  lastError: null,
  reconnectAttempts: 0,
  lastConnectedAt: null,
  lastMessageAt: null,
  isPolling: false,
  polledJobIds: new Set()
};

function createWebSocketStore() {
  let state = $state<WebSocketState>({
    ...initialState,
    polledJobIds: new Set(initialState.polledJobIds)
  });
  let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  let pollingInterval: ReturnType<typeof setInterval> | null = null;
  let eventListeners: Map<WebSocketEventType, Set<WebSocketEventCallback>> = new Map();

  /**
   * Calculate reconnect delay with exponential backoff.
   */
  function getReconnectDelay(): number {
    const delay =
      INITIAL_RECONNECT_DELAY_MS *
      Math.pow(RECONNECT_BACKOFF_MULTIPLIER, state.reconnectAttempts);
    return Math.min(delay, MAX_RECONNECT_DELAY_MS);
  }

  /**
   * Get the WebSocket URL based on current config.
   */
  function getWebSocketUrl(token: string): string {
    // Convert HTTP URL to WebSocket URL
    const apiUrl = config.apiUrl;
    const wsProtocol = apiUrl.startsWith('https') ? 'wss' : 'ws';
    const baseUrl = apiUrl.replace(/^https?/, wsProtocol).replace('/api', '');
    return `${baseUrl}/api/ws/jobs?token=${encodeURIComponent(token)}`;
  }

  /**
   * Schedule a reconnection attempt.
   * After max attempts, automatically switches to polling fallback.
   */
  function scheduleReconnect(): void {
    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout);
    }

    if (state.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      // Switch to polling fallback instead of giving up
      startPolling();
      return;
    }

    const delay = getReconnectDelay();
    state.status = 'reconnecting';

    reconnectTimeout = setTimeout(() => {
      state.reconnectAttempts++;
      store.connect();
    }, delay);
  }

  /**
   * Start polling fallback mode.
   * Polls all tracked jobs every POLLING_INTERVAL_MS.
   */
  function startPolling(): void {
    if (pollingInterval) {
      return; // Already polling
    }

    state.isPolling = true;
    state.status = 'polling';
    state.lastError = 'WebSocket unavailable, using polling fallback';

    pollingInterval = setInterval(pollJobs, POLLING_INTERVAL_MS);

    // Run an immediate poll
    pollJobs();
  }

  /**
   * Stop polling fallback mode.
   */
  function stopPolling(): void {
    if (pollingInterval) {
      clearInterval(pollingInterval);
      pollingInterval = null;
    }
    state.isPolling = false;
  }

  /**
   * Poll all tracked jobs for updates.
   */
  async function pollJobs(): Promise<void> {
    if (state.polledJobIds.size === 0) {
      return;
    }

    const authState = auth.value;
    if (!authState.accessToken) {
      return;
    }

    // Poll each tracked job
    const jobIds = Array.from(state.polledJobIds);
    for (const jobId of jobIds) {
      try {
        const response = await fetch(`${config.apiUrl}/jobs/${jobId}`, {
          headers: {
            Authorization: `Bearer ${authState.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 404) {
            // Job no longer exists, stop tracking it
            state.polledJobIds.delete(jobId);
          }
          continue;
        }

        const job: Job = await response.json();
        state.lastMessageAt = new Date();

        // Determine event type based on job status
        const eventType = getEventTypeForJob(job);

        // Update stores same as WebSocket would
        updateJobsStore(eventType, job);

        // Notify listeners
        const message: JobWebSocketMessage = {
          event: eventType,
          timestamp: new Date().toISOString(),
          job
        };
        const listeners = eventListeners.get(eventType);
        if (listeners) {
          listeners.forEach((callback) => callback(message));
        }

        // Stop polling completed/failed jobs
        if (
          job.status === JobStatus.COMPLETED ||
          job.status === JobStatus.FAILED ||
          job.status === JobStatus.CANCELLED
        ) {
          state.polledJobIds.delete(jobId);
        }
      } catch (error) {
        console.error(`Failed to poll job ${jobId}:`, error);
      }
    }
  }

  /**
   * Determine the appropriate event type based on job status.
   */
  function getEventTypeForJob(job: Job): WebSocketEventType {
    switch (job.status) {
      case JobStatus.COMPLETED:
        return WebSocketEventType.JOB_COMPLETED;
      case JobStatus.FAILED:
        return WebSocketEventType.JOB_FAILED;
      case JobStatus.CANCELLED:
        return WebSocketEventType.JOB_STATUS_CHANGE;
      case JobStatus.PROCESSING:
      case JobStatus.PENDING:
        return WebSocketEventType.JOB_PROGRESS;
      default:
        return WebSocketEventType.JOB_STATUS_CHANGE;
    }
  }

  /**
   * Handle incoming WebSocket message.
   */
  function handleMessage(event: MessageEvent): void {
    try {
      const message = JSON.parse(event.data) as WebSocketMessage;
      state.lastMessageAt = new Date();

      // Handle connection messages
      if (message.event === WebSocketEventType.CONNECTED) {
        // Stop polling if we were in polling mode
        stopPolling();
        state.status = 'connected';
        state.reconnectAttempts = 0;
        state.lastConnectedAt = new Date();
        state.lastError = null;
        return;
      }

      if (message.event === WebSocketEventType.ERROR) {
        const connMsg = message as ConnectionMessage;
        state.lastError = connMsg.message || 'Unknown error';
        return;
      }

      // Handle job messages
      const jobMessage = message as JobWebSocketMessage;
      if (jobMessage.job) {
        // Update jobs store with the new job data
        updateJobsStore(message.event, jobMessage.job);

        // Show toast notifications for job completions and failures
        showJobNotification(message.event, jobMessage.job);

        // Notify event listeners
        const listeners = eventListeners.get(message.event);
        if (listeners) {
          listeners.forEach((callback) => callback(jobMessage));
        }
      }
    } catch (error) {
      console.error('Failed to parse WebSocket message:', error);
    }
  }

  /**
   * Update the jobs store based on the event type.
   */
  function updateJobsStore(eventType: WebSocketEventType, job: Job): void {
    const jobsState = jobs.value;

    // Find existing job in the list
    const existingIndex = jobsState.jobs.findIndex((j) => j.id === job.id);

    if (eventType === WebSocketEventType.JOB_CREATED) {
      // Add new job to the beginning of the list
      if (existingIndex === -1) {
        jobsState.jobs = [job, ...jobsState.jobs];
        jobsState.pagination.total++;
      }
    } else {
      // Update existing job
      if (existingIndex !== -1) {
        jobsState.jobs[existingIndex] = job;
      } else {
        // Job not in list, add it
        jobsState.jobs = [job, ...jobsState.jobs];
      }
    }

    // Update current job if it matches
    if (jobsState.currentJob?.id === job.id) {
      jobsState.currentJob = job;
    }

    // For review item updates, refresh the review store summary
    if (eventType === WebSocketEventType.REVIEW_ITEM_UPDATE) {
      // Trigger a refresh of review summary (non-blocking)
      review.loadSummary().catch(() => {
        // Ignore errors - this is a best-effort refresh
      });
    }
  }

  /**
   * Show toast notifications for job completion and failure events.
   */
  function showJobNotification(eventType: WebSocketEventType, job: Job): void {
    const jobLabel = `${getJobTypeLabel(job.job_type)} (${getJobSourceLabel(job.source)})`;

    switch (eventType) {
      case WebSocketEventType.JOB_COMPLETED:
        notifications.showSuccess(`${jobLabel} completed successfully`);
        break;
      case WebSocketEventType.JOB_FAILED:
        notifications.showError(
          job.error_message
            ? `${jobLabel} failed: ${job.error_message}`
            : `${jobLabel} failed`
        );
        break;
      case WebSocketEventType.JOB_CREATED:
        notifications.showInfo(`${jobLabel} started`);
        break;
    }
  }

  const store = {
    get value() {
      return state;
    },

    /**
     * Connect to the WebSocket endpoint.
     * Requires authentication token from auth store.
     */
    connect: () => {
      if (!browser) return;

      const authState = auth.value;
      if (!authState.accessToken) {
        state.lastError = 'Not authenticated';
        return;
      }

      // Clean up existing connection
      if (state.connection) {
        state.connection.close();
        state.connection = null;
      }

      state.status = 'connecting';
      state.lastError = null;

      try {
        const url = getWebSocketUrl(authState.accessToken);
        const ws = new WebSocket(url);

        ws.onopen = () => {
          // Status will be updated when we receive the 'connected' message
        };

        ws.onmessage = handleMessage;

        ws.onerror = () => {
          state.lastError = 'WebSocket connection error';
        };

        ws.onclose = (event) => {
          state.connection = null;

          // Don't reconnect if this was a clean close or auth failure
          if (event.code === 1000 || event.code === 4001) {
            state.status = 'disconnected';
            if (event.code === 4001) {
              state.lastError = 'Authentication failed';
            }
          } else {
            // Schedule reconnect for unexpected disconnection
            scheduleReconnect();
          }
        };

        state.connection = ws;
      } catch (error) {
        state.lastError = error instanceof Error ? error.message : 'Connection failed';
        state.status = 'disconnected';
      }
    },

    /**
     * Disconnect from the WebSocket endpoint and stop polling.
     */
    disconnect: () => {
      if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
        reconnectTimeout = null;
      }

      // Stop polling if active
      stopPolling();

      if (state.connection) {
        state.connection.close(1000, 'Client disconnecting');
        state.connection = null;
      }

      state.status = 'disconnected';
      state.reconnectAttempts = 0;
    },

    /**
     * Subscribe to a specific event type.
     * Returns an unsubscribe function.
     */
    on: (eventType: WebSocketEventType, callback: WebSocketEventCallback): (() => void) => {
      if (!eventListeners.has(eventType)) {
        eventListeners.set(eventType, new Set());
      }

      eventListeners.get(eventType)!.add(callback);

      return () => {
        eventListeners.get(eventType)?.delete(callback);
      };
    },

    /**
     * Remove all listeners for a specific event type.
     */
    off: (eventType: WebSocketEventType) => {
      eventListeners.delete(eventType);
    },

    /**
     * Clear all event listeners.
     */
    clearListeners: () => {
      eventListeners.clear();
    },

    /**
     * Reset the store to initial state.
     */
    reset: () => {
      store.disconnect();
      store.clearListeners();
      state = { ...initialState, polledJobIds: new Set() };
    },

    /**
     * Check if currently connected and ready.
     */
    get isConnected() {
      return state.status === 'connected';
    },

    /**
     * Check if attempting to reconnect.
     */
    get isReconnecting() {
      return state.status === 'reconnecting';
    },

    /**
     * Check if currently using polling fallback.
     */
    get isPolling() {
      return state.isPolling;
    },

    /**
     * Check if receiving updates (either via WebSocket or polling).
     */
    get isReceivingUpdates() {
      return state.status === 'connected' || state.status === 'polling';
    },

    /**
     * Track a job for polling updates.
     * Call this when you want to receive updates for a specific job.
     * In WebSocket mode, updates come automatically.
     * In polling mode, only tracked jobs are polled.
     */
    trackJob: (jobId: string) => {
      state.polledJobIds.add(jobId);
    },

    /**
     * Stop tracking a job for polling updates.
     */
    untrackJob: (jobId: string) => {
      state.polledJobIds.delete(jobId);
    },

    /**
     * Clear all tracked jobs.
     */
    clearTrackedJobs: () => {
      state.polledJobIds.clear();
    },

    /**
     * Force switch to polling mode (useful for testing or manual override).
     */
    forcePolling: () => {
      if (state.connection) {
        state.connection.close(1000, 'Switching to polling');
        state.connection = null;
      }
      if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
        reconnectTimeout = null;
      }
      startPolling();
    },

    /**
     * Attempt to reconnect to WebSocket (exits polling mode if successful).
     */
    retryWebSocket: () => {
      stopPolling();
      state.reconnectAttempts = 0;
      state.lastError = null;
      store.connect();
    }
  };

  return store;
}

export const websocket = createWebSocketStore();
