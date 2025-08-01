import { test, expect } from '@playwright/test';
import { TestHelpers, TEST_ADMIN } from '../helpers/test-fixtures';

test.describe('Homepage', () => {
  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
    // Don't clear session - we want to inherit auth from previous tests
  });

  test.describe('Unauthenticated Homepage', () => {
    test('should redirect to login when not authenticated', async ({ page }) => {
      // Clear session for this specific test
      await helpers.clearSession();
      await page.goto('/');
      
      // Should redirect to login page since admin exists but user not authenticated
      await expect(page).toHaveURL('/login');
      await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
    });
  });

  test.describe('Authenticated Homepage', () => {
    test.beforeEach(async ({ page }) => {
      // Admin should already exist, just ensure we're logged in
      // Auth session should be inherited from auth tests, but login if needed
      if (!(await helpers.isAuthenticated())) {
        await helpers.loginAsAdmin();
      }
    });

    test('should display welcome message and user-specific content', async ({ page }) => {
      await page.goto('/');
      
      // Check main heading is still present
      await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
      
      // Check for authenticated user welcome message
      await expect(page.getByText(`Welcome back, ${TEST_ADMIN.username}!`)).toBeVisible();
      await expect(page.getByText('Ready to manage your game collection?')).toBeVisible();
      
      // Check for user avatar/initials
      const userAvatar = page.locator(`text=${TEST_ADMIN.username.charAt(0).toUpperCase()}`).first();
      await expect(userAvatar).toBeVisible();
    });

    test('should display quick action cards for authenticated users', async ({ page }) => {
      await page.goto('/');
      
      // Check quick action cards
      await expect(page.getByRole('link', { name: /My Games/ })).toBeVisible();
      await expect(page.getByRole('link', { name: /Add Game/ })).toBeVisible();
      await expect(page.getByRole('link', { name: /Dashboard/ })).toBeVisible();
      
      // Check card descriptions
      await expect(page.getByText('View your collection')).toBeVisible();
      await expect(page.getByText('Expand your library')).toBeVisible();
      await expect(page.getByText('View statistics')).toBeVisible();
    });

    test('should display feature information for authenticated users', async ({ page }) => {
      await page.goto('/');
      
      // Verify features section is present with correct content
      await expect(page.getByText('Organize Your Library')).toBeVisible();
      await expect(page.getByText('Track Progress')).toBeVisible();
      await expect(page.getByText('Self-Hosted Privacy')).toBeVisible();
      
      // Check feature descriptions
      await expect(page.getByText('Keep track of all your games across multiple platforms and storefronts')).toBeVisible();
      await expect(page.getByText('Monitor your gaming progress with detailed completion levels')).toBeVisible();
      await expect(page.getByText('Keep your gaming data private and secure with complete control')).toBeVisible();
    });

    test('should have working navigation links', async ({ page }) => {
      await page.goto('/');
      
      // Test My Games link
      await page.getByRole('link', { name: /My Games/ }).click();
      await expect(page).toHaveURL('/games');
      
      // Go back to homepage
      await page.goto('/');
      
      // Test Add Game link
      await page.getByRole('link', { name: /Add Game/ }).click();
      await expect(page).toHaveURL('/games/add');
      
      // Go back to homepage  
      await page.goto('/');
      
      // Test Dashboard link
      await page.getByRole('link', { name: /Dashboard/ }).click();
      await expect(page).toHaveURL('/dashboard');
    });

    test('should be responsive on mobile viewport', async ({ page }) => {
      // Set mobile viewport
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto('/');
      
      // Check main content is still visible and properly laid out
      await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
      await expect(page.getByText(`Welcome back, ${TEST_ADMIN.username}!`)).toBeVisible();
      
      // Quick action cards should still be visible on mobile
      await expect(page.getByRole('link', { name: /My Games/ })).toBeVisible();
      await expect(page.getByRole('link', { name: /Add Game/ })).toBeVisible();
      await expect(page.getByRole('link', { name: /Dashboard/ })).toBeVisible();
      
      // Feature cards should still be visible on mobile
      await expect(page.getByText('Organize Your Library')).toBeVisible();
      await expect(page.getByText('Track Progress')).toBeVisible();
      await expect(page.getByText('Self-Hosted Privacy')).toBeVisible();
    });

    test('should have proper page title', async ({ page }) => {
      await page.goto('/');
      
      // Check that the page title is set correctly
      await expect(page).toHaveTitle(/Nexorious/);
    });

    test('should show user menu and logout functionality', async ({ page }) => {
      await page.goto('/');
      
      // User menu should be visible in navigation
      await expect(page.getByRole('button', { name: TEST_ADMIN.username })).toBeVisible();
      
      // Click user menu
      await page.getByRole('button', { name: TEST_ADMIN.username }).click();
      
      // Should see logout option
      await expect(page.getByRole('menuitem', { name: 'Logout' })).toBeVisible();
      
      // Test logout
      await page.getByRole('menuitem', { name: 'Logout' }).click();
      
      // Should be redirected to login
      await expect(page).toHaveURL('/login');
    });
  });
});