// frontend/src/lib/env.ts
export const config = {
  apiUrl: import.meta.env.VITE_API_URL ?? '/api',
  staticUrl: import.meta.env.VITE_STATIC_URL ?? '',
  appName: import.meta.env.VITE_APP_NAME ?? 'Nexorious',
  appVersion: import.meta.env.VITE_APP_VERSION ?? '1.0.0',
  isDevelopment: import.meta.env.DEV,
  isProduction: import.meta.env.PROD,
} as const;
