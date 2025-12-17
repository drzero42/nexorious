import { config } from '@/lib/env';

export interface ApiError {
  message: string;
  status: number;
  details?: unknown;
}

export class ApiErrorException extends Error {
  constructor(
    public override message: string,
    public status: number,
    public details?: unknown
  ) {
    super(message);
    this.name = 'ApiErrorException';
  }
}

export interface ApiCallOptions extends RequestInit {
  skipAuth?: boolean;
  params?: Record<string, string | number | boolean | undefined>;
}

type TokenGetter = () => string | null;
type TokenRefresher = () => Promise<boolean>;
type LogoutHandler = () => void;

let getAccessToken: TokenGetter = () => null;
let refreshTokens: TokenRefresher = async () => false;
let handleLogout: LogoutHandler = () => {};
let refreshPromise: Promise<boolean> | null = null;

export function setAuthHandlers(
  tokenGetter: TokenGetter,
  tokenRefresher: TokenRefresher,
  logoutHandler: LogoutHandler
) {
  getAccessToken = tokenGetter;
  refreshTokens = tokenRefresher;
  handleLogout = logoutHandler;
}

function buildUrl(path: string, params?: Record<string, string | number | boolean | undefined>): string {
  const baseUrl = `${config.apiUrl}${path.startsWith('/') ? path : `/${path}`}`;

  if (!params) return baseUrl;

  const searchParams = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined) {
      searchParams.append(key, String(value));
    }
  });

  const queryString = searchParams.toString();
  return queryString ? `${baseUrl}?${queryString}` : baseUrl;
}

async function handleApiError(response: Response): Promise<never> {
  let errorDetails: unknown;
  let errorMessage = `HTTP ${response.status}: ${response.statusText}`;

  try {
    errorDetails = await response.json();
    if (typeof errorDetails === 'object' && errorDetails !== null) {
      const details = errorDetails as Record<string, unknown>;
      if (typeof details.detail === 'string') {
        errorMessage = details.detail;
      } else if (typeof details.message === 'string') {
        errorMessage = details.message;
      }
    }
  } catch {
    // Use default error message
  }

  throw new ApiErrorException(errorMessage, response.status, errorDetails);
}

async function handleTokenRefresh(): Promise<boolean> {
  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = refreshTokens();
  const result = await refreshPromise;
  refreshPromise = null;

  return result;
}

export async function apiCall(
  path: string,
  options: ApiCallOptions = {}
): Promise<Response> {
  const { skipAuth = false, params, ...fetchOptions } = options;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(fetchOptions.headers as Record<string, string>),
  };

  if (!skipAuth) {
    const token = getAccessToken();
    if (!token) {
      throw new ApiErrorException('Not authenticated', 401);
    }
    headers['Authorization'] = `Bearer ${token}`;
  }

  const url = buildUrl(path, params);

  let response = await fetch(url, {
    ...fetchOptions,
    headers,
  });

  // Handle 401 with token refresh
  if (!response.ok && response.status === 401 && !skipAuth) {
    const refreshed = await handleTokenRefresh();

    if (refreshed) {
      const newToken = getAccessToken();
      if (newToken) {
        headers['Authorization'] = `Bearer ${newToken}`;
        response = await fetch(url, {
          ...fetchOptions,
          headers,
        });
      }
    } else {
      handleLogout();
    }
  }

  if (!response.ok) {
    await handleApiError(response);
  }

  return response;
}

export const api = {
  get: <T = unknown>(path: string, options?: Omit<ApiCallOptions, 'method'>): Promise<T> =>
    apiCall(path, { ...options, method: 'GET' }).then((r) => r.json()),

  post: <T = unknown>(path: string, data?: unknown, options?: Omit<ApiCallOptions, 'method' | 'body'>): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => r.json()),

  put: <T = unknown>(path: string, data?: unknown, options?: Omit<ApiCallOptions, 'method' | 'body'>): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'PUT',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => r.json()),

  patch: <T = unknown>(path: string, data?: unknown, options?: Omit<ApiCallOptions, 'method' | 'body'>): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'PATCH',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => r.json()),

  delete: <T = void>(path: string, options?: Omit<ApiCallOptions, 'method'>): Promise<T> =>
    apiCall(path, { ...options, method: 'DELETE' }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),
};
