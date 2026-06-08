import { useState, useMemo } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
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
import { SyncStorefront, SyncFrequency } from '@/types/sync';
import { formatHoursPlayed, resolveImageUrl, toDateInputValue } from '@/lib/game-utils';
import { planPlatformChanges, type PlatformDetailState } from './platform-reconcile';
import { PlatformDetailFields } from './platform-detail-fields';
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
    Object.fromEntries(game.platforms.map((p) => [p.id, p.hours_played])),
  );
  const [platformOwnership, setPlatformOwnership] = useState<
    Record<
      string,
      {
        ownershipStatus: OwnershipStatus;
        acquiredDate: string;
      }
    >
  >(
    Object.fromEntries(
      game.platforms.map((p) => [
        p.id,
        {
          ownershipStatus: p.ownership_status,
          acquiredDate: toDateInputValue(p.acquired_date),
        },
      ]),
    ),
  );
  const [personalNotes, setPersonalNotes] = useState(game.personal_notes ?? '');
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>(game.tags?.map((t) => t.id) ?? []);
  const [selectedPlatforms, setSelectedPlatforms] = useState<PlatformSelection[]>(
    game.platforms
      .filter((p) => p.platform)
      .map((p) => ({
        key: p.id,
        id: p.id,
        platform: p.platform!,
        storefront: p.storefront,
      })),
  );

  // Data fetching
  const { data: platforms = [], isLoading: platformsLoading } = useAllPlatforms();
  const { data: tags = [], isLoading: tagsLoading } = useAllTags();
  const { data: steamSyncConfig } = useSyncConfig(SyncStorefront.STEAM);

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

  // Selections that have a platform chosen (blank rows from "Add platform" are
  // ignored until the user picks a platform).
  const activeSelections = useMemo(
    () => selectedPlatforms.filter((s) => s.platform),
    [selectedPlatforms],
  );

  // Per-row detail state resolved for one selection: edited value, else the
  // persisted row's value, else a sensible default for a brand-new row.
  const detailFor = (s: PlatformSelection) => {
    const persisted = game.platforms.find((p) => p.id === s.id);
    const ownership = platformOwnership[s.key];
    return {
      hoursPlayed: platformPlaytimes[s.key] ?? persisted?.hours_played ?? 0,
      ownershipStatus:
        ownership?.ownershipStatus ?? persisted?.ownership_status ?? OwnershipStatus.OWNED,
      acquiredDate: ownership?.acquiredDate ?? toDateInputValue(persisted?.acquired_date),
    };
  };

  // Total hours = sum of the per-platform playtime across the selected rows, kept
  // live so in-progress edits (and newly-added rows) are reflected.
  const totalHoursPlayed = useMemo(
    () => activeSelections.reduce((sum, s) => sum + detailFor(s).hoursPlayed, 0),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activeSelections, platformPlaytimes],
  );

  // Track original tag ids for comparison
  const originalTagIds = useMemo(() => game.tags?.map((t) => t.id) ?? [], [game.tags]);

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

      // 2. Reconcile platform associations by row identity. Details are keyed by
      //    the row `key` (present on both persisted and newly-added rows).
      const details: Record<string, PlatformDetailState> = {};
      for (const s of activeSelections) {
        details[s.key] = detailFor(s);
      }

      const { adds, removes, updates } = planPlatformChanges(
        game.platforms,
        activeSelections,
        details,
      );

      for (const add of adds) {
        await addPlatform.mutateAsync({
          userGameId: game.id,
          data: {
            platform: add.platform,
            storefront: add.storefront,
            hoursPlayed: add.hoursPlayed,
            ownershipStatus: add.ownershipStatus,
            acquiredDate: add.acquiredDate,
          },
        });
      }

      for (const remove of removes) {
        await removePlatform.mutateAsync({
          userGameId: game.id,
          platformAssociationId: remove.id,
        });
      }

      for (const update of updates) {
        await updatePlatformAssoc.mutateAsync({
          userGameId: game.id,
          platformAssociationId: update.id,
          data: {
            platform: update.platform,
            storefront: update.storefront,
            hoursPlayed: update.hoursPlayed,
            ownershipStatus: update.ownershipStatus,
            acquiredDate: update.acquiredDate,
          },
        });
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
                <div className="h-full w-full flex items-center justify-center text-2xl">🎮</div>
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
              <StarRating value={personalRating} onChange={setPersonalRating} clearable size="lg" />
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
            <p className="text-lg font-medium">{formatHoursPlayed(totalHoursPlayed)} total</p>
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
              />

              {/* Per-platform details — one card per selected row (incl. unsaved adds) */}
              {activeSelections.length > 0 && (
                <div className="space-y-4 pt-4 border-t">
                  {activeSelections.map((s) => {
                    const isSteamSynced = isSteamSyncEnabled && s.storefront === 'steam';
                    const platform = platforms.find((pp) => pp.name === s.platform);
                    const storefront = platform?.storefronts?.find(
                      (sf) => sf.name === s.storefront,
                    );
                    const label = platform
                      ? storefront
                        ? `${platform.display_name} / ${storefront.display_name}`
                        : platform.display_name
                      : s.platform;
                    const detail = detailFor(s);

                    return (
                      <PlatformDetailFields
                        key={s.key}
                        label={label}
                        value={detail}
                        hoursSynced={isSteamSynced}
                        onChange={(next) => {
                          setPlatformOwnership((prev) => ({
                            ...prev,
                            [s.key]: {
                              ownershipStatus: next.ownershipStatus,
                              acquiredDate: next.acquiredDate,
                            },
                          }));
                          setPlatformPlaytimes((prev) => ({
                            ...prev,
                            [s.key]: next.hoursPlayed,
                          }));
                        }}
                      />
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
