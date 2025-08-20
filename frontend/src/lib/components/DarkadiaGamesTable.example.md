# DarkadiaGamesTable Component Usage Examples

## Basic Usage

```svelte
<script>
  import { DarkadiaGamesTable } from '$lib/components';
  import { darkadia } from '$lib/stores/darkadia.svelte';
  import { onMount } from 'svelte';
  
  let games = $state([]);
  
  onMount(async () => {
    const result = await darkadia.listDarkadiaGames();
    games = result.games;
  });
  
  async function handleRefresh() {
    const result = await darkadia.listDarkadiaGames();
    games = result.games;
  }
</script>

<DarkadiaGamesTable
  title="All Imported Games"
  description="Games from your Darkadia CSV import"
  icon="🎮"
  {games}
  showMatchButton={true}
  showSyncButton={true}
  showIgnoreButton={true}
  onRefresh={handleRefresh}
/>
```

## Unmatched Games Section

```svelte
<DarkadiaGamesTable
  title="Unmatched Games"
  description="These games need IGDB matching before they can be synced"
  icon="🔍"
  games={unmatchedGames}
  emptyMessage="All games have been matched!"
  showMatchButton={true}
  showIgnoreButton={true}
  onRefresh={handleRefresh}
  collapsible={true}
  {collapsed}
  onToggleCollapse={toggleCollapsed}
/>
```

## Matched Games Section

```svelte
<DarkadiaGamesTable
  title="Matched Games"
  description="Ready to sync to your collection"
  icon="✅"
  games={matchedGames}
  emptyMessage="No matched games yet"
  showSyncButton={true}
  showUnmatchButton={true}
  showIgnoreButton={true}
  onRefresh={handleRefresh}
/>
```

## Synced Games Section

```svelte
<DarkadiaGamesTable
  title="Synced Games"
  description="Successfully added to your collection"
  icon="🔥"
  games={syncedGames}
  emptyMessage="No synced games yet"
  showGameLink={true}
  showUnsyncButton={true}
  onRefresh={handleRefresh}
/>
```

## Ignored Games Section

```svelte
<DarkadiaGamesTable
  title="Ignored Games"
  description="Games you've marked to ignore"
  icon="🚫"
  games={ignoredGames}
  emptyMessage="No ignored games"
  showUnignoreButton={true}
  showUnmatchButton={true}
  onRefresh={handleRefresh}
/>
```

## Complete Management Interface

```svelte
<script>
  import { DarkadiaGamesTable } from '$lib/components';
  import { darkadia } from '$lib/stores/darkadia.svelte';
  import { onMount } from 'svelte';
  
  let allGames = $state([]);
  let sectionStates = $state({
    unmatched: false,
    matched: false,
    ignored: true, // Start collapsed
    synced: false
  });
  
  const unmatchedGames = $derived(
    allGames.filter(g => !g.igdb_id && !g.ignored)
  );
  
  const matchedGames = $derived(
    allGames.filter(g => g.igdb_id && !g.game_id && !g.ignored)
  );
  
  const ignoredGames = $derived(
    allGames.filter(g => g.ignored)
  );
  
  const syncedGames = $derived(
    allGames.filter(g => g.game_id)
  );
  
  onMount(async () => {
    await loadGames();
  });
  
  async function loadGames() {
    const result = await darkadia.listDarkadiaGames();
    allGames = result.games;
  }
  
  function toggleSection(section) {
    sectionStates[section] = !sectionStates[section];
  }
  
  function handleGameAction(gameId, action) {
    console.log(`Game ${gameId} action: ${action}`);
    // Refresh data after action
    loadGames();
  }
</script>

<!-- Unmatched Games -->
<DarkadiaGamesTable
  title="Unmatched Games"
  description="Games that need IGDB matching"
  icon="🔍"
  games={unmatchedGames}
  showMatchButton={true}
  showIgnoreButton={true}
  onRefresh={loadGames}
  onGameAction={handleGameAction}
  collapsible={true}
  collapsed={sectionStates.unmatched}
  onToggleCollapse={() => toggleSection('unmatched')}
/>

<!-- Matched Games -->
<DarkadiaGamesTable
  title="Matched Games"
  description="Ready to sync to your collection"
  icon="✅"
  games={matchedGames}
  showSyncButton={true}
  showUnmatchButton={true}
  showIgnoreButton={true}
  onRefresh={loadGames}
  onGameAction={handleGameAction}
  collapsible={true}
  collapsed={sectionStates.matched}
  onToggleCollapse={() => toggleSection('matched')}
/>

<!-- Synced Games -->
<DarkadiaGamesTable
  title="In Your Collection"
  description="Successfully synced games"
  icon="🔥"
  games={syncedGames}
  showGameLink={true}
  showUnsyncButton={true}
  onRefresh={loadGames}
  onGameAction={handleGameAction}
  collapsible={true}
  collapsed={sectionStates.synced}
  onToggleCollapse={() => toggleSection('synced')}
/>

<!-- Ignored Games -->
<DarkadiaGamesTable
  title="Ignored Games"
  description="Games you've marked to skip"
  icon="🚫"
  games={ignoredGames}
  showUnignoreButton={true}
  showUnmatchButton={true}
  onRefresh={loadGames}
  onGameAction={handleGameAction}
  collapsible={true}
  collapsed={sectionStates.ignored}
  onToggleCollapse={() => toggleSection('ignored')}
/>
```

## Props Reference

### Required Props
- `title: string` - Section title
- `games: DarkadiaGameResponse[]` - Array of games to display

### Optional Props
- `description?: string` - Section description
- `icon?: string` - Emoji icon for the section
- `emptyMessage?: string` - Message when no games (default: "No games in this section")

### Action Button Props
- `showMatchButton?: boolean` - Show IGDB match button
- `showSyncButton?: boolean` - Show sync to collection button  
- `showIgnoreButton?: boolean` - Show ignore button
- `showUnignoreButton?: boolean` - Show unignore button
- `showUnmatchButton?: boolean` - Show unmatch button
- `showUnsyncButton?: boolean` - Show unsync button
- `showGameLink?: boolean` - Show link to game in collection

### Callback Props
- `onRefresh?: () => void` - Called after actions to refresh data
- `onGameAction?: (gameId: string, action: string) => void` - Called for custom handling

### UI Props
- `collapsible?: boolean` - Enable collapse/expand functionality
- `collapsed?: boolean` - Current collapsed state
- `onToggleCollapse?: () => void` - Callback for collapse toggle
- `isLoading?: boolean` - Show loading spinner

## Features

### Desktop View
- Full table layout with columns for Game, Status, IGDB Match, Import Date, Actions
- Hover effects and responsive design
- Inline action buttons with loading states

### Mobile View
- Card-based layout using DarkadiaGameCard component
- Touch-friendly buttons and spacing
- Status indicators with icons

### IGDB Matching
- Modal dialog with IGDBSearchWidget
- Search by game name with autocomplete
- Confirmation and cancellation handling

### Accessibility
- Proper ARIA labels and roles
- Keyboard navigation support
- Screen reader friendly status indicators
- Focus management for modal dialogs

### Error Handling
- Individual game loading states
- Confirmation dialogs for destructive actions
- Integration with UI notification system
- Graceful error recovery