/**
 * Adapter to bridge the existing Steam service with the generic ImportService interface.
 * 
 * This allows the Steam functionality to work with the new generic import components
 * while maintaining compatibility with the existing Steam-specific implementation.
 */

import type { ImportService, ImportGame, BulkOperationResponse, BatchSession } from '$lib/types/import';
import { steamGames, type SteamGameResponse, type SteamGameStatusFilter } from '$lib/stores/steam-games.svelte';

export class SteamImportServiceAdapter implements ImportService {
  constructor(private steamGamesService: typeof steamGames) {}

  // Convert SteamGameResponse to ImportGame
  private mapSteamGameToImportGame(steamGame: SteamGameResponse): ImportGame {
    return {
      id: steamGame.id,
      name: steamGame.name,
      external_id: steamGame.external_id,
      igdb_id: steamGame.igdb_id,
      igdb_title: steamGame.igdb_title,
      game_id: steamGame.game_id,
      user_game_id: steamGame.user_game_id,
      ignored: steamGame.ignored,
      created_at: steamGame.created_at,
      updated_at: steamGame.updated_at
    };
  }

  // Game management
  async listGames(offset: number, limit: number, status?: string, search?: string): Promise<{ games: ImportGame[]; total: number }> {
    const result = await this.steamGamesService.listSteamGames(offset, limit, status as SteamGameStatusFilter | undefined, search);
    return {
      games: result.games.map(game => this.mapSteamGameToImportGame(game)),
      total: result.total
    };
  }

  // Individual game actions
  async matchGameToIGDB(gameId: string, igdbId: string | null): Promise<void> {
    await this.steamGamesService.matchSteamGameToIGDB(gameId, igdbId);
  }

  async autoMatchSingleGame(gameId: string): Promise<void> {
    await this.steamGamesService.autoMatchSingleGame(gameId);
  }

  async syncGameToCollection(gameId: string): Promise<void> {
    await this.steamGamesService.syncSteamGameToCollection(gameId);
  }

  async toggleGameIgnored(gameId: string): Promise<void> {
    await this.steamGamesService.toggleSteamGameIgnored(gameId);
  }

  async unsyncGameFromCollection(gameId: string): Promise<void> {
    await this.steamGamesService.unsyncSteamGameFromCollection(gameId);
  }

  // Bulk operations
  async unmatchAllGames(): Promise<BulkOperationResponse> {
    return await this.steamGamesService.unmatchAllGames();
  }

  async unsyncAllGames(): Promise<BulkOperationResponse> {
    return await this.steamGamesService.unsyncAllGames();
  }

  async unignoreAllGames(): Promise<BulkOperationResponse> {
    return await this.steamGamesService.unignoreAllGames();
  }

  // Batch processing
  async startBatchAutoMatch(): Promise<{ session_id: string; total_items: number; operation_type: string; status: string; message: string }> {
    return await this.steamGamesService.startBatchAutoMatch();
  }

  async startBatchSync(): Promise<{ session_id: string; total_items: number; operation_type: string; status: string; message: string }> {
    return await this.steamGamesService.startBatchSync();
  }

  async processBatchAutoMatch(sessionId: string): Promise<void> {
    await this.steamGamesService.processBatchAutoMatch(sessionId);
  }

  async processBatchSync(sessionId: string): Promise<void> {
    await this.steamGamesService.processBatchSync(sessionId);
  }

  async cancelBatchOperation(sessionId: string): Promise<void> {
    await this.steamGamesService.cancelBatchOperation(sessionId);
  }

  clearBatchSession(): void {
    this.steamGamesService.clearBatchSession();
  }

  // Library import
  async importLibrary(): Promise<void> {
    await this.steamGamesService.importSteamLibrary();
  }

  // State getters
  get isImporting(): boolean {
    return this.steamGamesService.value.isImporting;
  }

  get isBatchProcessing(): boolean {
    return this.steamGamesService.value.isBatchProcessing;
  }

  get isUnmatchingAll(): boolean {
    return this.steamGamesService.value.isUnmatchingAll;
  }

  get isUnsyncingAll(): boolean {
    return this.steamGamesService.value.isUnsyncingAll;
  }

  get isUnignoringAll(): boolean {
    return this.steamGamesService.value.isUnignoringAll;
  }

  get activeBatchSession(): BatchSession | null {
    const steamSession = this.steamGamesService.value.activeBatchSession;
    if (!steamSession) return null;

    return {
      sessionId: steamSession.sessionId,
      operationType: steamSession.operationType as 'auto_match' | 'sync',
      totalItems: steamSession.totalItems,
      processedItems: steamSession.processedItems,
      successfulItems: steamSession.successfulItems,
      failedItems: steamSession.failedItems,
      remainingItems: steamSession.remainingItems,
      progressPercentage: steamSession.progressPercentage,
      status: steamSession.status,
      isComplete: steamSession.isComplete,
      isProcessing: steamSession.isProcessing,
      errors: steamSession.errors
    };
  }
}

// Create and export the adapter instance
export const steamImportService = new SteamImportServiceAdapter(steamGames);