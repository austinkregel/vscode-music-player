/**
 * Metadata models
 * TypeScript interfaces for database entities
 */

export interface Track {
  id: string;
  path: string;
  title: string | null;
  artist: string | null;
  album: string | null;
  albumArtist: string | null;
  genre: string | null;
  year: number | null;
  trackNumber: number | null;
  discNumber: number | null;
  durationMs: number | null;
  coverArtPath: string | null;
  fileModifiedAt: number | null;
  scannedAt: number | null;
}

export interface Album {
  id: string;
  name: string;
  artist: string | null;
  year: number | null;
  coverArtPath: string | null;
}

export interface Artist {
  id: string;
  name: string;
}

export interface Playlist {
  id: string;
  name: string;
  createdAt: number;
  updatedAt: number;
}

export interface PlaylistTrack {
  playlistId: string;
  trackId: string;
  position: number;
}

/**
 * Input types for creating/updating records
 */
export interface TrackInput {
  path: string;
  title?: string | null;
  artist?: string | null;
  album?: string | null;
  albumArtist?: string | null;
  genre?: string | null;
  year?: number | null;
  trackNumber?: number | null;
  discNumber?: number | null;
  durationMs?: number | null;
  coverArtPath?: string | null;
  fileModifiedAt?: number | null;
}

export interface PlaylistInput {
  name: string;
}

/**
 * Generate a unique ID for a track based on its path
 */
export function generateTrackId(path: string): string {
  // Simple hash of the path
  let hash = 0;
  for (let i = 0; i < path.length; i++) {
    const char = path.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash; // Convert to 32-bit integer
  }
  return Math.abs(hash).toString(16).padStart(8, '0');
}

/**
 * Generate a unique ID for an album based on name and artist
 */
export function generateAlbumId(name: string, artist: string | null): string {
  const key = `${name}::${artist || ''}`;
  let hash = 0;
  for (let i = 0; i < key.length; i++) {
    const char = key.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }
  return Math.abs(hash).toString(16).padStart(8, '0');
}

/**
 * Generate a unique ID for an artist
 */
export function generateArtistId(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    const char = name.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }
  return Math.abs(hash).toString(16).padStart(8, '0');
}

/**
 * Generate a unique playlist ID
 */
export function generatePlaylistId(): string {
  return Date.now().toString(36) + Math.random().toString(36).substring(2, 8);
}
