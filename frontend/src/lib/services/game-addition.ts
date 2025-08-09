import { games } from '$lib/stores';
import { userGames, OwnershipStatus, PlayStatus, type UserGameCreateRequest } from '$lib/stores/user-games.svelte';
import type { IGDBGameCandidate } from '$lib/stores/games.svelte';
import { notifications } from '$lib/stores/notifications.svelte';

// Types
export interface PlatformData {
  platform_id: string;
  storefront_id: string | null;
  store_game_id: string | null;
  store_url: string | null;
  is_available: boolean;
}

// Re-export for convenience
export type { UserGameCreateRequest as AddGameRequest } from '$lib/stores/user-games.svelte';

export interface GameFormData {
  personal_rating: number | null;
  play_status: string;
  hours_played: number;
  personal_notes: string;
  ownership_status: string;
  is_loved: boolean;
  title: string;
  description: string;
  release_date: string;
  cover_art_url: string;
}

export interface GameAdditionResult {
  success: boolean;
  game?: any;
  partialErrors: string[];
}

/**
 * Service for handling game addition workflow
 * Separates complex game addition logic from UI components
 */
export class GameAdditionService {
  private static instance: GameAdditionService;

  static getInstance(): GameAdditionService {
    if (!GameAdditionService.instance) {
      GameAdditionService.instance = new GameAdditionService();
    }
    return GameAdditionService.instance;
  }

  /**
   * Import game from IGDB
   */
  async importFromIGDB(igdbId: string): Promise<any> {
    try {
      const createdGame = await games.createFromIGDB(igdbId, {});
      notifications.showSuccess(`Adding "${createdGame.title}" to your collection`);
      return createdGame;
    } catch (error) {
      console.error('Failed to create game:', error);
      notifications.showError('Failed to import game from IGDB. Please try a different search or contact support.');
      throw error;
    }
  }

  /**
   * Transform platform selection data into API format
   */
  transformPlatformData(
    selectedPlatforms: Set<string>,
    platformStorefronts: Map<string, Set<string>>,
    platformStoreUrls: Map<string, string>
  ): PlatformData[] {
    const platformData: PlatformData[] = [];
    
    for (const platformId of selectedPlatforms) {
      const storefronts = platformStorefronts.get(platformId) || new Set<string>();
      
      if (storefronts.size === 0) {
        // No storefronts selected for this platform
        platformData.push({
          platform_id: platformId,
          storefront_id: null,
          store_game_id: null,
          store_url: platformStoreUrls.get(platformId) || null,
          is_available: true
        });
      } else {
        // Create an entry for each selected storefront
        for (const storefrontId of storefronts) {
          platformData.push({
            platform_id: platformId,
            storefront_id: storefrontId,
            store_game_id: null,
            store_url: platformStoreUrls.get(platformId) || null,
            is_available: true
          });
        }
      }
    }
    
    return platformData;
  }

  /**
   * Add game to user's collection with platform associations
   */
  async addToCollection(
    createdGame: any,
    gameData: GameFormData,
    platformData: PlatformData[]
  ): Promise<any> {
    const addRequest: UserGameCreateRequest = {
      game_id: createdGame.id,
      ownership_status: gameData.ownership_status as OwnershipStatus || OwnershipStatus.OWNED,
      ...(platformData.length > 0 && { platforms: platformData.map(p => p.platform_id) })
    };
    
    try {
      const userGame = await userGames.addGameToCollection(addRequest);
      return userGame;
    } catch (error) {
      console.error('Failed to add game to collection:', error);
      notifications.showError('Game was imported but couldn\'t be added to your collection. Please try again or contact support.');
      throw error;
    }
  }

  /**
   * Update game progress if provided
   */
  async updateGameProgress(
    userGameId: string,
    gameData: GameFormData
  ): Promise<string | null> {
    if (gameData.play_status === 'not_started' && gameData.hours_played === 0 && !gameData.personal_notes) {
      return null; // No progress to update
    }

    try {
      await userGames.updateProgress(userGameId, {
        play_status: gameData.play_status as PlayStatus || PlayStatus.NOT_STARTED,
        hours_played: gameData.hours_played || 0,
        personal_notes: gameData.personal_notes || ''
      });
      return null; // Success
    } catch (error) {
      console.error('Failed to update progress, but game was added to collection:', error);
      return 'Failed to save progress information';
    }
  }

  /**
   * Update game details (rating and loved status)
   */
  async updateGameDetails(
    userGameId: string,
    gameData: GameFormData
  ): Promise<string | null> {
    if (!gameData.personal_rating && !gameData.is_loved) {
      return null; // No details to update
    }

    try {
      const updateData: Partial<{ personal_rating: number; is_loved: boolean }> = {
        is_loved: gameData.is_loved || false
      };
      
      // Only include personal_rating if it has a value
      if (gameData.personal_rating) {
        updateData.personal_rating = gameData.personal_rating;
      }
      
      await userGames.updateUserGame(userGameId, updateData);
      return null; // Success
    } catch (error) {
      console.error('Failed to update game details, but game was added to collection:', error);
      return 'Failed to save rating and favorite status';
    }
  }

  /**
   * Complete game addition workflow
   */
  async addGameComplete(
    selectedGame: IGDBGameCandidate,
    gameData: GameFormData,
    selectedPlatforms: Set<string>,
    platformStorefronts: Map<string, Set<string>>,
    platformStoreUrls: Map<string, string>
  ): Promise<GameAdditionResult> {
    const partialErrors: string[] = [];

    try {
      // Step 1: Import game from IGDB
      const createdGame = await this.importFromIGDB(selectedGame.igdb_id);

      // Step 2: Transform platform data
      const platformData = this.transformPlatformData(
        selectedPlatforms,
        platformStorefronts,
        platformStoreUrls
      );

      // Step 3: Add to collection
      const userGame = await this.addToCollection(createdGame, gameData, platformData);

      // Step 4: Update progress (optional)
      const progressError = await this.updateGameProgress(userGame.id, gameData);
      if (progressError) partialErrors.push(progressError);

      // Step 5: Update details (optional)
      const detailsError = await this.updateGameDetails(userGame.id, gameData);
      if (detailsError) partialErrors.push(detailsError);

      // Step 6: Show appropriate success message
      this.showCompletionMessage(createdGame, partialErrors);

      return {
        success: true,
        game: createdGame,
        partialErrors
      };

    } catch (error) {
      // If we get here, either IGDB import or collection addition failed
      return {
        success: false,
        partialErrors: []
      };
    }
  }

  /**
   * Show appropriate completion message
   */
  private showCompletionMessage(createdGame: any, partialErrors: string[]): void {
    if (partialErrors.length > 0) {
      notifications.showWarning(
        `"${createdGame.title}" added to collection, but some details couldn't be saved: ${partialErrors.join(', ')}`
      );
    } else {
      notifications.showSuccess(
        `"${createdGame.title}" successfully added to your collection!`
      );
    }
  }
}

// Export singleton instance
export const gameAdditionService = GameAdditionService.getInstance();