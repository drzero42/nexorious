import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useUserGame } from '@/hooks';
import { GameEditForm } from '@/components/games/game-edit-form';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { ArrowLeft } from 'lucide-react';

export const Route = createFileRoute('/_authenticated/games/$id/edit')({
  component: GameEditPage,
});

function GameEditPage() {
  const { id: gameId } = Route.useParams();
  const navigate = useNavigate();

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
            <Button onClick={() => navigate({ to: '/games' })}>
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
