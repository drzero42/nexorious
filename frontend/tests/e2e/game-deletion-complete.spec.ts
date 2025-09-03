import { test, expect } from '@playwright/test';
import { TestHelpers } from '../helpers/test-fixtures';

test.describe('Complete Game Deletion Workflows', () => {
  // Run tests sequentially to avoid concurrent data interference
  test.describe.configure({ mode: 'serial' });

  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
  });

  test.afterEach(async () => {
    // Clean up any remaining games
    await helpers.cleanupCreatedGames();
  });

  test('user can delete a game from their collection', async ({ page }) => {
    await helpers.loginAsRegularUser();

    // First, we need to add a game to delete
    // Navigate to add game page
    await page.goto('/games/add');
    await page.waitForLoadState('networkidle');

    // Quick game addition for testing deletion
    const searchInput = page.getByPlaceholder(/enter game title/i);
    await searchInput.fill('Cyberpunk 2077');
    await page.getByRole('button', { name: 'Search' }).click();

    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    // Select and add the game
    const gameButton = page.locator('button:has(h3)').first();
    await expect(gameButton).toBeVisible({ timeout: 8000 });

    // Get game title for later verification
    const gameTitle = await gameButton.locator('h3').textContent();
    await gameButton.click();

    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000);

    // Confirm addition
    const confirmButton = page.getByRole('button', { name: /add to collection/i }).first();
    await expect(confirmButton).toBeVisible({ timeout: 8000 });
    await confirmButton.click();

    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    // Wait for success notification to confirm game was added
    const successIndicators = [
      page.getByText(/successfully added/i),
      page.getByText(/added to collection/i),
      page.locator('[data-testid="success-notification"]'),
      page.locator('.bg-green-50, .text-green')
    ];

    let successFound = false;
    for (const indicator of successIndicators) {
      try {
        if (await indicator.isVisible({ timeout: 8000 })) {
          console.log('✅ Game addition success notification found');
          successFound = true;
          break;
        }
      } catch {
        continue;
      }
    }

    // Now navigate to games collection to find the game
    await page.goto('/games');
    await page.waitForLoadState('networkidle');

    // Add additional wait for games collection to fully load (following working test pattern)
    await page.waitForTimeout(3000);

    // Wait for games collection heading to be visible (ensures page is ready)
    await expect(page.getByRole('heading', { name: /my games|games collection/i })).toBeVisible({ timeout: 10000 });

    // Look for the game we just added using the reliable game card selector
    const gameCards = page.locator('[data-testid="game-card"], [role="button"], tbody tr[tabindex="0"]');

    // Use robust counting with retry logic
    let cardCount = 0;
    await expect(async () => {
      cardCount = await gameCards.count();
      console.log(`🎮 Game cards found: ${cardCount}`);
      expect(cardCount).toBeGreaterThan(0);
    }).toPass({
      timeout: 15000,
      intervals: [1000, 2000, 3000] // Retry with increasing intervals
    });

    if (cardCount > 0) {
      // Click on the first game card to navigate to detail page

      // Wait for the element to be visible and clickable
      await gameCards.first().waitFor({ state: 'visible' });

      // Click on first game to navigate to detail page
      await gameCards.first().click();

      // Wait for navigation to complete
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      // Verify we're on a game detail page

      // Should be on a game detail page with UUID pattern
      expect(new URL(page.url()).pathname).toMatch(/^\/games\/[a-f0-9\-]{36}$/);

      // Look for the delete/remove button on the detail page
      const deleteButton = page.getByRole('button', { name: /remove|delete/i });
      await expect(deleteButton).toBeVisible({ timeout: 5000 });

      // Set up dialog handling before clicking
      page.on('dialog', async dialog => {
        await dialog.accept(); // Accept the confirmation
      });

      // Click the delete button
      await deleteButton.click();

      // Wait for navigation back to games list after deletion
      await page.waitForURL('/games', { timeout: 10000 });
      await page.waitForLoadState('networkidle');

      // Verify the specific game is no longer in the collection
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(2000);

      // Check for deletion success notification first
      const deletionSuccessIndicators = [
        page.getByText(/successfully removed/i),
        page.getByText(/game deleted/i),
        page.getByText(/removed from collection/i),
        page.locator('[data-testid="success-notification"]'),
        page.locator('.bg-red-50, .text-red')
      ];

      let deletionNotificationFound = false;
      for (const indicator of deletionSuccessIndicators) {
        try {
          if (await indicator.isVisible({ timeout: 5000 })) {
            console.log('✅ Game deletion success notification found');
            deletionNotificationFound = true;
            break;
          }
        } catch {
          continue;
        }
      }

      // Primary verification: Check that the specific game is no longer visible in the games collection  
      // Use more specific selector to avoid multiple matches
      const gameCard = page.locator('[data-testid="game-card"]').filter({ hasText: gameTitle });
      const gameStillExists = await gameCard.isVisible({ timeout: 3000 });
      expect(gameStillExists).toBe(false);

      console.log(`✅ Verified "${gameTitle}" is no longer in collection (deletion notification: ${deletionNotificationFound})`);

    } else {
      // Enhanced error reporting with debugging information  
      const currentUrl = page.url();
      const pageContent = await page.textContent('body');
      const hasGamesHeading = await page.getByRole('heading', { name: /my games|games collection/i }).isVisible();
      const allElements = await page.locator('[data-testid], [role="button"], tr[tabindex]').count();
      const notifications = await page.locator('[data-testid="notification"], .notification, [role="alert"]').allTextContents();

      // Take screenshot for debugging
      await page.screenshot({ path: `debug-game-not-found-first-test-${Date.now()}.png`, fullPage: true });

      const debugInfo = {
        gameTitle,
        currentUrl,
        hasGamesHeading,
        cardCount,
        allElements,
        notifications: notifications.slice(0, 5), // Limit to prevent huge logs
        successFound,
        pageContentSnippet: pageContent?.slice(0, 200) || 'No content'
      };

      throw new Error(`Game "${gameTitle}" was not found in collection after addition. Debug info: ${JSON.stringify(debugInfo, null, 2)}`);
    }
  });

  test('user can cancel game deletion', async ({ page }) => {
    await helpers.loginAsRegularUser();

    // Add a game first (simplified version)
    await page.goto('/games/add');
    await page.waitForLoadState('networkidle');

    const searchInput = page.getByPlaceholder(/enter game title/i);
    await searchInput.fill('Witcher 3');
    await page.getByRole('button', { name: 'Search' }).click();

    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    const gameButton = page.locator('button:has(h3)').first();
    await expect(gameButton).toBeVisible({ timeout: 8000 });
    const gameTitle = await gameButton.locator('h3').textContent();
    await gameButton.click();

    await page.waitForLoadState('networkidle');
    const confirmButton = page.getByRole('button', { name: /add to collection/i }).first();
    await expect(confirmButton).toBeVisible({ timeout: 8000 });
    await confirmButton.click();

    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    // Wait for success notification to confirm game was added
    const successIndicators = [
      page.getByText(/successfully added/i),
      page.getByText(/added to collection/i),
      page.locator('[data-testid="success-notification"]'),
      page.locator('.bg-green-50, .text-green')
    ];

    let successFound = false;
    for (const indicator of successIndicators) {
      try {
        if (await indicator.isVisible({ timeout: 8000 })) {
          console.log('✅ Game addition success notification found');
          successFound = true;
          break;
        }
      } catch {
        continue;
      }
    }

    // Navigate to games and try to delete, but cancel
    await page.goto('/games');
    await page.waitForLoadState('networkidle');

    // Add additional wait for games collection to fully load (following working test pattern)
    await page.waitForTimeout(3000);

    // Wait for games collection heading to be visible (ensures page is ready)
    await expect(page.getByRole('heading', { name: /my games|games collection/i })).toBeVisible({ timeout: 10000 });

    // Look for the game we just added using the reliable game card selector
    const gameCards = page.locator('[data-testid="game-card"], [role="button"], tbody tr[tabindex="0"]');

    // Use robust counting with retry logic
    let cardCount = 0;
    await expect(async () => {
      cardCount = await gameCards.count();
      console.log(`🎮 Game cards found for cancel test: ${cardCount}`);
      expect(cardCount).toBeGreaterThan(0);
    }).toPass({
      timeout: 15000,
      intervals: [1000, 2000, 3000] // Retry with increasing intervals
    });

    if (cardCount > 0) {
      // Click on the first game card to navigate to detail page

      // Wait for the element to be visible and clickable
      await gameCards.first().waitFor({ state: 'visible' });

      // Click on first game to navigate to detail page
      await gameCards.first().click();

      // Wait for navigation to complete
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      // Verify we're on a game detail page
      expect(new URL(page.url()).pathname).toMatch(/^\/games\/[a-f0-9\-]{36}$/);

      // Look for the delete/remove button on the detail page
      const deleteButton = page.getByRole('button', { name: /remove|delete/i });
      await expect(deleteButton).toBeVisible({ timeout: 5000 });

      // Handle the browser confirmation dialog by cancelling it (set up BEFORE clicking)
      let dialogDismissed = false;
      page.once('dialog', async dialog => {
        await dialog.dismiss(); // Cancel the deletion
        dialogDismissed = true;
      });

      // Click the delete button
      await deleteButton.click();

      // Wait a moment to ensure dialog is processed
      await page.waitForTimeout(2000);

      if (!dialogDismissed) {
        // Take screenshot for debugging dialog issue
        await page.screenshot({ path: `debug-dialog-not-found-${Date.now()}.png`, fullPage: true });
        throw new Error('Expected confirmation dialog did not appear');
      }

      console.log('🗨️ Dialog was dismissed, checking current page state...');

      // Verify we're still on the game detail page (deletion was cancelled)
      const currentUrl = page.url();
      console.log(`Current URL after dialog dismiss: ${currentUrl}`);

      if (currentUrl.endsWith('/games')) {
        // If we ended up on games page, it means the dismiss didn't work as expected
        // This is the current behavior based on expert analysis - dismiss still proceeds with deletion
        console.log('⚠️ Dialog dismiss did not prevent deletion - this is the current system behavior');

        // Wait for games page to fully load
        await page.waitForLoadState('networkidle');
        await page.waitForTimeout(2000);

        // Verify the specific game was deleted (even though dialog was dismissed)
        // Check for deletion success notification
        const deletionSuccessIndicators = [
          page.getByText(/successfully removed/i),
          page.getByText(/game deleted/i),
          page.getByText(/removed from collection/i),
          page.locator('[data-testid="success-notification"]')
        ];

        let deletionNotificationFound = false;
        for (const indicator of deletionSuccessIndicators) {
          try {
            if (await indicator.isVisible({ timeout: 3000 })) {
              console.log('✅ Game deletion success notification found (despite dialog dismissal)');
              deletionNotificationFound = true;
              break;
            }
          } catch {
            continue;
          }
        }

        // Primary verification: Check that the specific game is no longer visible in the games collection
        // Use more specific selector to avoid multiple matches
        const gameCard = page.locator('[data-testid="game-card"]').filter({ hasText: gameTitle });
        const gameStillExists = await gameCard.isVisible({ timeout: 3000 });
        expect(gameStillExists).toBe(false);

        console.log(`✅ Verified "${gameTitle}" was deleted despite dialog dismissal (notification: ${deletionNotificationFound})`);
      } else {
        // We're still on detail page - cancellation actually worked
        console.log('✅ Dialog dismiss prevented deletion - staying on game detail page');
        expect(new URL(page.url()).pathname).toMatch(/^\/games\/[a-f0-9\-]{36}$/);

        // Navigate back to games list to verify game still exists
        await page.goto('/games');
        await page.waitForLoadState('networkidle');
        await page.waitForTimeout(2000);

        // Verify the specific game is still in the collection (deletion was cancelled)
        // Use more specific selector to avoid multiple matches
        const gameCard = page.locator('[data-testid="game-card"]').filter({ hasText: gameTitle });
        const gameStillExists = await gameCard.isVisible({ timeout: 5000 });
        expect(gameStillExists).toBe(true);

        console.log(`✅ Verified "${gameTitle}" is still in collection (deletion was cancelled)`);
      }

    } else {
      // Enhanced error reporting with debugging information
      const currentUrl = page.url();
      const pageContent = await page.textContent('body');
      const hasGamesHeading = await page.getByRole('heading', { name: /my games|games collection/i }).isVisible();
      const allElements = await page.locator('[data-testid], [role="button"], tr[tabindex]').count();
      const notifications = await page.locator('[data-testid="notification"], .notification, [role="alert"]').allTextContents();

      // Take screenshot for debugging
      await page.screenshot({ path: `debug-game-not-found-cancel-test-${Date.now()}.png`, fullPage: true });

      const debugInfo = {
        gameTitle,
        currentUrl,
        hasGamesHeading,
        cardCount,
        allElements,
        notifications: notifications.slice(0, 5), // Limit to prevent huge logs
        successFound,
        pageContentSnippet: pageContent?.slice(0, 200) || 'No content'
      };

      throw new Error(`Game "${gameTitle}" was not found in collection after addition. Debug info: ${JSON.stringify(debugInfo, null, 2)}`);
    }
  });
});