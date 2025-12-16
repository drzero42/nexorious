# Next.js Frontend Rewrite Design

## Overview & Goals

**Purpose**: Rewrite the Nexorious frontend from SvelteKit to Next.js to improve AI-assisted code generation quality and UI consistency through shadcn components.

**Approach**: Incremental migration - both frontends run in parallel, features migrated one by one.

**MVP Scope**:
- Authentication (login, setup, token refresh)
- Game Library (grid/list views, search, filters, bulk operations)
- Game Details (view/edit, ratings, notes, tags)

**Out of MVP Scope** (migrate later):
- Import/Sync (Steam, Darkadia, review queue)
- Dashboard (statistics)
- Admin (user management, platform management)
- Wishlist
- Jobs page

**Tech Stack**:
- Next.js 15 (App Router)
- React 19
- TypeScript
- Tailwind CSS v4
- shadcn/ui (Default style, Zinc color)
- TanStack Query v5
- TipTap (rich text editor)

---

## Project Structure

```
frontend-next/
├── src/
│   ├── app/                    # Next.js App Router pages
│   │   ├── (auth)/             # Auth route group (no layout chrome)
│   │   │   ├── login/
│   │   │   └── setup/
│   │   ├── (main)/             # Main app route group (with nav/sidebar)
│   │   │   ├── games/
│   │   │   │   ├── page.tsx        # Game library
│   │   │   │   ├── add/
│   │   │   │   └── [id]/
│   │   │   └── layout.tsx
│   │   ├── layout.tsx          # Root layout
│   │   └── page.tsx            # Redirect to /games or /login
│   │
│   ├── components/
│   │   ├── ui/                 # shadcn components (auto-generated)
│   │   ├── games/              # Game-specific components
│   │   ├── auth/               # Auth components
│   │   └── layout/             # Nav, sidebar, etc.
│   │
│   ├── hooks/                  # TanStack Query hooks
│   │   ├── use-games.ts
│   │   ├── use-auth.ts
│   │   └── use-platforms.ts
│   │
│   ├── api/                    # API service layer
│   │   ├── client.ts           # Axios/fetch wrapper with auth
│   │   ├── games.ts
│   │   ├── auth.ts
│   │   └── platforms.ts
│   │
│   ├── lib/                    # Utilities
│   │   ├── utils.ts            # shadcn cn() helper
│   │   └── constants.ts
│   │
│   └── types/                  # TypeScript types
│       ├── game.ts
│       ├── auth.ts
│       └── platform.ts
│
├── public/                     # Static assets
├── tailwind.config.ts
├── next.config.ts
└── package.json
```

**Key decisions**:
- Route groups `(auth)` and `(main)` separate layouts cleanly
- `api/` layer mirrors current SvelteKit service pattern
- `hooks/` stays thin - just TanStack Query wrappers
- Types can be largely copied from existing `frontend/src/lib/types/`

---

## Authentication Flow

**Token Storage**: In-memory (React state/context) with localStorage backup for persistence across page refreshes.

**Components**:
```
src/
├── api/
│   └── client.ts         # Fetch wrapper with interceptors
├── hooks/
│   └── use-auth.ts       # Auth mutations (login, logout, refresh)
├── components/
│   └── auth/
│       ├── auth-provider.tsx    # Context provider, handles token state
│       └── route-guard.tsx      # Protects authenticated routes
```

**Flow**:
1. `AuthProvider` wraps the app, initializes from localStorage on mount
2. `client.ts` attaches `Authorization: Bearer <token>` to all requests
3. On 401 response, client attempts token refresh automatically
4. If refresh fails, redirect to `/login`
5. `RouteGuard` component checks auth state before rendering protected pages

**API endpoints used**:
- `POST /api/auth/login` - Returns access + refresh tokens
- `POST /api/auth/refresh` - Exchanges refresh token for new access token
- `GET /api/auth/me` - Validates token, returns user info
- `GET /api/setup/status` - Checks if initial setup is needed

**Token refresh strategy**:
- Intercept 401 responses in `client.ts`
- Queue failed requests while refresh is in progress
- Retry queued requests after successful refresh
- Same deduplication logic as current SvelteKit implementation

---

## Data Fetching with TanStack Query

**API Client** (`src/api/client.ts`):
```typescript
// Thin wrapper around fetch
// - Adds auth header from token store
// - Handles 401 → refresh → retry
// - Throws typed errors
```

**Service Layer** (`src/api/games.ts`):
```typescript
// Plain async functions - no React
export const getGames = (filters: GameFilters) => client.get<PaginatedResponse<UserGame>>('/user-games', { params: filters });
export const getGame = (id: string) => client.get<UserGame>(`/user-games/${id}`);
export const updateGame = (id: string, data: UpdateGameData) => client.patch<UserGame>(`/user-games/${id}`, data);
export const deleteGame = (id: string) => client.delete(`/user-games/${id}`);
export const searchIGDB = (query: string) => client.get<IGDBGame[]>('/igdb/search', { params: { query } });
```

**Hooks Layer** (`src/hooks/use-games.ts`):
```typescript
// Thin hooks that wire services to TanStack Query
export const useGames = (filters: GameFilters) =>
  useQuery({ queryKey: ['games', filters], queryFn: () => getGames(filters) });

export const useGame = (id: string) =>
  useQuery({ queryKey: ['games', id], queryFn: () => getGame(id) });

export const useUpdateGame = () =>
  useMutation({
    mutationFn: ({ id, data }) => updateGame(id, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['games'] })
  });
```

**Query Key Convention**:
- `['games']` - Game list
- `['games', id]` - Single game
- `['games', { filters }]` - Filtered list
- `['platforms']` - Platform list
- `['auth', 'user']` - Current user

**Optimistic Updates**: For quick operations like rating changes, update cache immediately and rollback on error.

---

## Component Strategy with shadcn

**shadcn components to install** (MVP):
- `button`, `input`, `label` - Forms
- `card` - Game cards
- `dialog`, `alert-dialog` - Modals, confirmations
- `dropdown-menu` - Actions, bulk operations
- `select`, `checkbox`, `radio-group` - Filters, forms
- `badge` - Platform/status indicators
- `avatar` - User, game covers
- `skeleton` - Loading states
- `toast`, `sonner` - Notifications
- `tabs` - View switching
- `pagination` - Game list pagination
- `form` - Form validation (uses react-hook-form)
- `command` - Search/combobox (IGDB search)
- `popover` - Dropdowns, color pickers
- `separator`, `scroll-area` - Layout utilities

**Custom components** (`src/components/`):

```
components/
├── games/
│   ├── game-card.tsx           # Grid view card
│   ├── game-row.tsx            # List view row
│   ├── game-grid.tsx           # Grid container
│   ├── game-list.tsx           # List container
│   ├── game-filters.tsx        # Search + filter bar
│   ├── game-bulk-actions.tsx   # Bulk operation toolbar
│   ├── platform-badges.tsx     # Platform ownership indicators
│   ├── play-status-badge.tsx   # Status indicator
│   ├── star-rating.tsx         # 5-star rating input
│   ├── igdb-search.tsx         # IGDB game search (uses Command)
│   └── notes-editor.tsx        # TipTap wrapper
│
├── auth/
│   ├── login-form.tsx
│   ├── setup-form.tsx
│   ├── auth-provider.tsx
│   └── route-guard.tsx
│
└── layout/
    ├── header.tsx
    ├── sidebar.tsx
    └── main-layout.tsx
```

**Form handling**: react-hook-form + zod (comes with shadcn's form component)

---

## Docker Integration

**New service in `docker-compose.yml`**:

```yaml
frontend-next:
  build:
    context: ./frontend-next
    dockerfile: Dockerfile
  ports:
    - "3000:3000"
  environment:
    - NEXT_PUBLIC_API_URL=http://api:8000/api
  depends_on:
    - api
  develop:
    watch:
      - action: sync
        path: ./frontend-next/src
        target: /app/src
```

**Dockerfile** (`frontend-next/Dockerfile`):
```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "run", "dev"]
```

**Port mapping**:
- Backend API: `8000`
- SvelteKit frontend: `5173`
- Next.js frontend: `3000`

**Environment variables**:
- `NEXT_PUBLIC_API_URL` - Backend API base URL (client-side)

Both frontends talk to the same backend, allowing side-by-side testing during migration.

---

## MVP Implementation Plan

**Phase 1: Project Setup**
- Initialize Next.js 15 with TypeScript, Tailwind, App Router
- Install and configure shadcn (Default/Zinc)
- Set up TanStack Query provider
- Create API client with auth interceptors
- Add `frontend-next` service to docker-compose.yml

**Phase 2: Authentication**
- Create AuthProvider context
- Build login page with shadcn form
- Build setup page (first-run admin creation)
- Implement RouteGuard component
- Token refresh logic in API client

**Phase 3: Game Library**
- Game list page with grid/list toggle
- GameCard and GameRow components
- Search and filter bar
- Pagination
- Bulk selection and actions (status update, delete)
- Platform badges component

**Phase 4: Game Details**
- Game detail page (`/games/[id]`)
- Edit form (status, rating, platforms, storefronts)
- Star rating component
- Notes editor with TipTap
- Tag management

**Phase 5: Add Game**
- IGDB search page (`/games/add`)
- Search results with Command component
- Game confirmation step
- Platform/storefront selection

---

## Migration Strategy

Once MVP is complete and validated:

1. **Test thoroughly** - Ensure feature parity with SvelteKit for MVP scope
2. **Gather feedback** - Use both frontends, note any issues
3. **Migrate remaining features** - Import/Sync, Dashboard, Admin, Wishlist, Jobs
4. **Deprecate SvelteKit** - Remove old frontend once migration complete
5. **Update docker-compose** - Rename `frontend-next` to `frontend`

---

## Type Migration

Most types can be copied directly from `frontend/src/lib/types/`:
- `game.ts` → `src/types/game.ts`
- `platform.ts` (extract from stores) → `src/types/platform.ts`
- `auth.ts` (extract from stores) → `src/types/auth.ts`

Minor adjustments may be needed for React/TanStack Query patterns.
