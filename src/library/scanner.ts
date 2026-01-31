/**
 * Library scanner
 * Walks the music library directory and populates the metadata cache
 */

import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { MetadataCache } from '../metadata/cache';
import { readTags, isSupportedAudioFile, SUPPORTED_EXTENSIONS } from '../metadata/tagReader';
import type { TrackInput } from '../metadata/models';

export interface ScanProgress {
  current: number;
  total: number;
  currentFile: string;
}

export interface ScanResult {
  scanned: number;
  skipped: number;
  errors: number;
  duration: number;
}

/**
 * Library scanner for populating the metadata cache
 */
export class LibraryScanner {
  private cache: MetadataCache;
  private scanning: boolean = false;
  private cancelled: boolean = false;
  public scannedCount: number = 0;

  constructor(cache: MetadataCache) {
    this.cache = cache;
  }

  /**
   * Check if currently scanning
   */
  get isScanning(): boolean {
    return this.scanning;
  }

  /**
   * Cancel the current scan
   */
  cancel(): void {
    this.cancelled = true;
  }

  /**
   * Discover all audio files in a directory tree
   */
  async discoverFiles(libraryPath: string): Promise<string[]> {
    const files: string[] = [];
    await this.walkDirectory(libraryPath, files);
    return files;
  }

  /**
   * Recursively walk a directory and collect audio files
   */
  private async walkDirectory(dir: string, files: string[]): Promise<void> {
    if (this.cancelled) {
      return;
    }

    let entries: fs.Dirent[];
    try {
      entries = await fs.promises.readdir(dir, { withFileTypes: true });
    } catch (err) {
      // Skip inaccessible directories
      return;
    }

    for (const entry of entries) {
      if (this.cancelled) {
        return;
      }

      const fullPath = path.join(dir, entry.name);

      if (entry.isDirectory()) {
        // Skip hidden directories
        if (!entry.name.startsWith('.')) {
          await this.walkDirectory(fullPath, files);
        }
      } else if (entry.isFile()) {
        if (isSupportedAudioFile(entry.name)) {
          files.push(fullPath);
        }
      }
    }
  }

  /**
   * Scan a single file and add to cache
   */
  async scanFile(filePath: string): Promise<boolean> {
    try {
      // Get file stats
      const stats = await fs.promises.stat(filePath);
      
      // Check if file has changed since last scan
      const existing = await this.cache.getTrackByPath(filePath);
      if (existing && existing.fileModifiedAt && existing.fileModifiedAt >= stats.mtimeMs) {
        return false; // Skip, file unchanged
      }

      // Read metadata
      const input = await readTags(filePath);
      input.fileModifiedAt = stats.mtimeMs;

      // Upsert to cache
      await this.cache.upsertTrack(input);
      this.scannedCount++;

      return true;
    } catch (err) {
      // Log error but continue scanning
      console.error(`Error scanning ${filePath}:`, err);
      return false;
    }
  }

  /**
   * Scan the entire library
   */
  async scan(
    libraryPath: string,
    onProgress?: (progress: ScanProgress) => void
  ): Promise<ScanResult> {
    if (this.scanning) {
      throw new Error('Scan already in progress');
    }

    this.scanning = true;
    this.cancelled = false;
    this.scannedCount = 0;

    const startTime = Date.now();
    let scanned = 0;
    let skipped = 0;
    let errors = 0;

    try {
      // Discover files
      const files = await this.discoverFiles(libraryPath);

      for (let i = 0; i < files.length; i++) {
        if (this.cancelled) {
          break;
        }

        const file = files[i];

        if (onProgress) {
          onProgress({
            current: i + 1,
            total: files.length,
            currentFile: file,
          });
        }

        try {
          const wasScanned = await this.scanFile(file);
          if (wasScanned) {
            scanned++;
          } else {
            skipped++;
          }
        } catch (err) {
          errors++;
        }
      }

      // Save the cache
      await this.cache.save();

      return {
        scanned,
        skipped,
        errors,
        duration: Date.now() - startTime,
      };
    } finally {
      this.scanning = false;
    }
  }

  /**
   * Scan with VS Code progress indicator
   */
  async scanWithProgress(libraryPath: string): Promise<ScanResult> {
    return vscode.window.withProgress(
      {
        location: vscode.ProgressLocation.Notification,
        title: 'Scanning music library',
        cancellable: true,
      },
      async (progress, token) => {
        token.onCancellationRequested(() => {
          this.cancel();
        });

        return this.scan(libraryPath, (scanProgress) => {
          const percentage = (scanProgress.current / scanProgress.total) * 100;
          const fileName = path.basename(scanProgress.currentFile);
          progress.report({
            increment: 100 / scanProgress.total,
            message: `${scanProgress.current}/${scanProgress.total}: ${fileName}`,
          });
        });
      }
    );
  }
}
