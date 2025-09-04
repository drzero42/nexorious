import type { PageLoad } from './$types';
import { userGames } from '$lib/stores';

export const load: PageLoad = async ({ params }) => {
  const gameId = params.id;
  
  console.log('[PAGE LOAD] Starting load function for game ID:', gameId);
  
  // Pre-load the game data during page load
  // This ensures data is available before the component renders
  try {
    console.log('[PAGE LOAD] Calling userGames.getUserGame...');
    await userGames.getUserGame(gameId);
    console.log('[PAGE LOAD] Successfully loaded game data');
  } catch (error) {
    console.error('[PAGE LOAD] Failed to load game during page load:', error);
    
    // Let the component handle the error state
    // This allows the component to show appropriate error messages
    // The store already handles and exposes the error state
  }
  
  console.log('[PAGE LOAD] Load function completed');
  return {
    gameId
  };
};