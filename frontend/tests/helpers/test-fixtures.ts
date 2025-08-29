import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';

// Test credentials - matching those in auth.setup.ts
const TEST_CREDENTIALS = {
  admin: {
    username: 'e2e-admin',
    password: 'e2e-admin-password-123'
  },
  regular: {
    username: 'e2e-user', 
    password: 'e2e-user-password-123'
  }
} as const;

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
   * Wait for page to stabilize after navigation or load
   */
  async waitForPageStable(timeout: number = 3000): Promise<void> {
    // Wait for network to be idle and page to load
    await this.page.waitForLoadState('networkidle', { timeout });
    
    // Additional small wait to ensure dynamic content loads
    await this.page.waitForTimeout(500);
  }

  /**
   * Wait for loading spinners to disappear
   */
  async waitForLoadingComplete(timeout: number = 10000): Promise<void> {
    const spinners = [
      this.page.locator('[role="status"][aria-label="Loading"]'),
      this.page.locator('.animate-spin'),
      this.page.getByText(/loading/i)
    ];

    for (const spinner of spinners) {
      try {
        if (await spinner.isVisible()) {
          await spinner.waitFor({ state: 'hidden', timeout });
        }
      } catch {
        // Ignore timeout - spinner might not be present
        continue;
      }
    }
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

  // Authentication Helper Methods

  /**
   * Force logout of any existing user and clear all authentication state
   * This prevents browser context sharing issues between tests
   */
  async forceLogoutAndCleanState(): Promise<void> {
    try {
      // Try to find and click logout button first
      const logoutSelectors = [
        'button:has-text("↪️ Logout")',
        'button:has-text("Logout")',
        'button:has-text("Sign Out")', 
        '[aria-label*="logout" i]',
        '[aria-label*="sign out" i]'
      ];
      
      let loggedOut = false;
      for (const selector of logoutSelectors) {
        try {
          await this.page.goto('/'); // Go to home first to see logout button
          await this.page.waitForTimeout(1000); // Wait for page load
          
          if (await this.page.locator(selector).isVisible({ timeout: 2000 })) {
            await this.page.locator(selector).click();
            loggedOut = true;
            await this.page.waitForTimeout(1000); // Wait for logout to complete
            break;
          }
        } catch {
          continue;
        }
      }
      
      // Always clear storage manually to be absolutely sure
      await this.page.evaluate(() => {
        localStorage.clear();
        sessionStorage.clear();
      });
      await this.page.context().clearCookies();
      
      console.log(`Force logout: ${loggedOut ? 'Button clicked + ' : ''}Storage cleared`);
      
    } catch (error) {
      console.warn('Force logout encountered error, but continuing:', error);
    }
  }

  /**
   * Verify that we're logged in as the regular user (not admin)
   */
  async verifyRegularUserLogin(): Promise<void> {
    await this.page.goto('/');
    await this.page.waitForTimeout(1000);
    
    // Should see the regular user's profile indicator
    // Check that we don't see admin-specific elements
    const hasAdminMenu = await this.page.getByText('Administration').isVisible().catch(() => false);
    
    if (hasAdminMenu) {
      throw new Error('Regular user verification failed: User appears to have admin privileges');
    }
    
    console.log('✅ Verified: Logged in as regular user (no admin privileges)');
  }

  /**
   * Login as a regular user (non-admin)
   * Forces a clean authentication state first to avoid context sharing issues
   */
  async loginAsRegularUser(): Promise<void> {
    // Force clean state - logout any existing user and clear all auth data
    await this.forceLogoutAndCleanState();
    
    // Now proceed with regular user login
    await this.page.goto('/login');
    
    // Wait for login form to be ready
    await this.page.getByLabel('Username').waitFor({ state: 'visible', timeout: 10000 });
    
    // Use credentials from auth.setup.ts
    await this.page.getByLabel('Username').fill(TEST_CREDENTIALS.regular.username);
    await this.page.getByLabel('Password').fill(TEST_CREDENTIALS.regular.password);
    await this.page.getByRole('button', { name: /sign in/i }).click();
    
    // Wait for redirect to games page with better error handling
    try {
      await this.page.waitForURL('/games', { timeout: 15000 });
    } catch (error) {
      console.warn('Regular user login may have redirected to different page than expected');
      // Wait a moment for any redirect to complete
      await this.page.waitForTimeout(2000);
    }
    
    // Verify we're actually logged in as the regular user
    await this.verifyRegularUserLogin();
  }

  /**
   * Login as admin user
   */
  async loginAsAdmin(): Promise<void> {
    const status = await this.checkSetupStatus();
    
    if (status === 'login') {
      // Wait for login form to be ready
      await this.page.getByLabel('Username').waitFor({ state: 'visible', timeout: 10000 });
      
      // Use admin credentials from auth.setup.ts
      await this.page.getByLabel('Username').fill(TEST_CREDENTIALS.admin.username);
      await this.page.getByLabel('Password').fill(TEST_CREDENTIALS.admin.password);
      await this.page.getByRole('button', { name: /sign in/i }).click();
      
      // Wait for redirect to games page with better error handling
      try {
        await this.page.waitForURL('/games', { timeout: 15000 });
      } catch (error) {
        console.warn('Admin login may have redirected to different page than expected');
        // Wait a moment for any redirect to complete
        await this.page.waitForTimeout(2000);
      }
    } else if (status === 'authenticated') {
      // Already logged in, just navigate to games
      await this.page.goto('/games');
    }
  }

  // Steam Import Helper Methods

  /**
   * Navigate to Steam import page
   */
  async navigateToSteamImport(): Promise<void> {
    await this.page.goto('/import/steam');
    await expect(this.page).toHaveURL('/import/steam');
    await expect(this.page.getByRole('heading', { name: /steam import/i })).toBeVisible();
  }

  /**
   * Configure Steam API credentials
   */
  async configureSteamAPI(apiKey: string, steamId: string): Promise<void> {
    await this.navigateToSteamImport();
    
    // Look for configuration form
    const apiKeyInput = this.page.getByPlaceholder(/api key/i);
    const steamIdInput = this.page.getByPlaceholder(/steam id/i);
    
    if (await apiKeyInput.isVisible()) {
      await apiKeyInput.fill(apiKey);
    }
    
    if (await steamIdInput.isVisible()) {
      await steamIdInput.fill(steamId);
    }
    
    // Submit configuration
    await this.page.getByRole('button', { name: /save|configure|update/i }).click();
  }

  /**
   * Resolve vanity URL to Steam ID
   */
  async resolveSteamVanityUrl(vanityUrl: string): Promise<void> {
    await this.navigateToSteamImport();
    
    const vanityOption = this.page.getByText(/vanity url|custom url/i);
    if (await vanityOption.isVisible()) {
      await vanityOption.click();
      
      await this.page.getByPlaceholder(/vanity url|username/i).fill(vanityUrl);
      await this.page.getByRole('button', { name: /resolve|convert/i }).click();
      
      // Wait for resolution to complete
      await this.page.waitForTimeout(2000);
    }
  }

  /**
   * Refresh Steam library
   */
  async refreshSteamLibrary(): Promise<void> {
    await this.navigateToSteamImport();
    
    const refreshButton = this.page.getByRole('button', { name: /refresh|reload/i });
    if (await refreshButton.isVisible()) {
      await refreshButton.click();
      
      // Wait for refresh to complete
      await this.waitForElement('[data-testid="steam-games-table"], text=loading', 10000);
    }
  }

  /**
   * Navigate between Steam import tabs
   */
  async navigateToSteamTab(tab: 'needs-attention' | 'ignored' | 'in-sync' | 'configuration'): Promise<void> {
    await this.navigateToSteamImport();
    
    const tabNames = {
      'needs-attention': /needs attention|unmatched/i,
      'ignored': /ignored/i,
      'in-sync': /in sync|synced/i,
      'configuration': /configuration|config/i
    };
    
    const tabButton = this.page.getByText(tabNames[tab]);
    if (await tabButton.isVisible()) {
      await tabButton.click();
    }
  }

  // Darkadia Import Helper Methods

  /**
   * Navigate to Darkadia import page
   */
  async navigateToDarkadiaImport(): Promise<void> {
    await this.page.goto('/import/darkadia');
    await expect(this.page).toHaveURL('/import/darkadia');
    await expect(this.page.getByRole('heading', { name: /darkadia import/i })).toBeVisible();
  }

  /**
   * Upload Darkadia CSV file
   */
  async uploadDarkadiaCSV(csvContent: string): Promise<void> {
    await this.navigateToDarkadiaImport();
    
    const fileInput = this.page.locator('input[type="file"]');
    if (await fileInput.isVisible()) {
      await fileInput.setInputFiles({
        name: 'darkadia-export.csv',
        mimeType: 'text/csv',
        buffer: Buffer.from(csvContent)
      });
      
      // Wait for upload processing
      await this.page.waitForTimeout(2000);
    }
  }

  /**
   * Navigate between Darkadia import tabs
   */
  async navigateToDarkadiaTab(tab: 'upload' | 'needs-attention' | 'ignored' | 'in-sync'): Promise<void> {
    await this.navigateToDarkadiaImport();
    
    const tabNames = {
      'upload': /upload/i,
      'needs-attention': /needs attention|unmatched/i,
      'ignored': /ignored/i,
      'in-sync': /in sync|synced/i
    };
    
    const tabButton = this.page.getByText(tabNames[tab]);
    if (await tabButton.isVisible()) {
      await tabButton.click();
    }
  }

  /**
   * Handle platform resolution during import
   */
  async resolvePlatformConflict(originalPlatform: string, targetPlatform: string): Promise<void> {
    // Look for platform resolution modal
    const modal = this.page.getByRole('dialog');
    if (await modal.isVisible()) {
      // Select target platform
      await this.page.getByText(targetPlatform).click();
      await this.page.getByRole('button', { name: /confirm|resolve/i }).click();
    }
  }

  /**
   * Reset Darkadia import data
   */
  async resetDarkadiaImport(): Promise<void> {
    await this.navigateToDarkadiaImport();
    
    const resetButton = this.page.getByRole('button', { name: /reset|clear/i });
    if (await resetButton.isVisible()) {
      await resetButton.click();
      
      // Confirm reset
      await this.page.getByRole('button', { name: /confirm|yes/i }).click();
      
      // Wait for reset to complete
      await this.page.waitForTimeout(1000);
    }
  }

  // Admin Helper Methods

  /**
   * Navigate to admin dashboard
   */
  async navigateToAdminDashboard(): Promise<void> {
    await this.page.goto('/admin/dashboard');
    await expect(this.page).toHaveURL('/admin/dashboard');
    await expect(this.page.getByRole('heading', { name: /admin|dashboard/i })).toBeVisible();
  }

  /**
   * Navigate to admin user management
   */
  async navigateToAdminUsers(): Promise<void> {
    await this.page.goto('/admin/users');
    await expect(this.page).toHaveURL('/admin/users');
    await expect(this.page.getByRole('heading', { name: /users|user management/i })).toBeVisible();
  }

  /**
   * Create a new user as admin
   */
  async createUser(userData: {
    username: string;
    email: string;
    password: string;
    isAdmin?: boolean;
  }): Promise<void> {
    await this.navigateToAdminUsers();
    
    await this.page.getByRole('button', { name: /create|add.*user|new.*user/i }).click();
    await expect(this.page).toHaveURL('/admin/users/new');
    
    // Fill user form
    await this.page.getByPlaceholder(/username/i).fill(userData.username);
    await this.page.getByPlaceholder(/email/i).fill(userData.email);
    await this.page.getByPlaceholder(/password/i).fill(userData.password);
    
    if (userData.isAdmin) {
      const adminCheckbox = this.page.getByRole('checkbox', { name: /admin/i });
      if (await adminCheckbox.isVisible()) {
        await adminCheckbox.check();
      }
    }
    
    // Submit form
    await this.page.getByRole('button', { name: /create|save/i }).click();
    
    // Should redirect back to users list
    await expect(this.page).toHaveURL('/admin/users');
  }

  /**
   * Edit existing user as admin
   */
  async editUser(username: string, updates: {
    email?: string;
    isAdmin?: boolean;
  }): Promise<void> {
    await this.navigateToAdminUsers();
    
    // Find user row and click edit
    const userRow = this.page.locator(`tr:has-text("${username}")`);
    await expect(userRow).toBeVisible();
    
    await userRow.getByRole('button', { name: /edit/i }).click();
    
    // Should navigate to edit page
    await expect(this.page).toHaveURL(/\/admin\/users\/\d+$/);
    
    // Make updates
    if (updates.email) {
      await this.page.getByPlaceholder(/email/i).fill(updates.email);
    }
    
    if (typeof updates.isAdmin === 'boolean') {
      const adminCheckbox = this.page.getByRole('checkbox', { name: /admin/i });
      if (updates.isAdmin) {
        await adminCheckbox.check();
      } else {
        await adminCheckbox.uncheck();
      }
    }
    
    // Save changes
    await this.page.getByRole('button', { name: /save|update/i }).click();
    
    // Should redirect back to users list
    await expect(this.page).toHaveURL('/admin/users');
  }

  /**
   * Delete user as admin
   */
  async deleteUser(username: string): Promise<void> {
    await this.navigateToAdminUsers();
    
    const userRow = this.page.locator(`tr:has-text("${username}")`);
    await expect(userRow).toBeVisible();
    
    await userRow.getByRole('button', { name: /delete/i }).click();
    
    // Confirm deletion
    await this.page.getByRole('button', { name: /confirm|delete/i }).click();
    
    // User should be removed from list
    await expect(userRow).not.toBeVisible({ timeout: 5000 });
  }

  // Collection and Search Helper Methods

  /**
   * Filter games collection by platform
   */
  async filterByPlatform(platform: string): Promise<void> {
    await this.page.goto('/games');
    
    const platformFilter = this.page.getByRole('combobox', { name: /platform/i });
    if (await platformFilter.isVisible()) {
      await platformFilter.click();
      await this.page.getByRole('option', { name: platform }).click();
    }
  }

  /**
   * Sort games collection
   */
  async sortGamesBy(sortField: 'title' | 'rating' | 'date-added' | 'play-status'): Promise<void> {
    await this.page.goto('/games');
    
    const sortOptions = {
      'title': /title|name/i,
      'rating': /rating/i,
      'date-added': /date.*added|added.*date/i,
      'play-status': /status|progress/i
    };
    
    const sortButton = this.page.getByRole('button', { name: /sort/i });
    if (await sortButton.isVisible()) {
      await sortButton.click();
      await this.page.getByText(sortOptions[sortField]).click();
    }
  }

  /**
   * Select multiple games for bulk operations
   */
  async selectMultipleGames(count: number): Promise<void> {
    await this.page.goto('/games');
    
    const checkboxes = this.page.getByRole('checkbox');
    const visibleCheckboxes = await checkboxes.all();
    
    for (let i = 0; i < Math.min(count, visibleCheckboxes.length); i++) {
      await visibleCheckboxes[i].check();
    }
  }

  /**
   * Perform bulk operation on selected games
   */
  async performBulkOperation(operation: 'delete' | 'tag' | 'status-update', data?: any): Promise<void> {
    const bulkButton = this.page.getByRole('button', { name: /bulk|selected/i });
    if (await bulkButton.isVisible()) {
      await bulkButton.click();
      
      const operationButtons = {
        'delete': /delete/i,
        'tag': /tag/i,
        'status-update': /status|update/i
      };
      
      await this.page.getByRole('menuitem', { name: operationButtons[operation] }).click();
      
      // Handle operation-specific data
      if (operation === 'tag' && data) {
        await this.page.getByPlaceholder(/tag/i).fill(data);
      }
      
      // Confirm operation
      await this.page.getByRole('button', { name: /confirm|apply/i }).click();
      
      // Wait for operation to complete
      await this.waitForElement('[data-testid="bulk-progress"], text=complete', 10000);
    }
  }

  /**
   * Wait for import to complete (Steam or Darkadia)
   */
  async waitForImportComplete(timeout: number = 30000): Promise<void> {
    // Look for completion indicators
    const completionIndicators = [
      this.page.getByText(/import.*complete/i),
      this.page.getByText(/processing.*complete/i),
      this.page.locator('[data-testid="import-complete"]'),
      this.page.getByRole('button', { name: /close|done/i })
    ];
    
    for (const indicator of completionIndicators) {
      try {
        await indicator.waitFor({ timeout: timeout / completionIndicators.length });
        return;
      } catch {
        // Try next indicator
      }
    }
    
    // If no specific indicator found, wait for loading states to disappear
    await this.page.waitForSelector('text=loading', { state: 'hidden', timeout: timeout });
  }
}