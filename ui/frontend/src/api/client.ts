import { config } from '@/lib/env';

export class ApiErrorException extends Error {
  constructor(
    public override message: string,
    public status: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = 'ApiErrorException';
  }
}

export interface ApiCallOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>;
}

function buildUrl(
  path: string,
  params?: Record<string, string | number | boolean | undefined>,
): string {
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

// Maps a backend app_state (see internal/migrate AppState.String) to the page a
// running SPA must hard-navigate to. Document loads are redirected server-side
// via 302, but an already-loaded tab never issues a document request, so the
// gates return a 503 with this app_state instead and the client navigates here.
const APP_STATE_REDIRECTS: Record<string, string> = {
  db_unavailable: '/db-error',
  needs_migration: '/migrate',
  migrating: '/migrate',
  migration_failed: '/migrate',
  needs_setup: '/setup',
};

// If the response is an app-state 503 (issue #771), hard-navigate the tab to the
// matching page so it self-heals instead of trying to JSON.parse an HTML page.
// Clones the response so the body remains readable for downstream error handling.
async function maybeRedirectForAppState(response: Response): Promise<void> {
  if (response.status !== 503) return;
  let appState: string | undefined;
  try {
    const data = (await response.clone().json()) as { app_state?: unknown };
    if (typeof data?.app_state === 'string') appState = data.app_state;
  } catch {
    return;
  }
  if (!appState) return;
  const target = APP_STATE_REDIRECTS[appState];
  if (target && window.location.pathname !== target) {
    window.location.assign(target);
  }
}

async function handleApiError(response: Response): Promise<never> {
  let errorDetails: unknown;
  let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
  try {
    errorDetails = await response.json();
    if (typeof errorDetails === 'object' && errorDetails !== null) {
      const details = errorDetails as Record<string, unknown>;
      if (typeof details.detail === 'string') errorMessage = details.detail;
      else if (typeof details.error === 'string') errorMessage = details.error;
      else if (typeof details.message === 'string') errorMessage = details.message;
    }
  } catch {
    // use default message
  }
  throw new ApiErrorException(errorMessage, response.status, errorDetails);
}

export async function apiCall(path: string, options: ApiCallOptions = {}): Promise<Response> {
  const { params, ...fetchOptions } = options;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(fetchOptions.headers as Record<string, string>),
  };

  const url = buildUrl(path, params);
  const response = await fetch(url, {
    ...fetchOptions,
    headers,
    credentials: 'include',
  });

  if (!response.ok) {
    if (response.status === 401 && window.location.pathname !== '/login') {
      window.location.replace('/login');
    }
    await maybeRedirectForAppState(response);
    await handleApiError(response);
  }

  return response;
}

export const api = {
  get: <T = unknown>(path: string, options?: Omit<ApiCallOptions, 'method'>): Promise<T> =>
    apiCall(path, { ...options, method: 'GET' }).then((r) => r.json()),

  post: <T = unknown>(
    path: string,
    data?: unknown,
    options?: Omit<ApiCallOptions, 'method' | 'body'>,
  ): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),

  put: <T = unknown>(
    path: string,
    data?: unknown,
    options?: Omit<ApiCallOptions, 'method' | 'body'>,
  ): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'PUT',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),

  patch: <T = unknown>(
    path: string,
    data?: unknown,
    options?: Omit<ApiCallOptions, 'method' | 'body'>,
  ): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'PATCH',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),

  delete: <T = void>(path: string, options?: Omit<ApiCallOptions, 'method'>): Promise<T> =>
    apiCall(path, { ...options, method: 'DELETE' }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),
};

export async function apiUploadFile<T>(
  path: string,
  file: File,
  fieldName: string = 'file',
  extraFields?: Record<string, string>,
): Promise<T> {
  const formData = new FormData();
  formData.append(fieldName, file);
  if (extraFields) {
    for (const [key, value] of Object.entries(extraFields)) {
      formData.append(key, value);
    }
  }
  const url = buildUrl(path);
  const response = await fetch(url, {
    method: 'POST',
    body: formData,
    credentials: 'include',
  });
  if (!response.ok) {
    if (response.status === 401 && window.location.pathname !== '/login') {
      window.location.replace('/login');
    }
    await maybeRedirectForAppState(response);
    await handleApiError(response);
  }
  return response.json();
}

export async function apiDownloadFile(path: string): Promise<{ blob: Blob; filename: string }> {
  const url = buildUrl(path);
  const response = await fetch(url, {
    method: 'GET',
    credentials: 'include',
  });
  if (!response.ok) {
    if (response.status === 401 && window.location.pathname !== '/login') {
      window.location.replace('/login');
    }
    await maybeRedirectForAppState(response);
    await handleApiError(response);
  }
  const contentDisposition = response.headers.get('Content-Disposition');
  let filename = 'download';
  if (contentDisposition) {
    const filenameMatch = contentDisposition.match(/filename="?([^";\n]+)"?/);
    if (filenameMatch) filename = filenameMatch[1];
  }
  const blob = await response.blob();
  return { blob, filename };
}
