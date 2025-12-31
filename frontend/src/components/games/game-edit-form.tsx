'use client';

import { useState, useCallback, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import Image from 'next/image';
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
      .filter((p) => p.platform)
      .map((p) => ({
        platform: p.platform!,
        storefront: p.storefront,
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
            <div className="w-20 h-28 flex-shrink-0 overflow-hidden rounded-md bg-muted relative">
              {coverArtUrl ? (
                <Image
                  src={coverArtUrl}
                  alt={game.game.title}
                  fill
                  className="object-cover"
                  unoptimized
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
