'use client';

import { useParams, useRouter } from 'next/navigation';
import { useUserGame, useDeleteUserGame } from '@/hooks';
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
import Image from 'next/image';
import { ArrowLeft, Edit, Trash2, Heart, Clock, Calendar, ExternalLink } from 'lucide-react';
import { StarRating } from '@/components/ui/star-rating';
import { config } from '@/lib/env';
import type { PlayStatus, OwnershipStatus } from '@/types';

// Helper to resolve image URLs
function resolveImageUrl(url: string | undefined): string {
  if (!url) return '';
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url;
  }
  return `${config.staticUrl}${url.startsWith('/') ? url : `/${url}`}`;
}

// Format play status for display
function formatPlayStatus(status: PlayStatus): string {
  const labels: Record<PlayStatus, string> = {
    not_started: 'Not Started',
    in_progress: 'In Progress',
    completed: 'Completed',
    mastered: 'Mastered',
    dominated: 'Dominated',
    shelved: 'Shelved',
    dropped: 'Dropped',
    replay: 'Replay',
  };
  return labels[status] || status;
}

// Get status color classes
function getStatusColor(status: PlayStatus): string {
  const colors: Record<PlayStatus, string> = {
    not_started: 'bg-gray-100 text-gray-800',
    in_progress: 'bg-blue-100 text-blue-800',
    completed: 'bg-green-100 text-green-800',
    mastered: 'bg-purple-100 text-purple-800',
    dominated: 'bg-yellow-100 text-yellow-800',
    shelved: 'bg-orange-100 text-orange-800',
    dropped: 'bg-red-100 text-red-800',
    replay: 'bg-cyan-100 text-cyan-800',
  };
  return colors[status] || 'bg-gray-100 text-gray-800';
}

// Format ownership status for display
function formatOwnershipStatus(status: OwnershipStatus): string {
  const labels: Record<OwnershipStatus, string> = {
    owned: 'Owned',
    borrowed: 'Borrowed',
    rented: 'Rented',
    subscription: 'Subscription',
    no_longer_owned: 'No Longer Owned',
  };
  return labels[status] || status;
}

export default function GameDetailPage() {
  const params = useParams();
  const router = useRouter();
  const gameId = params.id as string;

  const { data: game, isLoading, error } = useUserGame(gameId);
  const deleteGame = useDeleteUserGame();

  const handleDelete = async () => {
    await deleteGame.mutateAsync(gameId);
    router.push('/games');
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
            <Button onClick={() => router.push('/games')}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Games
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <Button variant="outline" onClick={() => router.push('/games')}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Games
        </Button>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => router.push(`/games/${gameId}/edit`)}>
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

      {/* Main Content */}
      <Card>
        <CardContent className="p-6">
          <div className="lg:grid lg:grid-cols-3 lg:gap-8">
            {/* Cover Art */}
            <div className="lg:col-span-1">
              <div className="aspect-[3/4] overflow-hidden rounded-lg bg-muted shadow-lg relative">
                {game.game.cover_art_url ? (
                  <Image
                    src={resolveImageUrl(game.game.cover_art_url)}
                    alt={game.game.title}
                    fill
                    className="object-cover object-center"
                    unoptimized
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
                {game.is_loved && (
                  <Heart className="h-8 w-8 text-red-500 fill-red-500" />
                )}
              </div>

              {/* Quick Stats */}
              <div className="flex flex-wrap items-center gap-3">
                <Badge className={getStatusColor(game.play_status)}>
                  {formatPlayStatus(game.play_status)}
                </Badge>
                <Badge variant="outline">{formatOwnershipStatus(game.ownership_status)}</Badge>
                <StarRating value={game.personal_rating} readonly size="md" showLabel />
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
                    <dd className="mt-1">
                      {new Date(game.game.release_date).toLocaleDateString()}
                    </dd>
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
                        className="text-blue-600 hover:text-blue-800 inline-flex items-center gap-1"
                      >
                        View
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    </dd>
                  </div>
                )}
              </div>

              {/* Platforms */}
              {game.platforms && game.platforms.length > 0 && (
                <div>
                  <h3 className="font-medium mb-2">Platforms</h3>
                  <div className="flex flex-wrap gap-2">
                    {game.platforms.map((p) => (
                      <Badge key={p.id} variant="outline">
                        {p.platform_details?.display_name || p.platform || 'Unknown'}
                        {p.storefront_details && ` (${p.storefront_details.display_name})`}
                      </Badge>
                    ))}
                  </div>
                </div>
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
                          {game.game.howlongtobeat_main}h
                        </div>
                      </div>
                    )}
                    {game.game.howlongtobeat_extra && (
                      <div className="bg-green-50 dark:bg-green-950 p-3 rounded-lg text-center">
                        <div className="text-xs text-muted-foreground">Main + Extra</div>
                        <div className="font-bold text-green-700 dark:text-green-300">
                          {game.game.howlongtobeat_extra}h
                        </div>
                      </div>
                    )}
                    {game.game.howlongtobeat_completionist && (
                      <div className="bg-purple-50 dark:bg-purple-950 p-3 rounded-lg text-center">
                        <div className="text-xs text-muted-foreground">Completionist</div>
                        <div className="font-bold text-purple-700 dark:text-purple-300">
                          {game.game.howlongtobeat_completionist}h
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
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div className="bg-muted/50 p-4 rounded-lg">
              <dt className="text-sm text-muted-foreground">Status</dt>
              <dd className="mt-1">
                <Badge className={getStatusColor(game.play_status)}>
                  {formatPlayStatus(game.play_status)}
                </Badge>
              </dd>
            </div>
            <div className="bg-muted/50 p-4 rounded-lg">
              <dt className="text-sm text-muted-foreground">Ownership</dt>
              <dd className="mt-1 font-medium">
                {formatOwnershipStatus(game.ownership_status)}
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
              <dd className="mt-1 font-medium">{game.hours_played || 0}h</dd>
            </div>
          </div>

          {game.acquired_date && (
            <div className="mt-4 flex items-center gap-2 text-sm text-muted-foreground">
              <Calendar className="h-4 w-4" />
              Acquired: {new Date(game.acquired_date).toLocaleDateString()}
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
