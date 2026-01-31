/**
 * Playlist manager
 * Handles creation, editing, and persistence of playlists
 */

import { MetadataCache } from '../metadata/cache';
import type { Playlist, Track } from '../metadata/models';

/**
 * Playlist manager for CRUD operations
 */
export class PlaylistManager {
  private cache: MetadataCache;

  constructor(cache: MetadataCache) {
    this.cache = cache;
  }

  /**
   * Create a new playlist
   */
  async create(name: string): Promise<Playlist> {
    return this.cache.createPlaylist({ name });
  }

  /**
   * Get all playlists
   */
  async getAll(): Promise<Playlist[]> {
    return this.cache.getAllPlaylists();
  }

  /**
   * Delete a playlist
   */
  async delete(playlistId: string): Promise<void> {
    await this.cache.deletePlaylist(playlistId);
    await this.cache.save();
  }

  /**
   * Rename a playlist
   */
  async rename(playlistId: string, newName: string): Promise<void> {
    // This would require adding an updatePlaylist method to the cache
    // For now, we'll handle this in a future iteration
    throw new Error('Not implemented');
  }

  /**
   * Add a track to a playlist
   */
  async addTrack(playlistId: string, trackId: string): Promise<void> {
    await this.cache.addTrackToPlaylist(playlistId, trackId);
    await this.cache.save();
  }

  /**
   * Add multiple tracks to a playlist
   */
  async addTracks(playlistId: string, trackIds: string[]): Promise<void> {
    for (const trackId of trackIds) {
      await this.cache.addTrackToPlaylist(playlistId, trackId);
    }
    await this.cache.save();
  }

  /**
   * Remove a track from a playlist
   */
  async removeTrack(playlistId: string, trackId: string): Promise<void> {
    await this.cache.removeTrackFromPlaylist(playlistId, trackId);
    await this.cache.save();
  }

  /**
   * Get tracks in a playlist
   */
  async getTracks(playlistId: string): Promise<Track[]> {
    return this.cache.getPlaylistTracks(playlistId);
  }

  /**
   * Reorder tracks in a playlist
   */
  async reorder(playlistId: string, fromIndex: number, toIndex: number): Promise<void> {
    const tracks = await this.getTracks(playlistId);
    
    if (fromIndex < 0 || fromIndex >= tracks.length ||
        toIndex < 0 || toIndex >= tracks.length) {
      throw new Error('Invalid index');
    }

    // Remove all tracks and re-add in new order
    for (const track of tracks) {
      await this.cache.removeTrackFromPlaylist(playlistId, track.id);
    }

    // Reorder
    const [removed] = tracks.splice(fromIndex, 1);
    tracks.splice(toIndex, 0, removed);

    // Re-add in new order
    for (const track of tracks) {
      await this.cache.addTrackToPlaylist(playlistId, track.id);
    }

    await this.cache.save();
  }

  /**
   * Get file paths for all tracks in a playlist
   * (for sending to the daemon)
   */
  async getTrackPaths(playlistId: string): Promise<string[]> {
    const tracks = await this.getTracks(playlistId);
    return tracks.map((t) => t.path);
  }

  /**
   * Duplicate a playlist
   */
  async duplicate(playlistId: string, newName: string): Promise<Playlist> {
    const tracks = await this.getTracks(playlistId);
    const newPlaylist = await this.create(newName);

    for (const track of tracks) {
      await this.cache.addTrackToPlaylist(newPlaylist.id, track.id);
    }

    await this.cache.save();
    return newPlaylist;
  }
}
