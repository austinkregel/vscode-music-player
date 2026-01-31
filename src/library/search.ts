/**
 * Full-text search over metadata cache
 */

import * as vscode from 'vscode';
import { MetadataCache } from '../metadata/cache';
import type { Track, Album, Artist } from '../metadata/models';

export interface SearchResult {
  type: 'track' | 'album' | 'artist';
  id: string;
  label: string;
  description?: string;
  detail?: string;
}

/**
 * Search provider for the metadata cache
 */
export class LibrarySearch {
  private cache: MetadataCache;

  constructor(cache: MetadataCache) {
    this.cache = cache;
  }

  /**
   * Search for tracks matching a query
   */
  async searchTracks(query: string): Promise<Track[]> {
    return this.cache.search(query);
  }

  /**
   * Search across all content types
   */
  async searchAll(query: string): Promise<SearchResult[]> {
    const results: SearchResult[] = [];

    // Search tracks
    const tracks = await this.cache.search(query);
    for (const track of tracks) {
      results.push({
        type: 'track',
        id: track.id,
        label: track.title || path.basename(track.path),
        description: track.artist || undefined,
        detail: track.album || undefined,
      });
    }

    // Search albums by name
    const albums = await this.cache.getAllAlbums();
    const matchingAlbums = albums.filter(
      (a) => a.name.toLowerCase().includes(query.toLowerCase())
    );
    for (const album of matchingAlbums) {
      results.push({
        type: 'album',
        id: album.id,
        label: album.name,
        description: album.artist || undefined,
        detail: album.year?.toString() || undefined,
      });
    }

    // Search artists by name
    const artists = await this.cache.getAllArtists();
    const matchingArtists = artists.filter(
      (a) => a.name.toLowerCase().includes(query.toLowerCase())
    );
    for (const artist of matchingArtists) {
      results.push({
        type: 'artist',
        id: artist.id,
        label: artist.name,
      });
    }

    return results;
  }

  /**
   * Show a quick pick for searching the library
   */
  async showSearchQuickPick(): Promise<SearchResult | undefined> {
    const result = await vscode.window.showInputBox({
      prompt: 'Search your music library',
      placeHolder: 'Enter song, artist, or album name',
    });

    if (!result) {
      return undefined;
    }

    const searchResults = await this.searchAll(result);

    if (searchResults.length === 0) {
      vscode.window.showInformationMessage('No results found');
      return undefined;
    }

    const items: vscode.QuickPickItem[] = searchResults.map((r) => ({
      label: r.label,
      description: r.description,
      detail: `${r.type}: ${r.detail || ''}`,
    }));

    const selected = await vscode.window.showQuickPick(items, {
      placeHolder: 'Select a result',
    });

    if (!selected) {
      return undefined;
    }

    const index = items.indexOf(selected);
    return searchResults[index];
  }
}

// Node.js path module for basename
import * as path from 'path';
