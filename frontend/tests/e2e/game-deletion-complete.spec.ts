import { test, expect } from '@playwright/test';
import { TestHelpers } from '../helpers/test-fixtures';

test.describe('Complete Game Deletion Workflows', () => {
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
    await searchInput.fill('Witcher 3');
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
    
    // Now navigate to games collection to find and delete the game
    await page.goto('/games');
    await page.waitForLoadState('networkidle');
    
    // Look for the game we just added
    const gameInCollection = page.locator(`text="${gameTitle}"`).first();
    
    if (await gameInCollection.isVisible({ timeout: 5000 })) {
      // Try to find and click game options/menu button
      const gameContainer = gameInCollection.locator('..').locator('..');
      
      const optionsSelectors = [
        gameContainer.getByRole('button', { name: /options/i }),
        gameContainer.getByRole('button', { name: /menu/i }),
        gameContainer.locator('[data-testid="game-options"]'),
        gameContainer.locator('button:has([class*="dots"]'), // Three dots menu
        gameContainer.locator('button:has([class*="menu"]'),
        gameContainer.locator('button:has(svg)').last() // Last button with icon (often options)
      ];
      
      let menuOpened = false;
      for (const optionsButton of optionsSelectors) {
        try {
          if (await optionsButton.isVisible({ timeout: 3000 })) {
            await optionsButton.click();
            menuOpened = true;
            break;
          }
        } catch (error) {
          continue;
        }
      }
      
      if (menuOpened) {
        // Look for delete option in dropdown
        const deleteSelectors = [
          page.getByRole('button', { name: /delete/i }),
          page.getByRole('menuitem', { name: /delete/i }),
          page.getByText('Delete'),
          page.locator('[data-testid="delete-game"]'),
          page.locator('button:has-text("Delete")'),
          page.locator('button:has([class*="trash"], [class*="delete"])')
        ];
        
        let deleteClicked = false;
        for (const deleteButton of deleteSelectors) {
          try {
            if (await deleteButton.isVisible({ timeout: 3000 })) {
              await deleteButton.click();
              deleteClicked = true;
              break;
            }
          } catch (error) {
            continue;
          }
        }
        
        if (deleteClicked) {
          // Look for confirmation dialog
          const confirmDeleteSelectors = [
            page.getByRole('button', { name: /confirm/i }),
            page.getByRole('button', { name: /yes/i }),
            page.getByRole('button', { name: /delete/i }),
            page.getByText('Confirm'),
            page.locator('[data-testid="confirm-delete"]'),
            page.locator('button:has-text("Yes")')
          ];
          
          let confirmed = false;
          for (const confirmButton of confirmDeleteSelectors) {
            try {
              if (await confirmButton.isVisible({ timeout: 5000 })) {
                await confirmButton.click();
                confirmed = true;
                break;
              }
            } catch (error) {
              continue;
            }
          }
          
          if (confirmed) {
            // Wait for deletion to complete
            await page.waitForLoadState('networkidle');
            await page.waitForTimeout(2000);
            
            // Verify game is no longer in collection
            const gameStillExists = await gameInCollection.isVisible({ timeout: 3000 }).catch(() => false);
            expect(gameStillExists).toBeFalsy();
            
            // Check for success message
            const successMessage = await Promise.race([
              page.getByText(/deleted/i).isVisible({ timeout: 3000 }).catch(() => false),
              page.getByText(/removed/i).isVisible({ timeout: 3000 }).catch(() => false)
            ]);
            
            // Success if either game is gone OR we got confirmation message
            expect(successMessage || !gameStillExists).toBeTruthy();
          } else {
            throw new Error('Could not find or click delete confirmation button');
          }
        } else {
          throw new Error('Could not find or click delete option in menu');
        }
      } else {
        // Alternative: Try right-click context menu
        await gameInCollection.click({ button: 'right' });
        await page.waitForTimeout(500);
        
        const contextDeleteButton = page.getByText('Delete');
        if (await contextDeleteButton.isVisible({ timeout: 2000 })) {
          await contextDeleteButton.click();
          
          // Handle confirmation if it appears
          const confirmButton = page.getByRole('button', { name: /confirm|yes|delete/i });
          if (await confirmButton.isVisible({ timeout: 3000 })) {
            await confirmButton.click();
          }
          
          await page.waitForLoadState('networkidle');
          await page.waitForTimeout(2000);
        } else {
          throw new Error('Could not find delete option via context menu either');
        }
      }
    } else {
      throw new Error(`Game "${gameTitle}" was not found in collection after addition`);
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
    
    // Navigate to games and try to delete, but cancel
    await page.goto('/games');
    await page.waitForLoadState('networkidle');
    
    const gameInCollection = page.locator(`text="${gameTitle}"`).first();
    
    if (await gameInCollection.isVisible({ timeout: 5000 })) {
      const gameContainer = gameInCollection.locator('..').locator('..');
      
      // Try to open options menu
      const optionsButton = gameContainer.locator('button:has(svg)').last();
      if (await optionsButton.isVisible({ timeout: 3000 })) {
        await optionsButton.click();
        
        // Click delete
        const deleteButton = page.getByRole('button', { name: /delete/i });
        if (await deleteButton.isVisible({ timeout: 3000 })) {
          await deleteButton.click();
          
          // Look for cancel button in confirmation dialog
          const cancelSelectors = [
            page.getByRole('button', { name: /cancel/i }),
            page.getByRole('button', { name: /no/i }),
            page.getByText('Cancel'),
            page.locator('[data-testid="cancel-delete"]'),
            page.locator('button:has-text("No")')
          ];
          
          let cancelled = false;
          for (const cancelButton of cancelSelectors) {
            try {
              if (await cancelButton.isVisible({ timeout: 3000 })) {
                await cancelButton.click();
                cancelled = true;
                break;
              }
            } catch (error) {
              continue;
            }
          }
          
          if (cancelled) {
            await page.waitForTimeout(1000);
            
            // Verify game is still in collection
            const gameStillExists = await gameInCollection.isVisible({ timeout: 3000 });
            expect(gameStillExists).toBeTruthy();
          } else {
            // If no cancel found, escape key as fallback
            await page.keyboard.press('Escape');
            await page.waitForTimeout(500);
            
            const gameStillExists = await gameInCollection.isVisible({ timeout: 3000 });
            expect(gameStillExists).toBeTruthy();
          }
        }
      }
    } else {
      throw new Error(`Game "${gameTitle}" was not found in collection after addition`);
    }
  });
});