# Game Edit Form Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a game edit form at `/games/[id]/edit` that allows users to modify all editable properties of a game including platform/storefront associations and tags.

**Architecture:** The edit form will be a client component that fetches the game data, displays editable fields using existing UI components (StarRating, NotesEditor, PlatformSelector, TagSelector), and saves changes via mutations. Platform and tag changes are handled separately from basic game updates since they use different API endpoints.

**Tech Stack:** Next.js 14 (App Router), React Query, TypeScript, Radix UI, TipTap, MSW for testing

---

## Task 1: Add Platform Management API Functions

**Files:**
- Modify: `frontend-next/src/api/games.ts`

**Step 1: Add the UserGamePlatformData type**

Add after line 180 (after `UserGameUpdateData`):

```typescript
export interface UserGamePlatformData {
  platformId: string;
  storefrontId?: string;
  storeGameId?: string;
  storeUrl?: string;
  isAvailable?: boolean;
}
```

**Step 2: Add addPlatformToUserGame function**

Add at the end of the file (before the closing):

```typescript
/**
 * Add a platform association to a user game.
 */
export async function addPlatformToUserGame(
  userGameId: string,
  data: UserGamePlatformData
): Promise<UserGamePlatform> {
  const requestBody = {
    platform_id: data.platformId,
    storefront_id: data.storefrontId,
    store_game_id: data.storeGameId,
    store_url: data.storeUrl,
    is_available: data.isAvailable ?? true,
  };

  const response = await api.post<UserGamePlatformApiResponse>(
    `/user-games/${userGameId}/platforms`,
    requestBody
  );
  return transformUserGamePlatform(response);
}
```

**Step 3: Add updatePlatformAssociation function**

Add after addPlatformToUserGame:

```typescript
/**
 * Update a platform association on a user game.
 */
export async function updatePlatformAssociation(
  userGameId: string,
  platformAssociationId: string,
  data: UserGamePlatformData
): Promise<UserGamePlatform> {
  const requestBody = {
    platform_id: data.platformId,
    storefront_id: data.storefrontId,
    store_game_id: data.storeGameId,
    store_url: data.storeUrl,
    is_available: data.isAvailable ?? true,
  };

  const response = await api.put<UserGamePlatformApiResponse>(
    `/user-games/${userGameId}/platforms/${platformAssociationId}`,
    requestBody
  );
  return transformUserGamePlatform(response);
}
```

**Step 4: Add removePlatformFromUserGame function**

Add after updatePlatformAssociation:

```typescript
/**
 * Remove a platform association from a user game.
 */
export async function removePlatformFromUserGame(
  userGameId: string,
  platformAssociationId: string
): Promise<void> {
  await api.delete(`/user-games/${userGameId}/platforms/${platformAssociationId}`);
}
```

**Step 5: Verify TypeScript compiles**

Run: `cd frontend-next && npm run check`
Expected: No errors related to games.ts

**Step 6: Commit**

```bash
git add frontend-next/src/api/games.ts
git commit -m "feat(frontend-next): add platform management API functions"
```

---

## Task 2: Add Platform Management Hooks

**Files:**
- Modify: `frontend-next/src/hooks/use-games.ts`

**Step 1: Add useAddPlatformToUserGame hook**

Add at the end of the file:

```typescript
/**
 * Hook to add a platform to a user game.
 */
export function useAddPlatformToUserGame() {
  const queryClient = useQueryClient();

  return useMutation<
    UserGamePlatform,
    Error,
    { userGameId: string; data: gamesApi.UserGamePlatformData }
  >({
    mutationFn: ({ userGameId, data }) =>
      gamesApi.addPlatformToUserGame(userGameId, data),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}
```

**Step 2: Add useUpdatePlatformAssociation hook**

Add after useAddPlatformToUserGame:

```typescript
/**
 * Hook to update a platform association.
 */
export function useUpdatePlatformAssociation() {
  const queryClient = useQueryClient();

  return useMutation<
    UserGamePlatform,
    Error,
    {
      userGameId: string;
      platformAssociationId: string;
      data: gamesApi.UserGamePlatformData;
    }
  >({
    mutationFn: ({ userGameId, platformAssociationId, data }) =>
      gamesApi.updatePlatformAssociation(userGameId, platformAssociationId, data),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
    },
  });
}
```

**Step 3: Add useRemovePlatformFromUserGame hook**

Add after useUpdatePlatformAssociation:

```typescript
/**
 * Hook to remove a platform from a user game.
 */
export function useRemovePlatformFromUserGame() {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    Error,
    { userGameId: string; platformAssociationId: string }
  >({
    mutationFn: ({ userGameId, platformAssociationId }) =>
      gamesApi.removePlatformFromUserGame(userGameId, platformAssociationId),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}
```

**Step 4: Add import for UserGamePlatform type**

Update the imports at the top of use-games.ts to include UserGamePlatform:

```typescript
import type { UserGame, IGDBGameCandidate, Game, GameId, UserGamePlatform } from '@/types';
```

**Step 5: Verify TypeScript compiles**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 6: Commit**

```bash
git add frontend-next/src/hooks/use-games.ts
git commit -m "feat(frontend-next): add platform management hooks"
```

---

## Task 3: Create Tags Hooks File

**Files:**
- Create: `frontend-next/src/hooks/use-tags.ts`

**Step 1: Create the use-tags.ts file**

Create file `frontend-next/src/hooks/use-tags.ts` with this content:

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as tagsApi from '@/api/tags';
import type { GetTagsParams, TagsListResponse, TagCreateData, TagUpdateData } from '@/api/tags';
import type { Tag } from '@/types';
import { gameKeys } from './use-games';

// ============================================================================
// Query Keys
// ============================================================================

export const tagKeys = {
  all: ['tags'] as const,
  lists: () => [...tagKeys.all, 'list'] as const,
  list: (params?: GetTagsParams) => [...tagKeys.lists(), params] as const,
  details: () => [...tagKeys.all, 'detail'] as const,
  detail: (id: string) => [...tagKeys.details(), id] as const,
};

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to fetch user's tags with pagination.
 */
export function useTags(params?: GetTagsParams) {
  return useQuery<TagsListResponse, Error>({
    queryKey: tagKeys.list(params),
    queryFn: () => tagsApi.getTags(params),
  });
}

/**
 * Hook to fetch all tags.
 */
export function useAllTags() {
  return useQuery<Tag[], Error>({
    queryKey: tagKeys.list({ page: 1, perPage: 100, includeGameCount: true }),
    queryFn: () => tagsApi.getAllTags(),
  });
}

/**
 * Hook to fetch a single tag.
 */
export function useTag(id: string | undefined) {
  return useQuery<Tag, Error>({
    queryKey: tagKeys.detail(id ?? ''),
    queryFn: () => tagsApi.getTag(id!),
    enabled: !!id,
  });
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to create a new tag.
 */
export function useCreateTag() {
  const queryClient = useQueryClient();

  return useMutation<Tag, Error, TagCreateData>({
    mutationFn: (data) => tagsApi.createTag(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}

/**
 * Hook to create or get existing tag by name.
 */
export function useCreateOrGetTag() {
  const queryClient = useQueryClient();

  return useMutation<
    { tag: Tag; created: boolean },
    Error,
    { name: string; color?: string }
  >({
    mutationFn: ({ name, color }) => tagsApi.createOrGetTag(name, color),
    onSuccess: (result) => {
      if (result.created) {
        queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
      }
    },
  });
}

/**
 * Hook to update an existing tag.
 */
export function useUpdateTag() {
  const queryClient = useQueryClient();

  return useMutation<Tag, Error, { id: string; data: TagUpdateData }>({
    mutationFn: ({ id, data }) => tagsApi.updateTag(id, data),
    onSuccess: (_result, { id }) => {
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
      queryClient.invalidateQueries({ queryKey: tagKeys.detail(id) });
    },
  });
}

/**
 * Hook to delete a tag.
 */
export function useDeleteTag() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, string>({
    mutationFn: (id) => tagsApi.deleteTag(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}

/**
 * Hook to assign tags to a user game.
 */
export function useAssignTagsToGame() {
  const queryClient = useQueryClient();

  return useMutation<
    { message: string; newAssociations: number; totalRequested: number },
    Error,
    { userGameId: string; tagIds: string[] }
  >({
    mutationFn: ({ userGameId, tagIds }) =>
      tagsApi.assignTagsToGame(userGameId, tagIds),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}

/**
 * Hook to remove tags from a user game.
 */
export function useRemoveTagsFromGame() {
  const queryClient = useQueryClient();

  return useMutation<
    { message: string; removedAssociations: number; totalRequested: number },
    Error,
    { userGameId: string; tagIds: string[] }
  >({
    mutationFn: ({ userGameId, tagIds }) =>
      tagsApi.removeTagsFromGame(userGameId, tagIds),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend-next/src/hooks/use-tags.ts
git commit -m "feat(frontend-next): add tags hooks"
```

---

## Task 4: Update Hooks Index Exports

**Files:**
- Modify: `frontend-next/src/hooks/index.ts`

**Step 1: Add new exports to index.ts**

Replace the entire file with:

```typescript
// Game hooks
export {
  gameKeys,
  useUserGames,
  useUserGame,
  useSearchIGDB,
  useCollectionStats,
  useCreateUserGame,
  useUpdateUserGame,
  useDeleteUserGame,
  useImportFromIGDB,
  useBulkUpdateUserGames,
  useBulkDeleteUserGames,
  useAddPlatformToUserGame,
  useUpdatePlatformAssociation,
  useRemovePlatformFromUserGame,
} from './use-games';

// Platform hooks
export {
  platformKeys,
  storefrontKeys,
  usePlatforms,
  useAllPlatforms,
  usePlatform,
  usePlatformStorefronts,
  usePlatformNames,
  useStorefronts,
  useAllStorefronts,
  useStorefront,
  useStorefrontNames,
} from './use-platforms';

// Tag hooks
export {
  tagKeys,
  useTags,
  useAllTags,
  useTag,
  useCreateTag,
  useCreateOrGetTag,
  useUpdateTag,
  useDeleteTag,
  useAssignTagsToGame,
  useRemoveTagsFromGame,
} from './use-tags';
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend-next/src/hooks/index.ts
git commit -m "feat(frontend-next): export new hooks from index"
```

---

## Task 5: Create Game Edit Form Component

**Files:**
- Create: `frontend-next/src/components/games/game-edit-form.tsx`

**Step 1: Create the game-edit-form.tsx file**

Create file `frontend-next/src/components/games/game-edit-form.tsx`:

```typescript
'use client';

import { useState, useCallback, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { StarRating } from '@/components/ui/star-rating';
import { NotesEditor } from '@/components/ui/notes-editor';
import { PlatformSelector, type PlatformSelection } from '@/components/ui/platform-selector';
import { TagSelector } from '@/components/ui/tag-selector';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useUpdateUserGame,
  useAddPlatformToUserGame,
  useRemovePlatformFromUserGame,
  useAssignTagsToGame,
  useRemoveTagsFromGame,
  useAllPlatforms,
  useAllTags,
  useCreateOrGetTag,
} from '@/hooks';
import { config } from '@/lib/env';
import type { UserGame, PlayStatus, OwnershipStatus } from '@/types';
import { ArrowLeft, Save, Loader2, Heart } from 'lucide-react';

// Status options
const PLAY_STATUS_OPTIONS: { value: PlayStatus; label: string }[] = [
  { value: 'not_started', label: 'Not Started' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'completed', label: 'Completed' },
  { value: 'mastered', label: 'Mastered' },
  { value: 'dominated', label: 'Dominated' },
  { value: 'shelved', label: 'Shelved' },
  { value: 'replay', label: 'Replay' },
];

const OWNERSHIP_STATUS_OPTIONS: { value: OwnershipStatus; label: string }[] = [
  { value: 'owned', label: 'Owned' },
  { value: 'borrowed', label: 'Borrowed' },
  { value: 'rented', label: 'Rented' },
  { value: 'subscription', label: 'Subscription' },
  { value: 'no_longer_owned', label: 'No Longer Owned' },
];

// Helper to resolve image URLs
function resolveImageUrl(url: string | undefined): string {
  if (!url) return '';
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url;
  }
  return `${config.staticUrl}${url.startsWith('/') ? url : `/${url}`}`;
}

export interface GameEditFormProps {
  game: UserGame;
}

export function GameEditForm({ game }: GameEditFormProps) {
  const router = useRouter();

  // Form state
  const [playStatus, setPlayStatus] = useState<PlayStatus>(game.play_status);
  const [ownershipStatus, setOwnershipStatus] = useState<OwnershipStatus>(game.ownership_status);
  const [personalRating, setPersonalRating] = useState<number | null>(game.personal_rating ?? null);
  const [isLoved, setIsLoved] = useState(game.is_loved);
  const [hoursPlayed, setHoursPlayed] = useState(game.hours_played);
  const [acquiredDate, setAcquiredDate] = useState(game.acquired_date ?? '');
  const [personalNotes, setPersonalNotes] = useState(game.personal_notes ?? '');
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>(
    game.tags?.map((t) => t.id) ?? []
  );
  const [selectedPlatforms, setSelectedPlatforms] = useState<PlatformSelection[]>(
    game.platforms
      .filter((p) => p.platform_id)
      .map((p) => ({
        platform_id: p.platform_id!,
        storefront_id: p.storefront_id,
      }))
  );

  // Data fetching
  const { data: platforms = [], isLoading: platformsLoading } = useAllPlatforms();
  const { data: tags = [], isLoading: tagsLoading } = useAllTags();

  // Mutations
  const updateGame = useUpdateUserGame();
  const addPlatform = useAddPlatformToUserGame();
  const removePlatform = useRemovePlatformFromUserGame();
  const assignTags = useAssignTagsToGame();
  const removeTags = useRemoveTagsFromGame();
  const createOrGetTag = useCreateOrGetTag();

  const isSaving =
    updateGame.isPending ||
    addPlatform.isPending ||
    removePlatform.isPending ||
    assignTags.isPending ||
    removeTags.isPending;

  // Track original values for comparison
  const originalTagIds = useMemo(
    () => game.tags?.map((t) => t.id) ?? [],
    [game.tags]
  );
  const originalPlatformIds = useMemo(
    () => game.platforms.map((p) => p.platform_id).filter(Boolean) as string[],
    [game.platforms]
  );

  // Get platform association ID by platform_id
  const getPlatformAssociationId = useCallback(
    (platformId: string): string | undefined => {
      const assoc = game.platforms.find((p) => p.platform_id === platformId);
      return assoc?.id;
    },
    [game.platforms]
  );

  const handleSave = async () => {
    try {
      // 1. Update basic game properties
      await updateGame.mutateAsync({
        id: game.id,
        data: {
          playStatus,
          ownershipStatus,
          personalRating,
          isLoved,
          hoursPlayed,
          acquiredDate: acquiredDate || undefined,
          personalNotes: personalNotes || undefined,
        },
      });

      // 2. Handle platform changes
      const currentPlatformIds = selectedPlatforms.map((p) => p.platform_id);
      const platformsToAdd = selectedPlatforms.filter(
        (p) => !originalPlatformIds.includes(p.platform_id)
      );
      const platformsToRemove = originalPlatformIds.filter(
        (id) => !currentPlatformIds.includes(id)
      );

      // Add new platforms
      for (const platform of platformsToAdd) {
        await addPlatform.mutateAsync({
          userGameId: game.id,
          data: {
            platformId: platform.platform_id,
            storefrontId: platform.storefront_id,
          },
        });
      }

      // Remove platforms
      for (const platformId of platformsToRemove) {
        const associationId = getPlatformAssociationId(platformId);
        if (associationId) {
          await removePlatform.mutateAsync({
            userGameId: game.id,
            platformAssociationId: associationId,
          });
        }
      }

      // 3. Handle tag changes
      const tagsToAdd = selectedTagIds.filter((id) => !originalTagIds.includes(id));
      const tagsToRemove = originalTagIds.filter((id) => !selectedTagIds.includes(id));

      if (tagsToAdd.length > 0) {
        await assignTags.mutateAsync({
          userGameId: game.id,
          tagIds: tagsToAdd,
        });
      }

      if (tagsToRemove.length > 0) {
        await removeTags.mutateAsync({
          userGameId: game.id,
          tagIds: tagsToRemove,
        });
      }

      toast.success('Game updated successfully');
      router.push(`/games/${game.id}`);
    } catch (error) {
      console.error('Failed to update game:', error);
      toast.error('Failed to update game');
    }
  };

  const handleCreateTag = async (name: string) => {
    try {
      const result = await createOrGetTag.mutateAsync({ name });
      setSelectedTagIds((prev) => [...prev, result.tag.id]);
      if (result.created) {
        toast.success(`Tag "${name}" created`);
      }
    } catch (error) {
      console.error('Failed to create tag:', error);
      toast.error('Failed to create tag');
    }
  };

  const handleCancel = () => {
    router.push(`/games/${game.id}`);
  };

  const coverArtUrl = resolveImageUrl(game.game.cover_art_url);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <Button variant="outline" onClick={handleCancel}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={isSaving}>
          {isSaving ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Save className="mr-2 h-4 w-4" />
          )}
          Save Changes
        </Button>
      </div>

      {/* Game Info Header */}
      <Card>
        <CardContent className="p-6">
          <div className="flex items-start gap-4">
            {/* Cover Art Thumbnail */}
            <div className="w-20 h-28 flex-shrink-0 overflow-hidden rounded-md bg-muted">
              {coverArtUrl ? (
                <img
                  src={coverArtUrl}
                  alt={game.game.title}
                  className="h-full w-full object-cover"
                />
              ) : (
                <div className="h-full w-full flex items-center justify-center text-2xl">
                  🎮
                </div>
              )}
            </div>
            <div>
              <h1 className="text-2xl font-bold">{game.game.title}</h1>
              {game.game.developer && (
                <p className="text-muted-foreground">{game.game.developer}</p>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Status & Rating */}
      <Card>
        <CardHeader>
          <CardTitle>Status & Rating</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Play Status */}
            <div className="space-y-2">
              <Label htmlFor="play-status">Play Status</Label>
              <Select value={playStatus} onValueChange={(v) => setPlayStatus(v as PlayStatus)}>
                <SelectTrigger id="play-status">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PLAY_STATUS_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Ownership Status */}
            <div className="space-y-2">
              <Label htmlFor="ownership-status">Ownership Status</Label>
              <Select
                value={ownershipStatus}
                onValueChange={(v) => setOwnershipStatus(v as OwnershipStatus)}
              >
                <SelectTrigger id="ownership-status">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {OWNERSHIP_STATUS_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Personal Rating */}
            <div className="space-y-2">
              <Label>Personal Rating</Label>
              <StarRating
                value={personalRating}
                onChange={setPersonalRating}
                clearable
                size="lg"
              />
            </div>

            {/* Is Loved */}
            <div className="space-y-2">
              <Label>Favorite</Label>
              <div className="flex items-center gap-2 pt-2">
                <Checkbox
                  id="is-loved"
                  checked={isLoved}
                  onCheckedChange={(checked) => setIsLoved(checked === true)}
                />
                <label
                  htmlFor="is-loved"
                  className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70 flex items-center gap-1"
                >
                  <Heart className={`h-4 w-4 ${isLoved ? 'text-red-500 fill-red-500' : ''}`} />
                  Mark as loved
                </label>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Progress & Dates */}
      <Card>
        <CardHeader>
          <CardTitle>Progress & Dates</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Hours Played */}
            <div className="space-y-2">
              <Label htmlFor="hours-played">Hours Played</Label>
              <Input
                id="hours-played"
                type="number"
                min="0"
                step="0.5"
                value={hoursPlayed}
                onChange={(e) => setHoursPlayed(parseFloat(e.target.value) || 0)}
              />
            </div>

            {/* Acquired Date */}
            <div className="space-y-2">
              <Label htmlFor="acquired-date">Acquired Date</Label>
              <Input
                id="acquired-date"
                type="date"
                value={acquiredDate}
                onChange={(e) => setAcquiredDate(e.target.value)}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Platforms */}
      <Card>
        <CardHeader>
          <CardTitle>Platforms</CardTitle>
        </CardHeader>
        <CardContent>
          {platformsLoading ? (
            <Skeleton className="h-10 w-full" />
          ) : (
            <PlatformSelector
              selectedPlatforms={selectedPlatforms}
              availablePlatforms={platforms}
              onChange={setSelectedPlatforms}
              placeholder="Select platforms..."
            />
          )}
        </CardContent>
      </Card>

      {/* Tags */}
      <Card>
        <CardHeader>
          <CardTitle>Tags</CardTitle>
        </CardHeader>
        <CardContent>
          {tagsLoading ? (
            <Skeleton className="h-10 w-full" />
          ) : (
            <TagSelector
              selectedTagIds={selectedTagIds}
              availableTags={tags}
              onChange={setSelectedTagIds}
              onCreateTag={handleCreateTag}
              allowCreate
              placeholder="Select tags..."
            />
          )}
        </CardContent>
      </Card>

      {/* Personal Notes */}
      <Card>
        <CardHeader>
          <CardTitle>Personal Notes</CardTitle>
        </CardHeader>
        <CardContent>
          <NotesEditor
            value={personalNotes}
            onChange={setPersonalNotes}
            placeholder="Add your personal notes about this game..."
          />
        </CardContent>
      </Card>

      {/* Bottom Actions */}
      <div className="flex items-center justify-end gap-4">
        <Button variant="outline" onClick={handleCancel}>
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={isSaving}>
          {isSaving ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Save className="mr-2 h-4 w-4" />
          )}
          Save Changes
        </Button>
      </div>
    </div>
  );
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend-next/src/components/games/game-edit-form.tsx
git commit -m "feat(frontend-next): create GameEditForm component"
```

---

## Task 6: Update Games Components Index

**Files:**
- Modify: `frontend-next/src/components/games/index.ts`

**Step 1: Add GameEditForm export**

Add at the end of the file:

```typescript
export { GameEditForm } from './game-edit-form';
export type { GameEditFormProps } from './game-edit-form';
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend-next/src/components/games/index.ts
git commit -m "feat(frontend-next): export GameEditForm from index"
```

---

## Task 7: Create Game Edit Page

**Files:**
- Create: `frontend-next/src/app/(main)/games/[id]/edit/page.tsx`

**Step 1: Create the edit directory**

Run: `mkdir -p frontend-next/src/app/\(main\)/games/\[id\]/edit`

**Step 2: Create the page.tsx file**

Create file `frontend-next/src/app/(main)/games/[id]/edit/page.tsx`:

```typescript
'use client';

import { useParams, useRouter } from 'next/navigation';
import { useUserGame } from '@/hooks';
import { GameEditForm } from '@/components/games/game-edit-form';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { ArrowLeft } from 'lucide-react';

export default function GameEditPage() {
  const params = useParams();
  const router = useRouter();
  const gameId = params.id as string;

  const { data: game, isLoading, error } = useUserGame(gameId);

  if (isLoading) {
    return <GameEditSkeleton />;
  }

  if (error || !game) {
    return (
      <div className="text-center py-12">
        <div className="mx-auto max-w-md">
          <h3 className="mt-4 text-lg font-medium">Game not found</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            The requested game could not be found in your collection.
          </p>
          <div className="mt-6">
            <Button onClick={() => router.push('/games')}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Games
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return <GameEditForm game={game} />;
}

function GameEditSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <Skeleton className="h-10 w-24" />
        <Skeleton className="h-10 w-32" />
      </div>
      <Card>
        <CardContent className="p-6">
          <div className="flex items-start gap-4">
            <Skeleton className="w-20 h-28 rounded-md" />
            <div className="space-y-2">
              <Skeleton className="h-8 w-48" />
              <Skeleton className="h-4 w-32" />
            </div>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="p-6">
          <Skeleton className="h-6 w-32 mb-4" />
          <div className="grid grid-cols-2 gap-4">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="p-6">
          <Skeleton className="h-6 w-32 mb-4" />
          <Skeleton className="h-40" />
        </CardContent>
      </Card>
    </div>
  );
}
```

**Step 3: Verify TypeScript compiles**

Run: `cd frontend-next && npm run check`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend-next/src/app/\(main\)/games/\[id\]/edit/
git commit -m "feat(frontend-next): create game edit page"
```

---

## Task 8: Add MSW Handlers for Platform and Tag Operations

**Files:**
- Modify: `frontend-next/src/test/mocks/handlers.ts`

**Step 1: Add mock platforms data**

Add after the mockTokens constant (around line 25):

```typescript
export const mockPlatforms = [
  {
    id: 'platform-1',
    name: 'pc',
    display_name: 'PC',
    icon_url: null,
    is_active: true,
    source: 'system',
    default_storefront_id: 'storefront-1',
    storefronts: [
      {
        id: 'storefront-1',
        name: 'steam',
        display_name: 'Steam',
        icon_url: null,
        base_url: 'https://store.steampowered.com',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'platform-2',
    name: 'playstation-5',
    display_name: 'PlayStation 5',
    icon_url: null,
    is_active: true,
    source: 'system',
    default_storefront_id: null,
    storefronts: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

export const mockTags = [
  {
    id: 'tag-1',
    user_id: 'test-user-id',
    name: 'RPG',
    color: '#FF5733',
    description: 'Role-playing games',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 5,
  },
  {
    id: 'tag-2',
    user_id: 'test-user-id',
    name: 'Action',
    color: '#33FF57',
    description: 'Action games',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 3,
  },
];
```

**Step 2: Update platform handler to return proper structure**

Replace the existing platforms handler (around line 96) with:

```typescript
  // Platform endpoints
  http.get(`${API_URL}/platforms/`, () => {
    return HttpResponse.json({
      platforms: mockPlatforms,
      total: mockPlatforms.length,
      page: 1,
      per_page: 100,
      pages: 1,
    });
  }),
```

**Step 3: Add platform management handlers**

Add after the platforms handler:

```typescript
  // Add platform to user game
  http.post(`${API_URL}/user-games/:userGameId/platforms`, async ({ params, request }) => {
    const body = (await request.json()) as {
      platform_id: string;
      storefront_id?: string;
    };

    const platform = mockPlatforms.find((p) => p.id === body.platform_id);

    return HttpResponse.json(
      {
        id: `ugp-${Date.now()}`,
        platform_id: body.platform_id,
        storefront_id: body.storefront_id ?? null,
        platform: platform ?? null,
        storefront: null,
        store_game_id: null,
        store_url: null,
        is_available: true,
        original_platform_name: null,
        created_at: new Date().toISOString(),
      },
      { status: 201 }
    );
  }),

  // Update platform association
  http.put(
    `${API_URL}/user-games/:userGameId/platforms/:associationId`,
    async ({ params, request }) => {
      const body = (await request.json()) as {
        platform_id: string;
        storefront_id?: string;
      };

      const platform = mockPlatforms.find((p) => p.id === body.platform_id);

      return HttpResponse.json({
        id: params.associationId,
        platform_id: body.platform_id,
        storefront_id: body.storefront_id ?? null,
        platform: platform ?? null,
        storefront: null,
        store_game_id: null,
        store_url: null,
        is_available: true,
        original_platform_name: null,
        created_at: new Date().toISOString(),
      });
    }
  ),

  // Remove platform from user game
  http.delete(`${API_URL}/user-games/:userGameId/platforms/:associationId`, () => {
    return HttpResponse.json({ message: 'Platform removed successfully' });
  }),
```

**Step 4: Update tags handler**

Replace the existing tags handler with:

```typescript
  // Tags endpoints
  http.get(`${API_URL}/tags/`, () => {
    return HttpResponse.json({
      tags: mockTags,
      total: mockTags.length,
      page: 1,
      per_page: 100,
      total_pages: 1,
    });
  }),

  // Assign tags to game
  http.post(`${API_URL}/tags/assign/:userGameId`, async ({ request }) => {
    const body = (await request.json()) as { tag_ids: string[] };
    return HttpResponse.json({
      message: 'Tags assigned successfully',
      new_associations: body.tag_ids.length,
      total_requested: body.tag_ids.length,
    });
  }),

  // Remove tags from game
  http.delete(`${API_URL}/tags/remove/:userGameId`, async ({ request }) => {
    const body = (await request.json()) as { tag_ids: string[] };
    return HttpResponse.json({
      message: 'Tags removed successfully',
      removed_associations: body.tag_ids.length,
      total_requested: body.tag_ids.length,
    });
  }),

  // Create or get tag
  http.post(`${API_URL}/tags/create-or-get`, ({ request }) => {
    const url = new URL(request.url);
    const name = url.searchParams.get('name') ?? 'New Tag';
    const color = url.searchParams.get('color') ?? '#808080';

    return HttpResponse.json({
      tag: {
        id: `tag-${Date.now()}`,
        user_id: 'test-user-id',
        name,
        color,
        description: null,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        game_count: 0,
      },
      created: true,
    });
  }),
```

**Step 5: Verify tests still pass**

Run: `cd frontend-next && npm run test`
Expected: All tests pass

**Step 6: Commit**

```bash
git add frontend-next/src/test/mocks/handlers.ts
git commit -m "feat(frontend-next): add MSW handlers for platform and tag operations"
```

---

## Task 9: Write Tests for Game Edit Form

**Files:**
- Create: `frontend-next/src/components/games/game-edit-form.test.tsx`

**Step 1: Create the test file**

Create file `frontend-next/src/components/games/game-edit-form.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameEditForm } from './game-edit-form';
import type { UserGame, PlayStatus, OwnershipStatus } from '@/types';
import { server } from '@/test/mocks/server';
import { http, HttpResponse } from 'msw';

// Mock next/navigation
const mockPush = vi.fn();
const mockBack = vi.fn();

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    back: mockBack,
  }),
  useParams: () => ({ id: 'test-game-id' }),
}));

// Mock sonner toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const mockGame: UserGame = {
  id: 'test-user-game-id' as UserGame['id'],
  game: {
    id: 123 as any,
    title: 'Test Game',
    description: 'A test game description',
    genre: 'RPG',
    developer: 'Test Developer',
    publisher: 'Test Publisher',
    release_date: '2024-01-01',
    cover_art_url: '/covers/test.jpg',
    rating_average: 4.5,
    rating_count: 100,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  ownership_status: 'owned' as OwnershipStatus,
  personal_rating: 4,
  is_loved: false,
  play_status: 'in_progress' as PlayStatus,
  hours_played: 10,
  personal_notes: '<p>Some notes</p>',
  acquired_date: '2024-01-15',
  platforms: [
    {
      id: 'ugp-1',
      platform_id: 'platform-1',
      storefront_id: 'storefront-1',
      platform: {
        id: 'platform-1',
        name: 'pc',
        display_name: 'PC',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      storefront: {
        id: 'storefront-1',
        name: 'steam',
        display_name: 'Steam',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      is_available: true,
      created_at: '2024-01-01T00:00:00Z',
    },
  ],
  tags: [
    {
      id: 'tag-1',
      user_id: 'test-user-id',
      name: 'RPG',
      color: '#FF5733',
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    },
  ],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

describe('GameEditForm', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders the form with game title', async () => {
    render(<GameEditForm game={mockGame} />);

    expect(screen.getByText('Test Game')).toBeInTheDocument();
    expect(screen.getByText('Test Developer')).toBeInTheDocument();
  });

  it('renders status dropdowns with current values', async () => {
    render(<GameEditForm game={mockGame} />);

    // Check play status is shown
    expect(screen.getByText('In Progress')).toBeInTheDocument();

    // Check ownership status is shown
    expect(screen.getByText('Owned')).toBeInTheDocument();
  });

  it('renders hours played input with current value', () => {
    render(<GameEditForm game={mockGame} />);

    const hoursInput = screen.getByLabelText('Hours Played');
    expect(hoursInput).toHaveValue(10);
  });

  it('renders acquired date input with current value', () => {
    render(<GameEditForm game={mockGame} />);

    const dateInput = screen.getByLabelText('Acquired Date');
    expect(dateInput).toHaveValue('2024-01-15');
  });

  it('renders cancel button that navigates back', async () => {
    const user = userEvent.setup();
    render(<GameEditForm game={mockGame} />);

    const cancelButtons = screen.getAllByRole('button', { name: /cancel/i });
    await user.click(cancelButtons[0]);

    expect(mockPush).toHaveBeenCalledWith('/games/test-user-game-id');
  });

  it('renders save button', () => {
    render(<GameEditForm game={mockGame} />);

    expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument();
  });

  it('updates hours played when input changes', async () => {
    const user = userEvent.setup();
    render(<GameEditForm game={mockGame} />);

    const hoursInput = screen.getByLabelText('Hours Played');
    await user.clear(hoursInput);
    await user.type(hoursInput, '25');

    expect(hoursInput).toHaveValue(25);
  });

  it('renders is loved checkbox', () => {
    render(<GameEditForm game={mockGame} />);

    expect(screen.getByRole('checkbox', { name: /mark as loved/i })).toBeInTheDocument();
  });

  it('toggles is loved checkbox', async () => {
    const user = userEvent.setup();
    render(<GameEditForm game={mockGame} />);

    const checkbox = screen.getByRole('checkbox', { name: /mark as loved/i });
    expect(checkbox).not.toBeChecked();

    await user.click(checkbox);
    expect(checkbox).toBeChecked();
  });
});
```

**Step 2: Run tests to verify they pass**

Run: `cd frontend-next && npm run test -- game-edit-form`
Expected: All tests pass

**Step 3: Commit**

```bash
git add frontend-next/src/components/games/game-edit-form.test.tsx
git commit -m "test(frontend-next): add tests for GameEditForm component"
```

---

## Task 10: Final Verification

**Step 1: Run all checks**

```bash
cd frontend-next
npm run check   # TypeScript + ESLint
npm run test    # All tests
npm run build   # Production build
```

Expected: All pass with no errors

**Step 2: Commit any final fixes if needed**

**Step 3: Update the beads issue**

```bash
bd close nexorious-983 --reason="Game edit form implemented with platform/storefront management, tag management, and all editable fields"
bd sync
```

---

## Summary

| Task | Description                           | Files                                            |
|------|---------------------------------------|--------------------------------------------------|
| 1    | Add platform management API functions | `api/games.ts`                                   |
| 2    | Add platform management hooks         | `hooks/use-games.ts`                             |
| 3    | Create tags hooks file                | `hooks/use-tags.ts` (new)                        |
| 4    | Update hooks index exports            | `hooks/index.ts`                                 |
| 5    | Create game edit form component       | `components/games/game-edit-form.tsx` (new)      |
| 6    | Update games components index         | `components/games/index.ts`                      |
| 7    | Create game edit page                 | `app/(main)/games/[id]/edit/page.tsx` (new)      |
| 8    | Add MSW handlers                      | `test/mocks/handlers.ts`                         |
| 9    | Write tests                           | `components/games/game-edit-form.test.tsx` (new) |
| 10   | Final verification                    | -                                                |

**New files created:** 4
**Files modified:** 5
