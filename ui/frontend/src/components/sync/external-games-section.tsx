import { useEffect, useState } from 'react';
import { ChevronDown, ExternalLink } from 'lucide-react';
import { useRouterState } from '@tanstack/react-router';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
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
import { EditionChoice } from './edition-choice';
import {
  useExternalGames,
  useSkipExternalGame,
  useUnskipExternalGame,
  useRematchExternalGame,
  useRetryFailedExternalGames,
  syncKeys,
} from '@/hooks/use-sync';
import { retryJobItem } from '@/api/jobs';
import { isOrphanActionRequired } from '@/api/sync';
import { editionLabel } from '@/lib/edition-label';
import { useQueryClient } from '@tanstack/react-query';
import type { ExternalGame, IGDBGameCandidate, SyncStorefront } from '@/types';

interface ExternalGamesSectionProps {
  storefront: SyncStorefront;
  isSyncing?: boolean;
}

/** A Rematch request: to an entry's id (all editions) or to one edition's id. */
interface RematchRequest {
  id: string;
  igdbId: number;
  onlyThisEdition: boolean;
}

/** The entry in the match dialog, and the edition chosen (null: all editions). */
interface Matching {
  game: ExternalGame;
  editionId: string | null;
}

/** A Skip or Unskip of an entry of more than one edition, waiting for the choice. */
interface EditionDecision {
  kind: 'skip' | 'unskip';
  game: ExternalGame;
}

function hasSeveralEditions(game: ExternalGame) {
  return game.editions.length > 1;
}

// GameTitle renders the storefront title, linking to the game's store page when
// a store URL is available so users can disambiguate same-named titles (e.g. the
// 1993 vs 2024 "Alone in the Dark"). Falls back to plain text otherwise. When
// another entry has the same title, its platforms and the storefront's id of
// its representative go under the title to tell the two apart.
function GameTitle({ game, sameTitle = false }: { game: ExternalGame; sameTitle?: boolean }) {
  const title = game.store_url ? (
    <a
      href={game.store_url}
      target="_blank"
      rel="noopener noreferrer"
      className="inline-flex items-center gap-1 underline-offset-2 hover:underline"
    >
      {game.title}
      <ExternalLink className="h-3 w-3 text-muted-foreground" aria-hidden />
    </a>
  ) : (
    <>{game.title}</>
  );
  if (!sameTitle) return title;
  return (
    <div>
      {title}
      <div className="text-xs text-muted-foreground">{editionLabel(game)}</div>
    </div>
  );
}

// The note a choice of one edition gets where the decision will not last: a
// game without a decision takes the oldest same-title decision when the sync
// next works on it (ADR 0008).
function editionDecisionNote(decision: EditionDecision): string | null {
  const unmatched = decision.game.resolved_igdb_id === null;
  if (decision.kind === 'unskip' && unmatched) {
    return 'This edition has no match, so the next sync skips it again while its other editions stay skipped. Find it a match to keep it apart.';
  }
  if (decision.kind === 'skip' && unmatched) {
    return 'The other editions have no match yet: when a sync next works on them, they take this skip too, unless you decide them first.';
  }
  return null;
}

interface EditionDecisionDialogProps {
  decision: EditionDecision;
  onCancel: () => void;
  onConfirm: (editionId: string | null) => void;
}

// EditionDecisionDialog asks, before a Skip or Unskip of an entry of more than
// one edition, whether it is for all editions (the default) or one alone.
function EditionDecisionDialog({ decision, onCancel, onConfirm }: EditionDecisionDialogProps) {
  const [editionId, setEditionId] = useState<string | null>(null);
  const verb = decision.kind === 'skip' ? 'Skip' : 'Unskip';
  const note = editionId !== null ? editionDecisionNote(decision) : null;
  return (
    <AlertDialog
      open
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {verb} "{decision.game.title}"
          </AlertDialogTitle>
          <AlertDialogDescription>
            This game has more than one edition on the storefront. {verb} all of them, or one alone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <EditionChoice
          name={`${decision.kind}-edition`}
          editions={decision.game.editions}
          value={editionId}
          onChange={setEditionId}
        />
        {note && <p className="text-sm text-muted-foreground">{note}</p>}
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onCancel}>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={() => onConfirm(editionId)}>{verb}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function ExternalGamesSection({ storefront, isSyncing = false }: ExternalGamesSectionProps) {
  const { data: games = [], isLoading } = useExternalGames(storefront, {
    refetchInterval: isSyncing ? 5000 : undefined,
  });
  const { mutate: skip, isPending: isSkipping } = useSkipExternalGame();
  const { mutate: unskip, isPending: isUnskipping } = useUnskipExternalGame();
  const { mutate: rematch, isPending: isRematching } = useRematchExternalGame();
  const { mutate: retryAll, isPending: isRetryingAll } = useRetryFailedExternalGames();
  const queryClient = useQueryClient();
  const hash = useRouterState({ select: (s) => s.location.hash });

  const [matching, setMatching] = useState<Matching | null>(null);
  const [pendingRematch, setPendingRematch] = useState<RematchRequest | null>(null);
  const [editionDecision, setEditionDecision] = useState<EditionDecision | null>(null);
  const [skippedOpen, setSkippedOpen] = useState(false);
  const [matchedOpen, setMatchedOpen] = useState(false);
  const [retryingItemId, setRetryingItemId] = useState<string | null>(null);

  const needsReview = games.filter((g) => g.sync_status === 'needs_review');
  const failed = games.filter((g) => g.sync_status === 'failed');
  const unmatched = games.filter((g) => g.sync_status === 'unmatched');
  const skipped = games.filter((g) => g.sync_status === 'skipped');
  const matched = games.filter((g) => g.sync_status === 'matched');
  const titleCounts = new Map<string, number>();
  for (const g of games) titleCounts.set(g.title, (titleCounts.get(g.title) ?? 0) + 1);
  const sameTitle = (g: ExternalGame) => (titleCounts.get(g.title) ?? 0) > 1;

  // Initial hash navigation fires before the games query resolves, so the
  // browser can't find #needs-review on first paint. Re-trigger the scroll
  // once the section has rendered.
  useEffect(() => {
    if (hash === 'needs-review' && needsReview.length > 0) {
      document.getElementById('needs-review')?.scrollIntoView();
    }
  }, [hash, needsReview.length]);

  if (isLoading || games.length === 0) return null;

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

  // sendRematch sends a Rematch; when the server asks what to do with a
  // library game it would leave with no storefront link, the keep-or-remove
  // question opens and the request is repeated with the answer.
  function sendRematch(req: RematchRequest, orphanAction?: 'keep' | 'remove') {
    rematch(
      {
        id: req.id,
        igdbId: req.igdbId,
        orphanAction,
        onlyThisEdition: req.onlyThisEdition || undefined,
      },
      {
        onError: (err) => {
          if (isOrphanActionRequired(err)) setPendingRematch(req);
        },
      },
    );
  }

  function handleSelect({ game, editionId }: Matching, candidate: IGDBGameCandidate) {
    setMatching(null);
    const req: RematchRequest = {
      id: editionId ?? game.id,
      igdbId: candidate.igdb_id,
      onlyThisEdition: editionId !== null,
    };
    // The entry's orphan fields are for the whole set; for one edition the
    // server says whether it would orphan a library game.
    const wouldOrphan =
      !req.onlyThisEdition && game.has_user_game && game.user_game_other_platform_count === 0;
    if (wouldOrphan) {
      setPendingRematch(req);
    } else {
      sendRematch(req);
    }
  }

  function handleSkip(game: ExternalGame) {
    if (hasSeveralEditions(game)) setEditionDecision({ kind: 'skip', game });
    else skip({ id: game.id });
  }

  function handleUnskip(game: ExternalGame) {
    if (hasSeveralEditions(game)) setEditionDecision({ kind: 'unskip', game });
    else unskip({ id: game.id });
  }

  function handleEditionDecision(decision: EditionDecision, editionId: string | null) {
    setEditionDecision(null);
    const req = { id: editionId ?? decision.game.id, onlyThisEdition: editionId !== null };
    if (decision.kind === 'skip') skip(req);
    else unskip(req);
  }

  return (
    <>
      <div className="space-y-4">
        {needsReview.length > 0 && (
          <Card id="needs-review" className="scroll-mt-4">
            <CardHeader>
              <CardTitle className="text-base">Pending Review ({needsReview.length})</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableBody>
                  {needsReview.map((game) => (
                    <TableRow key={game.id}>
                      <TableCell>
                        <GameTitle game={game} sameTitle={sameTitle(game)} />
                      </TableCell>
                      <TableCell className="text-right space-x-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setMatching({ game, editionId: null })}
                          disabled={isRematching}
                        >
                          Find Match
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleSkip(game)}
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
                      <TableCell>
                        <GameTitle game={game} sameTitle={sameTitle(game)} />
                      </TableCell>
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

        {unmatched.length > 0 && (
          <Card id="unmatched" className="scroll-mt-4">
            <CardHeader>
              <CardTitle className="text-base">Unmatched ({unmatched.length})</CardTitle>
              <p className="text-sm text-muted-foreground">
                Not matched to a game yet, for example after unskipping. Find a match to add it to
                your library.
              </p>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableBody>
                  {unmatched.map((game) => (
                    <TableRow key={game.id}>
                      <TableCell>
                        <GameTitle game={game} sameTitle={sameTitle(game)} />
                      </TableCell>
                      <TableCell className="text-right space-x-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setMatching({ game, editionId: null })}
                          disabled={isRematching}
                        >
                          Find Match
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => handleSkip(game)}
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

        {matched.length > 0 && (
          <Collapsible open={matchedOpen} onOpenChange={setMatchedOpen}>
            <Card>
              <CardHeader className="py-3">
                <CollapsibleTrigger className="flex w-full items-center justify-between">
                  <CardTitle className="text-base">Matched ({matched.length})</CardTitle>
                  <ChevronDown
                    className={cn(
                      'h-4 w-4 text-muted-foreground transition-transform',
                      matchedOpen && 'rotate-180',
                    )}
                  />
                </CollapsibleTrigger>
              </CardHeader>
              <CollapsibleContent>
                <CardContent className="p-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Storefront Title</TableHead>
                        <TableHead>IGDB Title</TableHead>
                        <TableHead>Platform</TableHead>
                        <TableHead />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {matched.map((game) => (
                        <TableRow key={game.id}>
                          <TableCell>
                            <GameTitle game={game} sameTitle={sameTitle(game)} />
                          </TableCell>
                          <TableCell className="text-muted-foreground">{game.igdb_title}</TableCell>
                          <TableCell className="text-muted-foreground">
                            {game.platforms.join(', ')}
                          </TableCell>
                          <TableCell className="text-right space-x-2">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setMatching({ game, editionId: null })}
                              disabled={isRematching}
                            >
                              Change Match
                            </Button>
                            {hasSeveralEditions(game) && (
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => handleSkip(game)}
                                disabled={isSkipping}
                              >
                                Skip
                              </Button>
                            )}
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

        {skipped.length > 0 && (
          <Collapsible open={skippedOpen} onOpenChange={setSkippedOpen}>
            <Card>
              <CardHeader className="py-3">
                <CollapsibleTrigger className="flex w-full items-center justify-between">
                  <CardTitle className="text-base">Skipped ({skipped.length})</CardTitle>
                  <ChevronDown
                    className={cn(
                      'h-4 w-4 text-muted-foreground transition-transform',
                      skippedOpen && 'rotate-180',
                    )}
                  />
                </CollapsibleTrigger>
              </CardHeader>
              <CollapsibleContent>
                <CardContent className="p-0">
                  <Table>
                    <TableBody>
                      {skipped.map((game) => (
                        <TableRow key={game.id}>
                          <TableCell>
                            <GameTitle game={game} sameTitle={sameTitle(game)} />
                          </TableCell>
                          <TableCell className="text-right">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => handleUnskip(game)}
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
      </div>

      {matching && (
        <IGDBMatchDialog
          open
          title={`Match "${matching.game.title}"`}
          initialQuery={matching.game.title}
          externalGameId={matching.editionId ?? matching.game.id}
          editions={matching.game.editions}
          editionId={matching.editionId}
          onEditionChange={(editionId) => setMatching({ ...matching, editionId })}
          onClose={() => setMatching(null)}
          onSelect={(candidate) => handleSelect(matching, candidate)}
        />
      )}

      {editionDecision && (
        <EditionDecisionDialog
          decision={editionDecision}
          onCancel={() => setEditionDecision(null)}
          onConfirm={(editionId) => handleEditionDecision(editionDecision, editionId)}
        />
      )}

      {pendingRematch && (
        <AlertDialog
          open
          onOpenChange={(o) => {
            if (!o) setPendingRematch(null);
          }}
        >
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
                  sendRematch(pendingRematch, 'keep');
                  setPendingRematch(null);
                }}
              >
                Keep in Library
              </AlertDialogAction>
              <AlertDialogAction
                className="bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90"
                onClick={() => {
                  sendRematch(pendingRematch, 'remove');
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
