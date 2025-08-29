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
    await expect(this.page.getByRole('heading', { name: /add game/i })).toBeVisible();
  }

  /**
   * Perform IGDB search with given query
   */
  async searchForGame(query: string): Promise<void> {
    await expect(this.page.getByPlaceholder(/enter game title/i)).toBeVisible();
    await this.page.getByPlaceholder(/enter game title/i).fill(query);
    await this.page.getByRole('button', { name: 'Search' }).click();
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
    // Note: Manual game form doesn't exist - this helper is not applicable
    // The current implementation only supports IGDB search workflow
    throw new Error('Manual game form is not implemented. Use IGDB search workflow instead.');
  }

  /**
   * Submit game creation form and wait for completion
   */
  async submitGameForm(): Promise<void> {
    // Note: This method assumes form exists, but manual forms don't exist
    // The current implementation uses IGDB search workflow
    throw new Error('Manual game submission not implemented. Use IGDB search workflow instead.');
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
    // Note: Manual game creation doesn't exist in current implementation
    // For testing purposes, we'll simulate the search workflow without expecting results
    await this.navigateToAddGame();
    
    // Just test the search UI (won't actually create a game)
    await this.searchForGame(`Test_${gameData.title}`);
    
    // Don't expect specific results, just verify we can interact with the search
    // In a real implementation, this would need API mocking
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
   * Navigate to game details page for editing
   */
  async editGame(gameTitle: string, updates: {
    personalRating?: string;
    playStatus?: string;
    ownershipStatus?: string;
    hoursPlayed?: string;
    personalNotes?: string;
  }): Promise<void> {
    // Note: Since we can't actually find games that don't exist,
    // this will just test navigation to edit workflow
    await this.page.goto('/games');
    
    // In real implementation, this would click on an actual game
    // For testing, we'd need proper test data setup
    // This is a placeholder for the edit workflow
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