import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';

// NOTE: TEST_ADMIN is now imported from auth.setup.ts
// This ensures consistency between setup and test files

/**
 * Common test utilities for Playwright E2E tests
 */
export class TestHelpers {
  constructor(private page: Page) {}

  // NOTE: Admin setup is now handled by auth.setup.ts
  // This method is removed to avoid conflicts with global auth setup


  /**
   * Check if setup is needed by visiting the homepage
   */
  async checkSetupStatus(): Promise<'setup' | 'login' | 'authenticated'> {
    await this.page.goto('/');
    
    // Wait a moment for any redirects to complete
    await this.page.waitForTimeout(1000);
    
    const currentUrl = this.page.url();
    
    if (currentUrl.includes('/setup')) {
      return 'setup';
    } else if (currentUrl.includes('/login')) {
      return 'login';
    } else {
      return 'authenticated';
    }
  }

  /**
   * Wait for element to be visible with custom timeout
   */
  async waitForElement(selector: string, timeout: number = 5000): Promise<void> {
    await this.page.waitForSelector(selector, { state: 'visible', timeout });
  }


  /**
   * Navigate to a specific app section (requires authentication)
   */
  async navigateToSection(section: 'games' | 'dashboard' | 'profile' | 'admin'): Promise<void> {
    const sectionLinks = {
      games: 'My Games',
      dashboard: 'Dashboard', 
      profile: 'Profile',
      admin: 'Admin'
    };

    const linkText = sectionLinks[section];
    await this.page.getByRole('link', { name: linkText }).click();
    await expect(this.page).toHaveURL(new RegExp(`/${section}`));
  }


  // Game Management Helper Methods

  /**
   * Navigate to add game page
   */
  async navigateToAddGame(): Promise<void> {
    await this.page.goto('/games');
    await this.page.getByRole('button', { name: /add game/i }).click();
    await expect(this.page).toHaveURL('/games/add');
    await expect(this.page.getByRole('heading', { name: /add new game/i })).toBeVisible();
  }

  /**
   * Perform IGDB search with given query
   */
  async searchForGame(query: string): Promise<void> {
    await expect(this.page.getByPlaceholder(/search for a game/i)).toBeVisible();
    await this.page.getByPlaceholder(/search for a game/i).fill(query);
    await this.page.getByRole('button', { name: /search/i }).click();
  }

  /**
   * Fill manual game creation form
   */
  async fillManualGameForm(gameData: {
    title: string;
    description?: string;
    personalRating?: string;
    playStatus?: string;
    ownershipStatus?: string;
    hoursPlayed?: string;
    platforms?: string[];
  }): Promise<void> {
    // Fill required title
    await this.page.getByLabel(/game title/i).fill(gameData.title);

    // Fill optional description
    if (gameData.description) {
      await this.page.getByLabel(/description/i).fill(gameData.description);
    }

    // Fill personal data
    if (gameData.personalRating) {
      await this.page.getByLabel(/personal rating/i).fill(gameData.personalRating);
    }

    if (gameData.playStatus) {
      await this.page.getByLabel(/play status/i).selectOption(gameData.playStatus);
    }

    if (gameData.ownershipStatus) {
      await this.page.getByLabel(/ownership status/i).selectOption(gameData.ownershipStatus);
    }

    if (gameData.hoursPlayed) {
      await this.page.getByLabel(/hours played/i).fill(gameData.hoursPlayed);
    }

    // Select platforms
    if (gameData.platforms) {
      for (const platform of gameData.platforms) {
        await this.page.getByRole('checkbox', { name: new RegExp(platform, 'i') }).check();
      }
    }
  }

  /**
   * Submit game creation form and wait for completion
   */
  async submitGameForm(): Promise<void> {
    await this.page.getByRole('button', { name: /add game/i }).click();
    
    // Wait for success message or redirect
    try {
      await expect(this.page.getByText(/game added successfully/i)).toBeVisible({ timeout: 5000 });
    } catch {
      // If no success message, check for redirect to games list
      await expect(this.page).toHaveURL('/games', { timeout: 5000 });
    }
  }

  /**
   * Create a test game using manual entry
   */
  async createTestGame(gameData: {
    title: string;
    description?: string;
    personalRating?: string;
    playStatus?: string;
    ownershipStatus?: string;
    hoursPlayed?: string;
    platforms?: string[];
  }): Promise<void> {
    await this.navigateToAddGame();
    
    // Trigger manual entry by searching for non-existent game
    await this.searchForGame(`NonExistent_${gameData.title}`);
    await expect(this.page.getByText(/no games found/i)).toBeVisible();
    await this.page.getByRole('button', { name: /add manually/i }).click();
    
    // Fill and submit form
    await this.fillManualGameForm(gameData);
    await this.submitGameForm();
  }

  /**
   * Wait for a game to appear in the games list
   */
  async waitForGameInList(gameTitle: string, timeout: number = 10000): Promise<void> {
    await this.page.goto('/games');
    await this.page.waitForSelector(`text=${gameTitle}`, { 
      state: 'visible', 
      timeout 
    });
  }

  /**
   * Delete a game from the collection
   */
  async deleteGame(gameTitle: string): Promise<void> {
    await this.page.goto('/games');
    
    // Find game and open context menu or click delete button
    const gameCard = this.page.locator(`text=${gameTitle}`).first();
    await expect(gameCard).toBeVisible();
    
    // Look for delete button or three-dot menu
    try {
      await gameCard.locator('button[aria-label*="delete"], button[title*="delete"]').click();
    } catch {
      // Try right-click context menu
      await gameCard.click({ button: 'right' });
      await this.page.getByRole('menuitem', { name: /delete/i }).click();
    }
    
    // Confirm deletion
    await this.page.getByRole('button', { name: /confirm|delete/i }).click();
    
    // Wait for game to be removed from list
    await expect(gameCard).not.toBeVisible({ timeout: 5000 });
  }

  /**
   * Edit game details
   */
  async editGame(gameTitle: string, updates: {
    personalRating?: string;
    playStatus?: string;
    ownershipStatus?: string;
    hoursPlayed?: string;
    personalNotes?: string;
  }): Promise<void> {
    await this.page.goto('/games');
    
    // Find and click on the game to open details
    const gameCard = this.page.locator(`text=${gameTitle}`).first();
    await gameCard.click();
    
    // Should be on game details page
    await expect(this.page).toHaveURL(/\/games\/[^/]+$/);
    
    // Look for edit button
    await this.page.getByRole('button', { name: /edit/i }).click();
    
    // Update fields
    if (updates.personalRating) {
      await this.page.getByLabel(/personal rating/i).fill(updates.personalRating);
    }
    
    if (updates.playStatus) {
      await this.page.getByLabel(/play status/i).selectOption(updates.playStatus);
    }
    
    if (updates.ownershipStatus) {
      await this.page.getByLabel(/ownership status/i).selectOption(updates.ownershipStatus);
    }
    
    if (updates.hoursPlayed) {
      await this.page.getByLabel(/hours played/i).fill(updates.hoursPlayed);
    }
    
    if (updates.personalNotes) {
      await this.page.getByLabel(/personal notes|notes/i).fill(updates.personalNotes);
    }
    
    // Save changes
    await this.page.getByRole('button', { name: /save|update/i }).click();
    
    // Wait for success message
    await expect(this.page.getByText(/updated successfully/i)).toBeVisible({ timeout: 5000 });
  }

  /**
   * View game details page
   */
  async viewGameDetails(gameTitle: string): Promise<void> {
    await this.page.goto('/games');
    
    const gameCard = this.page.locator(`text=${gameTitle}`).first();
    await gameCard.click();
    
    // Should navigate to game details
    await expect(this.page).toHaveURL(/\/games\/[^/]+$/);
    await expect(this.page.getByRole('heading', { name: gameTitle })).toBeVisible();
  }
}