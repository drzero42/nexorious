/// <reference types="@sveltejs/kit" />
/// <reference types="vite/client" />
/// <reference types="vite-plugin-pwa/client" />

import type { GameId } from '$lib/types/game';

// Global environment variables
interface ImportMetaEnv {
  readonly PUBLIC_API_URL: string;
  readonly PUBLIC_STATIC_URL: string;
  readonly PUBLIC_APP_NAME: string;
  readonly PUBLIC_APP_VERSION: string;
  readonly PUBLIC_ENVIRONMENT: 'development' | 'production' | 'staging';
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

// Global types for the application
declare global {
  namespace App {
    interface Error {
      message: string;
      code?: string;
      statusCode?: number;
    }

    interface Locals {
      user?: {
        id: string;
        username: string;
        email: string;
        isAdmin: boolean;
      };
    }

    interface PageData {
      user?: {
        id: string;
        username: string;
        email: string;
        isAdmin: boolean;
      };
    }

    interface PageState {
      selected?: string;
    }

    interface Platform {
      desktop: boolean;
      mobile: boolean;
    }
  }

  // Custom events
  interface CustomEventMap {
    'auth:login': CustomEvent<{ user: App.Locals['user'] }>;
    'auth:logout': CustomEvent<void>;
    'game:added': CustomEvent<{ gameId: GameId }>;
    'game:updated': CustomEvent<{ gameId: GameId }>;
    'game:deleted': CustomEvent<{ gameId: GameId }>;
  }

  // Extend the global Window interface
  interface Window {
    // Add any global window properties here
  }
}

// API Response types
export interface ApiResponse<T = any> {
  data?: T;
  error?: {
    message: string;
    code?: string;
    details?: Record<string, any>;
  };
  meta?: {
    total?: number;
    page?: number;
    limit?: number;
    hasNext?: boolean;
    hasPrevious?: boolean;
  };
}

// Common utility types
export type Optional<T, K extends keyof T> = Omit<T, K> & Partial<Pick<T, K>>;
export type RequiredKeys<T> = {
  [K in keyof T]-?: {} extends Pick<T, K> ? never : K;
}[keyof T];
export type OptionalKeys<T> = {
  [K in keyof T]-?: {} extends Pick<T, K> ? K : never;
}[keyof T];

// Form validation types
export interface ValidationError {
  field: string;
  message: string;
  code?: string;
}

export interface FormValidationResult {
  isValid: boolean;
  errors: ValidationError[];
}

// Pagination types
export interface PaginationParams {
  page?: number;
  limit?: number;
  sort?: string;
  order?: 'asc' | 'desc';
}

export interface PaginationResult<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
  totalPages: number;
  hasNext: boolean;
  hasPrevious: boolean;
}

export type { GameId, UserGameId } from '$lib/types/game';
export {};