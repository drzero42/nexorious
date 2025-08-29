import { test, expect } from '@playwright/test';
import { TestHelpers } from '../helpers/test-fixtures';

test.describe('Game Management Flow', () => {
  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
  });

  test('should display games collection page correctly', async ({ page }) => {
    // Navigate to the games page first
    await page.goto('/games');
    await expect(page).toHaveURL('/games');
    
    // Check main page elements
    await expect(page.getByRole('heading', { name: /my games|games collection/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /add game/i })).toBeVisible();
    
    // Check view toggles
    await expect(page.getByRole('button', { name: /grid view/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /list view/i })).toBeVisible();
  });

  test('should navigate to add game form', async ({ page }) => {
    // Start from games page
    await page.goto('/games');
    
    // Click add game button
    await page.getByRole('button', { name: /add game/i }).click();
    
    // Should navigate to add game page
    await expect(page).toHaveURL('/games/add');
    await expect(page.getByRole('heading', { name: /add game/i })).toBeVisible();
  });

  test('should perform IGDB search for a game', async ({ page }) => {
    // Navigate to add game page
    await page.goto('/games/add');
    
    // Check search form is present
    await expect(page.getByPlaceholder(/enter game title/i)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Search' })).toBeVisible();
    
    // Perform search
    const searchInput = page.getByPlaceholder(/enter game title/i);
    await searchInput.fill('The Witcher 3');
    await page.getByRole('button', { name: 'Search' }).click();
    
    // Wait for search results (this will depend on IGDB mock responses)
    // For now, check that search was initiated
    await expect(page.getByText(/searching/i)).toBeVisible();
  });

  test('should validate add game form inputs', async ({ page }) => {
    await page.goto('/games/add');
    
    // Try to submit empty search - button should be disabled
    const searchButton = page.getByRole('button', { name: 'Search' });
    await expect(searchButton).toBeDisabled();
    
    // Fill with search term to enable button
    await page.getByPlaceholder(/enter game title/i).fill('test search');
    await expect(searchButton).toBeEnabled();
  });

  test('should handle search form interactions', async ({ page }) => {
    await page.goto('/games/add');
    
    // Test search form interaction
    await page.getByPlaceholder(/enter game title/i).fill('TestGame');
    
    // Button should be enabled with text
    await expect(page.getByRole('button', { name: 'Search' })).toBeEnabled();
    
    // Clear text and button should be disabled
    await page.getByPlaceholder(/enter game title/i).clear();
    await expect(page.getByRole('button', { name: 'Search' })).toBeDisabled();
  });

  test('should display search information correctly', async ({ page }) => {
    await page.goto('/games/add');
    
    // Check that search information is displayed
    await expect(page.getByText(/how game search works/i)).toBeVisible();
    await expect(page.getByText(/search for games using the igdb database/i)).toBeVisible();
    await expect(page.getByText(/automatic metadata/i)).toBeVisible();
  });

  test('should handle search input validation', async ({ page }) => {
    await page.goto('/games/add');
    
    // Empty search should disable button
    await expect(page.getByRole('button', { name: 'Search' })).toBeDisabled();
    
    // Adding text should enable button
    await page.getByPlaceholder(/enter game title/i).fill('a');
    await expect(page.getByRole('button', { name: 'Search' })).toBeEnabled();
    
    // Clearing should disable again
    await page.getByPlaceholder(/enter game title/i).clear();
    await expect(page.getByRole('button', { name: 'Search' })).toBeDisabled();
  });

  test('should have proper form elements', async ({ page }) => {
    await page.goto('/games/add');
    
    // Check form elements
    const searchInput = page.getByPlaceholder(/enter game title/i);
    const searchButton = page.getByRole('button', { name: 'Search' });
    
    await expect(searchInput).toBeVisible();
    await expect(searchButton).toBeVisible();
    
    // Input should have correct placeholder
    await expect(searchInput).toHaveAttribute('placeholder', 'Enter game title...');
  });

  test('should show proper page elements', async ({ page }) => {
    await page.goto('/games/add');
    
    // Check that all expected elements are present
    await expect(page.getByRole('heading', { name: /add game/i })).toBeVisible();
    await expect(page.getByText(/add a new game to your collection/i)).toBeVisible();
    await expect(page.getByPlaceholder(/enter game title/i)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Search' })).toBeVisible();
    
    // Check info text about IGDB
    await expect(page.getByText(/how game search works/i)).toBeVisible();
    await expect(page.getByText(/search for games using the igdb database/i)).toBeVisible();
  });

  test('should navigate back from add game form', async ({ page }) => {
    await page.goto('/games/add');
    
    // Should have back/cancel button
    const backButton = page.getByRole('button', { name: /back|cancel/i });
    await expect(backButton).toBeVisible();
    
    // Click back button
    await backButton.click();
    
    // Should return to games list
    await expect(page).toHaveURL('/games');
  });

  test('should handle keyboard navigation in add game form', async ({ page }) => {
    await page.goto('/games/add');
    
    const searchInput = page.getByPlaceholder(/enter game title/i);
    const searchButton = page.getByRole('button', { name: 'Search' });
    
    // Click into search input to focus it
    await searchInput.click();
    
    // Type to enable the button first
    await page.keyboard.type('Test Game');
    
    // Now tab to the button - it should be enabled and focusable
    await page.keyboard.press('Tab');
    await expect(searchButton).toBeFocused();
    
    // Button should be enabled
    await expect(searchButton).toBeEnabled();
  });

  test('should have correct page title and structure', async ({ page }) => {
    await page.goto('/games/add');
    
    // Check page title
    await expect(page).toHaveTitle(/Add Game/);
    
    // Check page structure
    await expect(page.getByRole('heading', { name: /add game/i })).toBeVisible();
    await expect(page.getByText(/add a new game to your collection/i)).toBeVisible();
  });

  test('should handle input changes correctly', async ({ page }) => {
    await page.goto('/games/add');
    
    const searchInput = page.getByPlaceholder(/enter game title/i);
    const searchButton = page.getByRole('button', { name: 'Search' });
    
    // Initially disabled
    await expect(searchButton).toBeDisabled();
    
    // Type and check enabled
    await searchInput.fill('First Search');
    await expect(searchButton).toBeEnabled();
    
    // Clear and check disabled
    await searchInput.clear();
    await expect(searchButton).toBeDisabled();
    
    // Type again
    await searchInput.fill('Second Search');
    await expect(searchButton).toBeEnabled();
  });

  test('should navigate back to games list', async ({ page }) => {
    await page.goto('/games/add');
    
    // Check that back navigation works
    const backButton = page.getByRole('button', { name: /back|cancel/i });
    await expect(backButton).toBeVisible();
    
    await backButton.click();
    await expect(page).toHaveURL('/games');
  });

  test('should maintain form state during navigation', async ({ page }) => {
    await page.goto('/games/add');
    
    // Fill search form
    await page.getByPlaceholder(/enter game title/i).fill('State Test');
    
    // Navigate away (but don't actually submit)
    await page.goto('/games');
    
    // Navigate back
    await page.goto('/games/add');
    
    // Form should be reset (fresh start)
    await expect(page.getByPlaceholder(/enter game title/i)).toHaveValue('');
    await expect(page.getByRole('heading', { name: /add game/i })).toBeVisible();
  });
});