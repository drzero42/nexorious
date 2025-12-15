import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { WebSocketState } from './websocket.svelte';
import { WebSocketEventType } from './websocket.svelte';

// Mock WebSocket
class MockWebSocket {
  static instances: MockWebSocket[] = [];
  url: string;
  readyState: number = 0;
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  close(code?: number, reason?: string) {
    this.readyState = 3;
    if (this.onclose) {
      this.onclose({ code: code || 1000, reason: reason || '' } as CloseEvent);
    }
  }

  send(_data: string) {
    // Mock send
  }

  // Test helpers
  simulateOpen() {
    this.readyState = 1;
    if (this.onopen) {
      this.onopen(new Event('open'));
    }
  }

  simulateMessage(data: object) {
    if (this.onmessage) {
      this.onmessage({ data: JSON.stringify(data) } as MessageEvent);
    }
  }

  simulateError() {
    if (this.onerror) {
      this.onerror(new Event('error'));
    }
  }

  simulateClose(code: number = 1000, reason: string = '') {
    this.readyState = 3;
    if (this.onclose) {
      this.onclose({ code, reason } as CloseEvent);
    }
  }

  static clear() {
    MockWebSocket.instances = [];
  }

  static getLatest(): MockWebSocket {
    const ws = MockWebSocket.instances[MockWebSocket.instances.length - 1];
    if (!ws) throw new Error('No MockWebSocket instance found');
    return ws;
  }
}

// Store the original WebSocket
const originalWebSocket = global.WebSocket;

// Mock the config module
vi.mock('$lib/env', () => ({
  config: {
    apiUrl: 'http://localhost:8000/api'
  }
}));

// Mock $app/environment
vi.mock('$app/environment', () => ({
  browser: true
}));

// Mock auth store
vi.mock('./auth.svelte', () => ({
  auth: {
    value: {
      accessToken: 'test-token',
      refreshToken: 'test-refresh',
      user: { id: '1', username: 'test', isAdmin: false },
      isLoading: false,
      error: null
    }
  }
}));

// Mock jobs store
vi.mock('./jobs.svelte', () => ({
  jobs: {
    value: {
      jobs: [],
      currentJob: null,
      isLoading: false,
      error: null,
      filters: {},
      pagination: { page: 1, per_page: 20, total: 0, pages: 0 }
    }
  }
}));

// Mock review store
vi.mock('./review.svelte', () => ({
  review: {
    loadSummary: vi.fn().mockResolvedValue({})
  }
}));

describe('WebSocket Store', () => {
  let websocket: any;

  beforeEach(async () => {
    // Clear mock instances
    MockWebSocket.clear();

    // Set up WebSocket mock
    global.WebSocket = MockWebSocket as any;

    // Dynamic import to get fresh store instance
    vi.resetModules();
    const module = await import('./websocket.svelte');
    websocket = module.websocket;

    // Ensure clean state
    websocket.reset();
  });

  afterEach(() => {
    // Clean up
    websocket.reset();
    MockWebSocket.clear();
    global.WebSocket = originalWebSocket;
  });

  describe('Store Structure', () => {
    it('should have correct structure and methods', () => {
      expect(websocket).toBeDefined();
      expect(websocket.value).toBeDefined();
      expect(typeof websocket.connect).toBe('function');
      expect(typeof websocket.disconnect).toBe('function');
      expect(typeof websocket.on).toBe('function');
      expect(typeof websocket.off).toBe('function');
      expect(typeof websocket.clearListeners).toBe('function');
      expect(typeof websocket.reset).toBe('function');
      // Polling methods
      expect(typeof websocket.trackJob).toBe('function');
      expect(typeof websocket.untrackJob).toBe('function');
      expect(typeof websocket.clearTrackedJobs).toBe('function');
      expect(typeof websocket.forcePolling).toBe('function');
      expect(typeof websocket.retryWebSocket).toBe('function');
    });

    it('should have correct initial state structure', () => {
      expect(websocket.value).toMatchObject({
        connection: null,
        status: 'disconnected',
        lastError: null,
        reconnectAttempts: 0,
        lastConnectedAt: null,
        lastMessageAt: null,
        isPolling: false
      });
      expect(websocket.value.polledJobIds).toBeInstanceOf(Set);
      expect(websocket.value.polledJobIds.size).toBe(0);
    });

    it('should have correct state types', () => {
      const state = websocket.value as WebSocketState;
      expect(state.connection === null || typeof state.connection === 'object').toBe(true);
      expect(typeof state.status).toBe('string');
      expect(state.lastError === null || typeof state.lastError === 'string').toBe(true);
      expect(typeof state.reconnectAttempts).toBe('number');
      expect(state.lastConnectedAt === null || state.lastConnectedAt instanceof Date).toBe(true);
      expect(state.lastMessageAt === null || state.lastMessageAt instanceof Date).toBe(true);
    });
  });

  describe('WebSocketEventType Enum', () => {
    it('should have all expected event types', () => {
      expect(WebSocketEventType.CONNECTED).toBe('connected');
      expect(WebSocketEventType.ERROR).toBe('error');
      expect(WebSocketEventType.JOB_CREATED).toBe('job_created');
      expect(WebSocketEventType.JOB_PROGRESS).toBe('job_progress');
      expect(WebSocketEventType.JOB_STATUS_CHANGE).toBe('job_status_change');
      expect(WebSocketEventType.JOB_COMPLETED).toBe('job_completed');
      expect(WebSocketEventType.JOB_FAILED).toBe('job_failed');
      expect(WebSocketEventType.REVIEW_ITEM_UPDATE).toBe('review_item_update');
    });
  });

  describe('Connection Management', () => {
    it('should create WebSocket connection when connect is called', () => {
      websocket.connect();

      expect(MockWebSocket.instances.length).toBe(1);
      expect(websocket.value.status).toBe('connecting');
    });

    it('should construct correct WebSocket URL', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      expect(ws.url).toBe('ws://localhost:8000/api/ws/jobs?token=test-token');
    });

    it('should update status to connected when receiving connected message', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.CONNECTED,
        timestamp: new Date().toISOString(),
        user_id: '1'
      });

      expect(websocket.value.status).toBe('connected');
      expect(websocket.value.lastConnectedAt).toBeInstanceOf(Date);
      expect(websocket.value.reconnectAttempts).toBe(0);
    });

    it('should handle disconnection', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.CONNECTED,
        timestamp: new Date().toISOString()
      });

      websocket.disconnect();

      expect(websocket.value.status).toBe('disconnected');
      expect(websocket.value.connection).toBeNull();
    });

    it('should handle clean close without reconnecting', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateClose(1000, 'Normal closure');

      expect(websocket.value.status).toBe('disconnected');
    });

    it('should handle auth failure close without reconnecting', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateClose(4001, 'Authentication failed');

      expect(websocket.value.status).toBe('disconnected');
      expect(websocket.value.lastError).toBe('Authentication failed');
    });

    it('should set error on WebSocket error', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateError();

      expect(websocket.value.lastError).toBe('WebSocket connection error');
    });

    it('should handle error message from server', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.ERROR,
        timestamp: new Date().toISOString(),
        message: 'Server error occurred'
      });

      expect(websocket.value.lastError).toBe('Server error occurred');
    });
  });

  describe('Event Listeners', () => {
    it('should register and trigger event listeners', () => {
      const callback = vi.fn();

      const unsubscribe = websocket.on(WebSocketEventType.JOB_PROGRESS, callback);

      websocket.connect();
      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();

      const jobMessage = {
        event: WebSocketEventType.JOB_PROGRESS,
        timestamp: new Date().toISOString(),
        job: {
          id: 'job-1',
          user_id: '1',
          job_type: 'import',
          source: 'steam',
          status: 'processing',
          priority: 'high',
          progress_current: 50,
          progress_total: 100,
          progress_percent: 50,
          result_summary: {},
          error_message: null,
          file_path: null,
          taskiq_task_id: null,
          created_at: new Date().toISOString(),
          started_at: new Date().toISOString(),
          completed_at: null,
          is_terminal: false,
          duration_seconds: null,
          review_item_count: 0,
          pending_review_count: 0
        }
      };

      ws.simulateMessage(jobMessage);

      expect(callback).toHaveBeenCalledTimes(1);
      expect(callback).toHaveBeenCalledWith(expect.objectContaining({
        event: WebSocketEventType.JOB_PROGRESS
      }));

      // Clean up
      unsubscribe();
    });

    it('should unsubscribe from events', () => {
      const callback = vi.fn();

      const unsubscribe = websocket.on(WebSocketEventType.JOB_COMPLETED, callback);
      unsubscribe();

      websocket.connect();
      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.JOB_COMPLETED,
        timestamp: new Date().toISOString(),
        job: { id: 'job-1' }
      });

      expect(callback).not.toHaveBeenCalled();
    });

    it('should remove all listeners for specific event type', () => {
      const callback1 = vi.fn();
      const callback2 = vi.fn();

      websocket.on(WebSocketEventType.JOB_FAILED, callback1);
      websocket.on(WebSocketEventType.JOB_FAILED, callback2);

      websocket.off(WebSocketEventType.JOB_FAILED);

      websocket.connect();
      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.JOB_FAILED,
        timestamp: new Date().toISOString(),
        job: { id: 'job-1' }
      });

      expect(callback1).not.toHaveBeenCalled();
      expect(callback2).not.toHaveBeenCalled();
    });

    it('should clear all listeners', () => {
      const callback1 = vi.fn();
      const callback2 = vi.fn();

      websocket.on(WebSocketEventType.JOB_CREATED, callback1);
      websocket.on(WebSocketEventType.JOB_COMPLETED, callback2);

      websocket.clearListeners();

      websocket.connect();
      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.JOB_CREATED,
        timestamp: new Date().toISOString(),
        job: { id: 'job-1' }
      });
      ws.simulateMessage({
        event: WebSocketEventType.JOB_COMPLETED,
        timestamp: new Date().toISOString(),
        job: { id: 'job-2' }
      });

      expect(callback1).not.toHaveBeenCalled();
      expect(callback2).not.toHaveBeenCalled();
    });
  });

  describe('Message Handling', () => {
    it('should update lastMessageAt on message received', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();

      const before = websocket.value.lastMessageAt;
      expect(before).toBeNull();

      ws.simulateMessage({
        event: WebSocketEventType.CONNECTED,
        timestamp: new Date().toISOString()
      });

      expect(websocket.value.lastMessageAt).toBeInstanceOf(Date);
    });

    it('should handle malformed JSON gracefully', () => {
      websocket.connect();

      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();

      // Simulate malformed message
      if (ws.onmessage) {
        ws.onmessage({ data: 'not-valid-json' } as MessageEvent);
      }

      // Should not throw, state should remain valid
      expect(websocket.value).toBeDefined();
    });
  });

  describe('Computed Properties', () => {
    it('should report isConnected correctly', () => {
      expect(websocket.isConnected).toBe(false);

      websocket.connect();
      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.CONNECTED,
        timestamp: new Date().toISOString()
      });

      expect(websocket.isConnected).toBe(true);

      websocket.disconnect();

      expect(websocket.isConnected).toBe(false);
    });

    it('should report isReconnecting correctly', () => {
      expect(websocket.isReconnecting).toBe(false);
    });

    it('should report isPolling correctly', () => {
      expect(websocket.isPolling).toBe(false);

      websocket.forcePolling();

      expect(websocket.isPolling).toBe(true);

      websocket.disconnect();

      expect(websocket.isPolling).toBe(false);
    });

    it('should report isReceivingUpdates for WebSocket', () => {
      expect(websocket.isReceivingUpdates).toBe(false);

      websocket.connect();
      const ws = MockWebSocket.getLatest();
      ws.simulateOpen();
      ws.simulateMessage({
        event: WebSocketEventType.CONNECTED,
        timestamp: new Date().toISOString()
      });

      expect(websocket.isReceivingUpdates).toBe(true);
    });

    it('should report isReceivingUpdates for polling', () => {
      expect(websocket.isReceivingUpdates).toBe(false);

      websocket.forcePolling();

      expect(websocket.isReceivingUpdates).toBe(true);
    });
  });

  describe('Reset', () => {
    it('should reset to initial state', () => {
      websocket.connect();
      websocket.on(WebSocketEventType.JOB_PROGRESS, vi.fn());

      websocket.reset();

      expect(websocket.value).toMatchObject({
        connection: null,
        status: 'disconnected',
        lastError: null,
        reconnectAttempts: 0,
        lastConnectedAt: null,
        lastMessageAt: null
      });
    });
  });

  describe('Edge Cases', () => {
    it('should handle multiple connect calls', () => {
      websocket.connect();
      websocket.connect();
      websocket.connect();

      // Should only have one active connection (previous ones closed)
      // Each connect should close the previous
      expect(MockWebSocket.instances.length).toBe(3);
      expect(websocket.value.status).toBe('connecting');
    });

    it('should handle multiple disconnect calls', () => {
      websocket.connect();
      websocket.disconnect();
      websocket.disconnect();
      websocket.disconnect();

      expect(websocket.value.status).toBe('disconnected');
      expect(websocket.value.connection).toBeNull();
    });

    it('should handle connect without auth token', async () => {
      // Re-mock auth with no token
      vi.doMock('./auth.svelte', () => ({
        auth: {
          value: {
            accessToken: null,
            refreshToken: null,
            user: null,
            isLoading: false,
            error: null
          }
        }
      }));

      vi.resetModules();
      const module = await import('./websocket.svelte');
      const unauthWebsocket = module.websocket;

      unauthWebsocket.connect();

      expect(unauthWebsocket.value.lastError).toBe('Not authenticated');
      expect(unauthWebsocket.value.status).toBe('disconnected');
    });
  });

  describe('Polling Fallback', () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    describe('Job Tracking', () => {
      it('should track jobs for polling', () => {
        websocket.trackJob('job-1');
        websocket.trackJob('job-2');

        expect(websocket.value.polledJobIds.size).toBe(2);
        expect(websocket.value.polledJobIds.has('job-1')).toBe(true);
        expect(websocket.value.polledJobIds.has('job-2')).toBe(true);
      });

      it('should untrack jobs', () => {
        websocket.trackJob('job-1');
        websocket.trackJob('job-2');
        websocket.untrackJob('job-1');

        expect(websocket.value.polledJobIds.size).toBe(1);
        expect(websocket.value.polledJobIds.has('job-1')).toBe(false);
        expect(websocket.value.polledJobIds.has('job-2')).toBe(true);
      });

      it('should clear all tracked jobs', () => {
        websocket.trackJob('job-1');
        websocket.trackJob('job-2');
        websocket.trackJob('job-3');
        websocket.clearTrackedJobs();

        expect(websocket.value.polledJobIds.size).toBe(0);
      });

      it('should not duplicate tracked jobs', () => {
        websocket.trackJob('job-1');
        websocket.trackJob('job-1');
        websocket.trackJob('job-1');

        expect(websocket.value.polledJobIds.size).toBe(1);
      });
    });

    describe('Polling Mode', () => {
      it('should switch to polling when forcePolling is called', () => {
        websocket.forcePolling();

        expect(websocket.value.status).toBe('polling');
        expect(websocket.value.isPolling).toBe(true);
        expect(websocket.isPolling).toBe(true);
      });

      it('should set lastError when entering polling mode', () => {
        websocket.forcePolling();

        expect(websocket.value.lastError).toBe('WebSocket unavailable, using polling fallback');
      });

      it('should stop polling when disconnect is called', () => {
        websocket.forcePolling();
        expect(websocket.value.isPolling).toBe(true);

        websocket.disconnect();

        expect(websocket.value.isPolling).toBe(false);
        expect(websocket.value.status).toBe('disconnected');
      });
    });

    describe('Reset with Polling', () => {
      it('should reset polling state', () => {
        websocket.forcePolling();
        websocket.trackJob('job-1');
        websocket.trackJob('job-2');

        websocket.reset();

        expect(websocket.value.isPolling).toBe(false);
        expect(websocket.value.polledJobIds.size).toBe(0);
        expect(websocket.value.status).toBe('disconnected');
      });
    });
  });
});
