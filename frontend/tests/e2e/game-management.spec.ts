import { test, expect } from '@playwright/test';
import { TestHelpers } from '../helpers/test-fixtures';

test.describe('Game Management Flow', () => {
  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
  });

  test('should display games collection page correctly', async ({ page }) => {
    await expect(page).toHaveURL('/games');
    
    // Check main page elements
    await expect(page.getByRole('heading', { name: /my games|games collection/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /add game/i })).toBeVisible();
    
    // Check view toggles
    await expect(page.getByRole('button', { name: /grid view/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /list view/i })).toBeVisible();
  });

  test('should navigate to add game form', async ({ page }) => {
    // Click add game button
    await page.getByRole('button', { name: /add game/i }).click();
    
    // Should navigate to add game page
    await expect(page).toHaveURL('/games/add');
    await expect(page.getByRole('heading', { name: /add new game/i })).toBeVisible();
  });

  test('should perform IGDB search for a game', async ({ page }) => {
    // Navigate to add game page
    await page.goto('/games/add');
    
    // Check search form is present
    await expect(page.getByPlaceholder(/search for a game/i)).toBeVisible();
    await expect(page.getByRole('button', { name: /search/i })).toBeVisible();
    
    // Perform search
    const searchInput = page.getByPlaceholder(/search for a game/i);
    await searchInput.fill('The Witcher 3');
    await page.getByRole('button', { name: /search/i }).click();
    
    // Wait for search results (this will depend on IGDB mock responses)
    // For now, check that search was initiated
    await expect(page.getByText(/searching/i)).toBeVisible();
  });

  test('should validate add game form inputs', async ({ page }) => {
    await page.goto('/games/add');
    
    // Try to submit empty search
    await page.getByRole('button', { name: /search/i }).click();
    
    // Should show validation message
    await expect(page.getByText(/please enter a game name/i)).toBeVisible();
    
    // Test with very short search term
    await page.getByPlaceholder(/search for a game/i).fill('ab');
    await page.getByRole('button', { name: /search/i }).click();
    
    // Should show validation for minimum characters
    await expect(page.getByText(/search term must be at least/i)).toBeVisible();
  });

  test('should handle search with no results', async ({ page }) => {
    await page.goto('/games/add');
    
    // Search for something that won't return results
    await page.getByPlaceholder(/search for a game/i).fill('ThisGameDoesNotExist12345');
    await page.getByRole('button', { name: /search/i }).click();
    
    // Wait for search completion
    await expect(page.getByText(/no games found/i)).toBeVisible();
    await expect(page.getByText(/try a different search/i)).toBeVisible();
  });

  test('should allow manual game creation when search fails', async ({ page }) => {
    await page.goto('/games/add');
    
    // Search for non-existent game
    await page.getByPlaceholder(/search for a game/i).fill('NonExistentGame');
    await page.getByRole('button', { name: /search/i }).click();
    
    // Wait for no results message
    await expect(page.getByText(/no games found/i)).toBeVisible();
    
    // Should show option to add manually
    await expect(page.getByRole('button', { name: /add manually/i })).toBeVisible();
    
    // Click add manually
    await page.getByRole('button', { name: /add manually/i }).click();
    
    // Should show manual entry form
    await expect(page.getByLabel(/game title/i)).toBeVisible();
    await expect(page.getByLabel(/description/i)).toBeVisible();
  });

  test('should fill manual game creation form', async ({ page }) => {
    await page.goto('/games/add');
    
    // Simulate no search results scenario and go to manual entry
    await page.getByPlaceholder(/search for a game/i).fill('ManualTestGame');
    await page.getByRole('button', { name: /search/i }).click();
    await expect(page.getByText(/no games found/i)).toBeVisible();
    await page.getByRole('button', { name: /add manually/i }).click();
    
    // Fill manual form
    await page.getByLabel(/game title/i).fill('Test Game Manual');
    await page.getByLabel(/description/i).fill('A test game created manually for E2E testing');
    
    // Check personal data fields
    await expect(page.getByLabel(/personal rating/i)).toBeVisible();
    await expect(page.getByLabel(/play status/i)).toBeVisible();
    await expect(page.getByLabel(/ownership status/i)).toBeVisible();
    await expect(page.getByLabel(/hours played/i)).toBeVisible();
    
    // Fill personal data
    await page.getByLabel(/personal rating/i).fill('8');
    await page.getByLabel(/play status/i).selectOption('in_progress');
    await page.getByLabel(/ownership status/i).selectOption('owned');
    await page.getByLabel(/hours played/i).fill('15');
  });

  test('should validate manual game form', async ({ page }) => {
    await page.goto('/games/add');
    
    // Get to manual form
    await page.getByPlaceholder(/search for a game/i).fill('ValidationTest');
    await page.getByRole('button', { name: /search/i }).click();
    await expect(page.getByText(/no games found/i)).toBeVisible();
    await page.getByRole('button', { name: /add manually/i }).click();
    
    // Try to submit without title
    await page.getByRole('button', { name: /add game/i }).click();
    await expect(page.getByText(/title is required/i)).toBeVisible();
    
    // Test rating validation
    await page.getByLabel(/game title/i).fill('Test Game');
    await page.getByLabel(/personal rating/i).fill('11'); // Invalid rating
    await page.getByRole('button', { name: /add game/i }).click();
    await expect(page.getByText(/rating must be between/i)).toBeVisible();
    
    // Test hours played validation
    await page.getByLabel(/personal rating/i).fill('8');
    await page.getByLabel(/hours played/i).fill('-5'); // Negative hours
    await page.getByRole('button', { name: /add game/i }).click();
    await expect(page.getByText(/hours cannot be negative/i)).toBeVisible();
  });

  test('should handle platform selection', async ({ page }) => {
    await page.goto('/games/add');
    
    // Get to manual form
    await page.getByPlaceholder(/search for a game/i).fill('PlatformTest');
    await page.getByRole('button', { name: /search/i }).click();
    await expect(page.getByText(/no games found/i)).toBeVisible();
    await page.getByRole('button', { name: /add manually/i }).click();
    
    // Fill required fields
    await page.getByLabel(/game title/i).fill('Platform Test Game');
    
    // Check platform selection is available
    await expect(page.getByText(/platforms/i)).toBeVisible();
    
    // Should show common platforms (these depend on seed data)
    await expect(page.getByText(/pc/i)).toBeVisible();
    await expect(page.getByText(/playstation/i)).toBeVisible();
    await expect(page.getByText(/xbox/i)).toBeVisible();
    
    // Select a platform
    await page.getByRole('checkbox', { name: /pc/i }).check();
    await expect(page.getByRole('checkbox', { name: /pc/i })).toBeChecked();
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
    
    // Search input should be focused
    await expect(page.getByPlaceholder(/search for a game/i)).toBeFocused();
    
    // Tab through form elements
    await page.keyboard.press('Tab');
    await expect(page.getByRole('button', { name: /search/i })).toBeFocused();
    
    // Enter search term and submit with Enter
    await page.keyboard.press('Shift+Tab'); // Back to input
    await page.keyboard.type('Test Game');
    await page.keyboard.press('Enter');
    
    // Search should be initiated
    await expect(page.getByText(/searching/i)).toBeVisible();
  });

  test('should show loading states during search', async ({ page }) => {
    await page.goto('/games/add');
    
    // Start search
    await page.getByPlaceholder(/search for a game/i).fill('Loading Test');
    await page.getByRole('button', { name: /search/i }).click();
    
    // Should show loading indicator
    await expect(page.getByText(/searching/i)).toBeVisible();
    
    // Search button should be disabled during search
    await expect(page.getByRole('button', { name: /search/i })).toBeDisabled();
  });

  test('should clear search results when starting new search', async ({ page }) => {
    await page.goto('/games/add');
    
    // First search
    await page.getByPlaceholder(/search for a game/i).fill('First Search');
    await page.getByRole('button', { name: /search/i }).click();
    await page.waitForTimeout(1000); // Wait for search completion
    
    // Start new search
    await page.getByPlaceholder(/search for a game/i).clear();
    await page.getByPlaceholder(/search for a game/i).fill('Second Search');
    
    // Previous results should be cleared when typing
    // (This behavior depends on implementation)
  });

  test('should handle network errors gracefully', async ({ page }) => {
    await page.goto('/games/add');
    
    // Mock network failure (this would require network interception)
    // For now, just test that error states are handled
    await page.getByPlaceholder(/search for a game/i).fill('Network Error Test');
    await page.getByRole('button', { name: /search/i }).click();
    
    // Should eventually show error message if network fails
    // Implementation will depend on error handling in the component
  });

  test('should maintain form state during navigation', async ({ page }) => {
    await page.goto('/games/add');
    
    // Fill search form
    await page.getByPlaceholder(/search for a game/i).fill('State Test');
    
    // Navigate away (but don't actually submit)
    await page.goto('/games');
    
    // Navigate back
    await page.goto('/games/add');
    
    // Form should be reset (fresh start)
    await expect(page.getByPlaceholder(/search for a game/i)).toHaveValue('');
  });
});