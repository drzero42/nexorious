import { config } from '$lib/env';
import { auth } from '$lib/stores/auth.svelte';

export interface ApiError {
  message: string;
  status: number;
  details?: any;
}

export class ApiErrorException extends Error {
  constructor(
    public override message: string,
    public status: number,
    public details?: any
  ) {
    super(message);
    this.name = 'ApiErrorException';
  }
}

export interface ApiCallOptions extends RequestInit {
  skipAuth?: boolean;
  retryOnUnauthorized?: boolean;
}

/**
 * Unified API service for consistent error handling, token refresh, and retry logic
 */
export class ApiService {
  private static instance: ApiService;
  private refreshPromise: Promise<boolean> | null = null;

  static getInstance(): ApiService {
    if (!ApiService.instance) {
      ApiService.instance = new ApiService();
    }
    return ApiService.instance;
  }

  /**
   * Standardized API call with automatic token refresh and error handling
   */
  async call(url: string, options: ApiCallOptions = {}): Promise<Response> {
    const {
      skipAuth = false,
      retryOnUnauthorized = true,
      ...fetchOptions
    } = options;

    // Prepare headers
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(fetchOptions.headers as Record<string, string>),
    };

    // Add authorization header if not skipped
    if (!skipAuth) {
      const authState = auth.value;
      if (!authState.accessToken) {
        throw new ApiErrorException('Not authenticated', 401);
      }
      headers['Authorization'] = `Bearer ${authState.accessToken}`;
    }

    // Make the initial request
    let response = await fetch(url, {
      ...fetchOptions,
      headers,
    });

    // Handle 401 errors with token refresh
    if (!response.ok && response.status === 401 && !skipAuth && retryOnUnauthorized) {
      const refreshed = await this.handleTokenRefresh();
      
      if (refreshed) {
        // Retry the request with new token
        const newAuthState = auth.value;
        headers['Authorization'] = `Bearer ${newAuthState.accessToken}`;
        
        response = await fetch(url, {
          ...fetchOptions,
          headers,
        });
      }
    }

    // Handle non-success responses
    if (!response.ok) {
      await this.handleApiError(response);
    }

    return response;
  }

  /**
   * Convenience method for GET requests
   */
  async get(url: string, options: Omit<ApiCallOptions, 'method'> = {}): Promise<Response> {
    return this.call(url, { ...options, method: 'GET' });
  }

  /**
   * Convenience method for POST requests
   */
  async post(url: string, data?: any, options: Omit<ApiCallOptions, 'method' | 'body'> = {}): Promise<Response> {
    return this.call(url, {
      ...options,
      method: 'POST',
      body: data ? JSON.stringify(data) : null,
    });
  }

  /**
   * Convenience method for PUT requests
   */
  async put(url: string, data?: any, options: Omit<ApiCallOptions, 'method' | 'body'> = {}): Promise<Response> {
    return this.call(url, {
      ...options,
      method: 'PUT',
      body: data ? JSON.stringify(data) : null,
    });
  }

  /**
   * Convenience method for DELETE requests
   */
  async delete(url: string, options: Omit<ApiCallOptions, 'method'> = {}): Promise<Response> {
    return this.call(url, { ...options, method: 'DELETE' });
  }

  /**
   * Get JSON data from response with proper error handling
   */
  async json<T = any>(response: Response): Promise<T> {
    try {
      return await response.json();
    } catch (error) {
      throw new ApiErrorException(
        'Invalid JSON response',
        response.status,
        { originalError: error }
      );
    }
  }

  /**
   * Combined call and JSON parsing for convenience
   */
  async getJson<T = any>(url: string, options: Omit<ApiCallOptions, 'method'> = {}): Promise<T> {
    const response = await this.get(url, options);
    return this.json<T>(response);
  }

  /**
   * Combined POST call and JSON parsing for convenience
   */
  async postJson<T = any>(url: string, data?: any, options: Omit<ApiCallOptions, 'method' | 'body'> = {}): Promise<T> {
    const response = await this.post(url, data, options);
    return this.json<T>(response);
  }

  /**
   * Handle token refresh with deduplication
   */
  private async handleTokenRefresh(): Promise<boolean> {
    // Return existing promise if refresh is already in progress
    if (this.refreshPromise) {
      return this.refreshPromise;
    }

    this.refreshPromise = auth.refreshAuth();
    const result = await this.refreshPromise;
    this.refreshPromise = null;
    
    return result;
  }

  /**
   * Handle API errors with consistent formatting
   */
  private async handleApiError(response: Response): Promise<never> {
    let errorDetails;
    let errorMessage = `HTTP ${response.status}: ${response.statusText}`;

    try {
      // Try to parse error response as JSON
      errorDetails = await response.json();
      if (errorDetails.detail) {
        errorMessage = errorDetails.detail;
      } else if (errorDetails.message) {
        errorMessage = errorDetails.message;
      }
    } catch {
      // If JSON parsing fails, use default error message
    }

    throw new ApiErrorException(errorMessage, response.status, errorDetails);
  }

  /**
   * Build full API URL
   */
  buildUrl(path: string): string {
    return `${config.apiUrl}${path.startsWith('/') ? path : `/${path}`}`;
  }
}

// Export singleton instance
export const api = ApiService.getInstance();

// Export helper for legacy compatibility
export const apiCall = (url: string, options: ApiCallOptions = {}) => api.call(url, options);