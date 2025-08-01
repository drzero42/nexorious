import { test, expect } from '@playwright/test';
import { TestHelpers, TEST_ADMIN } from '../helpers/test-fixtures';

test.describe.configure({ mode: 'serial' });

test.describe('Initial Setup Flow', () => {
  let helpers: TestHelpers;

  test.beforeEach(async ({ page }) => {
    helpers = new TestHelpers(page);
    // Clear any existing session data
    await helpers.clearSession();
  });

  test('should redirect to setup page when no admin exists', async ({ page }) => {
    // Visit homepage - should redirect to setup since no admin exists
    await page.goto('/');
    
    // Should be redirected to setup page
    await expect(page).toHaveURL('/setup');
    
    // Verify setup page content
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    await expect(page.getByText('Let\'s set up your admin account to get started')).toBeVisible();
  });

  test('should display setup form with proper validation', async ({ page }) => {
    await page.goto('/setup');
    
    // Check form elements are present
    await expect(page.getByLabel('Admin Username')).toBeVisible();
    await expect(page.getByLabel('Password', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Confirm Password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Admin Account' })).toBeVisible();
    
    // Check helper text
    await expect(page.getByText('This will be your administrator username')).toBeVisible();
    await expect(page.getByText('Must be at least 8 characters long')).toBeVisible();
  });

  test('should validate form inputs', async ({ page }) => {
    await page.goto('/setup');
    
    // Try to submit with empty fields
    await page.getByRole('button', { name: 'Create Admin Account' }).click();
    await expect(page.getByText('Please fill in all fields')).toBeVisible();
    
    // Try with short username
    await page.getByLabel('Admin Username').fill('ab');
    await page.getByLabel('Password', { exact: true }).fill('password123');
    await page.getByLabel('Confirm Password').fill('password123');
    await page.getByRole('button', { name: 'Create Admin Account' }).click();
    await expect(page.getByText('Username must be at least 3 characters long')).toBeVisible();
    
    // Try with short password
    await page.getByLabel('Admin Username').fill('testuser');
    await page.getByLabel('Password', { exact: true }).fill('short');
    await page.getByLabel('Confirm Password').fill('short');
    await page.getByRole('button', { name: 'Create Admin Account' }).click();
    await expect(page.getByText('Password must be at least 8 characters long')).toBeVisible();
    
    // Try with mismatched passwords
    await page.getByLabel('Admin Username').fill('testuser');
    await page.getByLabel('Password', { exact: true }).fill('password123');
    await page.getByLabel('Confirm Password').fill('password456');
    await page.getByRole('button', { name: 'Create Admin Account' }).click();
    await expect(page.getByText('Passwords do not match')).toBeVisible();
  });

  test('should successfully create admin account and redirect to login', async ({ page }) => {
    await page.goto('/setup');
    
    // Fill in valid admin credentials
    await page.getByLabel('Admin Username').fill(TEST_ADMIN.username);
    await page.getByLabel('Password', { exact: true }).fill(TEST_ADMIN.password);
    await page.getByLabel('Confirm Password').fill(TEST_ADMIN.password);
    
    // Submit form
    await page.getByRole('button', { name: 'Create Admin Account' }).click();
    
    // Should show loading state briefly
    await expect(page.getByText('Creating Admin Account...')).toBeVisible();
    
    // Should redirect to login page after successful setup
    await expect(page).toHaveURL('/login');
    await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
  });

  test('should redirect to login if setup is already completed', async ({ page }) => {
    // First create an admin account
    await helpers.setupInitialAdmin();
    
    // Now try to visit setup page again
    await page.goto('/setup');
    
    // Should be redirected to login page
    await expect(page).toHaveURL('/login');
    await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
  });

  test('should focus username field when setup page loads', async ({ page }) => {
    await page.goto('/setup');
    
    // Wait for page to load
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    
    // Username field should be focused
    const usernameField = page.getByLabel('Admin Username');
    await expect(usernameField).toBeFocused();
  });

  test('should handle keyboard navigation', async ({ page }) => {
    await page.goto('/setup');
    
    // Fill form using keyboard navigation
    await page.getByLabel('Admin Username').fill(TEST_ADMIN.username);
    await page.keyboard.press('Tab');
    await page.keyboard.type(TEST_ADMIN.password);
    await page.keyboard.press('Tab');
    await page.keyboard.type(TEST_ADMIN.password);
    
    // Submit with Enter key
    await page.keyboard.press('Enter');
    
    // Should redirect to login
    await expect(page).toHaveURL('/login');
  });
});