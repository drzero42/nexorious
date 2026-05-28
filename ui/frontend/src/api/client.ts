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
): Promise<T> {
  const formData = new FormData();
  formData.append(fieldName, file);
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
