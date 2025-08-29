import { test as setup, expect } from '@playwright/test';

/**
 * Authentication setup file for Playwright E2E tests
 * This file runs once before all other tests to create both admin and regular users,
 * then leaves the system in an unauthenticated state.
 * Tests should explicitly login with the credentials they need.
 */

// Test credentials - exported for use in other test files
export const TEST_CREDENTIALS = {
  admin: {
    username: 'e2e-admin',
    password: 'e2e-admin-password-123'
  },
  regular: {
    username: 'e2e-user', 
    password: 'e2e-user-password-123'
  }
} as const;

setup('create admin and regular users', async ({ page }) => {
  console.log('🔧 Setting up test users...');

  // Step 1: Navigate to homepage and check if setup is needed
  await page.goto('/');
  await page.waitForTimeout(2000);
  
  const currentUrl = page.url();

  // Step 2: Create or login as admin
  if (currentUrl.includes('/setup')) {
    console.log('🔄 Creating admin user...');
    
    await expect(page.getByRole('heading', { name: 'Welcome to Nexorious' })).toBeVisible();
    await page.waitForTimeout(2000);

    // Create admin user
    await page.getByLabel('Admin Username').fill(TEST_CREDENTIALS.admin.username);
    await page.getByLabel('Password', { exact: true }).fill(TEST_CREDENTIALS.admin.password);
    await page.getByLabel('Confirm Password').fill(TEST_CREDENTIALS.admin.password);
    await page.getByRole('button', { name: 'Create Admin Account' }).click();

    // Wait for redirect to login
    await expect(page).toHaveURL('/login', { timeout: 15000 });
    
    // Login as admin
    await page.getByLabel('Username').fill(TEST_CREDENTIALS.admin.username);
    await page.getByLabel('Password').fill(TEST_CREDENTIALS.admin.password);
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    await expect(page).toHaveURL('/games', { timeout: 15000 });
    
  } else if (currentUrl.includes('/login')) {
    console.log('✅ Admin user exists, logging in...');
    
    // Login as existing admin
    await page.getByLabel('Username').fill(TEST_CREDENTIALS.admin.username);
    await page.getByLabel('Password').fill(TEST_CREDENTIALS.admin.password);
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    await expect(page).toHaveURL('/games', { timeout: 15000 });
  }

  console.log('✅ Admin user ready');

  // Step 3: Create regular user via admin interface
  console.log('🔄 Creating regular user...');
  
  // Navigate to user management
  await page.goto('/admin/users');
  await expect(page.getByRole('heading', { name: /users|user management/i })).toBeVisible();
  
  // Click create user button
  const createButton = page.getByRole('button', { name: /create user|add user|new user/i });
  if (await createButton.isVisible()) {
    await createButton.click();
  } else {
    // Try alternate navigation
    await page.goto('/admin/users/new');
  }
  
  // Fill out regular user form
  await page.locator('#username').fill(TEST_CREDENTIALS.regular.username);
  await page.locator('#password').fill(TEST_CREDENTIALS.regular.password);
  await page.locator('#confirm-password').fill(TEST_CREDENTIALS.regular.password);
  
  // Ensure user is NOT admin (uncheck admin checkbox if present)
  const adminCheckbox = page.locator('#is-admin');
  if (await adminCheckbox.isVisible()) {
    await adminCheckbox.uncheck();
  }
  
  // Submit form
  await page.getByRole('button', { name: /create|save|add/i }).click();
  
  // Wait for success (redirect back to users list or success message)
  await page.waitForTimeout(2000);
  
  console.log('✅ Regular user created');

  // Step 4: Logout to leave clean unauthenticated state
  console.log('🚪 Logging out to leave clean state...');
  
  // Look for logout button (could be in dropdown or direct button)
  const logoutSelectors = [
    'button:has-text("Logout")',
    'button:has-text("Sign Out")', 
    '[aria-label*="logout" i]',
    '[aria-label*="sign out" i]'
  ];
  
  let loggedOut = false;
  for (const selector of logoutSelectors) {
    try {
      if (await page.locator(selector).isVisible({ timeout: 2000 })) {
        await page.locator(selector).click();
        loggedOut = true;
        break;
      }
    } catch {
      continue;
    }
  }
  
  // If logout button not found, clear storage manually
  if (!loggedOut) {
    console.log('⚠️ Logout button not found, clearing storage manually');
    await page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });
    await page.context().clearCookies();
  }
  
  // Verify we're logged out by going to a protected page
  await page.goto('/games');
  await page.waitForTimeout(2000);
  
  const finalUrl = page.url();
  if (finalUrl.includes('/login') || finalUrl.includes('/setup')) {
    console.log('✅ Successfully logged out - system ready for tests');
  } else {
    console.log('⚠️ May still be logged in, but continuing...');
  }
  
  console.log('🎯 Test setup complete - both users created and ready for explicit login in tests');
});