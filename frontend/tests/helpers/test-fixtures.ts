import type { Page } from '@playwright/test';

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
 * Simplified test utilities for Playwright E2E tests
 * Contains only basic utilities - complex workflows are now in dedicated E2E tests
 */
export class TestHelpers {
  constructor(private page: Page) {}

  // Store created game IDs for cleanup
  private createdGameIds: number[] = [];

  /**
   * Check if setup is needed by visiting the homepage
   */
  async checkSetupStatus(): Promise<'setup' | 'login' | 'authenticated'> {
    await this.page.goto('/');
    await this.page.waitForLoadState('networkidle');
    
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
   * Login as regular user via UI workflow
   */
  async loginAsRegularUser(): Promise<void> {
    const status = await this.checkSetupStatus();
    
    if (status === 'authenticated') {
      return; // Already logged in
    }
    
    if (status === 'setup') {
      throw new Error('Admin setup required - run auth.setup.ts first');
    }
    
    // Navigate to login page
    await this.page.goto('/login');
    await this.page.waitForLoadState('networkidle');
    
    // Fill login form
    await this.page.getByPlaceholder(/username/i).fill(TEST_CREDENTIALS.regular.username);
    await this.page.getByPlaceholder(/password/i).fill(TEST_CREDENTIALS.regular.password);
    await this.page.getByRole('button', { name: /login|sign in/i }).click();
    
    // Wait for redirect to dashboard or games
    await this.page.waitForLoadState('networkidle');
    await this.page.waitForTimeout(1000);
    
    // Verify login success
    const currentUrl = this.page.url();
    if (currentUrl.includes('/login')) {
      throw new Error('Login failed - still on login page');
    }
    
    // Additional wait to ensure auth state is fully loaded
    await this.page.waitForTimeout(500);
    console.log('✅ UI login completed for regular user');
  }

  /**
   * Login as admin user via UI workflow
   */
  async loginAsAdmin(): Promise<void> {
    const status = await this.checkSetupStatus();
    
    if (status === 'setup') {
      throw new Error('Admin setup required - run auth.setup.ts first');
    }
    
    // Navigate to login page
    await this.page.goto('/login');
    await this.page.waitForLoadState('networkidle');
    
    // Fill login form
    await this.page.getByPlaceholder(/username/i).fill(TEST_CREDENTIALS.admin.username);
    await this.page.getByPlaceholder(/password/i).fill(TEST_CREDENTIALS.admin.password);
    await this.page.getByRole('button', { name: /login|sign in/i }).click();
    
    // Wait for redirect
    await this.page.waitForLoadState('networkidle');
    await this.page.waitForTimeout(1000);
    
    // Verify login success
    const currentUrl = this.page.url();
    if (currentUrl.includes('/login')) {
      throw new Error('Admin login failed - still on login page');
    }
    
    // Additional wait to ensure auth state is fully loaded
    await this.page.waitForTimeout(500);
    console.log('✅ UI login completed for admin user');
  }

  /**
   * Track a game ID for cleanup (used by E2E tests)
   */
  trackGameForCleanup(gameId: number): void {
    this.createdGameIds.push(gameId);
  }

  /**
   * Create test data game via API (for setup only, not UI testing)
   * This is acceptable for test data setup - UI workflows should be tested in dedicated E2E tests
   */
  async createGameForTestData(gameData: {
    title: string;
    description?: string;
    personal_rating?: number;
    play_status?: string;
    ownership_status?: string;
    hours_played?: number;
    platforms?: string[];
  }): Promise<number> {
    try {
      // Get auth token from localStorage
      const authState = await this.page.evaluate(() => {
        const auth = localStorage.getItem('auth');
        return auth ? JSON.parse(auth) : null;
      });

      if (!authState?.accessToken) {
        throw new Error('No authentication token available - ensure user is logged in');
      }

      // Create the game via IGDB import API (this also adds to collection automatically)
      const gameResponse = await this.page.request.post('http://localhost:8001/api/games/igdb-import', {
        headers: {
          'Authorization': `Bearer ${authState.accessToken}`,
          'Content-Type': 'application/json'
        },
        data: {
          igdb_id: 11208, // Witcher 3 - reliable test game
          title: gameData.title,
          description: gameData.description || 'Test game description'
        }
      });

      if (!gameResponse.ok()) {
        const errorText = await gameResponse.text();
        throw new Error(`Failed to create game: ${gameResponse.status()} - ${errorText}`);
      }

      const gameResult = await gameResponse.json();
      const gameId = Number(gameResult.id);

      // Update user game data if additional fields are provided
      if (gameData.personal_rating || gameData.play_status !== 'not_started' || 
          gameData.ownership_status !== 'owned' || gameData.hours_played || 
          gameData.platforms?.length) {
        
        const updateResponse = await this.page.request.put(`http://localhost:8001/api/user-games/${gameId}`, {
          headers: {
            'Authorization': `Bearer ${authState.accessToken}`,
            'Content-Type': 'application/json'
          },
          data: {
            ownership_status: gameData.ownership_status || 'owned',
            personal_rating: gameData.personal_rating || null,
            play_status: gameData.play_status || 'not_started',
            hours_played: gameData.hours_played || 0,
            platforms: gameData.platforms?.map(p => ({ platform_id: p, is_available: true })) || []
          }
        });

        if (!updateResponse.ok()) {
          console.warn(`Failed to update user game data: ${updateResponse.status()}`);
          // Don't throw error - game was created successfully, just user data update failed
        }
      }

      // Track for cleanup
      this.createdGameIds.push(gameId);
      
      return gameId;

    } catch (error) {
      console.error('Failed to create test data game:', error);
      throw error;
    }
  }

  /**
   * Clean up all games created during tests
   */
  async cleanupCreatedGames(): Promise<void> {
    for (const gameId of this.createdGameIds) {
      await this.deleteGameViaAPI(gameId);
    }
    this.createdGameIds = [];
  }

  /**
   * Delete a game via API (for cleanup)
   */
  private async deleteGameViaAPI(gameId: number): Promise<void> {
    try {
      // Get auth token for cleanup
      const authState = await this.page.evaluate(() => {
        const auth = localStorage.getItem('auth');
        return auth ? JSON.parse(auth) : null;
      });

      if (!authState?.accessToken) {
        console.warn(`No auth token for cleanup of game: ${gameId}`);
        return;
      }

      await this.page.request.delete(`http://localhost:8001/api/user-games/${gameId}`, {
        headers: {
          'Authorization': `Bearer ${authState.accessToken}`
        }
      });
      console.log(`🗑️ Cleaned up game: ${gameId}`);
    } catch (error) {
      console.warn(`Failed to delete game ${gameId}:`, error);
      // Don't throw - cleanup should be best effort
    }
  }

  /**
   * Navigate to game details page by ID
   */
  async navigateToGameDetails(gameId: number): Promise<void> {
    await this.page.goto(`/games/${gameId}`);
    await this.page.waitForLoadState('networkidle');
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
   * Find a game card in the collection by title
   */
  async findGameInCollection(gameTitle: string) {
    await this.page.goto('/games');
    await this.page.waitForLoadState('networkidle');
    return this.page.locator(`text="${gameTitle}"`).first();
  }

  /**
   * Navigate to add game page
   */
  async navigateToAddGame(): Promise<void> {
    await this.page.goto('/games');
    await this.page.getByRole('button', { name: /add game/i }).click();
    await this.page.waitForLoadState('networkidle');
    // Verify we're on the add game page
    await this.page.waitForSelector('input[placeholder*="game title"], input[placeholder*="Enter game title"]', { 
      state: 'visible', 
      timeout: 5000 
    });
  }

  /**
   * Fast API-based login that bypasses UI interactions
   * Much faster than UI login for when we just need auth tokens
   */
  async fastApiLogin(username: string, password: string): Promise<void> {
    try {
      // Use the page context to make API request with proper CORS handling
      const response = await this.page.request.post('http://localhost:8001/api/auth/login', {
        data: {
          username: username,
          password: password
        },
        headers: {
          'Content-Type': 'application/json'
        }
      });

      if (response.ok()) {
        const loginData = await response.json();
        
        // Get user profile to complete the auth state
        const userResponse = await this.page.request.get('http://localhost:8001/api/auth/me', {
          headers: {
            'Authorization': `Bearer ${loginData.access_token}`
          }
        });

        if (userResponse.ok()) {
          const user = await userResponse.json();
          
          // Set localStorage auth state to match frontend auth store
          const authState = {
            user: {
              ...user,
              isAdmin: user.is_admin
            },
            accessToken: loginData.access_token,
            refreshToken: loginData.refresh_token,
            isLoading: false,
            error: null
          };

          // Store auth state in browser localStorage
          await this.page.evaluate((authData) => {
            localStorage.setItem('auth', JSON.stringify(authData));
          }, authState);

          console.log(`⚡ Fast API login successful for ${username}`);
          
          // Navigate to a page to trigger auth store sync and wait for frontend to process auth state
          await this.page.goto('/games');
          await this.page.waitForLoadState('networkidle');
          
          // Wait for auth-dependent UI to appear (give frontend time to read localStorage)
          await this.page.waitForTimeout(1000);
          
          // Verify frontend has processed the auth state by checking we're not redirected to login
          await this.page.waitForTimeout(500); // Small additional wait for any redirects
          const currentUrl = this.page.url();
          if (currentUrl.includes('/login')) {
            throw new Error(`Frontend auth sync failed - still redirected to login page after setting auth state`);
          }
          
          console.log(`✅ Frontend auth sync completed for ${username}`);
        } else {
          throw new Error(`Failed to get user profile: ${userResponse.status()}`);
        }
      } else {
        throw new Error(`Login failed: ${response.status()}`);
      }
    } catch (error) {
      console.error('Fast API login failed:', error);
      throw error;
    }
  }

  /**
   * Logout current user
   */
  async logout(): Promise<void> {
    // Clear auth from localStorage
    await this.page.evaluate(() => {
      localStorage.removeItem('auth');
    });
    
    // Navigate to login page
    await this.page.goto('/login');
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Force logout and clean all authentication state
   */
  async forceLogoutAndCleanState(): Promise<void> {
    // Clear cookies at context level (always works regardless of page state)
    await this.page.context().clearCookies();
    
    // Navigate to login page first to establish secure context
    await this.page.goto('/login');
    await this.page.waitForLoadState('domcontentloaded');
    
    // Now clear localStorage and sessionStorage with error handling
    try {
      await this.page.evaluate(() => {
        localStorage.clear();
        sessionStorage.clear();
      });
    } catch (error) {
      // Storage APIs may not be available in some contexts (e.g., Firefox security restrictions)
      console.warn('Storage clearing failed, but continuing:', error.message);
    }
    
    // Wait for page to be fully ready
    await this.page.waitForLoadState('networkidle');
    
    // Verify we're on login page
    const currentUrl = this.page.url();
    if (!currentUrl.includes('/login')) {
      // Force navigation if not already there
      await this.page.goto('/login', { waitUntil: 'networkidle' });
    }
  }

  /**
   * Optimized admin login that only performs auth if we're not already logged in as admin
   */
  async ensureAdminLogin(): Promise<void> {
    try {
      await this.page.goto('/games');
      await this.page.waitForLoadState('networkidle');
      
      // Check if we're on the login page (not authenticated)
      if (this.page.url().includes('/login')) {
        console.log('Not authenticated - performing fast admin API login');
        await this.fastApiLogin(TEST_CREDENTIALS.admin.username, TEST_CREDENTIALS.admin.password);
        // fastApiLogin already navigates to /games and verifies auth sync, so continue to admin verification
      }
      
      // Wait a bit for the layout to fully render and check for admin elements
      await this.page.waitForTimeout(1000);
      
      // Check multiple admin-specific elements to improve detection reliability
      const adminElements = [
        this.page.getByText('Administration').first(),
        this.page.getByText('Admin Dashboard').first(), 
        this.page.getByText('Manage Users').first(),
        this.page.getByText('Manage Platforms').first(),
        this.page.locator('[href*="/admin"]').first(),
        this.page.locator('[data-testid*="admin"]').first()
      ];
      
      let isAdmin = false;
      for (const element of adminElements) {
        try {
          if (await element.isVisible({ timeout: 2000 })) {
            isAdmin = true;
            console.log('✅ Already logged in as admin');
            break;
          }
        } catch (error) {
          continue;
        }
      }
      
      // If we don't see admin elements, we might be logged in as regular user
      if (!isAdmin) {
        console.log('Not logged in as admin - switching to admin login');
        await this.fastApiLogin(TEST_CREDENTIALS.admin.username, TEST_CREDENTIALS.admin.password);
        // After switching to admin, wait a bit more and re-check admin elements
        await this.page.waitForTimeout(1000);
        
        // Verify admin elements are now visible
        let adminVerified = false;
        for (const element of adminElements) {
          try {
            if (await element.isVisible({ timeout: 3000 })) {
              adminVerified = true;
              console.log('✅ Admin verification successful');
              break;
            }
          } catch (error) {
            continue;
          }
        }
        
        if (!adminVerified) {
          console.warn('⚠️ Admin elements not visible after login, but continuing...');
        }
      }
    } catch (error) {
      console.error('Error in ensureAdminLogin:', error);
      // Fallback to direct admin login
      await this.fastApiLogin(TEST_CREDENTIALS.admin.username, TEST_CREDENTIALS.admin.password);
    }
  }

  /**
   * Verify admin login by checking for admin-specific elements
   */
  async verifyAdminLogin(): Promise<void> {
    await this.page.goto('/games');
    await this.page.waitForLoadState('networkidle');
    await this.page.waitForTimeout(1000);

    const adminIndicators = [
      this.page.getByText('Administration'),
      this.page.getByText('Admin Dashboard'),
      this.page.getByText('Manage Users'),
      this.page.locator('[href*="/admin"]')
    ];

    let adminVerified = false;
    for (const indicator of adminIndicators) {
      try {
        if (await indicator.isVisible({ timeout: 3000 })) {
          adminVerified = true;
          break;
        }
      } catch (error) {
        continue;
      }
    }

    if (!adminVerified) {
      throw new Error('Admin verification failed - admin-specific elements not found');
    }
  }

  /**
   * Verify regular user login by ensuring we're not on login page and don't see admin elements
   */
  async verifyRegularUserLogin(): Promise<void> {
    await this.page.goto('/games');
    await this.page.waitForLoadState('networkidle');
    
    // Should not be on login page
    const currentUrl = this.page.url();
    if (currentUrl.includes('/login')) {
      throw new Error('Regular user verification failed - still on login page');
    }

    // Should not see admin-only elements
    const adminElements = [
      this.page.getByText('Administration'),
      this.page.getByText('Manage Users')
    ];

    for (const element of adminElements) {
      try {
        const isVisible = await element.isVisible({ timeout: 1000 });
        if (isVisible) {
          throw new Error('Regular user verification failed - admin elements are visible');
        }
      } catch (error) {
        // Not visible is good for regular user
        continue;
      }
    }
  }

  /**
   * Optimized regular user login that only performs auth if we're not already logged in
   */
  async ensureRegularUserLogin(): Promise<void> {
    try {
      await this.page.goto('/games');
      await this.page.waitForLoadState('networkidle');
      
      // Check if we're on the login page (not authenticated)
      if (this.page.url().includes('/login')) {
        console.log('Not authenticated - performing fast regular user API login');
        await this.fastApiLogin(TEST_CREDENTIALS.regular.username, TEST_CREDENTIALS.regular.password);
        // fastApiLogin already navigates to /games and verifies auth sync, so continue to regular user verification
      }
      
      // Check if we're already logged in as regular user (not admin)
      await this.page.waitForTimeout(1000);
      
      const adminElements = [
        this.page.getByText('Administration'),
        this.page.getByText('Admin Dashboard'),
        this.page.getByText('Manage Users')
      ];
      
      let isAdmin = false;
      for (const element of adminElements) {
        try {
          if (await element.isVisible({ timeout: 2000 })) {
            isAdmin = true;
            break;
          }
        } catch (error) {
          continue;
        }
      }
      
      // If we see admin elements, we're logged in as admin - need to switch to regular user
      if (isAdmin) {
        console.log('Logged in as admin - switching to regular user');
        await this.fastApiLogin(TEST_CREDENTIALS.regular.username, TEST_CREDENTIALS.regular.password);
        // After switching to regular user, verify admin elements are no longer visible
        await this.page.waitForTimeout(1000);
        
        // Re-check that admin elements are now hidden
        let stillAdmin = false;
        for (const element of adminElements) {
          try {
            if (await element.isVisible({ timeout: 2000 })) {
              stillAdmin = true;
              break;
            }
          } catch (error) {
            continue;
          }
        }
        
        if (stillAdmin) {
          console.warn('⚠️ Admin elements still visible after switching to regular user, but continuing...');
        } else {
          console.log('✅ Successfully switched to regular user');
        }
      } else {
        console.log('✅ Already logged in as regular user');
      }
    } catch (error) {
      console.error('Error in ensureRegularUserLogin:', error);
      // Fallback to direct regular user login
      await this.fastApiLogin(TEST_CREDENTIALS.regular.username, TEST_CREDENTIALS.regular.password);
    }
  }
}