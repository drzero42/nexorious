import { test as setup, expect } from '@playwright/test';
import { config } from '../src/lib/env';
import path from 'path';

/**
 * Authentication setup file for Playwright E2E tests
 * This file runs once before all other tests to create an admin user
 * and save the authentication state for reuse across all test projects.
 * 
 * Based on Playwright best practices:
 * https://playwright.dev/docs/auth
 * https://dev.to/playwright/a-better-global-setup-in-playwright-reusing-login-with-project-dependencies-14
 */

// Test admin credentials - consistent across all tests
export const TEST_ADMIN = {
  username: 'e2e-admin',
  password: 'e2e-test-password-123'
} as const;

// Storage state file path  
const authFile = path.join(process.cwd(), 'playwright/.auth/admin.json');

setup('authenticate as admin', async ({ page }) => {
  console.log('🔧 Setting up admin user for E2E tests...');
  
  // Step 1: Check if setup is needed by visiting the homepage
  await page.goto('/');
  
  // Wait for any redirects to complete
  await page.waitForTimeout(2000);
  
  const currentUrl = page.url();
  
  if (currentUrl.includes('/login')) {
    // Admin already exists, just need to login
    console.log('✅ Admin user already exists, logging in...');
    
    await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
    
    // Fill in admin credentials
    await page.getByLabel('Username').fill(TEST_ADMIN.username);
    await page.getByLabel('Password').fill(TEST_ADMIN.password);
    
    // Submit login form
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    // Wait for successful login and redirect
    await expect(page).toHaveURL('/games', { timeout: 15000 });
    
  } else if (currentUrl.includes('/setup')) {
    // Need to create admin user first
    console.log('🔄 Creating admin user...');
    
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    await expect(page.getByText('Let\'s set up your admin account')).toBeVisible();
    
    // Wait for setup page to fully load (checkSetupStatus completes)
    await page.waitForTimeout(2000);
    
    // Fill in admin credentials
    await page.getByLabel('Admin Username').fill(TEST_ADMIN.username);
    await page.getByLabel('Password', { exact: true }).fill(TEST_ADMIN.password);
    await page.getByLabel('Confirm Password').fill(TEST_ADMIN.password);
    
    // Submit setup form
    await page.getByRole('button', { name: 'Create Admin Account' }).click();
    
    // Wait for redirect to login page after successful setup
    await expect(page).toHaveURL('/login', { timeout: 15000 });
    await expect(page.getByRole('heading', { name: 'Welcome Back' })).toBeVisible();
    
    // Now login with the newly created admin
    await page.getByLabel('Username').fill(TEST_ADMIN.username);
    await page.getByLabel('Password').fill(TEST_ADMIN.password);
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    // Wait for successful login
    await expect(page).toHaveURL('/games', { timeout: 15000 });
    
  } else {
    // Already authenticated - this shouldn't happen in fresh test environment
    console.log('⚠️ Already authenticated state detected');
    await expect(page).toHaveURL('/games');
  }
  
  // Verify admin is properly authenticated
  await expect(page.getByRole('button', { name: TEST_ADMIN.username })).toBeVisible();
  
  console.log('✅ Admin authentication successful');
  
  // Save authentication state for reuse in other tests
  await page.context().storageState({ path: authFile });
  
  console.log(`📁 Authentication state saved to: ${authFile}`);
});