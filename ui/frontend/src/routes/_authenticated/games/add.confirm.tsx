import * as React from 'react';
import { createFileRoute, Link, useNavigate, useSearch } from '@tanstack/react-router';
import {
  ArrowLeft,
  Calendar,
  Clock,
  Gamepad2,
  Monitor,
  Plus,
  Loader2,
  AlertCircle,
} from 'lucide-react';
import { toast } from 'sonner';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { PlatformSelectorCompact } from '@/components/ui/platform-selector';
import type { PlatformSelection } from '@/components/ui/platform-selector';
import { useImportFromIGDB, useCreateUserGame } from '@/hooks/use-games';
import { useAllPlatforms } from '@/hooks/use-platforms';
import type { IGDBGameCandidate } from '@/types';
import type { Platform } from '@/types/platform';
import { toGameId } from '@/types';
import { SELECTED_GAME_STORAGE_KEY } from './add';

export const Route = createFileRoute('/_authenticated/games/add/confirm')({
  component: GameConfirmPage,
});

// ============================================================================
// Types
// ============================================================================

interface GamePreviewProps {
  game: IGDBGameCandidate;
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Format release date to readable format.
 */
function formatReleaseDate(releaseDate?: string): string | null {
  if (!releaseDate) return null;
  const date = new Date(releaseDate);
  if (isNaN(date.getTime())) return null;
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });
}

/**
 * Format playtime hours to readable string.
 */
function formatPlaytime(hours?: number): string | null {
  if (!hours || hours <= 0) return null;
  if (hours < 1) {
    return `${Math.round(hours * 60)} min`;
  }
  return `${Math.round(hours)} hrs`;
}

/**
 * Check if a platform matches any of the IGDB platform names.
 * Uses case-insensitive comparison on both display_name and name.
 */
function isPlatformInIGDB(platform: Platform, igdbPlatforms: string[]): boolean {
  if (!igdbPlatforms || igdbPlatforms.length === 0) return false;

  return igdbPlatforms.some(
    (igdbPlatform) =>
      igdbPlatform.toLowerCase() === platform.display_name.toLowerCase() ||
      igdbPlatform.toLowerCase() === platform.name.toLowerCase()
  );
}

/**
 * Filter platforms to only those available on IGDB for this game.
 */
function getIGDBPlatforms(platforms: Platform[], igdbPlatforms: string[]): Platform[] {
  if (!igdbPlatforms || igdbPlatforms.length === 0) return [];
  return platforms.filter((platform) => isPlatformInIGDB(platform, igdbPlatforms));
}

/**
 * Filter platforms to those NOT available on IGDB for this game.
 */
function getOtherPlatforms(platforms: Platform[], igdbPlatforms: string[]): Platform[] {
  if (!igdbPlatforms || igdbPlatforms.length === 0) return platforms;
  return platforms.filter((platform) => !isPlatformInIGDB(platform, igdbPlatforms));
}

// ============================================================================
// Sub-Components
// ============================================================================

function GamePreviewCard({ game }: GamePreviewProps) {
  const releaseDate = formatReleaseDate(game.release_date);
  const mainPlaytime = formatPlaytime(game.howlongtobeat_main);
  const extraPlaytime = formatPlaytime(game.howlongtobeat_extra);
  const completionistPlaytime = formatPlaytime(game.howlongtobeat_completionist);

  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex flex-col sm:flex-row gap-6">
          {/* Cover Art */}
          <div className="relative w-32 h-44 flex-shrink-0 mx-auto sm:mx-0 rounded-lg overflow-hidden bg-muted shadow-md">
            {game.cover_art_url ? (
              <img
                src={game.cover_art_url}
                alt={`${game.title} cover`}
                loading="lazy"
                className="object-cover w-full h-full"
              />
            ) : (
              <div className="h-full w-full flex items-center justify-center">
                <Gamepad2 className="h-12 w-12 text-muted-foreground/50" />
              </div>
            )}
          </div>

          {/* Game Info */}
          <div className="flex-1 space-y-3 min-w-0">
            <div>
              <h2 className="text-2xl font-bold">{game.title}</h2>
              {releaseDate && (
                <div className="flex items-center gap-1.5 text-sm text-muted-foreground mt-1">
                  <Calendar className="h-4 w-4" />
                  <span>{releaseDate}</span>
                </div>
              )}
            </div>

            {/* IGDB Platforms */}
            {game.platforms.length > 0 && (
              <div className="flex items-start gap-2 text-sm">
                <Monitor className="h-4 w-4 text-muted-foreground flex-shrink-0 mt-0.5" />
                <span className="text-muted-foreground">
                  {game.platforms.join(', ')}
                </span>
              </div>
            )}

            {/* HowLongToBeat Times */}
            {(mainPlaytime || extraPlaytime || completionistPlaytime) && (
              <div className="flex flex-wrap gap-3 text-sm">
                {mainPlaytime && (
                  <div className="flex items-center gap-1.5 bg-muted px-2 py-1 rounded">
                    <Clock className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="text-muted-foreground">Main:</span>
                    <span className="font-medium">{mainPlaytime}</span>
                  </div>
                )}
                {extraPlaytime && (
                  <div className="flex items-center gap-1.5 bg-muted px-2 py-1 rounded">
                    <Clock className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="text-muted-foreground">Extra:</span>
                    <span className="font-medium">{extraPlaytime}</span>
                  </div>
                )}
                {completionistPlaytime && (
                  <div className="flex items-center gap-1.5 bg-muted px-2 py-1 rounded">
                    <Clock className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="text-muted-foreground">100%:</span>
                    <span className="font-medium">{completionistPlaytime}</span>
                  </div>
                )}
              </div>
            )}

            {/* Description */}
            {game.description && (
              <p className="text-sm text-muted-foreground line-clamp-4">
                {game.description}
              </p>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

interface PlatformSelectionSectionProps {
  platforms: Platform[];
  igdbPlatformNames: string[];
  selectedPlatforms: PlatformSelection[];
  onChange: (selections: PlatformSelection[]) => void;
  disabled?: boolean;
}

function PlatformSelectionSection({
  platforms,
  igdbPlatformNames,
  selectedPlatforms,
  onChange,
  disabled = false,
}: PlatformSelectionSectionProps) {
  const [showOtherPlatforms, setShowOtherPlatforms] = React.useState(false);

  // Filter platforms based on IGDB data
  const igdbPlatforms = React.useMemo(
    () => getIGDBPlatforms(platforms, igdbPlatformNames),
    [platforms, igdbPlatformNames]
  );
  const otherPlatforms = React.useMemo(
    () => getOtherPlatforms(platforms, igdbPlatformNames),
    [platforms, igdbPlatformNames]
  );

  const hasIGDBPlatforms = igdbPlatforms.length > 0;
  const hasOtherPlatforms = otherPlatforms.length > 0;

  // If no IGDB platforms found, show all platforms
  if (!hasIGDBPlatforms) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-lg flex items-center gap-2">
            <Monitor className="h-5 w-5" />
            Select Your Platforms
          </CardTitle>
          <p className="text-sm text-muted-foreground">
            Choose which platforms you own this game on (optional)
          </p>
        </CardHeader>
        <CardContent>
          {platforms.length > 0 ? (
            <PlatformSelectorCompact
              selectedPlatforms={selectedPlatforms}
              availablePlatforms={platforms}
              onChange={onChange}
              disabled={disabled}
            />
          ) : (
            <div className="text-center py-8 text-muted-foreground">
              <Monitor className="w-12 h-12 mx-auto mb-4 text-muted-foreground/50" />
              <p className="text-sm">No platforms available</p>
            </div>
          )}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg flex items-center gap-2">
          <Monitor className="h-5 w-5" />
          Select Your Platforms
        </CardTitle>
        <p className="text-sm text-muted-foreground">
          Choose which platforms you own this game on (optional)
        </p>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* IGDB Platforms - Available for this game */}
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm font-medium text-green-600 dark:text-green-400">
            <Gamepad2 className="h-4 w-4" />
            <span>Available on these platforms</span>
          </div>
          <PlatformSelectorCompact
            selectedPlatforms={selectedPlatforms}
            availablePlatforms={igdbPlatforms}
            onChange={onChange}
            disabled={disabled}
          />
        </div>

        {/* Other Platforms - Collapsible */}
        {hasOtherPlatforms && (
          <div className="space-y-3 pt-2 border-t">
            <button
              type="button"
              onClick={() => setShowOtherPlatforms(!showOtherPlatforms)}
              className="flex items-center gap-2 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors w-full"
            >
              <Monitor className="h-4 w-4" />
              <span>Other platforms ({otherPlatforms.length})</span>
              <span className="ml-auto text-xs">
                {showOtherPlatforms ? '▼' : '▶'}
              </span>
            </button>
            {showOtherPlatforms && (
              <PlatformSelectorCompact
                selectedPlatforms={selectedPlatforms}
                availablePlatforms={otherPlatforms}
                onChange={onChange}
                disabled={disabled}
              />
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ConfirmPageSkeleton() {
  return (
    <div className="space-y-6">
      {/* Header Skeleton */}
      <div className="flex items-center gap-4">
        <Skeleton className="h-10 w-10 rounded-md" />
        <div>
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-4 w-64 mt-2" />
        </div>
      </div>

      {/* Game Preview Skeleton */}
      <Card>
        <CardContent className="p-6">
          <div className="flex flex-col sm:flex-row gap-6">
            <Skeleton className="w-32 h-44 rounded-lg mx-auto sm:mx-0" />
            <div className="flex-1 space-y-3">
              <Skeleton className="h-8 w-64" />
              <Skeleton className="h-4 w-40" />
              <Skeleton className="h-4 w-80" />
              <div className="flex gap-3">
                <Skeleton className="h-6 w-24" />
                <Skeleton className="h-6 w-24" />
              </div>
              <Skeleton className="h-20 w-full" />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Platform Selection Skeleton */}
      <Card>
        <CardHeader>
          <Skeleton className="h-6 w-48" />
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        </CardContent>
      </Card>

      {/* Action Buttons Skeleton */}
      <div className="flex justify-end gap-3">
        <Skeleton className="h-10 w-24" />
        <Skeleton className="h-10 w-40" />
      </div>
    </div>
  );
}

function ErrorState({ message, onBack }: { message: string; onBack: () => void }) {
  return (
    <div className="text-center py-12">
      <div className="mx-auto max-w-md">
        <AlertCircle className="h-12 w-12 text-destructive mx-auto" />
        <h3 className="mt-4 text-lg font-medium">Unable to load game</h3>
        <p className="mt-2 text-sm text-muted-foreground">{message}</p>
        <div className="mt-6">
          <Button onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Search
          </Button>
        </div>
      </div>
    </div>
  );
}

// ============================================================================
// Main Component
// ============================================================================

function GameConfirmPage() {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as Record<string, string>;

  // Get IGDB ID from query params
  const igdbIdParam = search['igdb_id'];
  const igdbId = React.useMemo(() => {
    if (!igdbIdParam) return null;
    try {
      return toGameId(igdbIdParam);
    } catch {
      return null;
    }
  }, [igdbIdParam]);

  // State for platform selection and game data
  const [selectedPlatforms, setSelectedPlatforms] = React.useState<
    PlatformSelection[]
  >([]);
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const [game, setGame] = React.useState<IGDBGameCandidate | null>(null);
  const [gameLoadError, setGameLoadError] = React.useState<string | null>(null);
  const [isHydrated, setIsHydrated] = React.useState(false);

  // Mark as hydrated after first client render
  React.useEffect(() => {
    setIsHydrated(true);
  }, []);

  // Load game data from sessionStorage after hydration
  React.useEffect(() => {
    // Wait for hydration to complete
    if (!isHydrated) return;

    // If no igdb_id in URL after hydration, show error
    if (!igdbId) {
      setGameLoadError('No game ID provided. Please search for a game first.');
      return;
    }

    try {
      const storedGame = sessionStorage.getItem(SELECTED_GAME_STORAGE_KEY);
      if (storedGame) {
        const parsedGame = JSON.parse(storedGame) as IGDBGameCandidate;
        // Verify the stored game matches the requested IGDB ID
        // Use Number() to ensure consistent comparison (JSON parse gives plain number)
        if (Number(parsedGame.igdb_id) === Number(igdbId)) {
          setGame(parsedGame);
          // Clear from storage after successful load
          sessionStorage.removeItem(SELECTED_GAME_STORAGE_KEY);
          return;
        }
      }
      // No valid game data found
      setGameLoadError(
        'Game data not found. Please search for the game again.'
      );
    } catch {
      setGameLoadError('Failed to load game data. Please try again.');
    }
  }, [isHydrated, igdbId]);

  // Fetch available platforms
  const { data: platforms, isLoading: isPlatformsLoading } = useAllPlatforms();

  // Mutations
  const importFromIGDB = useImportFromIGDB();
  const createUserGame = useCreateUserGame();

  // Show loading while hydrating or loading platforms/game data
  const isLoading = !isHydrated || isPlatformsLoading || (!game && !gameLoadError);

  const handleBack = () => {
    navigate({ to: '/games/add' });
  };

  const handleAddToLibrary = async () => {
    if (!igdbId || !game) {
      toast.error('No game selected');
      return;
    }

    setIsSubmitting(true);

    try {
      // Step 1: Import the game from IGDB (this ensures it exists in our database)
      const importedGame = await importFromIGDB.mutateAsync({
        igdbId,
        downloadCoverArt: true,
      });

      // Step 2: Create the user game entry with selected platforms
      const userGame = await createUserGame.mutateAsync({
        gameId: importedGame.id,
        platforms: selectedPlatforms.map((p) => ({
          platform: p.platform,
          storefront: p.storefront,
        })),
      });

      toast.success(`Added "${game.title}" to your library!`);

      // Navigate to the newly created user game
      navigate({ to: '/games/$id', params: { id: userGame.id } });
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Failed to add game to library';
      toast.error(message);
      setIsSubmitting(false);
    }
  };

  // Loading state (includes hydration)
  if (isLoading) {
    return <ConfirmPageSkeleton />;
  }

  // Error state - game not found
  if (gameLoadError || !game) {
    return (
      <ErrorState
        message={gameLoadError ?? 'The selected game could not be found. Please try searching again.'}
        onBack={handleBack}
      />
    );
  }

  return (
    <div className="space-y-6 max-w-3xl mx-auto">
      {/* Page header with back button */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link to="/games/add">
            <ArrowLeft className="h-4 w-4" />
            <span className="sr-only">Back to search</span>
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold">Confirm Game</h1>
          <p className="text-muted-foreground">
            Review the game details and select your platforms
          </p>
        </div>
      </div>

      {/* Game Preview Card */}
      <GamePreviewCard game={game} />

      {/* Platform Selection */}
      <PlatformSelectionSection
        platforms={platforms ?? []}
        igdbPlatformNames={game.platforms}
        selectedPlatforms={selectedPlatforms}
        onChange={setSelectedPlatforms}
        disabled={isSubmitting}
      />

      {/* Action Buttons */}
      <div className="flex justify-end gap-3 pt-2">
        <Button
          variant="outline"
          onClick={handleBack}
          disabled={isSubmitting}
        >
          Cancel
        </Button>
        <Button onClick={handleAddToLibrary} disabled={isSubmitting}>
          {isSubmitting ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Adding to Library...
            </>
          ) : (
            <>
              <Plus className="mr-2 h-4 w-4" />
              Add to Library
            </>
          )}
        </Button>
      </div>
    </div>
  );
}
