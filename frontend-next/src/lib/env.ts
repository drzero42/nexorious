const isDevelopment = process.env.NODE_ENV === 'development';

export const config = {
  apiUrl: process.env.NEXT_PUBLIC_API_URL || (isDevelopment ? 'http://localhost:8000/api' : '/api'),
  staticUrl: process.env.NEXT_PUBLIC_STATIC_URL || (isDevelopment ? 'http://localhost:8000' : ''),
  appName: process.env.NEXT_PUBLIC_APP_NAME || 'Nexorious',
  appVersion: process.env.NEXT_PUBLIC_APP_VERSION || '1.0.0',
  isDevelopment,
  isProduction: !isDevelopment,
} as const;
