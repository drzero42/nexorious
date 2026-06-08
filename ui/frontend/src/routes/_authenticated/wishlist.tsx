import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useUserGames } from '@/hooks';
import { GameGrid } from '@/components/games';
import { Button } from '@/components/ui/button';
import type { UserGame } from '@/types';

export const Route = createFileRoute('/_authenticated/wishlist')({
  head: () => ({ meta: [{ title: 'Wishlist | Nexorious' }] }),
  component: WishlistPage,
});

function WishlistPageContent() {
  const navigate = useNavigate();
  const { data, isLoading } = useUserGames({ wishlist: true });
  const games = data?.items ?? [];

  const handleClickGame = (game: UserGame) => {
    navigate({ to: '/games/$id', params: { id: game.id } });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold">Wishlist</h1>
          {data && (
            <span className="text-muted-foreground">
              {data.total} game{data.total !== 1 ? 's' : ''}
            </span>
          )}
        </div>
        <Button asChild>
          <Link to="/games/add">Add Game</Link>
        </Button>
      </div>

      {!isLoading && games.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <p className="mb-2">Your wishlist is empty.</p>
          <p className="text-sm">
            Add games you want from the{' '}
            <Link to="/games/add" className="underline underline-offset-4">
              Add Game page
            </Link>
            .
          </p>
        </div>
      ) : (
        <GameGrid games={games} isLoading={isLoading} onClickGame={handleClickGame} />
      )}
    </div>
  );
}

function WishlistPageLoading() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Wishlist</h1>
        <Button asChild>
          <Link to="/games/add">Add Game</Link>
        </Button>
      </div>
      <div className="text-muted-foreground">Loading...</div>
    </div>
  );
}

function WishlistPage() {
  return (
    <Suspense fallback={<WishlistPageLoading />}>
      <WishlistPageContent />
    </Suspense>
  );
}
