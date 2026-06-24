import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { ArrowLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { IGDBSearch } from '@/components/games/igdb-search';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { useHealthStatus } from '@/hooks/use-health-status';
import { setGameReturn } from '@/lib/game-return';
import type { IGDBGameCandidate } from '@/types';

export const SELECTED_GAME_STORAGE_KEY = 'nexorious_selected_game';

export const Route = createFileRoute('/_authenticated/games/add/')({
  head: () => ({ meta: [{ title: 'Add Game | Nexorious' }] }),
  component: AddGamePage,
});

export function AddGamePage() {
  const navigate = useNavigate();
  const { data: health } = useHealthStatus();
  const igdbUnavailable = health?.igdb_status !== undefined && health.igdb_status !== 'ok';

  const handleGameSelect = (game: IGDBGameCandidate) => {
    // Already in the library or wishlist: jump straight to the detail page
    // instead of walking the user through the add flow only to reject it as a
    // duplicate (#856). For wishlisted entries the detail page hosts the
    // wishlist UI; for library entries it lets the user navigate to edit.
    if (game.user_game_id) {
      // Reset the back-referrer so a stale prior referrer doesn't leak into
      // the existing game's detail page.
      setGameReturn({ to: '/games', label: 'Games' });
      navigate({ to: '/games/$id', params: { id: game.user_game_id } });
      return;
    }
    sessionStorage.setItem(SELECTED_GAME_STORAGE_KEY, JSON.stringify(game));
    navigate({ to: '/games/add/confirm', search: { igdb_id: String(game.igdb_id) } });
  };

  const search = (
    <IGDBSearch
      onSelect={handleGameSelect}
      autoFocus
      placeholder="Search for a game to add..."
      disabled={igdbUnavailable}
      showLibraryStatus
    />
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link to="/games">
            <ArrowLeft className="h-4 w-4" />
            <span className="sr-only">Back to library</span>
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold">Add Game</h1>
          <p className="text-muted-foreground">
            Search IGDB to find and add a game to your library
          </p>
        </div>
      </div>

      <div className="max-w-2xl">
        {igdbUnavailable ? (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div>{search}</div>
              </TooltipTrigger>
              <TooltipContent>IGDB not configured</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ) : (
          search
        )}
      </div>
    </div>
  );
}
