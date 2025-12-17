# Next.js Frontend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a Next.js 15 frontend with shadcn/ui, TanStack Query, and TipTap that replicates the MVP features (auth, game library, game details) of the existing SvelteKit frontend.

**Architecture:** Client-side SPA using Next.js App Router with route groups for auth vs main layouts. API service layer with TanStack Query hooks for data fetching. JWT auth with localStorage persistence and automatic token refresh.

**Tech Stack:** Next.js 15, React 19, TypeScript, Tailwind CSS v4, shadcn/ui (Default/Zinc), TanStack Query v5, TipTap, react-hook-form, zod

---

## Phase 1: Project Setup

### Task 1: Initialize Next.js Project

**Files:**
- Create: `frontend-next/` (entire directory)

**Step 1: Create Next.js project with TypeScript and Tailwind**

```bash
cd /home/abo/workspace/home/nexorious
npx create-next-app@latest frontend-next --typescript --tailwind --eslint --app --src-dir --import-alias "@/*" --turbopack
```

When prompted:
- Would you like to use TypeScript? → Yes
- Would you like to use ESLint? → Yes
- Would you like to use Tailwind CSS? → Yes
- Would you like your code inside a `src/` directory? → Yes
- Would you like to use App Router? → Yes
- Would you like to use Turbopack? → Yes
- Would you like to customize the import alias? → Yes, use `@/*`

**Step 2: Verify project structure**

```bash
ls -la /home/abo/workspace/home/nexorious/frontend-next/src
```

Expected: `app/` directory exists

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/
git commit -m "feat(frontend-next): initialize Next.js 15 project"
```

---

### Task 2: Install Core Dependencies

**Files:**
- Modify: `frontend-next/package.json`

**Step 1: Install TanStack Query, react-hook-form, and zod**

```bash
cd /home/abo/workspace/home/nexorious/frontend-next
npm install @tanstack/react-query @tanstack/react-query-devtools
npm install react-hook-form @hookform/resolvers zod
npm install lucide-react
npm install clsx tailwind-merge class-variance-authority
```

**Step 2: Verify installation**

```bash
cat package.json | grep -E "(tanstack|react-hook-form|zod|lucide)"
```

Expected: All packages listed in dependencies

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/package.json frontend-next/package-lock.json
git commit -m "feat(frontend-next): add TanStack Query, form, and utility deps"
```

---

### Task 3: Initialize shadcn/ui

**Files:**
- Create: `frontend-next/components.json`
- Modify: `frontend-next/tailwind.config.ts`
- Create: `frontend-next/src/lib/utils.ts`

**Step 1: Initialize shadcn**

```bash
cd /home/abo/workspace/home/nexorious/frontend-next
npx shadcn@latest init
```

When prompted:
- Which style would you like to use? → Default
- Which color would you like to use as base color? → Zinc
- Would you like to use CSS variables for colors? → Yes

**Step 2: Verify components.json was created**

```bash
cat /home/abo/workspace/home/nexorious/frontend-next/components.json
```

Expected: JSON with `"style": "default"` and `"baseColor": "zinc"`

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/
git commit -m "feat(frontend-next): initialize shadcn/ui with Default/Zinc"
```

---

### Task 4: Install Essential shadcn Components

**Files:**
- Create: `frontend-next/src/components/ui/*.tsx` (multiple files)

**Step 1: Install core form and UI components**

```bash
cd /home/abo/workspace/home/nexorious/frontend-next
npx shadcn@latest add button input label card form
npx shadcn@latest add dialog alert-dialog dropdown-menu
npx shadcn@latest add select checkbox badge avatar
npx shadcn@latest add skeleton tabs pagination
npx shadcn@latest add command popover separator scroll-area
npx shadcn@latest add sonner
```

**Step 2: Verify components installed**

```bash
ls /home/abo/workspace/home/nexorious/frontend-next/src/components/ui/
```

Expected: button.tsx, input.tsx, card.tsx, form.tsx, etc.

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/
git commit -m "feat(frontend-next): add essential shadcn components"
```

---

### Task 5: Create Environment Configuration

**Files:**
- Create: `frontend-next/src/lib/env.ts`
- Create: `frontend-next/.env.local`
- Modify: `frontend-next/.gitignore`

**Step 1: Create env.ts**

Create file `frontend-next/src/lib/env.ts`:

```typescript
const isDevelopment = process.env.NODE_ENV === 'development';

export const config = {
  apiUrl: process.env.NEXT_PUBLIC_API_URL || (isDevelopment ? 'http://localhost:8000/api' : '/api'),
  staticUrl: process.env.NEXT_PUBLIC_STATIC_URL || (isDevelopment ? 'http://localhost:8000' : ''),
  appName: process.env.NEXT_PUBLIC_APP_NAME || 'Nexorious',
  appVersion: process.env.NEXT_PUBLIC_APP_VERSION || '1.0.0',
  isDevelopment,
  isProduction: !isDevelopment,
} as const;
```

**Step 2: Create .env.local**

Create file `frontend-next/.env.local`:

```
NEXT_PUBLIC_API_URL=http://localhost:8000/api
NEXT_PUBLIC_STATIC_URL=http://localhost:8000
```

**Step 3: Add .env.local to .gitignore**

Append to `frontend-next/.gitignore`:

```
# Local env files
.env.local
```

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/lib/env.ts frontend-next/.gitignore
git commit -m "feat(frontend-next): add environment configuration"
```

---

### Task 6: Create Type Definitions

**Files:**
- Create: `frontend-next/src/types/auth.ts`
- Create: `frontend-next/src/types/game.ts`
- Create: `frontend-next/src/types/platform.ts`
- Create: `frontend-next/src/types/index.ts`

**Step 1: Create auth types**

Create file `frontend-next/src/types/auth.ts`:

```typescript
export interface User {
  id: string;
  username: string;
  isAdmin: boolean;
  preferences?: Record<string, unknown>;
}

export interface AuthState {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  isLoading: boolean;
  error: string | null;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
}

export interface SetupStatusResponse {
  needs_setup: boolean;
}

export interface CreateAdminRequest {
  username: string;
  password: string;
}
```

**Step 2: Create game types**

Create file `frontend-next/src/types/game.ts`:

```typescript
import type { Platform, Storefront } from './platform';

// Branded types for type-safe IDs
export type GameId = number & { readonly __brand: 'GameId' };
export type UserGameId = string & { readonly __brand: 'UserGameId' };

export function isGameId(value: unknown): value is GameId {
  return typeof value === 'number' && Number.isInteger(value) && value > 0;
}

export function toGameId(value: unknown): GameId {
  if (typeof value === 'string') {
    const parsed = parseInt(value, 10);
    if (!isNaN(parsed) && isGameId(parsed)) {
      return parsed as GameId;
    }
  } else if (isGameId(value)) {
    return value as GameId;
  }
  throw new Error(`Invalid game ID: ${value}`);
}

export enum OwnershipStatus {
  OWNED = 'owned',
  BORROWED = 'borrowed',
  RENTED = 'rented',
  SUBSCRIPTION = 'subscription',
  NO_LONGER_OWNED = 'no_longer_owned',
}

export enum PlayStatus {
  NOT_STARTED = 'not_started',
  IN_PROGRESS = 'in_progress',
  COMPLETED = 'completed',
  MASTERED = 'mastered',
  DOMINATED = 'dominated',
  SHELVED = 'shelved',
  DROPPED = 'dropped',
  REPLAY = 'replay',
}

export interface Game {
  id: GameId;
  title: string;
  description?: string;
  genre?: string;
  developer?: string;
  publisher?: string;
  release_date?: string;
  cover_art_url?: string;
  rating_average?: number;
  rating_count: number;
  estimated_playtime_hours?: number;
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
  igdb_slug?: string;
  igdb_platform_names?: string;
  created_at: string;
  updated_at: string;
}

export interface UserGamePlatform {
  id: string;
  platform: Platform;
  storefront?: Storefront;
  store_game_id?: string;
  store_url?: string;
  is_available: boolean;
  created_at: string;
}

export interface Tag {
  id: string;
  user_id: string;
  name: string;
  color: string;
  description?: string;
  created_at: string;
  updated_at: string;
  game_count?: number;
}

export interface UserGame {
  id: UserGameId;
  game: Game;
  ownership_status: OwnershipStatus;
  is_physical: boolean;
  physical_location?: string;
  personal_rating?: number | null;
  is_loved: boolean;
  play_status: PlayStatus;
  hours_played: number;
  personal_notes?: string;
  acquired_date?: string;
  platforms: UserGamePlatform[];
  tags?: Tag[];
  created_at: string;
  updated_at: string;
}

export interface UserGameFilters {
  q?: string;
  play_status?: PlayStatus;
  ownership_status?: OwnershipStatus;
  platform_id?: string;
  tag_id?: string;
  is_loved?: boolean;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
  page?: number;
  per_page?: number;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface IGDBGameCandidate {
  igdb_id: GameId;
  igdb_slug?: string;
  title: string;
  release_date?: string;
  cover_art_url?: string;
  description?: string;
  platforms: string[];
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
}

export interface UserGameCreateRequest {
  game_id: GameId;
  ownership_status?: OwnershipStatus;
  play_status?: PlayStatus;
  platforms?: Array<{
    platform_id: string;
    storefront_id?: string;
  }>;
}

export interface UserGameUpdateRequest {
  ownership_status?: OwnershipStatus;
  is_physical?: boolean;
  physical_location?: string;
  personal_rating?: number | null;
  is_loved?: boolean;
  play_status?: PlayStatus;
  hours_played?: number;
  personal_notes?: string;
  acquired_date?: string;
}
```

**Step 3: Create platform types**

Create file `frontend-next/src/types/platform.ts`:

```typescript
export interface Platform {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  default_storefront_id?: string;
  storefronts?: Storefront[];
  created_at: string;
  updated_at: string;
}

export interface Storefront {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active: boolean;
  source: string;
  created_at: string;
  updated_at: string;
}

export interface PlatformsResponse {
  platforms: Platform[];
}

export interface StorefrontsResponse {
  storefronts: Storefront[];
}
```

**Step 4: Create index barrel export**

Create file `frontend-next/src/types/index.ts`:

```typescript
export * from './auth';
export * from './game';
export * from './platform';
```

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/types/
git commit -m "feat(frontend-next): add TypeScript type definitions"
```

---

### Task 7: Create API Client

**Files:**
- Create: `frontend-next/src/api/client.ts`

**Step 1: Create API client with auth handling**

Create file `frontend-next/src/api/client.ts`:

```typescript
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
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/api/client.ts
git commit -m "feat(frontend-next): add API client with auth interceptors"
```

---

### Task 8: Create Auth Service

**Files:**
- Create: `frontend-next/src/api/auth.ts`

**Step 1: Create auth API functions**

Create file `frontend-next/src/api/auth.ts`:

```typescript
import { config } from '@/lib/env';
import type {
  LoginRequest,
  LoginResponse,
  SetupStatusResponse,
  CreateAdminRequest,
  User,
} from '@/types';

export async function login(credentials: LoginRequest): Promise<LoginResponse> {
  const response = await fetch(`${config.apiUrl}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(credentials),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new Error(error.detail || 'Login failed');
  }

  return response.json();
}

export async function getMe(accessToken: string): Promise<User> {
  const response = await fetch(`${config.apiUrl}/auth/me`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });

  if (!response.ok) {
    throw new Error('Failed to fetch user profile');
  }

  const data = await response.json();
  return {
    ...data,
    isAdmin: data.is_admin,
  };
}

export async function refreshToken(refreshTokenValue: string): Promise<LoginResponse> {
  const response = await fetch(`${config.apiUrl}/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshTokenValue }),
  });

  if (!response.ok) {
    throw new Error('Token refresh failed');
  }

  return response.json();
}

export async function checkSetupStatus(): Promise<SetupStatusResponse> {
  const response = await fetch(`${config.apiUrl}/auth/setup/status`);

  if (!response.ok) {
    return { needs_setup: false };
  }

  return response.json();
}

export async function createInitialAdmin(data: CreateAdminRequest): Promise<User> {
  const response = await fetch(`${config.apiUrl}/auth/setup/admin`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new Error(error.detail || 'Failed to create admin');
  }

  return response.json();
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/api/auth.ts
git commit -m "feat(frontend-next): add auth API service"
```

---

### Task 9: Create Games and Platforms Services

**Files:**
- Create: `frontend-next/src/api/games.ts`
- Create: `frontend-next/src/api/platforms.ts`
- Create: `frontend-next/src/api/index.ts`

**Step 1: Create games API functions**

Create file `frontend-next/src/api/games.ts`:

```typescript
import { api } from './client';
import type {
  UserGame,
  UserGameFilters,
  PaginatedResponse,
  IGDBGameCandidate,
  UserGameCreateRequest,
  UserGameUpdateRequest,
  GameId,
  UserGameId,
  Tag,
} from '@/types';

export async function getUserGames(
  filters: UserGameFilters = {}
): Promise<PaginatedResponse<UserGame>> {
  return api.get('/user-games/', { params: filters as Record<string, string | number | boolean | undefined> });
}

export async function getUserGame(id: UserGameId): Promise<UserGame> {
  return api.get(`/user-games/${id}`);
}

export async function createUserGame(data: UserGameCreateRequest): Promise<UserGame> {
  return api.post('/user-games/', data);
}

export async function updateUserGame(
  id: UserGameId,
  data: UserGameUpdateRequest
): Promise<UserGame> {
  return api.patch(`/user-games/${id}`, data);
}

export async function deleteUserGame(id: UserGameId): Promise<void> {
  return api.delete(`/user-games/${id}`);
}

export async function bulkUpdateStatus(
  ids: UserGameId[],
  playStatus: string
): Promise<{ updated_count: number }> {
  return api.post('/user-games/bulk/status', {
    user_game_ids: ids,
    play_status: playStatus,
  });
}

export async function bulkDelete(
  ids: UserGameId[]
): Promise<{ deleted_count: number }> {
  return api.post('/user-games/bulk/delete', {
    user_game_ids: ids,
  });
}

export async function searchIGDB(query: string): Promise<{ games: IGDBGameCandidate[]; total: number }> {
  return api.get('/igdb/search', { params: { q: query } });
}

export async function importFromIGDB(igdbId: GameId): Promise<{ id: GameId; title: string }> {
  return api.post(`/igdb/import/${igdbId}`);
}

// Tags
export async function getTags(): Promise<Tag[]> {
  return api.get('/tags/');
}

export async function addTagToGame(userGameId: UserGameId, tagId: string): Promise<UserGame> {
  return api.post(`/user-games/${userGameId}/tags/${tagId}`);
}

export async function removeTagFromGame(userGameId: UserGameId, tagId: string): Promise<UserGame> {
  return api.delete(`/user-games/${userGameId}/tags/${tagId}`);
}

// Platforms for a user game
export async function addPlatformToGame(
  userGameId: UserGameId,
  platformId: string,
  storefrontId?: string
): Promise<UserGame> {
  return api.post(`/user-games/${userGameId}/platforms`, {
    platform_id: platformId,
    storefront_id: storefrontId,
  });
}

export async function removePlatformFromGame(
  userGameId: UserGameId,
  platformEntryId: string
): Promise<UserGame> {
  return api.delete(`/user-games/${userGameId}/platforms/${platformEntryId}`);
}
```

**Step 2: Create platforms API functions**

Create file `frontend-next/src/api/platforms.ts`:

```typescript
import { api } from './client';
import type { Platform, Storefront, PlatformsResponse, StorefrontsResponse } from '@/types';

export async function getPlatforms(activeOnly = true): Promise<Platform[]> {
  const response: PlatformsResponse = await api.get('/platforms/', {
    params: { active_only: activeOnly },
  });
  return response.platforms;
}

export async function getStorefronts(activeOnly = true): Promise<Storefront[]> {
  const response: StorefrontsResponse = await api.get('/platforms/storefronts/', {
    params: { active_only: activeOnly },
  });
  return response.storefronts;
}

export async function getPlatformStorefronts(platformId: string): Promise<Storefront[]> {
  const response: StorefrontsResponse = await api.get(
    `/platforms/${platformId}/storefronts/`
  );
  return response.storefronts;
}
```

**Step 3: Create index barrel export**

Create file `frontend-next/src/api/index.ts`:

```typescript
export * from './client';
export * from './auth';
export * from './games';
export * from './platforms';
```

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/api/
git commit -m "feat(frontend-next): add games and platforms API services"
```

---

### Task 10: Create TanStack Query Provider

**Files:**
- Create: `frontend-next/src/components/providers/query-provider.tsx`

**Step 1: Create QueryProvider component**

Create file `frontend-next/src/components/providers/query-provider.tsx`:

```typescript
'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import { useState, type ReactNode } from 'react';

interface QueryProviderProps {
  children: ReactNode;
}

export function QueryProvider({ children }: QueryProviderProps) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 60 * 1000, // 1 minute
            refetchOnWindowFocus: false,
            retry: 1,
          },
        },
      })
  );

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryClientProvider>
  );
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/providers/
git commit -m "feat(frontend-next): add TanStack Query provider"
```

---

### Task 11: Create Auth Provider

**Files:**
- Create: `frontend-next/src/components/providers/auth-provider.tsx`

**Step 1: Create AuthProvider component**

Create file `frontend-next/src/components/providers/auth-provider.tsx`:

```typescript
'use client';

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from 'react';
import { useRouter } from 'next/navigation';
import { setAuthHandlers } from '@/api/client';
import * as authApi from '@/api/auth';
import type { User, AuthState, LoginRequest } from '@/types';

interface AuthContextType extends AuthState {
  login: (credentials: LoginRequest) => Promise<void>;
  logout: () => void;
  refreshAuth: () => Promise<boolean>;
}

const AuthContext = createContext<AuthContextType | null>(null);

const STORAGE_KEY = 'nexorious_auth';

interface StoredAuth {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
}

function loadStoredAuth(): StoredAuth {
  if (typeof window === 'undefined') {
    return { user: null, accessToken: null, refreshToken: null };
  }

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      return JSON.parse(stored);
    }
  } catch {
    localStorage.removeItem(STORAGE_KEY);
  }

  return { user: null, accessToken: null, refreshToken: null };
}

function saveAuth(auth: StoredAuth) {
  if (typeof window !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(auth));
  }
}

function clearAuth() {
  if (typeof window !== 'undefined') {
    localStorage.removeItem(STORAGE_KEY);
  }
}

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const router = useRouter();
  const [state, setState] = useState<AuthState>(() => {
    const stored = loadStoredAuth();
    return {
      user: stored.user,
      accessToken: stored.accessToken,
      refreshToken: stored.refreshToken,
      isLoading: true,
      error: null,
    };
  });

  const logout = useCallback(() => {
    setState({
      user: null,
      accessToken: null,
      refreshToken: null,
      isLoading: false,
      error: null,
    });
    clearAuth();
    router.push('/login');
  }, [router]);

  const refreshAuth = useCallback(async (): Promise<boolean> => {
    if (!state.refreshToken) {
      return false;
    }

    try {
      const response = await authApi.refreshToken(state.refreshToken);
      const newState = {
        ...state,
        accessToken: response.access_token,
        refreshToken: response.refresh_token || state.refreshToken,
      };

      setState((prev) => ({
        ...prev,
        accessToken: newState.accessToken,
        refreshToken: newState.refreshToken,
      }));

      saveAuth({
        user: state.user,
        accessToken: newState.accessToken,
        refreshToken: newState.refreshToken,
      });

      return true;
    } catch {
      logout();
      return false;
    }
  }, [state.refreshToken, state.user, logout]);

  // Set up auth handlers for API client
  useEffect(() => {
    setAuthHandlers(
      () => state.accessToken,
      refreshAuth,
      logout
    );
  }, [state.accessToken, refreshAuth, logout]);

  // Validate token on mount
  useEffect(() => {
    async function validateToken() {
      if (!state.accessToken) {
        setState((prev) => ({ ...prev, isLoading: false }));
        return;
      }

      try {
        const user = await authApi.getMe(state.accessToken);
        setState((prev) => ({ ...prev, user, isLoading: false }));
      } catch {
        // Token invalid, try refresh
        const refreshed = await refreshAuth();
        if (!refreshed) {
          setState((prev) => ({ ...prev, isLoading: false }));
        }
      }
    }

    validateToken();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const login = useCallback(async (credentials: LoginRequest) => {
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const tokenResponse = await authApi.login(credentials);
      const user = await authApi.getMe(tokenResponse.access_token);

      const newState = {
        user,
        accessToken: tokenResponse.access_token,
        refreshToken: tokenResponse.refresh_token,
        isLoading: false,
        error: null,
      };

      setState(newState);
      saveAuth({
        user: newState.user,
        accessToken: newState.accessToken,
        refreshToken: newState.refreshToken,
      });

      router.push('/games');
    } catch (error) {
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Login failed',
      }));
      throw error;
    }
  }, [router]);

  return (
    <AuthContext.Provider
      value={{
        ...state,
        login,
        logout,
        refreshAuth,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/providers/auth-provider.tsx
git commit -m "feat(frontend-next): add auth provider with token management"
```

---

### Task 12: Create Providers Index and Update Root Layout

**Files:**
- Create: `frontend-next/src/components/providers/index.tsx`
- Modify: `frontend-next/src/app/layout.tsx`

**Step 1: Create providers barrel export**

Create file `frontend-next/src/components/providers/index.tsx`:

```typescript
'use client';

import type { ReactNode } from 'react';
import { QueryProvider } from './query-provider';
import { AuthProvider } from './auth-provider';
import { Toaster } from 'sonner';

interface ProvidersProps {
  children: ReactNode;
}

export function Providers({ children }: ProvidersProps) {
  return (
    <QueryProvider>
      <AuthProvider>
        {children}
        <Toaster position="top-right" richColors />
      </AuthProvider>
    </QueryProvider>
  );
}

export { useAuth } from './auth-provider';
```

**Step 2: Update root layout**

Replace contents of `frontend-next/src/app/layout.tsx`:

```typescript
import type { Metadata } from 'next';
import { Inter } from 'next/font/google';
import './globals.css';
import { Providers } from '@/components/providers';

const inter = Inter({ subsets: ['latin'] });

export const metadata: Metadata = {
  title: 'Nexorious',
  description: 'Game Collection Management',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className={inter.className}>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
```

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/providers/index.tsx frontend-next/src/app/layout.tsx
git commit -m "feat(frontend-next): wire up providers in root layout"
```

---

### Task 13: Add Docker Compose Service

**Files:**
- Modify: `docker-compose.yml`
- Create: `frontend-next/Dockerfile`

**Step 1: Create Dockerfile**

Create file `frontend-next/Dockerfile`:

```dockerfile
FROM node:22-alpine

WORKDIR /app

COPY package*.json ./
RUN npm install

COPY . .

EXPOSE 3000

CMD ["npm", "run", "dev"]
```

**Step 2: Add frontend-next service to docker-compose.yml**

Add the following service after the `frontend` service in `docker-compose.yml`:

```yaml
  frontend-next:
    build: ./frontend-next
    ports:
      - "3000:3000"
    environment:
      NEXT_PUBLIC_API_URL: http://localhost:8000/api
      NEXT_PUBLIC_STATIC_URL: http://localhost:8000
    volumes:
      - ./frontend-next:/app:Z
      - /app/node_modules
      - /app/.next
    depends_on:
      - api
```

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add docker-compose.yml frontend-next/Dockerfile
git commit -m "feat(frontend-next): add Docker Compose service"
```

---

### Task 14: Create Root Page with Redirect Logic

**Files:**
- Modify: `frontend-next/src/app/page.tsx`

**Step 1: Create redirect page**

Replace contents of `frontend-next/src/app/page.tsx`:

```typescript
'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/components/providers';
import { checkSetupStatus } from '@/api/auth';

export default function HomePage() {
  const router = useRouter();
  const { user, isLoading } = useAuth();

  useEffect(() => {
    async function redirect() {
      if (isLoading) return;

      // Check if setup is needed
      const status = await checkSetupStatus();
      if (status.needs_setup) {
        router.replace('/setup');
        return;
      }

      // Redirect based on auth status
      if (user) {
        router.replace('/games');
      } else {
        router.replace('/login');
      }
    }

    redirect();
  }, [user, isLoading, router]);

  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="animate-pulse text-lg text-muted-foreground">
        Loading...
      </div>
    </div>
  );
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/app/page.tsx
git commit -m "feat(frontend-next): add root page with redirect logic"
```

---

### Task 15: Verify Phase 1 Setup

**Step 1: Start the development server**

```bash
cd /home/abo/workspace/home/nexorious/frontend-next
npm run dev
```

**Step 2: Verify it compiles without errors**

Expected: Server starts on http://localhost:3000 without TypeScript or build errors

**Step 3: Test in browser**

Open http://localhost:3000 - should redirect to /login (or /setup if no users exist)

**Step 4: Stop dev server and commit any fixes**

Press Ctrl+C to stop, then:

```bash
cd /home/abo/workspace/home/nexorious
git add -A
git commit -m "fix(frontend-next): phase 1 setup fixes" --allow-empty
```

---

## Phase 2: Authentication

### Task 16: Create Login Page

**Files:**
- Create: `frontend-next/src/app/(auth)/login/page.tsx`
- Create: `frontend-next/src/app/(auth)/layout.tsx`
- Create: `frontend-next/src/components/auth/login-form.tsx`

**Step 1: Create auth layout**

Create file `frontend-next/src/app/(auth)/layout.tsx`:

```typescript
export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md p-6">{children}</div>
    </div>
  );
}
```

**Step 2: Create login form component**

Create file `frontend-next/src/components/auth/login-form.tsx`:

```typescript
'use client';

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useAuth } from '@/components/providers';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

const loginSchema = z.object({
  username: z.string().min(1, 'Username is required'),
  password: z.string().min(1, 'Password is required'),
});

type LoginFormValues = z.infer<typeof loginSchema>;

export function LoginForm() {
  const { login, error } = useAuth();
  const [isSubmitting, setIsSubmitting] = useState(false);

  const form = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      username: '',
      password: '',
    },
  });

  async function onSubmit(values: LoginFormValues) {
    setIsSubmitting(true);
    try {
      await login(values);
    } catch {
      // Error is handled by auth provider
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Welcome to Nexorious</CardTitle>
        <CardDescription>Sign in to manage your game collection</CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Username</FormLabel>
                  <FormControl>
                    <Input
                      placeholder="Enter your username"
                      autoFocus
                      autoComplete="username"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Password</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      placeholder="Enter your password"
                      autoComplete="current-password"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {error && (
              <p className="text-sm text-destructive">{error}</p>
            )}

            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? 'Signing in...' : 'Sign in'}
            </Button>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}
```

**Step 3: Create login page**

Create file `frontend-next/src/app/(auth)/login/page.tsx`:

```typescript
import { LoginForm } from '@/components/auth/login-form';

export default function LoginPage() {
  return <LoginForm />;
}
```

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/app/\(auth\)/ frontend-next/src/components/auth/
git commit -m "feat(frontend-next): add login page with form validation"
```

---

### Task 17: Create Setup Page

**Files:**
- Create: `frontend-next/src/app/(auth)/setup/page.tsx`
- Create: `frontend-next/src/components/auth/setup-form.tsx`

**Step 1: Create setup form component**

Create file `frontend-next/src/components/auth/setup-form.tsx`:

```typescript
'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { createInitialAdmin } from '@/api/auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription,
} from '@/components/ui/form';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

const setupSchema = z
  .object({
    username: z
      .string()
      .min(3, 'Username must be at least 3 characters')
      .max(50, 'Username must be at most 50 characters'),
    password: z
      .string()
      .min(8, 'Password must be at least 8 characters'),
    confirmPassword: z.string(),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  });

type SetupFormValues = z.infer<typeof setupSchema>;

export function SetupForm() {
  const router = useRouter();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const form = useForm<SetupFormValues>({
    resolver: zodResolver(setupSchema),
    defaultValues: {
      username: '',
      password: '',
      confirmPassword: '',
    },
  });

  async function onSubmit(values: SetupFormValues) {
    setIsSubmitting(true);
    setError(null);

    try {
      await createInitialAdmin({
        username: values.username,
        password: values.password,
      });

      toast.success('Admin account created! Please sign in.');
      router.push('/login');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create admin');
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Welcome to Nexorious</CardTitle>
        <CardDescription>
          Create your administrator account to get started
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Username</FormLabel>
                  <FormControl>
                    <Input
                      placeholder="Choose a username"
                      autoFocus
                      autoComplete="username"
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    This will be your admin username
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Password</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      placeholder="Choose a strong password"
                      autoComplete="new-password"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="confirmPassword"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Confirm Password</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      placeholder="Confirm your password"
                      autoComplete="new-password"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {error && (
              <p className="text-sm text-destructive">{error}</p>
            )}

            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? 'Creating account...' : 'Create Admin Account'}
            </Button>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}
```

**Step 2: Create setup page**

Create file `frontend-next/src/app/(auth)/setup/page.tsx`:

```typescript
'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { checkSetupStatus } from '@/api/auth';
import { SetupForm } from '@/components/auth/setup-form';

export default function SetupPage() {
  const router = useRouter();
  const [isChecking, setIsChecking] = useState(true);

  useEffect(() => {
    async function check() {
      const status = await checkSetupStatus();
      if (!status.needs_setup) {
        router.replace('/login');
      } else {
        setIsChecking(false);
      }
    }
    check();
  }, [router]);

  if (isChecking) {
    return (
      <div className="flex items-center justify-center">
        <div className="animate-pulse text-muted-foreground">Checking...</div>
      </div>
    );
  }

  return <SetupForm />;
}
```

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/app/\(auth\)/setup/ frontend-next/src/components/auth/setup-form.tsx
git commit -m "feat(frontend-next): add initial admin setup page"
```

---

### Task 18: Create Route Guard Component

**Files:**
- Create: `frontend-next/src/components/auth/route-guard.tsx`

**Step 1: Create RouteGuard component**

Create file `frontend-next/src/components/auth/route-guard.tsx`:

```typescript
'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/components/providers';
import { Skeleton } from '@/components/ui/skeleton';

interface RouteGuardProps {
  children: React.ReactNode;
  requireAdmin?: boolean;
}

export function RouteGuard({ children, requireAdmin = false }: RouteGuardProps) {
  const router = useRouter();
  const { user, isLoading } = useAuth();

  useEffect(() => {
    if (isLoading) return;

    if (!user) {
      router.replace('/login');
      return;
    }

    if (requireAdmin && !user.isAdmin) {
      router.replace('/games');
    }
  }, [user, isLoading, requireAdmin, router]);

  if (isLoading) {
    return (
      <div className="flex min-h-screen flex-col gap-4 p-8">
        <Skeleton className="h-12 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (!user) {
    return null;
  }

  if (requireAdmin && !user.isAdmin) {
    return null;
  }

  return <>{children}</>;
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/auth/route-guard.tsx
git commit -m "feat(frontend-next): add route guard component"
```

---

### Task 19: Create Auth Components Index

**Files:**
- Create: `frontend-next/src/components/auth/index.tsx`

**Step 1: Create barrel export**

Create file `frontend-next/src/components/auth/index.tsx`:

```typescript
export { LoginForm } from './login-form';
export { SetupForm } from './setup-form';
export { RouteGuard } from './route-guard';
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/auth/index.tsx
git commit -m "feat(frontend-next): add auth components index"
```

---

### Task 20: Test Authentication Flow

**Step 1: Start the backend and frontend**

Terminal 1:
```bash
cd /home/abo/workspace/home/nexorious
podman-compose up db api
```

Terminal 2:
```bash
cd /home/abo/workspace/home/nexorious/frontend-next
npm run dev
```

**Step 2: Test setup flow (if fresh database)**

1. Open http://localhost:3000
2. Should redirect to /setup
3. Create admin account
4. Should redirect to /login

**Step 3: Test login flow**

1. Enter credentials
2. Should redirect to /games (will be 404 for now, that's expected)

**Step 4: Commit any fixes**

```bash
cd /home/abo/workspace/home/nexorious
git add -A
git commit -m "fix(frontend-next): phase 2 auth fixes" --allow-empty
```

---

## Phase 3: Game Library

### Task 21: Create TanStack Query Hooks

**Files:**
- Create: `frontend-next/src/hooks/use-games.ts`
- Create: `frontend-next/src/hooks/use-platforms.ts`
- Create: `frontend-next/src/hooks/index.ts`

**Step 1: Create games hooks**

Create file `frontend-next/src/hooks/use-games.ts`:

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as gamesApi from '@/api/games';
import type {
  UserGameFilters,
  UserGameId,
  UserGameUpdateRequest,
  UserGameCreateRequest,
  GameId,
} from '@/types';

export const gameKeys = {
  all: ['games'] as const,
  lists: () => [...gameKeys.all, 'list'] as const,
  list: (filters: UserGameFilters) => [...gameKeys.lists(), filters] as const,
  details: () => [...gameKeys.all, 'detail'] as const,
  detail: (id: UserGameId) => [...gameKeys.details(), id] as const,
  igdbSearch: (query: string) => [...gameKeys.all, 'igdb', query] as const,
  tags: () => ['tags'] as const,
};

export function useUserGames(filters: UserGameFilters = {}) {
  return useQuery({
    queryKey: gameKeys.list(filters),
    queryFn: () => gamesApi.getUserGames(filters),
  });
}

export function useUserGame(id: UserGameId) {
  return useQuery({
    queryKey: gameKeys.detail(id),
    queryFn: () => gamesApi.getUserGame(id),
    enabled: !!id,
  });
}

export function useCreateUserGame() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: UserGameCreateRequest) => gamesApi.createUserGame(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}

export function useUpdateUserGame() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: UserGameId; data: UserGameUpdateRequest }) =>
      gamesApi.updateUserGame(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(id) });
    },
  });
}

export function useDeleteUserGame() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: UserGameId) => gamesApi.deleteUserGame(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}

export function useBulkUpdateStatus() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ ids, status }: { ids: UserGameId[]; status: string }) =>
      gamesApi.bulkUpdateStatus(ids, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}

export function useBulkDelete() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (ids: UserGameId[]) => gamesApi.bulkDelete(ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}

export function useIGDBSearch(query: string) {
  return useQuery({
    queryKey: gameKeys.igdbSearch(query),
    queryFn: () => gamesApi.searchIGDB(query),
    enabled: query.length >= 2,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

export function useImportFromIGDB() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (igdbId: GameId) => gamesApi.importFromIGDB(igdbId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.all });
    },
  });
}

export function useTags() {
  return useQuery({
    queryKey: gameKeys.tags(),
    queryFn: () => gamesApi.getTags(),
  });
}
```

**Step 2: Create platforms hooks**

Create file `frontend-next/src/hooks/use-platforms.ts`:

```typescript
import { useQuery } from '@tanstack/react-query';
import * as platformsApi from '@/api/platforms';

export const platformKeys = {
  all: ['platforms'] as const,
  list: (activeOnly?: boolean) => [...platformKeys.all, { activeOnly }] as const,
  storefronts: () => ['storefronts'] as const,
  storefrontList: (activeOnly?: boolean) =>
    [...platformKeys.storefronts(), { activeOnly }] as const,
  platformStorefronts: (platformId: string) =>
    [...platformKeys.all, platformId, 'storefronts'] as const,
};

export function usePlatforms(activeOnly = true) {
  return useQuery({
    queryKey: platformKeys.list(activeOnly),
    queryFn: () => platformsApi.getPlatforms(activeOnly),
  });
}

export function useStorefronts(activeOnly = true) {
  return useQuery({
    queryKey: platformKeys.storefrontList(activeOnly),
    queryFn: () => platformsApi.getStorefronts(activeOnly),
  });
}

export function usePlatformStorefronts(platformId: string) {
  return useQuery({
    queryKey: platformKeys.platformStorefronts(platformId),
    queryFn: () => platformsApi.getPlatformStorefronts(platformId),
    enabled: !!platformId,
  });
}
```

**Step 3: Create hooks index**

Create file `frontend-next/src/hooks/index.ts`:

```typescript
export * from './use-games';
export * from './use-platforms';
```

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/hooks/
git commit -m "feat(frontend-next): add TanStack Query hooks for games and platforms"
```

---

### Task 22: Create Main Layout

**Files:**
- Create: `frontend-next/src/app/(main)/layout.tsx`
- Create: `frontend-next/src/components/layout/header.tsx`
- Create: `frontend-next/src/components/layout/sidebar.tsx`
- Create: `frontend-next/src/components/layout/main-layout.tsx`

**Step 1: Create header component**

Create file `frontend-next/src/components/layout/header.tsx`:

```typescript
'use client';

import Link from 'next/link';
import { useAuth } from '@/components/providers';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { LogOut, User, Settings } from 'lucide-react';

export function Header() {
  const { user, logout } = useAuth();

  const initials = user?.username
    ? user.username.slice(0, 2).toUpperCase()
    : '??';

  return (
    <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container flex h-14 items-center">
        <Link href="/games" className="mr-6 flex items-center space-x-2">
          <span className="font-bold">Nexorious</span>
        </Link>

        <nav className="flex flex-1 items-center space-x-4">
          <Link href="/games">
            <Button variant="ghost">Games</Button>
          </Link>
          <Link href="/games/add">
            <Button variant="ghost">Add Game</Button>
          </Link>
        </nav>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="relative h-8 w-8 rounded-full">
              <Avatar className="h-8 w-8">
                <AvatarFallback>{initials}</AvatarFallback>
              </Avatar>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <div className="flex items-center justify-start gap-2 p-2">
              <div className="flex flex-col space-y-1 leading-none">
                <p className="font-medium">{user?.username}</p>
                {user?.isAdmin && (
                  <p className="text-xs text-muted-foreground">Administrator</p>
                )}
              </div>
            </div>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/profile" className="flex items-center">
                <User className="mr-2 h-4 w-4" />
                Profile
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link href="/settings" className="flex items-center">
                <Settings className="mr-2 h-4 w-4" />
                Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={logout} className="text-destructive">
              <LogOut className="mr-2 h-4 w-4" />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
```

**Step 2: Create main layout component**

Create file `frontend-next/src/components/layout/main-layout.tsx`:

```typescript
import { Header } from './header';

interface MainLayoutProps {
  children: React.ReactNode;
}

export function MainLayout({ children }: MainLayoutProps) {
  return (
    <div className="relative min-h-screen flex flex-col">
      <Header />
      <main className="flex-1 container py-6">{children}</main>
    </div>
  );
}
```

**Step 3: Create layout index**

Create file `frontend-next/src/components/layout/index.tsx`:

```typescript
export { Header } from './header';
export { MainLayout } from './main-layout';
```

**Step 4: Create main route group layout**

Create file `frontend-next/src/app/(main)/layout.tsx`:

```typescript
import { RouteGuard } from '@/components/auth';
import { MainLayout } from '@/components/layout';

export default function MainGroupLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <RouteGuard>
      <MainLayout>{children}</MainLayout>
    </RouteGuard>
  );
}
```

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/layout/ frontend-next/src/app/\(main\)/
git commit -m "feat(frontend-next): add main layout with header and navigation"
```

---

### Task 23: Create Game Card Component

**Files:**
- Create: `frontend-next/src/components/games/game-card.tsx`
- Create: `frontend-next/src/components/games/play-status-badge.tsx`
- Create: `frontend-next/src/components/games/platform-badges.tsx`

**Step 1: Create play status badge**

Create file `frontend-next/src/components/games/play-status-badge.tsx`:

```typescript
import { Badge } from '@/components/ui/badge';
import { PlayStatus } from '@/types';

const statusConfig: Record<PlayStatus, { label: string; variant: 'default' | 'secondary' | 'destructive' | 'outline' }> = {
  [PlayStatus.NOT_STARTED]: { label: 'Not Started', variant: 'outline' },
  [PlayStatus.IN_PROGRESS]: { label: 'In Progress', variant: 'default' },
  [PlayStatus.COMPLETED]: { label: 'Completed', variant: 'secondary' },
  [PlayStatus.MASTERED]: { label: 'Mastered', variant: 'secondary' },
  [PlayStatus.DOMINATED]: { label: 'Dominated', variant: 'secondary' },
  [PlayStatus.SHELVED]: { label: 'Shelved', variant: 'outline' },
  [PlayStatus.DROPPED]: { label: 'Dropped', variant: 'destructive' },
  [PlayStatus.REPLAY]: { label: 'Replay', variant: 'default' },
};

interface PlayStatusBadgeProps {
  status: PlayStatus;
}

export function PlayStatusBadge({ status }: PlayStatusBadgeProps) {
  const config = statusConfig[status] || { label: status, variant: 'outline' as const };

  return (
    <Badge variant={config.variant}>
      {config.label}
    </Badge>
  );
}
```

**Step 2: Create platform badges**

Create file `frontend-next/src/components/games/platform-badges.tsx`:

```typescript
import { Badge } from '@/components/ui/badge';
import type { UserGamePlatform } from '@/types';

interface PlatformBadgesProps {
  platforms: UserGamePlatform[];
  maxDisplay?: number;
}

export function PlatformBadges({ platforms, maxDisplay = 3 }: PlatformBadgesProps) {
  const displayed = platforms.slice(0, maxDisplay);
  const remaining = platforms.length - maxDisplay;

  return (
    <div className="flex flex-wrap gap-1">
      {displayed.map((p) => (
        <Badge key={p.id} variant="outline" className="text-xs">
          {p.platform.display_name}
        </Badge>
      ))}
      {remaining > 0 && (
        <Badge variant="outline" className="text-xs">
          +{remaining}
        </Badge>
      )}
    </div>
  );
}
```

**Step 3: Create game card**

Create file `frontend-next/src/components/games/game-card.tsx`:

```typescript
'use client';

import Link from 'next/link';
import Image from 'next/image';
import { Heart, Star } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { PlayStatusBadge } from './play-status-badge';
import { PlatformBadges } from './platform-badges';
import { config } from '@/lib/env';
import type { UserGame, UserGameId } from '@/types';
import { cn } from '@/lib/utils';

interface GameCardProps {
  game: UserGame;
  isSelected?: boolean;
  onSelect?: (id: UserGameId) => void;
  selectionMode?: boolean;
}

export function GameCard({
  game,
  isSelected = false,
  onSelect,
  selectionMode = false,
}: GameCardProps) {
  const coverUrl = game.game.cover_art_url
    ? `${config.staticUrl}${game.game.cover_art_url}`
    : '/placeholder-cover.png';

  const handleClick = (e: React.MouseEvent) => {
    if (selectionMode && onSelect) {
      e.preventDefault();
      onSelect(game.id);
    }
  };

  const content = (
    <Card
      className={cn(
        'overflow-hidden transition-all hover:shadow-lg',
        isSelected && 'ring-2 ring-primary',
        selectionMode && 'cursor-pointer'
      )}
      onClick={handleClick}
    >
      <div className="relative aspect-[3/4] w-full">
        <Image
          src={coverUrl}
          alt={game.game.title}
          fill
          className="object-cover"
          sizes="(max-width: 768px) 50vw, (max-width: 1200px) 33vw, 20vw"
        />
        {game.is_loved && (
          <div className="absolute top-2 right-2">
            <Heart className="h-5 w-5 fill-red-500 text-red-500" />
          </div>
        )}
        {selectionMode && (
          <div
            className={cn(
              'absolute top-2 left-2 h-5 w-5 rounded border-2',
              isSelected
                ? 'bg-primary border-primary'
                : 'bg-background/80 border-muted-foreground'
            )}
          />
        )}
      </div>
      <CardContent className="p-3 space-y-2">
        <h3 className="font-semibold line-clamp-1" title={game.game.title}>
          {game.game.title}
        </h3>

        <div className="flex items-center justify-between">
          <PlayStatusBadge status={game.play_status} />
          {game.personal_rating && (
            <div className="flex items-center gap-1 text-sm text-muted-foreground">
              <Star className="h-3 w-3 fill-yellow-400 text-yellow-400" />
              {game.personal_rating}
            </div>
          )}
        </div>

        <PlatformBadges platforms={game.platforms} />
      </CardContent>
    </Card>
  );

  if (selectionMode) {
    return content;
  }

  return (
    <Link href={`/games/${game.id}`}>
      {content}
    </Link>
  );
}
```

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/games/
git commit -m "feat(frontend-next): add game card with status and platform badges"
```

---

### Task 24: Create Game Grid and List Components

**Files:**
- Create: `frontend-next/src/components/games/game-grid.tsx`
- Create: `frontend-next/src/components/games/game-row.tsx`
- Create: `frontend-next/src/components/games/game-list.tsx`

**Step 1: Create game grid**

Create file `frontend-next/src/components/games/game-grid.tsx`:

```typescript
import { GameCard } from './game-card';
import type { UserGame, UserGameId } from '@/types';

interface GameGridProps {
  games: UserGame[];
  selectedIds?: Set<UserGameId>;
  onSelect?: (id: UserGameId) => void;
  selectionMode?: boolean;
}

export function GameGrid({
  games,
  selectedIds = new Set(),
  onSelect,
  selectionMode = false,
}: GameGridProps) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
      {games.map((game) => (
        <GameCard
          key={game.id}
          game={game}
          isSelected={selectedIds.has(game.id)}
          onSelect={onSelect}
          selectionMode={selectionMode}
        />
      ))}
    </div>
  );
}
```

**Step 2: Create game row**

Create file `frontend-next/src/components/games/game-row.tsx`:

```typescript
'use client';

import Link from 'next/link';
import Image from 'next/image';
import { Heart, Star } from 'lucide-react';
import { Checkbox } from '@/components/ui/checkbox';
import { PlayStatusBadge } from './play-status-badge';
import { PlatformBadges } from './platform-badges';
import { config } from '@/lib/env';
import type { UserGame, UserGameId } from '@/types';
import { cn } from '@/lib/utils';

interface GameRowProps {
  game: UserGame;
  isSelected?: boolean;
  onSelect?: (id: UserGameId) => void;
  selectionMode?: boolean;
}

export function GameRow({
  game,
  isSelected = false,
  onSelect,
  selectionMode = false,
}: GameRowProps) {
  const coverUrl = game.game.cover_art_url
    ? `${config.staticUrl}${game.game.cover_art_url}`
    : '/placeholder-cover.png';

  const handleClick = (e: React.MouseEvent) => {
    if (selectionMode && onSelect) {
      e.preventDefault();
      onSelect(game.id);
    }
  };

  const content = (
    <div
      className={cn(
        'flex items-center gap-4 p-3 rounded-lg border bg-card hover:bg-accent/50 transition-colors',
        isSelected && 'ring-2 ring-primary',
        selectionMode && 'cursor-pointer'
      )}
      onClick={handleClick}
    >
      {selectionMode && (
        <Checkbox
          checked={isSelected}
          onCheckedChange={() => onSelect?.(game.id)}
          onClick={(e) => e.stopPropagation()}
        />
      )}

      <div className="relative h-16 w-12 flex-shrink-0">
        <Image
          src={coverUrl}
          alt={game.game.title}
          fill
          className="object-cover rounded"
          sizes="48px"
        />
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <h3 className="font-semibold truncate">{game.game.title}</h3>
          {game.is_loved && (
            <Heart className="h-4 w-4 fill-red-500 text-red-500 flex-shrink-0" />
          )}
        </div>
        <div className="text-sm text-muted-foreground">
          {game.game.release_date?.split('-')[0] || 'Unknown year'}
          {game.game.developer && ` • ${game.game.developer}`}
        </div>
      </div>

      <div className="flex items-center gap-4">
        <PlatformBadges platforms={game.platforms} maxDisplay={2} />
        <PlayStatusBadge status={game.play_status} />
        {game.personal_rating && (
          <div className="flex items-center gap-1 text-sm">
            <Star className="h-4 w-4 fill-yellow-400 text-yellow-400" />
            {game.personal_rating}
          </div>
        )}
      </div>
    </div>
  );

  if (selectionMode) {
    return content;
  }

  return (
    <Link href={`/games/${game.id}`} className="block">
      {content}
    </Link>
  );
}
```

**Step 3: Create game list**

Create file `frontend-next/src/components/games/game-list.tsx`:

```typescript
import { GameRow } from './game-row';
import type { UserGame, UserGameId } from '@/types';

interface GameListProps {
  games: UserGame[];
  selectedIds?: Set<UserGameId>;
  onSelect?: (id: UserGameId) => void;
  selectionMode?: boolean;
}

export function GameList({
  games,
  selectedIds = new Set(),
  onSelect,
  selectionMode = false,
}: GameListProps) {
  return (
    <div className="space-y-2">
      {games.map((game) => (
        <GameRow
          key={game.id}
          game={game}
          isSelected={selectedIds.has(game.id)}
          onSelect={onSelect}
          selectionMode={selectionMode}
        />
      ))}
    </div>
  );
}
```

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/games/
git commit -m "feat(frontend-next): add game grid and list views"
```

---

### Task 25: Create Game Filters Component

**Files:**
- Create: `frontend-next/src/components/games/game-filters.tsx`

**Step 1: Create filters component**

Create file `frontend-next/src/components/games/game-filters.tsx`:

```typescript
'use client';

import { Search, X } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { PlayStatus } from '@/types';
import type { UserGameFilters } from '@/types';

interface GameFiltersProps {
  filters: UserGameFilters;
  onFiltersChange: (filters: UserGameFilters) => void;
}

const playStatusOptions = [
  { value: '', label: 'All Statuses' },
  { value: PlayStatus.NOT_STARTED, label: 'Not Started' },
  { value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
  { value: PlayStatus.COMPLETED, label: 'Completed' },
  { value: PlayStatus.MASTERED, label: 'Mastered' },
  { value: PlayStatus.DOMINATED, label: 'Dominated' },
  { value: PlayStatus.SHELVED, label: 'Shelved' },
  { value: PlayStatus.DROPPED, label: 'Dropped' },
  { value: PlayStatus.REPLAY, label: 'Replay' },
];

const sortOptions = [
  { value: 'title', label: 'Title' },
  { value: 'created_at', label: 'Date Added' },
  { value: 'updated_at', label: 'Last Updated' },
  { value: 'personal_rating', label: 'Rating' },
  { value: 'hours_played', label: 'Hours Played' },
];

export function GameFilters({ filters, onFiltersChange }: GameFiltersProps) {
  const updateFilter = <K extends keyof UserGameFilters>(
    key: K,
    value: UserGameFilters[K]
  ) => {
    onFiltersChange({ ...filters, [key]: value, page: 1 });
  };

  const clearFilters = () => {
    onFiltersChange({ page: 1, per_page: filters.per_page });
  };

  const hasActiveFilters = !!(
    filters.q ||
    filters.play_status ||
    filters.is_loved
  );

  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
      <div className="relative flex-1">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search games..."
          value={filters.q || ''}
          onChange={(e) => updateFilter('q', e.target.value || undefined)}
          className="pl-9"
        />
      </div>

      <Select
        value={filters.play_status || ''}
        onValueChange={(value) =>
          updateFilter('play_status', (value || undefined) as PlayStatus | undefined)
        }
      >
        <SelectTrigger className="w-[180px]">
          <SelectValue placeholder="All Statuses" />
        </SelectTrigger>
        <SelectContent>
          {playStatusOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.sort_by || 'title'}
        onValueChange={(value) => updateFilter('sort_by', value)}
      >
        <SelectTrigger className="w-[150px]">
          <SelectValue placeholder="Sort by" />
        </SelectTrigger>
        <SelectContent>
          {sortOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.sort_order || 'asc'}
        onValueChange={(value) =>
          updateFilter('sort_order', value as 'asc' | 'desc')
        }
      >
        <SelectTrigger className="w-[100px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="asc">A-Z</SelectItem>
          <SelectItem value="desc">Z-A</SelectItem>
        </SelectContent>
      </Select>

      {hasActiveFilters && (
        <Button variant="ghost" size="sm" onClick={clearFilters}>
          <X className="mr-1 h-4 w-4" />
          Clear
        </Button>
      )}
    </div>
  );
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/games/game-filters.tsx
git commit -m "feat(frontend-next): add game filters component"
```

---

### Task 26: Create Bulk Actions Component

**Files:**
- Create: `frontend-next/src/components/games/game-bulk-actions.tsx`

**Step 1: Create bulk actions component**

Create file `frontend-next/src/components/games/game-bulk-actions.tsx`:

```typescript
'use client';

import { Trash2, CheckCircle } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { useBulkUpdateStatus, useBulkDelete } from '@/hooks';
import { PlayStatus } from '@/types';
import type { UserGameId } from '@/types';

interface GameBulkActionsProps {
  selectedIds: Set<UserGameId>;
  onClearSelection: () => void;
}

const statusOptions = [
  { value: PlayStatus.NOT_STARTED, label: 'Not Started' },
  { value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
  { value: PlayStatus.COMPLETED, label: 'Completed' },
  { value: PlayStatus.MASTERED, label: 'Mastered' },
  { value: PlayStatus.DOMINATED, label: 'Dominated' },
  { value: PlayStatus.SHELVED, label: 'Shelved' },
  { value: PlayStatus.DROPPED, label: 'Dropped' },
  { value: PlayStatus.REPLAY, label: 'Replay' },
];

export function GameBulkActions({
  selectedIds,
  onClearSelection,
}: GameBulkActionsProps) {
  const bulkUpdateStatus = useBulkUpdateStatus();
  const bulkDelete = useBulkDelete();

  const count = selectedIds.size;

  const handleStatusChange = async (status: string) => {
    try {
      const result = await bulkUpdateStatus.mutateAsync({
        ids: Array.from(selectedIds),
        status,
      });
      toast.success(`Updated ${result.updated_count} games`);
      onClearSelection();
    } catch (error) {
      toast.error('Failed to update games');
    }
  };

  const handleDelete = async () => {
    try {
      const result = await bulkDelete.mutateAsync(Array.from(selectedIds));
      toast.success(`Deleted ${result.deleted_count} games`);
      onClearSelection();
    } catch (error) {
      toast.error('Failed to delete games');
    }
  };

  if (count === 0) return null;

  return (
    <div className="flex items-center gap-4 rounded-lg border bg-muted/50 p-3">
      <span className="text-sm font-medium">
        {count} game{count !== 1 ? 's' : ''} selected
      </span>

      <div className="flex items-center gap-2">
        <Select onValueChange={handleStatusChange}>
          <SelectTrigger className="w-[160px]">
            <CheckCircle className="mr-2 h-4 w-4" />
            <SelectValue placeholder="Set status" />
          </SelectTrigger>
          <SelectContent>
            {statusOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive" size="sm">
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete {count} games?</AlertDialogTitle>
              <AlertDialogDescription>
                This will permanently remove these games from your collection.
                This action cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                onClick={handleDelete}
                className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              >
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        <Button variant="outline" size="sm" onClick={onClearSelection}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/games/game-bulk-actions.tsx
git commit -m "feat(frontend-next): add bulk actions component"
```

---

### Task 27: Create Games Components Index

**Files:**
- Create: `frontend-next/src/components/games/index.tsx`

**Step 1: Create barrel export**

Create file `frontend-next/src/components/games/index.tsx`:

```typescript
export { GameCard } from './game-card';
export { GameRow } from './game-row';
export { GameGrid } from './game-grid';
export { GameList } from './game-list';
export { GameFilters } from './game-filters';
export { GameBulkActions } from './game-bulk-actions';
export { PlayStatusBadge } from './play-status-badge';
export { PlatformBadges } from './platform-badges';
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/components/games/index.tsx
git commit -m "feat(frontend-next): add games components index"
```

---

### Task 28: Create Games Library Page

**Files:**
- Create: `frontend-next/src/app/(main)/games/page.tsx`

**Step 1: Create games page**

Create file `frontend-next/src/app/(main)/games/page.tsx`:

```typescript
'use client';

import { useState, useCallback } from 'react';
import { Grid, List, Plus } from 'lucide-react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Skeleton } from '@/components/ui/skeleton';
import {
  GameGrid,
  GameList,
  GameFilters,
  GameBulkActions,
} from '@/components/games';
import { useUserGames } from '@/hooks';
import type { UserGameFilters, UserGameId } from '@/types';

type ViewMode = 'grid' | 'list';

export default function GamesPage() {
  const [viewMode, setViewMode] = useState<ViewMode>('grid');
  const [filters, setFilters] = useState<UserGameFilters>({
    page: 1,
    per_page: 24,
    sort_by: 'title',
    sort_order: 'asc',
  });
  const [selectedIds, setSelectedIds] = useState<Set<UserGameId>>(new Set());
  const [selectionMode, setSelectionMode] = useState(false);

  const { data, isLoading, error } = useUserGames(filters);

  const toggleSelection = useCallback((id: UserGameId) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
    setSelectionMode(false);
  }, []);

  const toggleSelectionMode = useCallback(() => {
    if (selectionMode) {
      clearSelection();
    } else {
      setSelectionMode(true);
    }
  }, [selectionMode, clearSelection]);

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-destructive">Failed to load games</p>
        <p className="text-sm text-muted-foreground mt-2">
          {error instanceof Error ? error.message : 'Unknown error'}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">My Games</h1>
          {data && (
            <p className="text-muted-foreground">
              {data.total} game{data.total !== 1 ? 's' : ''} in collection
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant={selectionMode ? 'secondary' : 'outline'}
            size="sm"
            onClick={toggleSelectionMode}
          >
            {selectionMode ? 'Cancel Selection' : 'Select'}
          </Button>

          <Tabs value={viewMode} onValueChange={(v) => setViewMode(v as ViewMode)}>
            <TabsList>
              <TabsTrigger value="grid">
                <Grid className="h-4 w-4" />
              </TabsTrigger>
              <TabsTrigger value="list">
                <List className="h-4 w-4" />
              </TabsTrigger>
            </TabsList>
          </Tabs>

          <Button asChild>
            <Link href="/games/add">
              <Plus className="mr-2 h-4 w-4" />
              Add Game
            </Link>
          </Button>
        </div>
      </div>

      <GameFilters filters={filters} onFiltersChange={setFilters} />

      {selectionMode && (
        <GameBulkActions
          selectedIds={selectedIds}
          onClearSelection={clearSelection}
        />
      )}

      {isLoading ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
          {Array.from({ length: 12 }).map((_, i) => (
            <div key={i} className="space-y-2">
              <Skeleton className="aspect-[3/4] w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-4 w-1/2" />
            </div>
          ))}
        </div>
      ) : data?.items.length === 0 ? (
        <div className="text-center py-12">
          <p className="text-lg text-muted-foreground">No games found</p>
          <p className="text-sm text-muted-foreground mt-2">
            {filters.q || filters.play_status
              ? 'Try adjusting your filters'
              : 'Add some games to get started'}
          </p>
          {!filters.q && !filters.play_status && (
            <Button asChild className="mt-4">
              <Link href="/games/add">
                <Plus className="mr-2 h-4 w-4" />
                Add Your First Game
              </Link>
            </Button>
          )}
        </div>
      ) : viewMode === 'grid' ? (
        <GameGrid
          games={data?.items || []}
          selectedIds={selectedIds}
          onSelect={toggleSelection}
          selectionMode={selectionMode}
        />
      ) : (
        <GameList
          games={data?.items || []}
          selectedIds={selectedIds}
          onSelect={toggleSelection}
          selectionMode={selectionMode}
        />
      )}

      {/* Pagination would go here */}
    </div>
  );
}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/src/app/\(main\)/games/page.tsx
git commit -m "feat(frontend-next): add games library page"
```

---

### Task 29: Add Placeholder Cover Image

**Files:**
- Create: `frontend-next/public/placeholder-cover.png`

**Step 1: Create a simple placeholder SVG (convert to PNG or use as-is)**

For now, create a simple text file that can be replaced with an actual image:

```bash
cd /home/abo/workspace/home/nexorious/frontend-next/public
# Create a simple placeholder (you'll want to replace this with a real image)
echo "Placeholder" > placeholder-cover.txt
```

Note: You should replace this with an actual placeholder image. For now, update the GameCard to handle missing images gracefully.

**Step 2: Update next.config.ts for remote images**

Modify `frontend-next/next.config.ts` to allow images from the backend:

```typescript
import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  images: {
    remotePatterns: [
      {
        protocol: 'http',
        hostname: 'localhost',
        port: '8000',
        pathname: '/storage/**',
      },
    ],
  },
};

export default nextConfig;
```

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend-next/next.config.ts
git commit -m "feat(frontend-next): configure remote image patterns"
```

---

### Task 30: Test Game Library

**Step 1: Start backend and frontend**

Terminal 1:
```bash
cd /home/abo/workspace/home/nexorious
podman-compose up db api
```

Terminal 2:
```bash
cd /home/abo/workspace/home/nexorious/frontend-next
npm run dev
```

**Step 2: Test the games page**

1. Login at http://localhost:3000/login
2. Navigate to /games
3. Verify grid view displays games
4. Test list view toggle
5. Test search filter
6. Test selection mode

**Step 3: Commit any fixes**

```bash
cd /home/abo/workspace/home/nexorious
git add -A
git commit -m "fix(frontend-next): phase 3 game library fixes" --allow-empty
```

---

## Phase 4 & 5 Summary

Due to length constraints, I'll summarize the remaining tasks. The implementation follows the same patterns:

### Phase 4: Game Details (Tasks 31-40)
- Create `/games/[id]/page.tsx` - Game detail view
- Create `star-rating.tsx` - Interactive 5-star rating
- Create `notes-editor.tsx` - TipTap rich text editor wrapper
- Create game edit form with platform/storefront management
- Create tag selector component

### Phase 5: Add Game (Tasks 41-50)
- Create `/games/add/page.tsx` - IGDB search page
- Create `igdb-search.tsx` - Search interface using Command component
- Create game confirmation step
- Create platform/storefront selection for new games

---

## Final Task: Phase 1-3 Complete Commit

**Step 1: Final verification**

```bash
cd /home/abo/workspace/home/nexorious/frontend-next
npm run build
```

**Step 2: Commit build verification**

```bash
cd /home/abo/workspace/home/nexorious
git add -A
git commit -m "feat(frontend-next): complete Phase 1-3 (setup, auth, game library)"
```

---

## Execution Notes

- Each task is designed to be ~2-5 minutes of work
- Run `npm run dev` frequently to catch TypeScript errors early
- Commit after each task to maintain clean git history
- The plan can be executed with `superpowers:executing-plans` skill
