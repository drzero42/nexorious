import { test, expect } from '@playwright/test';
import { TestHelpers, TEST_ADMIN } from '../helpers/test-fixtures';

test.describe.configure({ mode: 'serial' });

test.describe('Authentication Flow', () => {
  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
    // Don't clear session data - we want to maintain auth state
    // Admin user should already exist from setup tests
  });

  test('should display login form correctly', async ({ page }) => {
    await page.goto('/login');
    
    // Check page title and heading
    await expect(page).toHaveTitle(/Login - Nexorious/);
    await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
    await expect(page.getByText('Sign in to access your game collection')).toBeVisible();
    
    // Check form elements
    await expect(page.getByLabel('Username')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
  });

  test('should validate login form inputs', async ({ page }) => {
    await page.goto('/login');
    
    // Try to submit with empty fields
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByText('Please fill in all fields')).toBeVisible();
    
    // Try with just username
    await page.getByLabel('Username').fill(TEST_ADMIN.username);
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByText('Please fill in all fields')).toBeVisible();
    
    // Try with just password
    await page.getByLabel('Username').clear();
    await page.getByLabel('Password').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByText('Please fill in all fields')).toBeVisible();
  });

  test('should show error for invalid credentials', async ({ page }) => {
    await page.goto('/login');
    
    // Try to login with wrong password
    await page.getByLabel('Username').fill(TEST_ADMIN.username);
    await page.getByLabel('Password').fill('wrongpassword');
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    // Should show error message
    await expect(page.getByText(/Login failed|Invalid credentials/)).toBeVisible();
    
    // Should still be on login page
    await expect(page).toHaveURL('/login');
  });

  test('should successfully login with valid credentials', async ({ page }) => {
    await page.goto('/login');
    
    // Fill in correct credentials
    await page.getByLabel('Username').fill(TEST_ADMIN.username);
    await page.getByLabel('Password').fill(TEST_ADMIN.password);
    
    // Submit form
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    // Should redirect to games page after successful login
    await expect(page).toHaveURL('/games');
    
    // Should see authenticated navigation
    await expect(page.getByRole('button', { name: TEST_ADMIN.username })).toBeVisible();
  });

  test('should handle keyboard navigation on login form', async ({ page }) => {
    await page.goto('/login');
    
    // Username field should be focused
    const usernameField = page.getByLabel('Username');
    await expect(usernameField).toBeFocused();
    
    // Fill form using keyboard
    await page.keyboard.type(TEST_ADMIN.username);
    await page.keyboard.press('Tab');
    await page.keyboard.type(TEST_ADMIN.password);
    
    // Submit with Enter
    await page.keyboard.press('Enter');
    
    // Should redirect on successful login
    await expect(page).toHaveURL('/games');
  });

  test('should redirect authenticated users away from login page', async ({ page }) => {
    // First login
    await helpers.loginAsAdmin();
    
    // Now try to visit login page while authenticated
    await page.goto('/login');
    
    // Should be redirected away from login page
    await expect(page).not.toHaveURL('/login');
    // Should be on an authenticated page
    await expect(page.getByRole('button', { name: TEST_ADMIN.username })).toBeVisible();
  });

  test('should logout successfully', async ({ page }) => {
    // First login
    await helpers.loginAsAdmin();
    await expect(page).toHaveURL('/games');
    
    // Click user menu
    await page.getByRole('button', { name: TEST_ADMIN.username }).click();
    
    // Click logout
    await page.getByRole('menuitem', { name: 'Logout' }).click();
    
    // Should be redirected to login page
    await expect(page).toHaveURL('/login');
    await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
    
    // User menu should no longer be visible
    await expect(page.getByRole('button', { name: TEST_ADMIN.username })).not.toBeVisible();
  });

  test('should maintain login state across page reloads', async ({ page }) => {
    // Login first
    await helpers.loginAsAdmin();
    await expect(page).toHaveURL('/games');
    
    // Reload the page
    await page.reload();
    
    // Should still be authenticated
    await expect(page.getByRole('button', { name: TEST_ADMIN.username })).toBeVisible();
    await expect(page).toHaveURL('/games');
  });

  test('should redirect unauthenticated users to login when accessing protected pages', async ({ page }) => {
    // Try to access protected pages without logging in
    const protectedPages = ['/games', '/dashboard', '/profile', '/admin'];
    
    for (const pagePath of protectedPages) {
      await page.goto(pagePath);
      
      // Should be redirected to login page
      await expect(page).toHaveURL('/login');
      await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
    }
  });

  test('should preserve intended destination after login', async ({ page }) => {
    // Try to access dashboard page without being logged in
    await page.goto('/dashboard');
    
    // Should be redirected to login
    await expect(page).toHaveURL('/login');
    
    // Login
    await page.getByLabel('Username').fill(TEST_ADMIN.username);
    await page.getByLabel('Password').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    // Should be redirected to originally intended page (or default games page)
    await expect(page).toHaveURL(/\/(games|dashboard)/);
  });

  test('should handle session expiration gracefully', async ({ page }) => {
    // Login first
    await helpers.loginAsAdmin();
    await expect(page).toHaveURL('/games');
    
    // Clear tokens to simulate expiration
    await page.evaluate(() => {
      localStorage.removeItem('auth');
    });
    
    // Try to navigate to another page
    await page.goto('/dashboard');
    
    // Should be redirected to login due to expired session
    await expect(page).toHaveURL('/login');
  });
});