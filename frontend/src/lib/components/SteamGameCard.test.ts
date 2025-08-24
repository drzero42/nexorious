import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import SteamGameCard from './SteamGameCard.svelte';
import type { SteamGameResponse } from '$lib/stores/steam-games.svelte';

// Mock date to ensure consistent testing
const mockDate = new Date('2025-01-15T10:00:00Z');
vi.setSystemTime(mockDate);

describe('SteamGameCard', () => {
  const baseSteamGame: SteamGameResponse = {
    id: 'steam-game-1',
    external_id: '730',
    name: 'Counter-Strike: Global Offensive',
    igdb_id: null,
    igdb_title: null,
    game_id: null,
    user_game_id: null,
    ignored: false,
    created_at: '2025-01-10T10:00:00Z',
    updated_at: '2025-01-10T10:00:00Z'
  };

  describe('Game Display', () => {
    it('displays game name and Steam AppID', () => {
      render(SteamGameCard, { game: baseSteamGame });

      expect(screen.getByText('Counter-Strike: Global Offensive')).toBeInTheDocument();
      expect(screen.getByText('Steam ID: 730')).toBeInTheDocument();
    });

    it('displays creation date', () => {
      render(SteamGameCard, { game: baseSteamGame });

      const expectedDate = new Date('2025-01-10T10:00:00Z').toLocaleDateString();
      expect(screen.getByText(`Added: ${expectedDate}`)).toBeInTheDocument();
    });

    it('displays updated date when different from creation date', () => {
      const updatedGame = {
        ...baseSteamGame,
        updated_at: '2025-01-12T15:30:00Z'
      };

      render(SteamGameCard, { game: updatedGame });

      const createdDate = new Date('2025-01-10T10:00:00Z').toLocaleDateString();
      const updatedDate = new Date('2025-01-12T15:30:00Z').toLocaleDateString();
      
      expect(screen.getByText((content) => {
        return content.includes(`Added: ${createdDate}`) && content.includes(`Updated: ${updatedDate}`);
      })).toBeInTheDocument();
    });

    it('does not display updated date when same as creation date', () => {
      render(SteamGameCard, { game: baseSteamGame });

      expect(screen.queryByText(/Updated:/)).not.toBeInTheDocument();
    });

    it('truncates long game names appropriately', () => {
      const longNameGame = {
        ...baseSteamGame,
        name: 'A Very Long Game Name That Should Be Truncated In The Display Because It Is Too Long To Fit Properly'
      };

      const { container } = render(SteamGameCard, { game: longNameGame });

      const titleElement = container.querySelector('h3');
      expect(titleElement).toHaveClass('truncate');
    });
  });

  describe('Status Display', () => {
    it('displays unmatched status for games without IGDB ID', () => {
      render(SteamGameCard, { game: baseSteamGame });

      expect(screen.getByText('Unmatched')).toBeInTheDocument();
      expect(screen.getByText('Needs IGDB matching before sync')).toBeInTheDocument();
    });

    it('displays matched status for games with IGDB ID but no game_id', () => {
      const matchedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive'
      };

      render(SteamGameCard, { game: matchedGame });

      // Check status badge text
      expect(screen.getByText('Matched')).toBeInTheDocument();
      expect(screen.getByText('IGDB Matched')).toBeInTheDocument();
      expect(screen.getByText('Ready to sync to your collection')).toBeInTheDocument();
    });

    it('displays in collection status for synced games', () => {
      const syncedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive',
        game_id: 'user-game-1'
      };

      render(SteamGameCard, { game: syncedGame });

      // Check status badge and indicators 
      expect(screen.getAllByText('In Collection')).toHaveLength(2); // Status badge and info icon
      expect(screen.getByText('IGDB Matched')).toBeInTheDocument();
    });

    it('displays ignored status for ignored games', () => {
      const ignoredGame = {
        ...baseSteamGame,
        ignored: true
      };

      render(SteamGameCard, { game: ignoredGame });

      expect(screen.getByText('Ignored')).toBeInTheDocument();
    });

    it('applies correct CSS classes for status colors', () => {
      const { container: unmatchedContainer } = render(SteamGameCard, { game: baseSteamGame });
      expect(unmatchedContainer.querySelector('.bg-yellow-100.text-yellow-600')).toBeInTheDocument();

      const matchedGame = { ...baseSteamGame, igdb_id: 'igdb-game-1', igdb_title: 'Counter-Strike: Global Offensive' };
      const { container: matchedContainer } = render(SteamGameCard, { game: matchedGame });
      expect(matchedContainer.querySelector('.bg-blue-100.text-blue-600')).toBeInTheDocument();

      const syncedGame = { ...baseSteamGame, igdb_id: 'igdb-game-1', igdb_title: 'Counter-Strike: Global Offensive', game_id: 'user-game-1' };
      const { container: syncedContainer } = render(SteamGameCard, { game: syncedGame });
      expect(syncedContainer.querySelector('.bg-green-100.text-green-600')).toBeInTheDocument();

      const ignoredGame = { ...baseSteamGame, ignored: true };
      const { container: ignoredContainer } = render(SteamGameCard, { game: ignoredGame });
      expect(ignoredContainer.querySelector('.bg-gray-100.text-gray-600')).toBeInTheDocument();
    });
  });

  describe('Action Buttons', () => {
    it('shows match button for unmatched games when onMatch is provided', () => {
      const onMatch = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onMatch
      });

      const matchButton = screen.getByTitle('Match to IGDB game');
      expect(matchButton).toBeInTheDocument();
      expect(matchButton).toHaveTextContent('Match');
    });

    it('shows sync button for matched games when onSync is provided', () => {
      const onSync = vi.fn();
      const matchedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive'
      };

      render(SteamGameCard, {
        game: matchedGame,
        onSync
      });

      const syncButton = screen.getByTitle('Add to collection');
      expect(syncButton).toBeInTheDocument();
      expect(syncButton).toHaveTextContent('Sync');
    });

    it('shows ignore button for non-ignored games when onIgnore is provided', () => {
      const onIgnore = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onIgnore
      });

      const ignoreButton = screen.getByTitle('Mark as ignored');
      expect(ignoreButton).toBeInTheDocument();
      expect(ignoreButton).toHaveTextContent('Ignore');
    });

    it('shows unignore button for ignored games when onUnignore is provided', () => {
      const onUnignore = vi.fn();
      const ignoredGame = {
        ...baseSteamGame,
        ignored: true
      };

      render(SteamGameCard, {
        game: ignoredGame,
        onUnignore
      });

      const unignoreButton = screen.getByTitle('Remove from ignored');
      expect(unignoreButton).toBeInTheDocument();
      expect(unignoreButton).toHaveTextContent('Unignore');
    });

    it('does not show match button for matched games', () => {
      const onMatch = vi.fn();
      const matchedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive'
      };

      render(SteamGameCard, {
        game: matchedGame,
        onMatch
      });

      expect(screen.queryByTitle('Match to IGDB game')).not.toBeInTheDocument();
    });

    it('does not show sync button for unmatched games', () => {
      const onSync = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onSync
      });

      expect(screen.queryByTitle('Add to collection')).not.toBeInTheDocument();
    });

    it('does not show sync button for already synced games', () => {
      const onSync = vi.fn();
      const syncedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive',
        game_id: 'user-game-1'
      };

      render(SteamGameCard, {
        game: syncedGame,
        onSync
      });

      expect(screen.queryByTitle('Add to collection')).not.toBeInTheDocument();
    });

    it('hides all actions when showActions is false', () => {
      const callbacks = {
        onMatch: vi.fn(),
        onSync: vi.fn(),
        onIgnore: vi.fn(),
        onUnignore: vi.fn()
      };

      render(SteamGameCard, {
        game: baseSteamGame,
        showActions: false,
        ...callbacks
      });

      expect(screen.queryByTitle('Match to IGDB game')).not.toBeInTheDocument();
      expect(screen.queryByTitle('Mark as ignored')).not.toBeInTheDocument();
    });
  });

  describe('Button Interactions', () => {
    it('calls onMatch when match button is clicked', async () => {
      const user = userEvent.setup();
      const onMatch = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onMatch
      });

      const matchButton = screen.getByTitle('Match to IGDB game');
      await user.click(matchButton);

      expect(onMatch).toHaveBeenCalledTimes(1);
    });

    it('calls onSync when sync button is clicked', async () => {
      const user = userEvent.setup();
      const onSync = vi.fn();
      const matchedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive'
      };

      render(SteamGameCard, {
        game: matchedGame,
        onSync
      });

      const syncButton = screen.getByTitle('Add to collection');
      await user.click(syncButton);

      expect(onSync).toHaveBeenCalledTimes(1);
    });

    it('calls onIgnore when ignore button is clicked', async () => {
      const user = userEvent.setup();
      const onIgnore = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onIgnore
      });

      const ignoreButton = screen.getByTitle('Mark as ignored');
      await user.click(ignoreButton);

      expect(onIgnore).toHaveBeenCalledTimes(1);
    });

    it('calls onUnignore when unignore button is clicked', async () => {
      const user = userEvent.setup();
      const onUnignore = vi.fn();
      const ignoredGame = {
        ...baseSteamGame,
        ignored: true
      };

      render(SteamGameCard, {
        game: ignoredGame,
        onUnignore
      });

      const unignoreButton = screen.getByTitle('Remove from ignored');
      await user.click(unignoreButton);

      expect(onUnignore).toHaveBeenCalledTimes(1);
    });
  });

  describe('Loading States', () => {
    it('shows loading spinner and disables match button when loading', () => {
      const onMatch = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onMatch,
        isLoading: true
      });

      const matchButton = screen.getByTitle('Match to IGDB game');
      expect(matchButton).toBeDisabled();
      // Check for spinner by looking for the SVG element  
      const spinner = matchButton.querySelector('svg.animate-spin');
      expect(spinner).toBeInTheDocument();
    });

    it('shows loading spinner and disables sync button when loading', () => {
      const onSync = vi.fn();
      const matchedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive'
      };

      render(SteamGameCard, {
        game: matchedGame,
        onSync,
        isLoading: true
      });

      const syncButton = screen.getByTitle('Add to collection');
      expect(syncButton).toBeDisabled();
      expect(syncButton).toHaveClass('disabled:opacity-50');
    });

    it('shows loading spinner and disables ignore button when loading', () => {
      const onIgnore = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onIgnore,
        isLoading: true
      });

      const ignoreButton = screen.getByTitle('Mark as ignored');
      expect(ignoreButton).toBeDisabled();
    });

    it('shows loading spinner and disables unignore button when loading', () => {
      const onUnignore = vi.fn();
      const ignoredGame = {
        ...baseSteamGame,
        ignored: true
      };

      render(SteamGameCard, {
        game: ignoredGame,
        onUnignore,
        isLoading: true
      });

      const unignoreButton = screen.getByTitle('Remove from ignored');
      expect(unignoreButton).toBeDisabled();
    });

    it('enables all buttons when not loading', () => {
      const callbacks = {
        onMatch: vi.fn(),
        onIgnore: vi.fn()
      };

      render(SteamGameCard, {
        game: baseSteamGame,
        isLoading: false,
        ...callbacks
      });

      const matchButton = screen.getByTitle('Match to IGDB game');
      const ignoreButton = screen.getByTitle('Mark as ignored');

      expect(matchButton).not.toBeDisabled();
      expect(ignoreButton).not.toBeDisabled();
    });
  });

  describe('Visual Styling', () => {
    it('applies hover shadow effect to card container', () => {
      const { container } = render(SteamGameCard, { game: baseSteamGame });

      const cardElement = container.firstChild as HTMLElement;
      expect(cardElement).toHaveClass('hover:shadow-md');
      expect(cardElement).toHaveClass('transition-shadow');
    });

    it('applies primary button styling to sync button', () => {
      const onSync = vi.fn();
      const matchedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive'
      };

      render(SteamGameCard, {
        game: matchedGame,
        onSync
      });

      const syncButton = screen.getByTitle('Add to collection');
      expect(syncButton).toHaveClass('btn-primary');
    });

    it('applies secondary button styling to other buttons', () => {
      const callbacks = {
        onMatch: vi.fn(),
        onIgnore: vi.fn()
      };

      render(SteamGameCard, {
        game: baseSteamGame,
        ...callbacks
      });

      const matchButton = screen.getByTitle('Match to IGDB game');
      const ignoreButton = screen.getByTitle('Mark as ignored');

      expect(matchButton).toHaveClass('btn-secondary');
      expect(ignoreButton).toHaveClass('btn-secondary');
    });

    it('applies red hover color to ignore button', () => {
      const onIgnore = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onIgnore
      });

      const ignoreButton = screen.getByTitle('Mark as ignored');
      expect(ignoreButton).toHaveClass('hover:text-red-600');
    });
  });

  describe('Conditional Content Display', () => {
    it('shows ready to sync message only for matched but not synced games', () => {
      // Test unmatched game - should not show message
      const { unmount } = render(SteamGameCard, { game: baseSteamGame });
      expect(screen.queryByText('Ready to sync to your collection')).not.toBeInTheDocument();
      unmount();

      // Test matched but not synced - should show message
      const matchedGame = { ...baseSteamGame, igdb_id: 'igdb-game-1', igdb_title: 'Counter-Strike: Global Offensive' };
      const { unmount: unmount2 } = render(SteamGameCard, { game: matchedGame });
      expect(screen.getByText('Ready to sync to your collection')).toBeInTheDocument();
      unmount2();

      // Test already synced - should not show message
      const syncedGame = { ...baseSteamGame, igdb_id: 'igdb-game-1', igdb_title: 'Counter-Strike: Global Offensive', game_id: 'user-game-1' };
      render(SteamGameCard, { game: syncedGame });
      expect(screen.queryByText('Ready to sync to your collection')).not.toBeInTheDocument();
    });

    it('shows matching needed message only for unmatched, non-ignored games', () => {
      // Test unmatched, non-ignored - should show message
      const { unmount } = render(SteamGameCard, { game: baseSteamGame });
      expect(screen.getByText('Needs IGDB matching before sync')).toBeInTheDocument();
      unmount();

      // Test matched - should not show message
      const matchedGame = { ...baseSteamGame, igdb_id: 'igdb-game-1', igdb_title: 'Counter-Strike: Global Offensive' };
      const { unmount: unmount2 } = render(SteamGameCard, { game: matchedGame });
      expect(screen.queryByText('Needs IGDB matching before sync')).not.toBeInTheDocument();
      unmount2();

      // Test ignored - should not show message
      const ignoredGame = { ...baseSteamGame, ignored: true };
      render(SteamGameCard, { game: ignoredGame });
      expect(screen.queryByText('Needs IGDB matching before sync')).not.toBeInTheDocument();
    });

    it('conditionally displays IGDB matched indicator', () => {
      // Test without IGDB ID
      const { unmount } = render(SteamGameCard, { game: baseSteamGame });
      expect(screen.queryByText('IGDB Matched')).not.toBeInTheDocument();
      unmount();

      // Test with IGDB ID
      const matchedGame = { ...baseSteamGame, igdb_id: 'igdb-game-1', igdb_title: 'Counter-Strike: Global Offensive' };
      render(SteamGameCard, { game: matchedGame });
      expect(screen.getByText('IGDB Matched')).toBeInTheDocument();
    });

    it('conditionally displays collection indicator', () => {
      // Test not in collection
      const { unmount } = render(SteamGameCard, { game: baseSteamGame });
      expect(screen.queryByText('In Collection')).not.toBeInTheDocument();
      unmount();

      // Test in collection
      const syncedGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive',
        game_id: 'user-game-1'
      };
      render(SteamGameCard, { game: syncedGame });
      expect(screen.getAllByText('In Collection')).toHaveLength(2); // Status badge and info icon
    });
  });

  describe('Accessibility', () => {
    it('provides meaningful button titles for screen readers', () => {
      const callbacks = {
        onMatch: vi.fn(),
        onIgnore: vi.fn()
      };

      render(SteamGameCard, {
        game: baseSteamGame,
        ...callbacks
      });

      expect(screen.getByTitle('Match to IGDB game')).toBeInTheDocument();
      expect(screen.getByTitle('Mark as ignored')).toBeInTheDocument();
    });

    it('provides aria-disabled state for loading buttons', () => {
      const onMatch = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onMatch,
        isLoading: true
      });

      const matchButton = screen.getByTitle('Match to IGDB game');
      expect(matchButton).toHaveAttribute('disabled');
    });

    it('uses semantic HTML structure', () => {
      const { container } = render(SteamGameCard, { game: baseSteamGame });

      // Card should be a div container
      expect(container.firstChild).toBeInstanceOf(HTMLDivElement);

      // Game name should be in an h3 heading
      const heading = container.querySelector('h3');
      expect(heading).toBeInTheDocument();
      expect(heading).toHaveTextContent('Counter-Strike: Global Offensive');
    });
  });

  describe('Dual Title Display', () => {
    it('shows both titles when Steam and IGDB titles are different', () => {
      const differentTitlesGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive - Special Edition'
      };

      render(SteamGameCard, { game: differentTitlesGame });

      // Should show both titles with labels
      expect(screen.getByText('Counter-Strike: Global Offensive')).toBeInTheDocument();
      expect(screen.getByText('Counter-Strike: Global Offensive - Special Edition')).toBeInTheDocument();
      expect(screen.getByText('IGDB:')).toBeInTheDocument();
    });

    it('shows single title when Steam and IGDB titles are identical', () => {
      const identicalTitlesGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive' // Exactly the same as game_name
      };

      render(SteamGameCard, { game: identicalTitlesGame });

      // Should only show one title, not both
      const csgoTexts = screen.getAllByText('Counter-Strike: Global Offensive');
      expect(csgoTexts).toHaveLength(1); // Only one instance should be visible
      expect(screen.queryByText('IGDB:')).not.toBeInTheDocument();
    });

    it('shows single title when IGDB title is null', () => {
      const noIgdbTitleGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: null
      };

      render(SteamGameCard, { game: noIgdbTitleGame });

      // Should only show Steam title
      expect(screen.getByText('Counter-Strike: Global Offensive')).toBeInTheDocument();
      expect(screen.queryByText('IGDB:')).not.toBeInTheDocument();
    });

    it('shows both titles for subtle differences', () => {
      const subtleDifferenceGame = {
        ...baseSteamGame,
        igdb_id: 'igdb-game-1',
        igdb_title: 'Counter-Strike: Global Offensive™'
      };

      render(SteamGameCard, { game: subtleDifferenceGame });

      // Should show both titles even for small differences
      expect(screen.getByText('Counter-Strike: Global Offensive')).toBeInTheDocument();
      expect(screen.getByText('Counter-Strike: Global Offensive™')).toBeInTheDocument();
      expect(screen.getByText('IGDB:')).toBeInTheDocument();
    });
  });

  describe('Props Validation', () => {
    it('handles missing optional callback props gracefully', () => {
      expect(() => {
        render(SteamGameCard, {
          game: baseSteamGame,
          // No callback props provided
        });
      }).not.toThrow();

      // No action buttons should be shown without callbacks
      expect(screen.queryByTitle('Match to IGDB game')).not.toBeInTheDocument();
      expect(screen.queryByTitle('Add to collection')).not.toBeInTheDocument();
      expect(screen.queryByTitle('Mark as ignored')).not.toBeInTheDocument();
      expect(screen.queryByTitle('Remove from ignored')).not.toBeInTheDocument();
    });

    it('uses default values for optional props', () => {
      const onMatch = vi.fn();

      render(SteamGameCard, {
        game: baseSteamGame,
        onMatch
        // showActions and isLoading not provided - should use defaults
      });

      // showActions defaults to true - button should be visible
      expect(screen.getByTitle('Match to IGDB game')).toBeInTheDocument();

      // isLoading defaults to false - button should be enabled
      expect(screen.getByTitle('Match to IGDB game')).not.toBeDisabled();
    });
  });

  describe('Edge Cases', () => {
    it('handles games with special characters in name', () => {
      const specialCharGame = {
        ...baseSteamGame,
        name: 'Game with "Quotes" & Symbols <>'
      };

      render(SteamGameCard, { game: specialCharGame });

      expect(screen.getByText('Game with "Quotes" & Symbols <>')).toBeInTheDocument();
    });

    it('handles games with very high Steam AppID', () => {
      const highAppIdGame = {
        ...baseSteamGame,
        external_id: '999999999'
      };

      render(SteamGameCard, { game: highAppIdGame });

      expect(screen.getByText('Steam ID: 999999999')).toBeInTheDocument();
    });

    it('handles games with empty game name gracefully', () => {
      const emptyNameGame = {
        ...baseSteamGame,
        name: ''
      };

      render(SteamGameCard, { game: emptyNameGame });

      // Component should still render without crashing
      expect(screen.getByText('Steam ID: 730')).toBeInTheDocument();
    });

    it('handles invalid date strings gracefully', () => {
      const invalidDateGame = {
        ...baseSteamGame,
        created_at: 'invalid-date-string'
      };

      // Should not throw error
      expect(() => {
        render(SteamGameCard, { game: invalidDateGame });
      }).not.toThrow();
    });
  });
});