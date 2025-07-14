import { env } from '$env/dynamic/public';
import { dev } from '$app/environment';

// Default configuration
const defaultConfig = {
  API_URL: dev ? 'http://localhost:8000' : '',
  APP_NAME: 'Nexorious',
  APP_VERSION: '1.0.0',
  ENVIRONMENT: dev ? 'development' : 'production'
} as const;

// Environment configuration with type safety
export const config = {
  apiUrl: env.PUBLIC_API_URL || defaultConfig.API_URL,
  appName: env.PUBLIC_APP_NAME || defaultConfig.APP_NAME,
  appVersion: env.PUBLIC_APP_VERSION || defaultConfig.APP_VERSION,
  environment: (env.PUBLIC_ENVIRONMENT || defaultConfig.ENVIRONMENT) as 'development' | 'production' | 'staging',
  isDevelopment: dev,
  isProduction: !dev
} as const;

// Type-safe environment validation
export function validateEnvironment(): void {
  const required = ['apiUrl', 'appName'] as const;
  const missing = required.filter(key => !config[key]);
  
  if (missing.length > 0) {
    throw new Error(`Missing required environment variables: ${missing.join(', ')}`);
  }
}

// Initialize environment validation
if (typeof window !== 'undefined') {
  validateEnvironment();
}