<!--
  Example: Steam Components Refactored Using Generic Import Components
  
  This demonstrates how the Steam page can be refactored to use the new generic
  import components while maintaining all existing functionality.
-->

<script lang="ts">
  import { 
    ImportGameTable, 
    ImportStatsOverview, 
    ImportConfigurationSection,
    BatchProgressModal 
  } from '$lib/components';
  import { steamImportService } from '$lib/adapters/SteamImportServiceAdapter';
  import type { ImportGame } from '$lib/types/import';
  import { 
    createImportActions, 
    createImportStatCards, 
    calculateImportStats,
    filterActionsForContext,
    createSteamConfigFields
  } from '$lib/utils/import-helpers';
  import { steam } from '$lib/stores';
  
  // Example of how the Steam page could be refactored
  
  // Game data (would be loaded from the service)
  let allGames: ImportGame[] = $state([]);
  let unmatchedGames: ImportGame[] = $state([]);
  let matchedGames: ImportGame[] = $state([]);
  let ignoredGames: ImportGame[] = $state([]);
  let syncedGames: ImportGame[] = $state([]);
  
  // Stats
  const stats = $derived(calculateImportStats(allGames));
  const statCards = $derived(createImportStatCards(stats));
  
  // Actions
  const allActions = createImportActions(steamImportService);
  const unmatchedActions = $derived(filterActionsForContext(allActions, 'unmatched'));
  const matchedActions = $derived(filterActionsForContext(allActions, 'matched'));
  const ignoredActions = $derived(filterActionsForContext(allActions, 'ignored'));
  const syncedActions = $derived(filterActionsForContext(allActions, 'synced'));
  
  // Configuration
  const configFields = createSteamConfigFields();
  const steamConfig = $derived({
    hasApiKey: steam.value.config?.has_api_key || false,
    isVerified: steam.value.config?.is_verified || false,
    isConfigured: steam.value.config?.has_api_key || false,
    maskedApiKey: steam.value.config?.api_key_masked,
    steamId: steam.value.config?.steam_id,
    configuredAt: steam.value.config?.configured_at ? new Date(steam.value.config.configured_at) : undefined
  });
  
  // Search service for IGDB matching
  const searchService = {
    searchGames: async (_query: string) => {
      // This would integrate with the IGDB search functionality
      // For now, return empty results
      return [];
    },
    matchGame: async (gameId: string, searchResult: any) => {
      await steamImportService.matchGameToIGDB(gameId, searchResult.igdb_id);
    }
  };
  
  // Configuration handlers
  async function handleConfigVerify(values: Record<string, string>) {
    try {
      await steam.verify(values.apiKey, values.steamId || undefined);
      const result = steam.value.verificationResult;
      return {
        isValid: result?.is_valid || false,
        errorMessage: result?.error_message,
        userInfo: result?.steam_user_info
      };
    } catch (error) {
      return {
        isValid: false,
        errorMessage: error instanceof Error ? error.message : 'Verification failed'
      };
    }
  }
  
  async function handleConfigSave(values: Record<string, string>) {
    await steam.setConfig(values.apiKey, values.steamId || undefined);
  }
  
  async function handleConfigDelete() {
    await steam.deleteConfig();
  }
  
  async function handleRefresh() {
    // Refresh game data
    const result = await steamImportService.listGames(0, 1000);
    allGames = result.games;
    
    // Filter games by status
    unmatchedGames = allGames.filter(g => !g.igdb_id && !g.ignored);
    matchedGames = allGames.filter(g => g.igdb_id && !g.game_id && !g.ignored);
    ignoredGames = allGames.filter(g => g.ignored);
    syncedGames = allGames.filter(g => g.game_id);
  }
</script>

<!-- This shows how the Steam page could be structured using generic components -->

<div class="space-y-6">
  <!-- Stats Overview -->
  <ImportStatsOverview {statCards} />
  
  <!-- Configuration Section -->
  <ImportConfigurationSection
    sourceName="Steam"
    sourceIcon="🎮"
    configuration={steamConfig}
    fields={configFields}
    onVerify={handleConfigVerify}
    onSave={handleConfigSave}
    onDelete={handleConfigDelete}
    verificationResult={steam.value.verificationResult}
    isVerifying={steam.value.isVerifying}
    isSubmitting={steam.value.isSubmitting}
    apiKeyHelpUrl="https://steamcommunity.com/dev/apikey"
  />
  
  <!-- Game Tables -->
  <div class="space-y-6">
    <!-- Unmatched Games -->
    {#if unmatchedGames.length > 0}
      <ImportGameTable
        title="Unmatched Games"
        description="These games need to be matched to IGDB entries before they can be imported."
        icon="❓"
        games={unmatchedGames}
        actions={unmatchedActions}
        sourcePrefix="Steam"
        {searchService}
        enableInlineMatching={true}
        onRefresh={handleRefresh}
        collapsible={true}
      />
    {/if}
    
    <!-- Matched Games -->
    {#if matchedGames.length > 0}
      <ImportGameTable
        title="Matched Games"
        description="These games are matched to IGDB and ready to be added to your collection."
        icon="✅"
        games={matchedGames}
        actions={matchedActions}
        sourcePrefix="Steam"
        onRefresh={handleRefresh}
        collapsible={true}
      />
    {/if}
    
    <!-- Ignored Games -->
    {#if ignoredGames.length > 0}
      <ImportGameTable
        title="Ignored Games"
        description="These games have been marked as ignored and won't be imported to your collection."
        icon="🚫"
        games={ignoredGames}
        actions={ignoredActions}
        sourcePrefix="Steam"
        onRefresh={handleRefresh}
      />
    {/if}
    
    <!-- Synced Games -->
    {#if syncedGames.length > 0}
      <ImportGameTable
        title="Games in Collection"
        description="These Steam games have been successfully added to your main game collection."
        icon="🔥"
        games={syncedGames}
        actions={syncedActions}
        sourcePrefix="Steam"
        showGameLink={true}
        onRefresh={handleRefresh}
      />
    {/if}
  </div>
</div>

<!-- Batch Progress Modal (reused as-is) -->
<BatchProgressModal
  isOpen={false}
  onClose={() => {}}
  onCancel={() => {}}
/>

<!--
  Benefits of this refactoring:
  
  1. **Reusability**: All components can be reused for Epic Games, GOG, etc.
  2. **Consistency**: Common behavior across all import sources
  3. **Maintainability**: Generic components are easier to maintain and test
  4. **Extensibility**: Easy to add new import sources
  5. **Type Safety**: Strong typing with TypeScript interfaces
  6. **Flexibility**: Configurable actions, stats, and behaviors
  
  The refactoring maintains all existing functionality while providing a
  foundation for expanding to other game import sources.
-->