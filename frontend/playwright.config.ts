import { defineConfig, devices } from '@playwright/test';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

/**
 * @see https://playwright.dev/docs/test-configuration
 */

// ES module equivalent of __dirname
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Generate unique temporary file path for test database
export const tempDbPath = path.join('/tmp', `nexorious_test_${Date.now()}.db`);

/**
 * Read environment variables from backend/.env file
 */
function readBackendEnv(key: string): string | undefined {
  const backendEnvPath = path.join(__dirname, '../backend/.env');
  
  if (!fs.existsSync(backendEnvPath)) {
    return undefined;
  }
  
  try {
    const envContent = fs.readFileSync(backendEnvPath, 'utf8');
    const lines = envContent.split('\n');
    
    for (const line of lines) {
      const trimmedLine = line.trim();
      if (trimmedLine.startsWith(`${key}=`)) {
        return trimmedLine.split('=')[1];
      }
    }
  } catch (error) {
    console.warn(`Failed to read backend .env file: ${error}`);
  }
  
  return undefined;
}

/**
 * Get IGDB credentials from environment or backend/.env file
 * Priority: 1. Environment variables, 2. backend/.env file, 3. Fail
 */
function getIGDBCredentials(): { clientId: string; clientSecret: string } {
  const clientId = process.env.IGDB_CLIENT_ID || readBackendEnv('IGDB_CLIENT_ID');
  const clientSecret = process.env.IGDB_CLIENT_SECRET || readBackendEnv('IGDB_CLIENT_SECRET');
  
  if (!clientId || !clientSecret) {
    throw new Error(
      'IGDB credentials not found. Please set IGDB_CLIENT_ID and IGDB_CLIENT_SECRET ' +
      'environment variables or ensure they are present in backend/.env file.'
    );
  }
  
  console.log('✅ IGDB credentials found for E2E tests');
  return { clientId, clientSecret };
}

// Validate IGDB credentials before test configuration
const { clientId, clientSecret } = getIGDBCredentials();

export default defineConfig({
  testDir: './tests/e2e',
  /* Cleanup temporary database file after tests complete */
  globalTeardown: './global-teardown.ts',
  /* Run tests in files in parallel */
  fullyParallel: true,
  /* Fail the build on CI if you accidentally left test.only in the source code. */
  forbidOnly: !!process.env.CI,
  /* Retry on CI only */
  retries: process.env.CI ? 2 : 0,
  /* Reduce parallel tests for better stability */
  workers: process.env.CI ? 1 : 2,
  /* Reporter to use. See https://playwright.dev/docs/test-reporters */
  reporter: 'html',
  /* Global timeout for entire test suite */
  globalTimeout: 10 * 60 * 1000, // 10 minutes
  /* Shared settings for all the projects below. See https://playwright.dev/docs/api/class-testoptions. */
  use: {
    /* Base URL to use in actions like `await page.goto('/')`. */
    baseURL: 'http://localhost:15173',

    /* Collect trace when retrying the failed test. See https://playwright.dev/docs/trace-viewer */
    trace: 'on-first-retry',

    /* Increase timeouts for E2E tests */
    actionTimeout: 30000,
    navigationTimeout: 30000,
  },
  
  /* Individual test timeout */
  timeout: 60000, // 1 minute per test

  /* Configure projects for major browsers */
  projects: [
    // Auth setup project - runs first, creates both admin and regular users
    {
      name: 'auth-setup',
      testMatch: 'tests/auth.setup.ts',
      testDir: './',
      use: {
        ...devices['Desktop Firefox'],
        baseURL: 'http://localhost:15173',
      },
    },

    // Main test project - each test logs in with needed credentials
    {
      name: 'firefox',
      testIgnore: [/.*\.setup\.ts/],
      dependencies: ['auth-setup'],
      use: {
        ...devices['Desktop Firefox'],
        baseURL: 'http://localhost:15173',
        // No storageState - tests login explicitly as needed
      },
    },

    // {
    //   name: 'webkit',
    //   use: { ...devices['Desktop Safari'] },
    // },

    /* Test against mobile viewports. */
    // {
    //   name: 'Mobile Chrome',
    //   use: { ...devices['Pixel 5'] },
    // },
    // {
    //   name: 'Mobile Safari',
    //   use: { ...devices['iPhone 12'] },
    // },

    /* Test against branded browsers. */
    // {
    //   name: 'Microsoft Edge',
    //   use: { ...devices['Desktop Edge'], channel: 'msedge' },
    // },
    // {
    //   name: 'Google Chrome',
    //   use: { ...devices['Desktop Chrome'], channel: 'chrome' },
    // },
  ],

  /* Run both backend and frontend servers before starting the tests */
  webServer: [
    {
      command: `cd ../backend && uv run uvicorn app.main:app --host 127.0.0.1 --port 8001 --log-level warning --workers 1`,
      port: 8001,
      reuseExistingServer: !process.env.CI,
      env: {
        // Database configuration for testing
        DATABASE_URL: `sqlite:///${tempDbPath}`,
        SECRET_KEY: 'test-secret-key',
        CORS_ORIGINS: 'http://localhost:15173',
        // IGDB API credentials for real API integration
        IGDB_CLIENT_ID: clientId,
        IGDB_CLIENT_SECRET: clientSecret,
        // SQLite optimizations for testing
        PRAGMA_SYNCHRONOUS: 'OFF',
        PRAGMA_JOURNAL_MODE: 'MEMORY'
      },
      timeout: 60000, // 1 minute for backend to start (reduced from 2 minutes)
    },
    {
      command: 'npm run dev -- --port 15173',
      port: 15173,
      reuseExistingServer: !process.env.CI,
      env: {
        // Configure frontend to use test backend
        PUBLIC_API_URL: 'http://localhost:8001/api',
        PUBLIC_STATIC_URL: 'http://localhost:8001'
      },
      timeout: 30000, // 30 seconds for frontend to start
    },
  ],
});