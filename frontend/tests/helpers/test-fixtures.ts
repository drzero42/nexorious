import { Page, expect } from '@playwright/test';

/**
 * Test user credentials for E2E tests
 */
export const TEST_ADMIN = {
  username: 'e2e-admin',
  password: 'e2e-test-password-123'
} as const;

/**
 * Common test utilities for Playwright E2E tests
 */
export class TestHelpers {
  constructor(private page: Page) {}

  /**
   * Navigate to the setup page and create initial admin user
   */
  async setupInitialAdmin(): Promise<void> {
    await this.page.goto('/setup');
    
    // Wait for setup page to load
    await expect(this.page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    await expect(this.page.getByText('Let\'s set up your admin account')).toBeVisible();

    // Fill in admin credentials
    await this.page.getByLabel('Admin Username').fill(TEST_ADMIN.username);
    await this.page.getByLabel('Password', { exact: true }).fill(TEST_ADMIN.password);
    await this.page.getByLabel('Confirm Password').fill(TEST_ADMIN.password);

    // Submit form
    await this.page.getByRole('button', { name: 'Create Admin Account' }).click();

    // Wait for redirect to login page
    await expect(this.page).toHaveURL('/login');
  }

  /**
   * Login with test admin credentials
   */
  async loginAsAdmin(): Promise<void> {
    await this.page.goto('/login');
    
    // Wait for login page to load
    await expect(this.page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();

    // Fill in credentials
    await this.page.getByLabel('Username').fill(TEST_ADMIN.username);
    await this.page.getByLabel('Password').fill(TEST_ADMIN.password);

    // Submit form
    await this.page.getByRole('button', { name: 'Sign In' }).click();

    // Wait for successful login and redirect
    await expect(this.page).toHaveURL('/games');
  }

  /**
   * Complete setup and login flow in one step
   */
  async setupAndLogin(): Promise<void> {
    await this.setupInitialAdmin();
    await this.loginAsAdmin();
  }

  /**
   * Logout the current user
   */
  async logout(): Promise<void> {
    // Click user menu in sidebar
    await this.page.getByRole('button', { name: TEST_ADMIN.username }).click();
    
    // Click logout
    await this.page.getByRole('menuitem', { name: 'Logout' }).click();
    
    // Verify redirect to login page
    await expect(this.page).toHaveURL('/login');
  }

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
   * Check if current user is authenticated by looking for user menu
   */
  async isAuthenticated(): Promise<boolean> {
    try {
      await this.page.getByRole('button', { name: TEST_ADMIN.username }).waitFor({ 
        state: 'visible', 
        timeout: 2000 
      });
      return true;
    } catch {
      return false;
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

  /**
   * Clear all cookies and localStorage to reset session
   */
  async clearSession(): Promise<void> {
    await this.page.context().clearCookies();
    try {
      await this.page.evaluate(() => {
        if (typeof Storage !== 'undefined') {
          if (localStorage) localStorage.clear();
          if (sessionStorage) sessionStorage.clear();
        }
      });
    } catch (error) {
      // Ignore localStorage/sessionStorage errors in test environment
      console.warn('Could not clear storage:', error);
    }
  }
}