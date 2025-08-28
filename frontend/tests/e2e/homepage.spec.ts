import { test, expect } from '@playwright/test';
import { TestHelpers } from '../helpers/test-fixtures';

test.describe('Homepage', () => {
  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
  });

  test('should display welcome message and user-specific content', async ({ page }) => {
    await page.goto('/');
    
    // Check main heading is still present
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    
    // Check for authenticated user welcome message
    await expect(page.getByText(/Welcome back,/)).toBeVisible();
    await expect(page.getByText('Ready to manage your game collection?')).toBeVisible();
    
    // Check for user avatar/initials (should show some letter)
    const userAvatar = page.locator('[data-testid="user-avatar"], .user-avatar').first();
    if (await userAvatar.count() > 0) {
      await expect(userAvatar).toBeVisible();
    }
  });

  test('should display quick action cards for authenticated users', async ({ page }) => {
    await page.goto('/');
    
    // Check quick action cards
    await expect(page.getByRole('link', { name: /My Games/ }).first()).toBeVisible();
    await expect(page.getByRole('link', { name: /Add Game/ }).first()).toBeVisible();
    await expect(page.getByRole('link', { name: /Dashboard/ }).first()).toBeVisible();
    
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
    await page.getByRole('link', { name: /My Games/ }).first().click();
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
    await expect(page.getByText(/Welcome back,/)).toBeVisible();
    
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
    
    // Check that the profile link is visible (shows username)
    const profileLink = page.getByRole('link', { name: /Profile Settings/ });
    await expect(profileLink).toBeVisible();
    
    // Check that logout button is visible
    const logoutButton = page.getByRole('button', { name: /Logout/ });
    await expect(logoutButton).toBeVisible();
    
    // Test logout
    await logoutButton.click();
    
    // Should be redirected to login
    await expect(page).toHaveURL('/login');
  });
});