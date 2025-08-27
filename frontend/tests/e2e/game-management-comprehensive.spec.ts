import { test, expect } from '@playwright/test';
import { TestHelpers } from '../helpers/test-fixtures';
import { TEST_ADMIN } from '../auth.setup';
import { TestGameFactory, TestScenarios, GameAssertions } from '../helpers/test-data';

test.describe('Comprehensive Game Management', () => {
  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
    // Ensure we're authenticated (depends on auth tests completing first)
    const isAuth = await helpers.isAuthenticated();
    if (!isAuth) {
      await helpers.loginAsAdmin();
    }
    
    // Reset test data counters for predictable naming
    TestGameFactory.reset();
  });

  test.describe('Game Creation Workflows', () => {
    test('should complete full manual game creation workflow', async ({ page }) => {
      const gameData = TestScenarios.MANUAL_GAME_CREATION.gameData;
      
      // Create the game using test helper
      await helpers.createTestGame(gameData);
      
      // Verify game appears in list
      await helpers.waitForGameInList(gameData.title);
      
      // Verify game data is correct
      const gameCard = page.locator(GameAssertions.gameInList(gameData.title).selector);
      await expect(gameCard).toBeVisible();
    });

    test('should handle IGDB search and selection workflow', async ({ page }) => {
      await helpers.navigateToAddGame();
      
      // Search for a game that should return results
      await helpers.searchForGame('The Witcher 3');
      
      // Should show search results (mocked)
      // In real implementation, we'd mock the IGDB API responses
      await expect(page.getByText(/search results|found/i)).toBeVisible({ timeout: 10000 });
    });

    test('should validate all form fields correctly', async ({ page }) => {
      await helpers.navigateToAddGame();
      
      // Trigger manual entry
      await helpers.searchForGame('ValidationTest');
      await expect(page.getByText(/no games found/i)).toBeVisible();
      await page.getByRole('button', { name: /add manually/i }).click();
      
      // Test each validation rule
      const validationTests = [
        {
          action: () => page.getByRole('button', { name: /add game/i }).click(),
          expectation: GameAssertions.validationMessages.REQUIRED_TITLE
        },
        {
          action: async () => {
            await page.getByLabel(/game title/i).fill('Valid Title');
            await page.getByLabel(/personal rating/i).fill('15');
            await page.getByRole('button', { name: /add game/i }).click();
          },
          expectation: GameAssertions.validationMessages.INVALID_RATING
        },
        {
          action: async () => {
            await page.getByLabel(/personal rating/i).fill('8');
            await page.getByLabel(/hours played/i).fill('-10');
            await page.getByRole('button', { name: /add game/i }).click();
          },
          expectation: GameAssertions.validationMessages.NEGATIVE_HOURS
        }
      ];

      for (const { action, expectation } of validationTests) {
        await action();
        await expect(page.getByText(expectation)).toBeVisible();
      }
    });

    test('should handle search validation correctly', async ({ page }) => {
      await helpers.navigateToAddGame();
      
      // Empty search
      await page.getByRole('button', { name: /search/i }).click();
      await expect(page.getByText(GameAssertions.validationMessages.EMPTY_SEARCH)).toBeVisible();
      
      // Too short search
      await page.getByPlaceholder(/search for a game/i).fill('a');
      await page.getByRole('button', { name: /search/i }).click();
      await expect(page.getByText(GameAssertions.validationMessages.MIN_SEARCH_LENGTH)).toBeVisible();
    });
  });

  test.describe('Game Editing and Management', () => {
    test('should edit game details successfully', async ({ page }) => {
      const originalGame = TestGameFactory.create({ 
        title: 'Game to Edit',
        personalRating: '6',
        playStatus: 'not_started',
        hoursPlayed: '0'
      });
      
      // Create the game first
      await helpers.createTestGame(originalGame);
      await helpers.waitForGameInList(originalGame.title);
      
      // Edit the game
      const updates = TestScenarios.GAME_EDITING.updates;
      await helpers.editGame(originalGame.title, updates);
      
      // Verify changes are saved (this depends on UI showing updated values)
      await helpers.viewGameDetails(originalGame.title);
      
      // Check that updated values are displayed
      if (updates.personalRating) {
        await expect(page.getByText(updates.personalRating)).toBeVisible();
      }
      if (updates.hoursPlayed) {
        await expect(page.getByText(`${updates.hoursPlayed} hours`)).toBeVisible();
      }
    });

    test('should delete game successfully', async ({ page }) => {
      const gameToDelete = TestGameFactory.create({ 
        title: 'Game to Delete'
      });
      
      // Create the game
      await helpers.createTestGame(gameToDelete);
      await helpers.waitForGameInList(gameToDelete.title);
      
      // Delete the game
      await helpers.deleteGame(gameToDelete.title);
      
      // Verify game is no longer in list
      await page.goto('/games');
      await expect(page.locator(`text=${gameToDelete.title}`)).not.toBeVisible();
    });

    test('should view game details page correctly', async ({ page }) => {
      const gameData = TestGameFactory.createCompleted({
        title: 'Detailed Game View'
      });
      
      // Create the game
      await helpers.createTestGame(gameData);
      await helpers.waitForGameInList(gameData.title);
      
      // View game details
      await helpers.viewGameDetails(gameData.title);
      
      // Verify details page shows correct information
      await expect(page.getByRole('heading', { name: gameData.title })).toBeVisible();
      
      // Check that personal data is displayed
      if (gameData.personalRating) {
        await expect(page.getByText(gameData.personalRating)).toBeVisible();
      }
      if (gameData.hoursPlayed) {
        await expect(page.getByText(`${gameData.hoursPlayed} hours`)).toBeVisible();
      }
      if (gameData.personalNotes) {
        await expect(page.getByText(gameData.personalNotes)).toBeVisible();
      }
    });
  });

  test.describe('Platform Management', () => {
    test('should add platform associations to games', async ({ page }) => {
      const gameData = TestGameFactory.create({ 
        title: 'Multi-Platform Game',
        platforms: ['PC', 'PlayStation 4', 'Xbox One']
      });
      
      await helpers.createTestGame(gameData);
      await helpers.waitForGameInList(gameData.title);
      
      // View game details to see platforms
      await helpers.viewGameDetails(gameData.title);
      
      // Verify platforms are displayed
      for (const platform of gameData.platforms || []) {
        await expect(page.getByText(platform)).toBeVisible();
      }
    });

    test('should remove platform associations', async ({ page }) => {
      const gameData = TestGameFactory.create({ 
        title: 'Platform Removal Test',
        platforms: ['PC', 'PlayStation 4']
      });
      
      await helpers.createTestGame(gameData);
      await helpers.waitForGameInList(gameData.title);
      
      // Edit game to remove a platform
      await helpers.viewGameDetails(gameData.title);
      await page.getByRole('button', { name: /edit/i }).click();
      
      // Uncheck PlayStation 4
      await page.getByRole('checkbox', { name: /playstation 4/i }).uncheck();
      await page.getByRole('button', { name: /save|update/i }).click();
      
      // Verify PlayStation 4 is no longer shown
      await expect(page.getByText('PlayStation 4')).not.toBeVisible();
      await expect(page.getByText('PC')).toBeVisible();
    });
  });

  test.describe('Loading States and Error Handling', () => {
    test('should show loading states during operations', async ({ page }) => {
      await helpers.navigateToAddGame();
      
      // Start search
      await helpers.searchForGame('Loading Test');
      
      // Should show loading state
      await expect(page.getByText(GameAssertions.loadingStates.SEARCHING)).toBeVisible();
      
      // Search button should be disabled
      await expect(page.getByRole('button', { name: /search/i })).toBeDisabled();
    });

    test('should handle form submission loading states', async ({ page }) => {
      const gameData = TestGameFactory.create({ title: 'Loading Form Test' });
      
      await helpers.navigateToAddGame();
      await helpers.searchForGame('NonExistentLoadingTest');
      await expect(page.getByText(/no games found/i)).toBeVisible();
      await page.getByRole('button', { name: /add manually/i }).click();
      
      // Fill form
      await helpers.fillManualGameForm(gameData);
      
      // Submit and check for loading state
      await page.getByRole('button', { name: /add game/i }).click();
      
      // Should show saving/loading state
      await expect(page.getByText(GameAssertions.loadingStates.SAVING)).toBeVisible();
    });

    test('should handle network errors gracefully', async ({ page }) => {
      await helpers.navigateToAddGame();
      
      // This test would require network mocking in a real scenario
      // For now, we just test the UI structure for error handling
      
      await helpers.searchForGame('Network Error Test');
      
      // Wait for potential error message
      // In implementation, this would show network error handling
      await page.waitForTimeout(2000);
    });
  });

  test.describe('Accessibility and Keyboard Navigation', () => {
    test('should support keyboard navigation throughout workflow', async ({ page }) => {
      await helpers.navigateToAddGame();
      
      // Search input should be focused
      await expect(page.getByPlaceholder(/search for a game/i)).toBeFocused();
      
      // Tab to search button
      await page.keyboard.press('Tab');
      await expect(page.getByRole('button', { name: /search/i })).toBeFocused();
      
      // Enter search term and submit with Enter
      await page.keyboard.press('Shift+Tab');
      await page.keyboard.type('Keyboard Test');
      await page.keyboard.press('Enter');
      
      // Should initiate search
      await expect(page.getByText(GameAssertions.loadingStates.SEARCHING)).toBeVisible();
    });

    test('should have proper ARIA labels and roles', async ({ page }) => {
      await helpers.navigateToAddGame();
      
      // Check form accessibility
      await expect(page.getByRole('button', { name: /search/i })).toBeVisible();
      await expect(page.getByPlaceholder(/search for a game/i)).toHaveAttribute('type', 'text');
      
      // Navigate to manual form
      await helpers.searchForGame('AccessibilityTest');
      await expect(page.getByText(/no games found/i)).toBeVisible();
      await page.getByRole('button', { name: /add manually/i }).click();
      
      // Check form labels
      await expect(page.getByLabel(/game title/i)).toBeVisible();
      await expect(page.getByLabel(/personal rating/i)).toBeVisible();
      await expect(page.getByLabel(/play status/i)).toBeVisible();
    });
  });

  test.describe('Game Collection Views', () => {
    test('should display games in grid view correctly', async ({ page }) => {
      // Create multiple games for testing views
      const games = TestGameFactory.createBatch(3, { title: 'Grid View Game' });
      
      for (const game of games) {
        await helpers.createTestGame(game);
      }
      
      await page.goto('/games');
      
      // Should be in grid view by default or switch to it
      const gridButton = page.getByRole('button', { name: /grid view/i });
      if (await gridButton.isVisible()) {
        await gridButton.click();
      }
      
      // Verify games are displayed in grid
      for (const game of games) {
        await expect(page.locator(`text=${game.title}`)).toBeVisible();
      }
    });

    test('should switch between grid and list views', async ({ page }) => {
      const gameData = TestGameFactory.create({ title: 'View Toggle Game' });
      await helpers.createTestGame(gameData);
      
      await page.goto('/games');
      
      // Test view toggles
      await page.getByRole('button', { name: /list view/i }).click();
      await expect(page.locator(`text=${gameData.title}`)).toBeVisible();
      
      await page.getByRole('button', { name: /grid view/i }).click();
      await expect(page.locator(`text=${gameData.title}`)).toBeVisible();
    });
  });
});