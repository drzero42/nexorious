import { useState, useCallback, useMemo } from 'react';
import { useNavigate } from '@tanstack/react-router';
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
  useUpdatePlatformAssociation,
  useAssignTagsToGame,
  useRemoveTagsFromGame,
  useAllPlatforms,
  useAllTags,
  useCreateOrGetTag,
  useSyncConfig,
} from '@/hooks';
import { SyncPlatform, SyncFrequency } from '@/types/sync';
import { config } from '@/lib/env';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame } from '@/types';
import { ArrowLeft, Save, Loader2, Heart } from 'lucide-react';

// Status options
const PLAY_STATUS_OPTIONS: { value: PlayStatus; label: string }[] = [
  { value: PlayStatus.NOT_STARTED, label: 'Not Started' },
  { value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
  { value: PlayStatus.COMPLETED, label: 'Completed' },
  { value: PlayStatus.MASTERED, label: 'Mastered' },
  { value: PlayStatus.DOMINATED, label: 'Dominated' },
  { value: PlayStatus.SHELVED, label: 'Shelved' },
  { value: PlayStatus.DROPPED, label: 'Dropped' },
  { value: PlayStatus.REPLAY, label: 'Replay' },
];

const OWNERSHIP_STATUS_OPTIONS: { value: OwnershipStatus; label: string }[] = [
  { value: OwnershipStatus.OWNED, label: 'Owned' },
  { value: OwnershipStatus.BORROWED, label: 'Borrowed' },
  { value: OwnershipStatus.RENTED, label: 'Rented' },
  { value: OwnershipStatus.SUBSCRIPTION, label: 'Subscription' },
  { value: OwnershipStatus.NO_LONGER_OWNED, label: 'No Longer Owned' },
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
  const navigate = useNavigate();

  // Form state
  const [playStatus, setPlayStatus] = useState<PlayStatus>(game.play_status);
  const [personalRating, setPersonalRating] = useState<number | null>(game.personal_rating ?? null);
  const [isLoved, setIsLoved] = useState(game.is_loved);
  const [platformPlaytimes, setPlatformPlaytimes] = useState<Record<string, number>>(
    Object.fromEntries(game.platforms.map((p) => [p.id, p.hours_played]))
  );
  const [platformOwnership, setPlatformOwnership] = useState<Record<string, {
    ownershipStatus: OwnershipStatus;
    acquiredDate: string;
  }>>(
    Object.fromEntries(
      game.platforms.map((p) => [
        p.id,
        {
          ownershipStatus: p.ownership_status,
          acquiredDate: p.acquired_date ?? ''
        }
      ])
    )
  );
  const [personalNotes, setPersonalNotes] = useState(game.personal_notes ?? '');
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>(
    game.tags?.map((t) => t.id) ?? []
  );
  const [selectedPlatforms, setSelectedPlatforms] = useState<PlatformSelection[]>(
    game.platforms
      .filter((p) => p.platform)
      .map((p) => ({
        platform: p.platform!,
        storefront: p.storefront,
      }))
  );

  // Data fetching
  const { data: platforms = [], isLoading: platformsLoading } = useAllPlatforms();
  const { data: tags = [], isLoading: tagsLoading } = useAllTags();
  const { data: steamSyncConfig } = useSyncConfig(SyncPlatform.STEAM);

  // Check if Steam sync is enabled (non-manual frequency when configured)
  const isSteamSyncEnabled =
    steamSyncConfig?.isConfigured && steamSyncConfig?.frequency !== SyncFrequency.MANUAL;

  // Mutations
  const updateGame = useUpdateUserGame();
  const addPlatform = useAddPlatformToUserGame();
  const removePlatform = useRemovePlatformFromUserGame();
  const updatePlatformAssoc = useUpdatePlatformAssociation();
  const assignTags = useAssignTagsToGame();
  const removeTags = useRemoveTagsFromGame();
  const createOrGetTag = useCreateOrGetTag();

  const isSaving =
    updateGame.isPending ||
    addPlatform.isPending ||
    removePlatform.isPending ||
    updatePlatformAssoc.isPending ||
    assignTags.isPending ||
    removeTags.isPending;

  // Compute total hours from platform playtimes
  const totalHoursPlayed = useMemo(() => {
    const platformHours = Object.values(platformPlaytimes).reduce((sum, h) => sum + h, 0);
    return platformHours > 0 ? platformHours : game.hours_played;
  }, [platformPlaytimes, game.hours_played]);

  // Track original values for comparison
  const originalTagIds = useMemo(
    () => game.tags?.map((t) => t.id) ?? [],
    [game.tags]
  );
  const originalPlatformNames = useMemo(
    () => game.platforms.map((p) => p.platform).filter(Boolean) as string[],
    [game.platforms]
  );

  // Get platform association ID by platform name
  const getPlatformAssociationId = useCallback(
    (platformName: string): string | undefined => {
      const assoc = game.platforms.find((p) => p.platform === platformName);
      return assoc?.id;
    },
    [game.platforms]
  );

  const handleSave = async () => {
    try {
      // 1. Update basic game properties (no longer updating hours_played or ownership - it's per platform now)
      await updateGame.mutateAsync({
        id: game.id,
        data: {
          playStatus,
          personalRating,
          isLoved,
          personalNotes: personalNotes || undefined,
        },
      });

      // 2. Handle platform changes
      const currentPlatformNames = selectedPlatforms.map((p) => p.platform);
      const platformsToAdd = selectedPlatforms.filter(
        (p) => !originalPlatformNames.includes(p.platform)
      );
      const platformsToRemove = originalPlatformNames.filter(
        (name) => !currentPlatformNames.includes(name)
      );

      // Add new platforms
      for (const platform of platformsToAdd) {
        await addPlatform.mutateAsync({
          userGameId: game.id,
          data: {
            platform: platform.platform,
            storefront: platform.storefront,
          },
        });
      }

      // Remove platforms
      for (const platformName of platformsToRemove) {
        const associationId = getPlatformAssociationId(platformName);
        if (associationId) {
          await removePlatform.mutateAsync({
            userGameId: game.id,
            platformAssociationId: associationId,
          });
        }
      }

      // 3. Update platform playtimes and ownership
      for (const [platformId, data] of Object.entries(platformOwnership)) {
        const originalPlatform = game.platforms.find((p) => p.id === platformId);
        if (originalPlatform) {
          const hours = platformPlaytimes[platformId] ?? originalPlatform.hours_played;
          const needsUpdate =
            originalPlatform.hours_played !== hours ||
            originalPlatform.ownership_status !== data.ownershipStatus ||
            (originalPlatform.acquired_date ?? '') !== data.acquiredDate;

          if (needsUpdate) {
            await updatePlatformAssoc.mutateAsync({
              userGameId: game.id,
              platformAssociationId: platformId,
              data: {
                platform: originalPlatform.platform || '',
                storefront: originalPlatform.storefront,
                hoursPlayed: hours,
                ownershipStatus: data.ownershipStatus,
                acquiredDate: data.acquiredDate || undefined,
              },
            });
          }
        }
      }

      // 4. Handle tag changes
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
      navigate({ to: `/games/${game.id}` });
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
    navigate({ to: `/games/${game.id}` });
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
            <div className="w-20 h-28 flex-shrink-0 overflow-hidden rounded-md bg-muted relative">
              {coverArtUrl ? (
                <img
                  src={coverArtUrl}
                  alt={game.game.title}
                  style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                  loading="lazy"
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
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
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

      {/* Progress */}
      <Card>
        <CardHeader>
          <CardTitle>Progress</CardTitle>
        </CardHeader>
        <CardContent>
          {/* Hours Played - now per-platform */}
          <div className="space-y-2">
            <Label>Total Hours Played</Label>
            <p className="text-sm text-muted-foreground">
              Playtime and ownership are tracked per platform below.
            </p>
            <p className="text-lg font-medium">{totalHoursPlayed} hours total</p>
          </div>
        </CardContent>
      </Card>

      {/* Platforms & Ownership */}
      <Card>
        <CardHeader>
          <CardTitle>Platforms & Ownership</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {platformsLoading ? (
            <Skeleton className="h-10 w-full" />
          ) : (
            <>
              <PlatformSelector
                selectedPlatforms={selectedPlatforms}
                availablePlatforms={platforms}
                onChange={setSelectedPlatforms}
                placeholder="Select platforms..."
              />

              {/* Per-platform details */}
              {game.platforms.length > 0 && (
                <div className="space-y-4 pt-4 border-t">
                  {game.platforms.map((p) => {
                    const isSteamPlatform = p.storefront === 'steam';
                    const isSteamSynced = isSteamSyncEnabled && isSteamPlatform;
                    const ownership = platformOwnership[p.id] ?? {
                      ownershipStatus: p.ownership_status,
                      acquiredDate: p.acquired_date ?? ''
                    };
                    const platformName =
                      p.storefront_details?.display_name ||
                      p.storefront ||
                      p.platform_details?.display_name ||
                      p.platform ||
                      'Unknown';

                    return (
                      <div key={p.id} className="p-4 rounded-lg border bg-muted/30">
                        <div className="font-medium mb-3">{platformName}</div>
                        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                          {/* Ownership Status */}
                          <div className="space-y-1">
                            <Label className="text-xs text-muted-foreground">Ownership</Label>
                            <Select
                              value={ownership.ownershipStatus}
                              onValueChange={(v) =>
                                setPlatformOwnership((prev) => ({
                                  ...prev,
                                  [p.id]: {
                                    ...prev[p.id],
                                    ownershipStatus: v as OwnershipStatus
                                  }
                                }))
                              }
                            >
                              <SelectTrigger className="h-9">
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

                          {/* Acquired Date */}
                          <div className="space-y-1">
                            <Label className="text-xs text-muted-foreground">Acquired</Label>
                            <Input
                              type="date"
                              className="h-9"
                              value={ownership.acquiredDate}
                              onChange={(e) =>
                                setPlatformOwnership((prev) => ({
                                  ...prev,
                                  [p.id]: {
                                    ...prev[p.id],
                                    acquiredDate: e.target.value
                                  }
                                }))
                              }
                            />
                          </div>

                          {/* Hours Played */}
                          <div className="space-y-1">
                            <Label className="text-xs text-muted-foreground">
                              Hours{isSteamSynced && ' (Synced)'}
                            </Label>
                            <div className="flex items-center gap-2">
                              <Input
                                type="number"
                                min="0"
                                className="h-9 w-24"
                                value={platformPlaytimes[p.id] ?? p.hours_played}
                                onChange={(e) =>
                                  setPlatformPlaytimes((prev) => ({
                                    ...prev,
                                    [p.id]: parseInt(e.target.value) || 0,
                                  }))
                                }
                                disabled={isSteamSynced}
                              />
                              <span className="text-sm text-muted-foreground">hrs</span>
                            </div>
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </>
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
