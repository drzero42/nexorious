import { config } from '$lib/env';
import { auth } from '$lib/stores';

export interface WebSocketEvent {
  event_type: 'import_status_change' | 'import_progress' | 'game_matched' | 'game_needs_review' | 
        'game_imported' | 'platform_added' | 'game_skipped' | 'import_complete' | 'import_error' |
        'heartbeat' | 'pong';
  data: any;
  timestamp: string;
}

export interface SteamImportWebSocketEventHandlers {
  onOpen?: () => void;
  onClose?: (event: CloseEvent) => void;
  onError?: (error: Event) => void;
  onMessage?: (event: WebSocketEvent) => void;
  onStatusChange?: (status: string, data: any) => void;
  onProgress?: (data: any) => void;
  onGameMatched?: (data: any) => void;
  onGameNeedsReview?: (data: any) => void;
  onGameImported?: (data: any) => void;
  onPlatformAdded?: (data: any) => void;
  onGameSkipped?: (data: any) => void;
  onImportComplete?: (data: any) => void;
  onImportError?: (error: string, data: any) => void;
}

export type WebSocketConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'error';

export class SteamImportWebSocketService {
  private websocket: WebSocket | null = null;
  private jobId: string;
  private eventHandlers: SteamImportWebSocketEventHandlers;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 10;
  private reconnectDelay = 1000; // Start with 1 second
  private maxReconnectDelay = 30000; // Max 30 seconds
  private reconnectTimeout: NodeJS.Timeout | null = null;
  private heartbeatInterval: NodeJS.Timeout | null = null;
  private connectionStatus: WebSocketConnectionStatus = 'disconnected';
  private lastActivity: Date | null = null;
  private isIntentionallyClosed = false;

  // Connection health monitoring
  private missedHeartbeats = 0;
  private maxMissedHeartbeats = 3;

  constructor(jobId: string, eventHandlers: SteamImportWebSocketEventHandlers = {}) {
    this.jobId = jobId;
    this.eventHandlers = eventHandlers;
  }

  /**
   * Connect to the WebSocket
   */
  async connect(): Promise<void> {
    if (this.websocket?.readyState === WebSocket.OPEN) {
      return; // Already connected
    }

    if (this.websocket?.readyState === WebSocket.CONNECTING) {
      return; // Already connecting
    }

    this.isIntentionallyClosed = false;
    this.setConnectionStatus('connecting');

    try {
      const token = auth.value.accessToken;
      if (!token) {
        throw new Error('No authentication token available');
      }

      const wsUrl = config.apiUrl.replace(/^http/, 'ws') + `/steam/import/ws/${this.jobId}?token=${encodeURIComponent(token)}`;
      
      this.websocket = new WebSocket(wsUrl);
      this.setupWebSocketEventListeners();
      
    } catch (error) {
      console.error('Failed to create WebSocket connection:', error);
      this.setConnectionStatus('error');
      this.eventHandlers.onError?.(error as Event);
      this.scheduleReconnect();
    }
  }

  /**
   * Disconnect from the WebSocket
   */
  disconnect(): void {
    this.isIntentionallyClosed = true;
    this.clearReconnectTimeout();
    this.clearHeartbeatInterval();
    
    if (this.websocket) {
      this.websocket.close(1000, 'Intentional disconnect');
      this.websocket = null;
    }
    
    this.setConnectionStatus('disconnected');
    this.reconnectAttempts = 0;
  }

  /**
   * Manually trigger reconnection
   */
  reconnect(): void {
    if (this.isIntentionallyClosed) {
      return;
    }

    this.disconnect();
    this.isIntentionallyClosed = false;
    setTimeout(() => this.connect(), 100);
  }

  /**
   * Send a message through the WebSocket
   */
  send(message: any): boolean {
    if (this.websocket?.readyState === WebSocket.OPEN) {
      try {
        this.websocket.send(JSON.stringify(message));
        this.updateLastActivity();
        return true;
      } catch (error) {
        console.error('Failed to send WebSocket message:', error);
        return false;
      }
    }
    return false;
  }

  /**
   * Get current connection status
   */
  getConnectionStatus(): WebSocketConnectionStatus {
    return this.connectionStatus;
  }

  /**
   * Get reconnection info
   */
  getReconnectionInfo() {
    return {
      attempts: this.reconnectAttempts,
      maxAttempts: this.maxReconnectAttempts,
      lastActivity: this.lastActivity
    };
  }

  /**
   * Setup WebSocket event listeners
   */
  private setupWebSocketEventListeners(): void {
    if (!this.websocket) return;

    this.websocket.onopen = () => {
      console.log(`WebSocket connected for job ${this.jobId}`);
      this.setConnectionStatus('connected');
      this.reconnectAttempts = 0;
      this.reconnectDelay = 1000; // Reset delay
      this.missedHeartbeats = 0;
      this.updateLastActivity();
      this.startHeartbeat();
      this.eventHandlers.onOpen?.();
    };

    this.websocket.onclose = (event) => {
      console.log(`WebSocket closed for job ${this.jobId}:`, event.code, event.reason);
      this.clearHeartbeatInterval();
      
      if (!this.isIntentionallyClosed) {
        this.setConnectionStatus('error');
        this.scheduleReconnect();
      } else {
        this.setConnectionStatus('disconnected');
      }
      
      this.eventHandlers.onClose?.(event);
    };

    this.websocket.onerror = (error) => {
      console.error(`WebSocket error for job ${this.jobId}:`, error);
      this.setConnectionStatus('error');
      this.eventHandlers.onError?.(error);
    };

    this.websocket.onmessage = (event) => {
      this.updateLastActivity();
      this.missedHeartbeats = 0; // Reset missed heartbeats on any message
      
      try {
        const wsEvent: WebSocketEvent = JSON.parse(event.data);
        this.handleWebSocketEvent(wsEvent);
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error, event.data);
      }
    };
  }

  /**
   * Handle incoming WebSocket events
   */
  private handleWebSocketEvent(event: WebSocketEvent): void {
    // Call general message handler
    this.eventHandlers.onMessage?.(event);

    // Call specific event handlers
    switch (event.event_type) {
      case 'import_status_change':
        this.eventHandlers.onStatusChange?.(event.data.status, event.data);
        break;
      case 'import_progress':
        this.eventHandlers.onProgress?.(event.data);
        break;
      case 'game_matched':
        this.eventHandlers.onGameMatched?.(event.data);
        break;
      case 'game_needs_review':
        this.eventHandlers.onGameNeedsReview?.(event.data);
        break;
      case 'game_imported':
        this.eventHandlers.onGameImported?.(event.data);
        break;
      case 'platform_added':
        this.eventHandlers.onPlatformAdded?.(event.data);
        break;
      case 'game_skipped':
        this.eventHandlers.onGameSkipped?.(event.data);
        break;
      case 'import_complete':
        this.eventHandlers.onImportComplete?.(event.data);
        break;
      case 'import_error':
        this.eventHandlers.onImportError?.(event.data.error, event.data);
        break;
      case 'pong':
        // Handle pong response - connection is healthy
        break;
      case 'heartbeat':
        // Respond to server heartbeat
        this.send({ type: 'heartbeat_response', timestamp: new Date().toISOString() });
        break;
      default:
        console.log('Unknown WebSocket event type:', event.event_type);
    }
  }

  /**
   * Schedule reconnection with exponential backoff
   */
  private scheduleReconnect(): void {
    if (this.isIntentionallyClosed || this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.log('Max reconnection attempts reached or intentionally closed');
      this.setConnectionStatus('error');
      return;
    }

    this.setConnectionStatus('reconnecting');
    this.reconnectAttempts++;
    
    // Exponential backoff with jitter
    const jitter = Math.random() * 1000;
    const delay = Math.min(this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1) + jitter, this.maxReconnectDelay);
    
    console.log(`Scheduling reconnection attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts} in ${Math.round(delay)}ms`);
    
    this.reconnectTimeout = setTimeout(() => {
      this.connect();
    }, delay);
  }

  /**
   * Start heartbeat mechanism
   */
  private startHeartbeat(): void {
    this.clearHeartbeatInterval();
    
    this.heartbeatInterval = setInterval(() => {
      if (this.websocket?.readyState === WebSocket.OPEN) {
        // Send ping
        const sent = this.send({ type: 'ping', timestamp: new Date().toISOString() });
        
        if (!sent) {
          this.missedHeartbeats++;
        } else {
          // Check for missed heartbeats
          this.missedHeartbeats++;
          
          if (this.missedHeartbeats >= this.maxMissedHeartbeats) {
            console.log('Too many missed heartbeats, reconnecting...');
            this.reconnect();
          }
        }
      }
    }, 30000); // Send heartbeat every 30 seconds
  }

  /**
   * Update connection status and notify handlers
   */
  private setConnectionStatus(status: WebSocketConnectionStatus): void {
    if (this.connectionStatus !== status) {
      this.connectionStatus = status;
      console.log(`WebSocket status changed to: ${status}`);
    }
  }

  /**
   * Update last activity timestamp
   */
  private updateLastActivity(): void {
    this.lastActivity = new Date();
  }

  /**
   * Clear reconnection timeout
   */
  private clearReconnectTimeout(): void {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
  }

  /**
   * Clear heartbeat interval
   */
  private clearHeartbeatInterval(): void {
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval);
      this.heartbeatInterval = null;
    }
  }

  /**
   * Cleanup when service is destroyed
   */
  destroy(): void {
    this.disconnect();
  }
}