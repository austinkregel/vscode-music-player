/**
 * Mock database for testing
 * Creates an in-memory SQLite database
 */

import * as os from 'os';
import * as path from 'path';
import * as fs from 'fs';
import { MetadataCache } from '../../metadata/cache';

/**
 * Create a temporary database path for testing
 */
export function createTempDbPath(): string {
  const tmpDir = os.tmpdir();
  const dbName = `test-db-${Date.now()}-${Math.random().toString(36).substring(2)}.db`;
  return path.join(tmpDir, dbName);
}

/**
 * Create an in-memory metadata cache for testing
 */
export async function createTestCache(): Promise<MetadataCache> {
  const dbPath = createTempDbPath();
  const cache = new MetadataCache(dbPath);
  await cache.initialize();
  return cache;
}

/**
 * Cleanup a test database
 */
export function cleanupTestDb(dbPath: string): void {
  try {
    if (fs.existsSync(dbPath)) {
      fs.unlinkSync(dbPath);
    }
  } catch {
    // Ignore cleanup errors
  }
}
