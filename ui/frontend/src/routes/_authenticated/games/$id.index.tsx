import * as React from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useUserGame, useDeleteUserGame, useMoveToLibrary } from '@/hooks';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import {
  ArrowLeft,
  Edit,
  ListPlus,
  Trash2,
  Heart,
  Clock,
  ExternalLink,
  Gamepad2,
  Library,
  Trophy,
} from 'lucide-react';
import { AddToPoolDialog } from '@/components/pools/add-to-pool-dialog';
import { StarRating } from '@/components/ui/star-rating';
import { StorefrontLabel } from '@/components/storefront-link';
import { PlatformIcon } from '@/components/ui/platform-icon';
import {
  formatIgdbRating,
  formatHoursPlayed,
  formatTtb,
  formatPlatformLabel,
  resolveImageUrl,
} from '@/lib/game-utils';
import { statusLabels } from '@/lib/play-status';
import {
  OwnershipStatus,
  type PlayStatus,
  type OwnershipStatus as OwnershipStatusType,
} from '@/types';
import { PlatformSelector, type PlatformSelection } from '@/components/ui/platform-selector';
import { TagBadge } from '@/components/ui/tag-selector';
import {
  PlatformDetailFields,
  type PlatformDetail,
} from '@/components/games/platform-detail-fields';
import { useAllPlatforms, useSettings, useDateFormat } from '@/hooks';
import { buildDealLinks } from '@/lib/deal-links';
import { getGameReturn, navigateToGameReturn } from '@/lib/game-return';
import type { UserGamePlatformData } from '@/api/games';

export const Route = createFileRoute('/_authenticated/games/$id/')({
  head: () => ({ meta: [{ title: 'Game Details | Nexorious' }] }),
  component: GameDetailPage,
});

// Get status color classes
function getStatusColor(status: PlayStatus): string {
  const colors: Record<PlayStatus, string> = {
    not_started: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
    in_progress: 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200',
    completed: 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200',
    mastered: 'bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-200',
    dominated: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-200',
    shelved: 'bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200',
    dropped: 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200',
    replay: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-950 dark:text-cyan-200',
  };
  return colors[status] || 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200';
}

// Format ownership status for display
function formatOwnershipStatus(status: OwnershipStatusType): string {
  const labels: Record<OwnershipStatusType, string> = {
    owned: 'Owned',
    borrowed: 'Borrowed',
    rented: 'Rented',
    subscription: 'Subscription',
    no_longer_owned: 'No Longer Owned',
  };
  return labels[status] || status;
}

// ============================================================================
// Move to Library dialog
// ============================================================================

function MoveToLibraryDialog({
  userGameId,
  open,
  onOpenChange,
}: {
  userGameId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { data: allPlatforms = [] } = useAllPlatforms();
  const moveToLibrary = useMoveToLibrary();

  const [selections, setSelections] = React.useState<PlatformSelection[]>([]);
  const [detail, setDetail] = React.useState<PlatformDetail>({
    ownershipStatus: OwnershipStatus.OWNED,
    acquiredDate: '',
    hoursPlayed: 0,
  });
  const [error, setError] = React.useState<string | null>(null);

  const canConfirm =
    selections.length > 0 && selections.every((s) => s.platform !== '') && !moveToLibrary.isPending;

  const handleConfirm = async () => {
    if (!canConfirm) return;
    setError(null);

    // Build platforms array — one PlatformDetailFields drives shared ownership/hours/acquired
    // for all selected rows; each row contributes its own platform + storefront.
    const platforms: UserGamePlatformData[] = selections.map((s) => ({
      platform: s.platform,
      storefront: s.storefront,
      ownershipStatus: detail.ownershipStatus,
      hoursPlayed: detail.hoursPlayed > 0 ? detail.hoursPlayed : undefined,
      acquiredDate: detail.acquiredDate !== '' ? detail.acquiredDate : undefined,
    }));

    try {
      await moveToLibrary.mutateAsync({ userGameId, platforms });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to move game to library.');
    }
  };

  const firstSelection = selections[0];
  const firstPlatform = firstSelection
    ? allPlatforms.find((p) => p.name === firstSelection.platform)
    : undefined;
  const detailLabel =
    selections.length > 1
      ? `Ownership & playtime (applies to all ${selections.length} selected)`
      : firstSelection?.platform !== ''
        ? (firstPlatform?.display_name ?? firstSelection?.platform ?? 'Platform')
        : 'Platform';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Move to library</DialogTitle>
          <DialogDescription>
            Choose at least one platform to add this game to your library.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <p className="text-sm font-medium mb-2">Platform</p>
            <PlatformSelector
              selectedPlatforms={selections}
              availablePlatforms={allPlatforms}
              onChange={setSelections}
              disabled={moveToLibrary.isPending}
              placeholder="Select platform..."
            />
          </div>

          {selections.length > 0 && selections[0].platform !== '' && (
            <PlatformDetailFields
              label={detailLabel}
              value={detail}
              onChange={setDetail}
              disabled={moveToLibrary.isPending}
            />
          )}

          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={moveToLibrary.isPending}
          >
            Cancel
          </Button>
          <Button onClick={handleConfirm} disabled={!canConfirm}>
            {moveToLibrary.isPending ? 'Moving…' : 'Move to library'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ============================================================================
// Main detail page
// ============================================================================

export function GameDetailPage() {
  const { id: gameId } = Route.useParams();
  const navigate = useNavigate();
  const [moveDialogOpen, setMoveDialogOpen] = React.useState(false);
  const [showPoolDialog, setShowPoolDialog] = React.useState(false);

  const { data: game, isLoading, error } = useUserGame(gameId);
  const deleteGame = useDeleteUserGame();
  const { data: settings } = useSettings();
  const { formatDate } = useDateFormat();

  const gameReturn = getGameReturn();

  const handleDelete = async () => {
    await deleteGame.mutateAsync(gameId);
    navigateToGameReturn(navigate);
  };

  if (isLoading) {
    return <GameDetailSkeleton />;
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
            <Button onClick={() => navigateToGameReturn(navigate)}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to {gameReturn.label}
            </Button>
          </div>
        </div>
      </div>
    );
  }

  const dealLinks = buildDealLinks(game.game.title, settings?.dealRegion ?? 'us');

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <Button variant="outline" onClick={() => navigateToGameReturn(navigate)}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to {gameReturn.label}
        </Button>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => setShowPoolDialog(true)}>
            <ListPlus className="mr-2 h-4 w-4" />
            Add to pool
          </Button>
          <Button
            variant="outline"
            onClick={() => navigate({ to: '/games/$id/edit', params: { id: gameId } })}
          >
            <Edit className="mr-2 h-4 w-4" />
            Edit
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="destructive">
                <Trash2 className="mr-2 h-4 w-4" />
                Remove
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Remove from collection?</AlertDialogTitle>
                <AlertDialogDescription>
                  Are you sure you want to remove &ldquo;{game.game.title}&rdquo; from your
                  collection? This action cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={handleDelete}>Remove</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>
      <AddToPoolDialog
        userGameId={game.id}
        open={showPoolDialog}
        onOpenChange={setShowPoolDialog}
      />

      {/* Main Content */}
      <Card>
        <CardContent className="p-6">
          <div className="lg:grid lg:grid-cols-3 lg:gap-8">
            {/* Cover Art */}
            <div className="lg:col-span-1">
              <div className="aspect-[3/4] overflow-hidden rounded-lg bg-muted shadow-lg relative">
                {game.game.cover_art_url ? (
                  <img
                    src={resolveImageUrl(game.game.cover_art_url)}
                    alt={game.game.title}
                    loading="lazy"
                    className="object-cover object-center w-full h-full"
                  />
                ) : (
                  <div className="h-full w-full flex items-center justify-center text-muted-foreground">
                    <div className="text-center">
                      <div className="text-4xl mb-2">🎮</div>
                      <p className="text-sm">No Cover</p>
                    </div>
                  </div>
                )}
              </div>
            </div>

            {/* Game Info */}
            <div className="lg:col-span-2 mt-6 lg:mt-0 space-y-6">
              {/* Title and Love */}
              <div className="flex items-start justify-between">
                <div>
                  <h1 className="text-3xl font-bold">{game.game.title}</h1>
                  {game.game.developer && (
                    <p className="text-muted-foreground mt-1">{game.game.developer}</p>
                  )}
                </div>
                {game.is_loved && <Heart className="h-8 w-8 text-red-500 fill-red-500" />}
              </div>

              {/* Quick Stats */}
              <div className="flex flex-wrap items-center gap-3">
                <Badge className={getStatusColor(game.play_status)}>
                  {statusLabels[game.play_status]}
                </Badge>
                <StarRating value={game.personal_rating} readonly size="md" showLabel />
                {game.game.rating_average != null && (
                  <div className="flex items-center gap-1">
                    <Gamepad2 className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm font-medium">
                      {formatIgdbRating(game.game.rating_average)}
                    </span>
                    <span className="text-sm text-muted-foreground">IGDB</span>
                  </div>
                )}
              </div>

              {/* Game Metadata Grid */}
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 text-sm">
                {game.game.publisher && (
                  <div>
                    <dt className="font-medium text-muted-foreground">Publisher</dt>
                    <dd className="mt-1">{game.game.publisher}</dd>
                  </div>
                )}
                {game.game.genre && (
                  <div>
                    <dt className="font-medium text-muted-foreground">Genre</dt>
                    <dd className="mt-1">{game.game.genre}</dd>
                  </div>
                )}
                {game.game.release_date && (
                  <div>
                    <dt className="font-medium text-muted-foreground">Release Date</dt>
                    <dd className="mt-1">{formatDate(game.game.release_date)}</dd>
                  </div>
                )}
                {game.game.game_modes && (
                  <div>
                    <dt className="font-medium text-muted-foreground">Game Modes</dt>
                    <dd className="mt-1">{game.game.game_modes}</dd>
                  </div>
                )}
                {game.game.themes && (
                  <div>
                    <dt className="font-medium text-muted-foreground">Themes</dt>
                    <dd className="mt-1">{game.game.themes}</dd>
                  </div>
                )}
                {game.game.player_perspectives && (
                  <div>
                    <dt className="font-medium text-muted-foreground">Perspectives</dt>
                    <dd className="mt-1">{game.game.player_perspectives}</dd>
                  </div>
                )}
                {game.game.igdb_slug && (
                  <div>
                    <dt className="font-medium text-muted-foreground">IGDB</dt>
                    <dd className="mt-1">
                      <a
                        href={`https://www.igdb.com/games/${game.game.igdb_slug}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 inline-flex items-center gap-1"
                      >
                        View
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    </dd>
                  </div>
                )}
              </div>

              {/* Wishlist section — shown instead of platform attachment when wishlisted */}
              {game.is_wishlisted ? (
                <div className="space-y-4 p-4 rounded-lg border border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/30">
                  {/* Move-to-library dialog — remounts on each open so state is always fresh */}
                  <MoveToLibraryDialog
                    key={String(moveDialogOpen)}
                    userGameId={game.id}
                    open={moveDialogOpen}
                    onOpenChange={setMoveDialogOpen}
                  />
                  <div className="flex items-center justify-between">
                    <h3 className="font-medium">Wishlist</h3>
                    <Button size="sm" onClick={() => setMoveDialogOpen(true)}>
                      <Library className="mr-2 h-4 w-4" />
                      Move to library
                    </Button>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <a
                      href={dealLinks.itad}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={`Search IsThereAnyDeal for ${game.game?.title ?? 'this game'} deals (opens in new tab)`}
                      className="flex items-center gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <div className="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-md bg-[#2f2e35]">
                        <img
                          src="/logos/deals/isthereanydeal/isthereanydeal-icon.svg"
                          alt=""
                          className="h-6 w-6"
                        />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-medium">IsThereAnyDeal</div>
                        <div className="text-xs text-muted-foreground">PC deals</div>
                      </div>
                      <ExternalLink className="h-4 w-4 shrink-0 text-muted-foreground" />
                    </a>
                    <a
                      href={dealLinks.psprices}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={`Search PSprices for ${game.game?.title ?? 'this game'} deals (opens in new tab)`}
                      className="flex items-center gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <div className="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-md">
                        <img
                          src="/logos/deals/psprices/psprices-icon.svg"
                          alt=""
                          className="h-full w-full object-cover"
                        />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-medium">PSprices</div>
                        <div className="text-xs text-muted-foreground">Console deals</div>
                      </div>
                      <ExternalLink className="h-4 w-4 shrink-0 text-muted-foreground" />
                    </a>
                  </div>
                </div>
              ) : (
                /* Platforms & Ownership — shown only for library entries */
                game.platforms &&
                game.platforms.length > 0 && (
                  <div>
                    <h3 className="font-medium mb-2">Platforms & Ownership</h3>
                    <div className="space-y-2">
                      {game.platforms.map((p) => (
                        <div
                          key={p.id}
                          className="flex items-center justify-between bg-muted/50 px-3 py-2 rounded-lg"
                        >
                          <div className="flex items-center gap-2">
                            <span className="inline-flex items-center gap-1.5 font-medium">
                              {p.platform_details && (
                                <PlatformIcon platform={p.platform_details} size="sm" decorative />
                              )}
                              {p.platform_details?.display_name || p.platform || 'Unknown'}
                            </span>
                            {p.storefront_details && (
                              <StorefrontLabel
                                storefront={p.storefront_details}
                                storeUrl={p.store_url}
                              />
                            )}
                          </div>
                          <div className="flex items-center gap-2">
                            {p.achievements_total != null && p.achievements_total > 0 && (
                              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                                <Trophy className="h-3 w-3" />
                                {p.achievements_unlocked ?? 0}/{p.achievements_total}
                              </span>
                            )}
                            <Badge variant="outline">
                              {formatOwnershipStatus(p.ownership_status ?? OwnershipStatus.OWNED)}
                            </Badge>
                            {p.acquired_date && (
                              <span className="text-xs text-muted-foreground">
                                {formatDate(p.acquired_date)}
                              </span>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )
              )}

              {/* How Long to Beat */}
              {(game.game.howlongtobeat_main ||
                game.game.howlongtobeat_extra ||
                game.game.howlongtobeat_completionist) && (
                <div>
                  <h3 className="font-medium mb-2">How Long to Beat</h3>
                  <div className="grid grid-cols-3 gap-2">
                    {game.game.howlongtobeat_main && (
                      <div className="bg-blue-50 dark:bg-blue-950 p-3 rounded-lg text-center">
                        <div className="text-xs text-muted-foreground">Main Story</div>
                        <div className="font-bold text-blue-700 dark:text-blue-300">
                          {formatTtb(game.game.howlongtobeat_main)}
                        </div>
                      </div>
                    )}
                    {game.game.howlongtobeat_extra && (
                      <div className="bg-green-50 dark:bg-green-950 p-3 rounded-lg text-center">
                        <div className="text-xs text-muted-foreground">Main + Extra</div>
                        <div className="font-bold text-green-700 dark:text-green-300">
                          {formatTtb(game.game.howlongtobeat_extra)}
                        </div>
                      </div>
                    )}
                    {game.game.howlongtobeat_completionist && (
                      <div className="bg-purple-50 dark:bg-purple-950 p-3 rounded-lg text-center">
                        <div className="text-xs text-muted-foreground">Completionist</div>
                        <div className="font-bold text-purple-700 dark:text-purple-300">
                          {formatTtb(game.game.howlongtobeat_completionist)}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              )}

              {/* Description */}
              {game.game.description && (
                <div>
                  <h3 className="font-medium mb-2">Description</h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {game.game.description}
                  </p>
                </div>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Personal Information Card */}
      <Card>
        <CardHeader>
          <CardTitle>Your Information</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            <div className="bg-muted/50 p-4 rounded-lg">
              <dt className="text-sm text-muted-foreground">Status</dt>
              <dd className="mt-1">
                <Badge className={getStatusColor(game.play_status)}>
                  {statusLabels[game.play_status]}
                </Badge>
              </dd>
            </div>
            <div className="bg-muted/50 p-4 rounded-lg">
              <dt className="text-sm text-muted-foreground mb-1">Rating</dt>
              <dd>
                <StarRating value={game.personal_rating} readonly size="sm" />
              </dd>
            </div>
            <div className="bg-muted/50 p-4 rounded-lg">
              <dt className="text-sm text-muted-foreground flex items-center gap-1">
                <Clock className="h-4 w-4" /> Hours Played
              </dt>
              <dd className="mt-1 font-medium">{formatHoursPlayed(game.hours_played)}</dd>
              {/* Playtime breakdown by storefront */}
              {game.platforms.some((p) => p.hours_played > 0) && (
                <div className="mt-2 space-y-1 text-xs text-muted-foreground">
                  {game.platforms
                    .filter((p) => p.hours_played > 0)
                    .map((p) => (
                      <div key={p.id} className="flex justify-between">
                        <span>{formatPlatformLabel(p)}</span>
                        <span>{formatHoursPlayed(p.hours_played)}</span>
                      </div>
                    ))}
                </div>
              )}
            </div>
          </div>

          {game.tags && game.tags.length > 0 && (
            <div className="mt-4">
              <h4 className="text-sm font-medium text-muted-foreground mb-2">Tags</h4>
              <div className="flex flex-wrap gap-2">
                {game.tags.map((tag) => (
                  <TagBadge key={tag.id} tag={tag} size="sm" />
                ))}
              </div>
            </div>
          )}

          {game.personal_notes && (
            <div className="mt-4">
              <h4 className="text-sm font-medium text-muted-foreground mb-2">Personal Notes</h4>
              <div
                className="prose prose-sm dark:prose-invert max-w-none bg-muted/50 p-4 rounded-lg"
                dangerouslySetInnerHTML={{ __html: game.personal_notes }}
              />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function GameDetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <Skeleton className="h-10 w-32" />
        <div className="flex gap-2">
          <Skeleton className="h-10 w-20" />
          <Skeleton className="h-10 w-24" />
        </div>
      </div>
      <Card>
        <CardContent className="p-6">
          <div className="lg:grid lg:grid-cols-3 lg:gap-8">
            <Skeleton className="aspect-[3/4] rounded-lg" />
            <div className="lg:col-span-2 mt-6 lg:mt-0 space-y-4">
              <Skeleton className="h-10 w-3/4" />
              <Skeleton className="h-6 w-1/2" />
              <div className="flex gap-2">
                <Skeleton className="h-6 w-24" />
                <Skeleton className="h-6 w-20" />
              </div>
              <div className="grid grid-cols-3 gap-4">
                <Skeleton className="h-16" />
                <Skeleton className="h-16" />
                <Skeleton className="h-16" />
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
