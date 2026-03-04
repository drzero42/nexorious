import * as React from 'react';
import { Search, Loader2, Gamepad2, Calendar, Monitor } from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import { useSearchIGDB } from '@/hooks/use-games';
import type { IGDBGameCandidate } from '@/types';

// ============================================================================
// Types
// ============================================================================

export interface IGDBSearchProps {
  /** Callback when a game is selected */
  onSelect: (game: IGDBGameCandidate) => void;
  /** Whether the search is disabled */
  disabled?: boolean;
  /** Placeholder text */
  placeholder?: string;
  /** Additional class name */
  className?: string;
  /** Auto-focus the input on mount */
  autoFocus?: boolean;
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Format release date to year only.
 */
function formatReleaseYear(releaseDate?: string): string | null {
  if (!releaseDate) return null;
  const date = new Date(releaseDate);
  if (isNaN(date.getTime())) return null;
  return date.getFullYear().toString();
}

/**
 * Truncate platform list for display.
 */
function formatPlatforms(platforms: string[], maxDisplay = 3): string {
  if (platforms.length === 0) return '';
  if (platforms.length <= maxDisplay) return platforms.join(', ');
  return `${platforms.slice(0, maxDisplay).join(', ')} +${platforms.length - maxDisplay}`;
}

// ============================================================================
// Sub-Components
// ============================================================================

interface GameResultItemProps {
  game: IGDBGameCandidate;
  onSelect: () => void;
}

function GameResultItem({ game, onSelect }: GameResultItemProps) {
  const releaseYear = formatReleaseYear(game.release_date);
  const platformsDisplay = formatPlatforms(game.platforms);

  return (
    <CommandItem
      value={game.igdb_id.toString()}
      onSelect={onSelect}
      className="cursor-pointer py-3"
    >
      <div className="flex items-start gap-3 w-full">
        {/* Cover Art */}
        <div className="relative h-16 w-12 flex-shrink-0 rounded overflow-hidden bg-muted">
          {game.cover_art_url ? (
            <img
              src={game.cover_art_url}
              alt={`${game.title} cover`}
              style={{ width: '100%', height: '100%', objectFit: 'cover' }}
              loading="lazy"
            />
          ) : (
            <div className="h-full w-full flex items-center justify-center">
              <Gamepad2 className="h-6 w-6 text-muted-foreground/50" />
            </div>
          )}
        </div>

        {/* Game Info */}
        <div className="flex-1 min-w-0 space-y-1">
          <div className="font-medium text-sm truncate">{game.title}</div>

          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {releaseYear && (
              <span className="flex items-center gap-1">
                <Calendar className="h-3 w-3" />
                {releaseYear}
              </span>
            )}
            {platformsDisplay && (
              <span className="flex items-center gap-1 truncate">
                <Monitor className="h-3 w-3 flex-shrink-0" />
                <span className="truncate">{platformsDisplay}</span>
              </span>
            )}
          </div>

          {game.description && (
            <p className="text-xs text-muted-foreground line-clamp-2">
              {game.description}
            </p>
          )}
        </div>
      </div>
    </CommandItem>
  );
}

// ============================================================================
// Main Component
// ============================================================================

export function IGDBSearch({
  onSelect,
  disabled = false,
  placeholder = 'Search for a game...',
  className,
  autoFocus = false,
}: IGDBSearchProps) {
  const [query, setQuery] = React.useState('');
  const debouncedQuery = useDebounce(query, 300);

  const { data: results, isLoading, isFetching } = useSearchIGDB(debouncedQuery);

  const showLoading = isLoading || isFetching;
  const showResults = !showLoading && results && results.length > 0;
  const showEmpty = !showLoading && debouncedQuery.length >= 3 && (!results || results.length === 0);
  const showMinChars = debouncedQuery.length > 0 && debouncedQuery.length < 3;

  const handleSelect = (game: IGDBGameCandidate) => {
    onSelect(game);
  };

  return (
    <div className={cn('w-full', className)}>
      <Command shouldFilter={false} className="rounded-lg border shadow-md">
        <CommandInput
          placeholder={placeholder}
          value={query}
          onValueChange={setQuery}
          disabled={disabled}
          autoFocus={autoFocus}
        />
        <CommandList>
          {/* Loading State */}
          {showLoading && (
            <div className="flex items-center justify-center py-6">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              <span className="ml-2 text-sm text-muted-foreground">
                Searching IGDB...
              </span>
            </div>
          )}

          {/* Minimum Characters Message */}
          {showMinChars && (
            <div className="flex flex-col items-center justify-center py-6 text-muted-foreground">
              <Search className="h-8 w-8 mb-2 opacity-50" />
              <p className="text-sm">Type at least 3 characters to search</p>
            </div>
          )}

          {/* Empty State */}
          {showEmpty && (
            <CommandEmpty>
              <div className="flex flex-col items-center justify-center py-6">
                <Gamepad2 className="h-8 w-8 mb-2 text-muted-foreground/50" />
                <p className="text-sm text-muted-foreground">
                  No games found for &quot;{debouncedQuery}&quot;
                </p>
                <p className="text-xs text-muted-foreground mt-1">
                  Try a different search term
                </p>
              </div>
            </CommandEmpty>
          )}

          {/* Results */}
          {showResults && (
            <CommandGroup heading={`Found ${results.length} game${results.length !== 1 ? 's' : ''}`}>
              {results.map((game) => (
                <GameResultItem
                  key={game.igdb_id}
                  game={game}
                  onSelect={() => handleSelect(game)}
                />
              ))}
            </CommandGroup>
          )}

          {/* Initial State */}
          {!query && (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <Search className="h-10 w-10 mb-3 opacity-50" />
              <p className="text-sm font-medium">Search IGDB</p>
              <p className="text-xs mt-1">
                Find games from the Internet Game Database
              </p>
            </div>
          )}
        </CommandList>
      </Command>
    </div>
  );
}

// ============================================================================
// Hooks
// ============================================================================

/**
 * Debounce hook to delay value updates.
 */
function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = React.useState(value);

  React.useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedValue(value);
    }, delay);

    return () => {
      clearTimeout(timer);
    };
  }, [value, delay]);

  return debouncedValue;
}

// ============================================================================
// Exports
// ============================================================================

export type { IGDBGameCandidate };
