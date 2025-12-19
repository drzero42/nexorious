'use client';

import { useRouter } from 'next/navigation';
import { ArrowLeft } from 'lucide-react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { IGDBSearch } from '@/components/games/igdb-search';
import type { IGDBGameCandidate } from '@/types';

// Session storage key for passing game data to confirmation page
export const SELECTED_GAME_STORAGE_KEY = 'nexorious_selected_game';

export default function AddGamePage() {
  const router = useRouter();

  const handleGameSelect = (game: IGDBGameCandidate) => {
    // Store the selected game in sessionStorage for the confirmation page
    sessionStorage.setItem(SELECTED_GAME_STORAGE_KEY, JSON.stringify(game));
    // Navigate to confirmation step with the selected game's IGDB ID
    router.push(`/games/add/confirm?igdb_id=${game.igdb_id}`);
  };

  return (
    <div className="space-y-6">
      {/* Page header with back button */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link href="/games">
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

      {/* IGDB Search */}
      <div className="max-w-2xl">
        <IGDBSearch
          onSelect={handleGameSelect}
          autoFocus
          placeholder="Search for a game to add..."
        />
      </div>
    </div>
  );
}
