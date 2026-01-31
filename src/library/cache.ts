/**
 * Library metadata cache
 * Persists scan results in VS Code's global state
 */

import * as vscode from 'vscode';
import type { 
  ScanResponse, 
  ScanFileInfo, 
  ArtistNFO, 
  AlbumNFO, 
  ScanMetadata 
} from '../types';

/**
 * Cached library data structure
 */
export interface CachedLibrary {
  lastScanTime: number;
  libraryPaths: string[];
  tracks: ScanFileInfo[];
  artists: ArtistNFO[];
  albums: AlbumNFO[];
  artwork: Record<string, string[]>;
  playlists: CachedPlaylist[];
}

export interface CachedPlaylist {
  id: string;
  name: string;
  createdAt: number;
  tracks: string[]; // File paths
}

const CACHE_KEY = 'localMedia.libraryCache';
const CACHE_VERSION = 1;

/**
 * Library cache manager
 * Persists library metadata between sessions
 */
export class LibraryCache {
  private globalState: vscode.Memento;
  private cache: CachedLibrary;
  private onChangeEmitter = new vscode.EventEmitter<void>();
  
  readonly onChange = this.onChangeEmitter.event;

  constructor(globalState: vscode.Memento) {
    this.globalState = globalState;
    this.cache = this.loadCache();
  }

  /**
   * Load cache from storage
   */
  private loadCache(): CachedLibrary {
    const stored = this.globalState.get<{ version: number; data: CachedLibrary }>(CACHE_KEY);
    
    if (stored && stored.version === CACHE_VERSION) {
      return stored.data;
    }

    // Return empty cache
    return {
      lastScanTime: 0,
      libraryPaths: [],
      tracks: [],
      artists: [],
      albums: [],
      artwork: {},
      playlists: [],
    };
  }

  /**
   * Save cache to storage
   */
  private async saveCache(): Promise<void> {
    await this.globalState.update(CACHE_KEY, {
      version: CACHE_VERSION,
      data: this.cache,
    });
    this.onChangeEmitter.fire();
  }

  /**
   * Check if cache has any data
   */
  hasData(): boolean {
    return this.cache.tracks.length > 0 || 
           this.cache.artists.length > 0 || 
           this.cache.albums.length > 0;
  }

  /**
   * Get last scan time
   */
  getLastScanTime(): Date | null {
    return this.cache.lastScanTime ? new Date(this.cache.lastScanTime) : null;
  }

  /**
   * Get library paths
   */
  getLibraryPaths(): string[] {
    return this.cache.libraryPaths;
  }

  /**
   * Update cache from scan results
   */
  async updateFromScan(scanResponse: ScanResponse, libraryPaths: string[]): Promise<void> {
    // Collect all tracks
    const tracks: ScanFileInfo[] = [];
    for (const result of scanResponse.results) {
      tracks.push(...result.files);
    }

    this.cache.lastScanTime = Date.now();
    this.cache.libraryPaths = libraryPaths;
    this.cache.tracks = tracks;

    // Update metadata if available
    if (scanResponse.metadata) {
      this.cache.artists = scanResponse.metadata.artists;
      this.cache.albums = scanResponse.metadata.albums;
      this.cache.artwork = scanResponse.metadata.artwork;
    }

    await this.saveCache();
  }

  /**
   * Get all tracks
   */
  getTracks(): ScanFileInfo[] {
    return this.cache.tracks;
  }

  /**
   * Get a track by its path
   */
  getTrackByPath(filePath: string): ScanFileInfo | undefined {
    return this.cache.tracks.find(t => t.path === filePath);
  }

  /**
   * Get all artists
   */
  getArtists(): ArtistNFO[] {
    return this.cache.artists;
  }

  /**
   * Get all albums
   */
  getAlbums(): AlbumNFO[] {
    return this.cache.albums;
  }

  /**
   * Get artwork map
   */
  getArtwork(): Record<string, string[]> {
    return this.cache.artwork;
  }

  /**
   * Get all playlists
   */
  getPlaylists(): CachedPlaylist[] {
    return this.cache.playlists;
  }

  /**
   * Create a new playlist
   */
  async createPlaylist(name: string): Promise<CachedPlaylist> {
    const playlist: CachedPlaylist = {
      id: Date.now().toString(),
      name,
      createdAt: Date.now(),
      tracks: [],
    };
    
    this.cache.playlists.push(playlist);
    await this.saveCache();
    
    return playlist;
  }

  /**
   * Delete a playlist
   */
  async deletePlaylist(playlistId: string): Promise<void> {
    this.cache.playlists = this.cache.playlists.filter(p => p.id !== playlistId);
    await this.saveCache();
  }

  /**
   * Rename a playlist
   */
  async renamePlaylist(playlistId: string, newName: string): Promise<void> {
    const playlist = this.cache.playlists.find(p => p.id === playlistId);
    if (playlist) {
      playlist.name = newName;
      await this.saveCache();
    }
  }

  /**
   * Add track to playlist
   */
  async addToPlaylist(playlistId: string, trackPath: string): Promise<void> {
    const playlist = this.cache.playlists.find(p => p.id === playlistId);
    if (playlist && !playlist.tracks.includes(trackPath)) {
      playlist.tracks.push(trackPath);
      await this.saveCache();
    }
  }

  /**
   * Remove track from playlist
   */
  async removeFromPlaylist(playlistId: string, trackPath: string): Promise<void> {
    const playlist = this.cache.playlists.find(p => p.id === playlistId);
    if (playlist) {
      playlist.tracks = playlist.tracks.filter(t => t !== trackPath);
      await this.saveCache();
    }
  }

  /**
   * Reorder tracks in playlist
   */
  async reorderPlaylist(playlistId: string, fromIndex: number, toIndex: number): Promise<void> {
    const playlist = this.cache.playlists.find(p => p.id === playlistId);
    if (playlist && fromIndex >= 0 && fromIndex < playlist.tracks.length) {
      const [track] = playlist.tracks.splice(fromIndex, 1);
      playlist.tracks.splice(toIndex, 0, track);
      await this.saveCache();
    }
  }

  /**
   * Get playlist by ID
   */
  getPlaylist(playlistId: string): CachedPlaylist | undefined {
    return this.cache.playlists.find(p => p.id === playlistId);
  }

  /**
   * Search tracks by query
   */
  searchTracks(query: string): ScanFileInfo[] {
    const lowerQuery = query.toLowerCase();
    return this.cache.tracks.filter(track => 
      track.path.toLowerCase().includes(lowerQuery)
    );
  }

  /**
   * Search artists by query
   */
  searchArtists(query: string): ArtistNFO[] {
    const lowerQuery = query.toLowerCase();
    return this.cache.artists.filter(artist =>
      artist.name.toLowerCase().includes(lowerQuery)
    );
  }

  /**
   * Search albums by query
   */
  searchAlbums(query: string): AlbumNFO[] {
    const lowerQuery = query.toLowerCase();
    return this.cache.albums.filter(album =>
      album.title.toLowerCase().includes(lowerQuery) ||
      (album.artist?.toLowerCase().includes(lowerQuery))
    );
  }

  /**
   * Get albums for an artist
   */
  getAlbumsForArtist(artistName: string): AlbumNFO[] {
    return this.cache.albums.filter(album => album.artist === artistName);
  }

  /**
   * Get tracks for an album path
   */
  getTracksForAlbum(albumPath: string): ScanFileInfo[] {
    const path = require('path');
    return this.cache.tracks.filter(track => 
      path.dirname(track.path) === albumPath
    );
  }

  /**
   * Clear all cached data
   */
  async clear(): Promise<void> {
    this.cache = {
      lastScanTime: 0,
      libraryPaths: [],
      tracks: [],
      artists: [],
      albums: [],
      artwork: {},
      playlists: this.cache.playlists, // Keep playlists
    };
    await this.saveCache();
  }

  /**
   * Get statistics
   */
  getStats(): { tracks: number; artists: number; albums: number; playlists: number } {
    return {
      tracks: this.cache.tracks.length,
      artists: this.cache.artists.length,
      albums: this.cache.albums.length,
      playlists: this.cache.playlists.length,
    };
  }
}
