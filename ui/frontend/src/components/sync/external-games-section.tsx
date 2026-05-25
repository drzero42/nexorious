import { useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { cn } from '@/lib/utils';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { IGDBMatchDialog } from './igdb-match-dialog';
import {
  useExternalGames,
  useSkipExternalGame,
  useUnskipExternalGame,
  useRematchExternalGame,
  useRetryFailedExternalGames,
  syncKeys,
} from '@/hooks/use-sync';
import { retryJobItem } from '@/api/jobs';
import { useQueryClient } from '@tanstack/react-query';
import type { ExternalGame, IGDBGameCandidate, SyncStorefront } from '@/types';

interface ExternalGamesSectionProps {
  storefront: SyncStorefront;
}

interface PendingRematch {
  game: ExternalGame;
  candidate: IGDBGameCandidate;
}

export function ExternalGamesSection({ storefront }: ExternalGamesSectionProps) {
  const { data: games = [], isLoading } = useExternalGames(storefront);
  const { mutate: skip, isPending: isSkipping } = useSkipExternalGame();
  const { mutate: unskip, isPending: isUnskipping } = useUnskipExternalGame();
  const { mutate: rematch, isPending: isRematching } = useRematchExternalGame();
  const { mutate: retryAll, isPending: isRetryingAll } = useRetryFailedExternalGames();
  const queryClient = useQueryClient();

  const [matchingGame, setMatchingGame] = useState<ExternalGame | null>(null);
  const [pendingRematch, setPendingRematch] = useState<PendingRematch | null>(null);
  const [skippedOpen, setSkippedOpen] = useState(false);
  const [matchedOpen, setMatchedOpen] = useState(false);
  const [retryingItemId, setRetryingItemId] = useState<string | null>(null);

  if (isLoading || games.length === 0) return null;

  const needsReview = games.filter((g) => g.sync_status === 'needs_review');
  const failed = games.filter((g) => g.sync_status === 'failed');
  const skipped = games.filter((g) => g.sync_status === 'skipped');
  const matched = games.filter((g) => g.sync_status === 'matched');

  async function handleRetryGame(game: ExternalGame) {
    if (!game.failed_job_item_id) return;
    setRetryingItemId(game.id);
    try {
      await retryJobItem(game.failed_job_item_id);
      queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(storefront) });
    } finally {
      setRetryingItemId(null);
    }
  }

  function handleSelect(game: ExternalGame, candidate: IGDBGameCandidate) {
    setMatchingGame(null);
    const wouldOrphan = game.has_user_game && game.user_game_other_platform_count === 0;
    if (wouldOrphan) {
      setPendingRematch({ game, candidate });
    } else {
      rematch({ id: game.id, igdbId: candidate.igdb_id });
    }
  }

  return (
    <>
      <div className="space-y-4" id="needs-review">
        {needsReview.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Needs Review ({needsReview.length})</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableBody>
                  {needsReview.map((game) => (
                    <TableRow key={game.id}>
                      <TableCell>{game.title}</TableCell>
                      <TableCell className="text-right space-x-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setMatchingGame(game)}
                          disabled={isRematching}
                        >
                          Find Match
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => skip(game.id)}
                          disabled={isSkipping}
                        >
                          Skip
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        {failed.length > 0 && (
          <Card>
            <CardHeader className="flex flex-row items-center justify-between py-3">
              <CardTitle className="text-base">Failed ({failed.length})</CardTitle>
              <Button
                size="sm"
                variant="outline"
                onClick={() => retryAll(storefront)}
                disabled={isRetryingAll}
              >
                Retry All
              </Button>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableBody>
                  {failed.map((game) => (
                    <TableRow key={game.id}>
                      <TableCell>{game.title}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => handleRetryGame(game)}
                          disabled={retryingItemId === game.id || !game.failed_job_item_id}
                        >
                          Retry
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        {skipped.length > 0 && (
          <Collapsible open={skippedOpen} onOpenChange={setSkippedOpen}>
            <Card>
              <CardHeader className="py-3">
                <CollapsibleTrigger className="flex w-full items-center justify-between">
                  <CardTitle className="text-base">Skipped ({skipped.length})</CardTitle>
                  <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', skippedOpen && 'rotate-180')} />
                </CollapsibleTrigger>
              </CardHeader>
              <CollapsibleContent>
                <CardContent className="p-0">
                  <Table>
                    <TableBody>
                      {skipped.map((game) => (
                        <TableRow key={game.id}>
                          <TableCell>{game.title}</TableCell>
                          <TableCell className="text-right">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => unskip(game.id)}
                              disabled={isUnskipping}
                            >
                              Unskip
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </CollapsibleContent>
            </Card>
          </Collapsible>
        )}

        {matched.length > 0 && (
          <Collapsible open={matchedOpen} onOpenChange={setMatchedOpen}>
            <Card>
              <CardHeader className="py-3">
                <CollapsibleTrigger className="flex w-full items-center justify-between">
                  <CardTitle className="text-base">Matched ({matched.length})</CardTitle>
                  <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', matchedOpen && 'rotate-180')} />
                </CollapsibleTrigger>
              </CardHeader>
              <CollapsibleContent>
                <CardContent className="p-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Storefront Title</TableHead>
                        <TableHead>IGDB Title</TableHead>
                        <TableHead />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {matched.map((game) => (
                        <TableRow key={game.id}>
                          <TableCell>{game.title}</TableCell>
                          <TableCell className="text-muted-foreground">{game.igdb_title}</TableCell>
                          <TableCell className="text-right">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setMatchingGame(game)}
                              disabled={isRematching}
                            >
                              Change Match
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </CollapsibleContent>
            </Card>
          </Collapsible>
        )}
      </div>

      {matchingGame && (
        <IGDBMatchDialog
          open
          title={`Match "${matchingGame.title}"`}
          initialQuery={matchingGame.title}
          onClose={() => setMatchingGame(null)}
          onSelect={(candidate) => handleSelect(matchingGame, candidate)}
        />
      )}

      {pendingRematch && (
        <AlertDialog open onOpenChange={(o) => { if (!o) setPendingRematch(null); }}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Storefront link will be removed</AlertDialogTitle>
              <AlertDialogDescription>
                This game's only storefront link will be removed when rematching. Do you want to
                keep it in your library (unlinked) or remove it from your collection entirely?
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel onClick={() => setPendingRematch(null)}>Cancel</AlertDialogCancel>
              <AlertDialogAction
                className="border border-input bg-background text-foreground shadow-sm hover:bg-accent hover:text-accent-foreground"
                onClick={() => {
                  rematch({ id: pendingRematch.game.id, igdbId: pendingRematch.candidate.igdb_id, orphanAction: 'keep' });
                  setPendingRematch(null);
                }}
              >
                Keep in Library
              </AlertDialogAction>
              <AlertDialogAction
                className="bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90"
                onClick={() => {
                  rematch({ id: pendingRematch.game.id, igdbId: pendingRematch.candidate.igdb_id, orphanAction: 'remove' });
                  setPendingRematch(null);
                }}
              >
                Remove from Collection
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </>
  );
}
