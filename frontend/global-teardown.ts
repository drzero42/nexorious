import { FullConfig } from '@playwright/test';
import fs from 'fs';
import { exec } from 'child_process';
import { promisify } from 'util';
import { tempDbPath } from './playwright.config';

const execAsync = promisify(exec);

async function globalTeardown(config: FullConfig): Promise<void> {
  // Kill backend server (port 8001) - graceful SIGTERM
  try {
    await execAsync("lsof -ti:8001 | xargs kill 2>/dev/null || true");
    console.log('Stopped backend server on port 8001');
  } catch (error) {
    // Server might not be running, that's ok
  }

  // Kill frontend server (port 15173) - graceful SIGTERM
  try {
    await execAsync("lsof -ti:15173 | xargs kill 2>/dev/null || true");
    console.log('Stopped frontend server on port 15173');
  } catch (error) {
    // Server might not be running, that's ok
  }

  // Clean up database file
  try {
    if (fs.existsSync(tempDbPath)) {
      fs.unlinkSync(tempDbPath);
      console.log(`Cleaned up temporary test database: ${tempDbPath}`);
    }
  } catch (error) {
    console.warn(`Failed to cleanup temporary database file: ${error}`);
  }
}

export default globalTeardown;