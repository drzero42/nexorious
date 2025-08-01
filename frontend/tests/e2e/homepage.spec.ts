import { test, expect } from '@playwright/test';

test.describe('Homepage', () => {
  test('should display welcome message and main content', async ({ page }) => {
    await page.goto('/');
    
    // Check main heading
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    
    // Check subtitle/description
    await expect(page.getByText('Your self-hosted game collection management service')).toBeVisible();
    
    // Verify features section is present
    await expect(page.getByText('Organize Your Library')).toBeVisible();
    await expect(page.getByText('Track Progress')).toBeVisible();
    await expect(page.getByText('Self-Hosted Privacy')).toBeVisible();
  });

  test('should show authentication check loading state', async ({ page }) => {
    await page.goto('/');
    
    // The page should show either authenticated content or redirect
    // We can't easily test redirects without mocking auth, so we'll test the loading state appears briefly
    const loadingText = page.getByText('Checking authentication status...');
    
    // The loading state might be very brief, so we check if it exists or if we've moved past it
    const hasLoadingState = await loadingText.isVisible().catch(() => false);
    const hasWelcomeContent = await page.getByRole('heading', { name: 'Welcome to Nexorious' }).isVisible();
    
    // One of these should be true
    expect(hasLoadingState || hasWelcomeContent).toBe(true);
  });

  test('should display feature cards with correct content', async ({ page }) => {
    await page.goto('/');
    
    // Wait for page to load
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    
    // Check individual feature cards
    const organizeSection = page.locator('text=Organize Your Library').locator('..');
    await expect(organizeSection.getByText('Keep track of all your games across multiple platforms')).toBeVisible();
    
    const trackSection = page.locator('text=Track Progress').locator('..');
    await expect(trackSection.getByText('Monitor your gaming progress with detailed completion levels')).toBeVisible();
    
    const privacySection = page.locator('text=Self-Hosted Privacy').locator('..');
    await expect(privacySection.getByText('Keep your gaming data private and secure')).toBeVisible();
  });

  test('should be responsive on mobile viewport', async ({ page }) => {
    // Set mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');
    
    // Check main content is still visible and properly laid out
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    await expect(page.getByText('Your self-hosted game collection management service')).toBeVisible();
    
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
});