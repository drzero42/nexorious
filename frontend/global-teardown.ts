import { FullConfig } from '@playwright/test';
import fs from 'fs';
import { tempDbPath } from './playwright.config';

async function globalTeardown(config: FullConfig): Promise<void> {
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