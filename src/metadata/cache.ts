/**
 * SQLite metadata cache
 * Stores track, album, artist, and playlist information
 */

import * as fs from 'fs';
import * as path from 'path';
import type {
  Track,
  Album,
  Artist,
  Playlist,
  PlaylistTrack,
  TrackInput,
  PlaylistInput,
} from './models';
import {
  generateTrackId,
  generateAlbumId,
  generateArtistId,
  generatePlaylistId,
} from './models';

// Dynamic import for sql.js to handle ESM/CJS compatibility
type SqlJsDatabase = import('sql.js').Database;
type SqlJsStatic = import('sql.js').SqlJsStatic;

const SCHEMA_VERSION = 1;

/**
 * Metadata cache using SQLite (via sql.js WASM)
 */
export class MetadataCache {
  private db: SqlJsDatabase | null = null;
  private dbPath: string;
  private sqlPromise: Promise<SqlJsStatic> | null = null;

  constructor(dbPath: string) {
    this.dbPath = dbPath;
  }

  /**
   * Initialize the database
   */
  async initialize(): Promise<void> {
    // Dynamic import for sql.js
    const initSqlJs = (await import('sql.js')).default;
    const SQL = await initSqlJs();

    // Load existing database if it exists
    if (fs.existsSync(this.dbPath)) {
      const buffer = fs.readFileSync(this.dbPath);
      this.db = new SQL.Database(buffer);
    } else {
      this.db = new SQL.Database();
    }

    // Run migrations
    await this.runMigrations();
  }

  /**
   * Run database migrations
   */
  private async runMigrations(): Promise<void> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    // Create migrations table if not exists
    this.db.run(`
      CREATE TABLE IF NOT EXISTS migrations (
        version INTEGER PRIMARY KEY,
        applied_at INTEGER NOT NULL
      )
    `);

    // Get current version
    const result = this.db.exec('SELECT MAX(version) as version FROM migrations');
    const currentVersion = result[0]?.values[0]?.[0] as number || 0;

    // Run pending migrations
    if (currentVersion < 1) {
      await this.migration001();
    }
  }

  /**
   * Initial schema migration
   */
  private async migration001(): Promise<void> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    // Create tables
    this.db.run(`
      CREATE TABLE IF NOT EXISTS tracks (
        id TEXT PRIMARY KEY,
        path TEXT UNIQUE NOT NULL,
        title TEXT,
        artist TEXT,
        album TEXT,
        album_artist TEXT,
        genre TEXT,
        year INTEGER,
        track_number INTEGER,
        disc_number INTEGER,
        duration_ms INTEGER,
        cover_art_path TEXT,
        file_modified_at INTEGER,
        scanned_at INTEGER
      )
    `);

    this.db.run(`
      CREATE TABLE IF NOT EXISTS albums (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        artist TEXT,
        year INTEGER,
        cover_art_path TEXT
      )
    `);

    this.db.run(`
      CREATE TABLE IF NOT EXISTS artists (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL
      )
    `);

    this.db.run(`
      CREATE TABLE IF NOT EXISTS playlists (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
      )
    `);

    this.db.run(`
      CREATE TABLE IF NOT EXISTS playlist_tracks (
        playlist_id TEXT NOT NULL,
        track_id TEXT NOT NULL,
        position INTEGER NOT NULL,
        PRIMARY KEY (playlist_id, track_id),
        FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE,
        FOREIGN KEY (track_id) REFERENCES tracks(id) ON DELETE CASCADE
      )
    `);

    // Create indexes
    this.db.run('CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album)');
    this.db.run('CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist)');
    this.db.run('CREATE INDEX IF NOT EXISTS idx_tracks_title ON tracks(title)');

    // Record migration
    this.db.run('INSERT INTO migrations (version, applied_at) VALUES (1, ?)', [Date.now()]);

    await this.save();
  }

  /**
   * Save database to disk
   */
  async save(): Promise<void> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const data = this.db.export();
    const buffer = Buffer.from(data);

    // Ensure directory exists
    const dir = path.dirname(this.dbPath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }

    fs.writeFileSync(this.dbPath, buffer);
  }

  /**
   * Close the database
   */
  close(): void {
    if (this.db) {
      this.db.close();
      this.db = null;
    }
  }

  // =========================================================================
  // Track Operations
  // =========================================================================

  /**
   * Upsert a track
   */
  async upsertTrack(input: TrackInput): Promise<Track> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const id = generateTrackId(input.path);
    const now = Date.now();

    this.db.run(`
      INSERT INTO tracks (id, path, title, artist, album, album_artist, genre, year, track_number, disc_number, duration_ms, cover_art_path, file_modified_at, scanned_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(path) DO UPDATE SET
        title = excluded.title,
        artist = excluded.artist,
        album = excluded.album,
        album_artist = excluded.album_artist,
        genre = excluded.genre,
        year = excluded.year,
        track_number = excluded.track_number,
        disc_number = excluded.disc_number,
        duration_ms = excluded.duration_ms,
        cover_art_path = excluded.cover_art_path,
        file_modified_at = excluded.file_modified_at,
        scanned_at = excluded.scanned_at
    `, [
      id,
      input.path,
      input.title ?? null,
      input.artist ?? null,
      input.album ?? null,
      input.albumArtist ?? null,
      input.genre ?? null,
      input.year ?? null,
      input.trackNumber ?? null,
      input.discNumber ?? null,
      input.durationMs ?? null,
      input.coverArtPath ?? null,
      input.fileModifiedAt ?? null,
      now,
    ]);

    // Also update albums and artists tables
    if (input.album) {
      await this.upsertAlbum(input.album, input.albumArtist || input.artist || null, input.year || null);
    }

    if (input.artist) {
      await this.upsertArtist(input.artist);
    }

    if (input.albumArtist && input.albumArtist !== input.artist) {
      await this.upsertArtist(input.albumArtist);
    }

    return this.getTrackById(id) as Promise<Track>;
  }

  /**
   * Get a track by ID
   */
  async getTrackById(id: string): Promise<Track | null> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec('SELECT * FROM tracks WHERE id = ?', [id]);
    if (result.length === 0 || result[0].values.length === 0) {
      return null;
    }

    return this.rowToTrack(result[0].columns, result[0].values[0]);
  }

  /**
   * Get a track by path
   */
  async getTrackByPath(filePath: string): Promise<Track | null> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec('SELECT * FROM tracks WHERE path = ?', [filePath]);
    if (result.length === 0 || result[0].values.length === 0) {
      return null;
    }

    return this.rowToTrack(result[0].columns, result[0].values[0]);
  }

  /**
   * Get all tracks
   */
  async getAllTracks(): Promise<Track[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec('SELECT * FROM tracks ORDER BY artist, album, track_number, title');
    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => this.rowToTrack(result[0].columns, row));
  }

  /**
   * Get tracks by album
   */
  async getTracksByAlbum(albumName: string): Promise<Track[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec(
      'SELECT * FROM tracks WHERE album = ? ORDER BY disc_number, track_number, title',
      [albumName]
    );
    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => this.rowToTrack(result[0].columns, row));
  }

  /**
   * Get tracks by artist
   */
  async getTracksByArtist(artistName: string): Promise<Track[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec(
      'SELECT * FROM tracks WHERE artist = ? OR album_artist = ? ORDER BY album, disc_number, track_number, title',
      [artistName, artistName]
    );
    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => this.rowToTrack(result[0].columns, row));
  }

  /**
   * Search tracks by title, artist, or album
   */
  async search(query: string): Promise<Track[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const pattern = `%${query}%`;
    const result = this.db.exec(
      `SELECT * FROM tracks 
       WHERE title LIKE ? OR artist LIKE ? OR album LIKE ?
       ORDER BY artist, album, track_number, title
       LIMIT 100`,
      [pattern, pattern, pattern]
    );

    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => this.rowToTrack(result[0].columns, row));
  }

  /**
   * Delete a track
   */
  async deleteTrack(id: string): Promise<void> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    this.db.run('DELETE FROM tracks WHERE id = ?', [id]);
  }

  /**
   * Get track count
   */
  async getTrackCount(): Promise<number> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec('SELECT COUNT(*) FROM tracks');
    return result[0].values[0][0] as number;
  }

  private rowToTrack(columns: string[], row: unknown[]): Track {
    const obj: Record<string, unknown> = {};
    columns.forEach((col, i) => {
      obj[col] = row[i];
    });

    return {
      id: obj.id as string,
      path: obj.path as string,
      title: obj.title as string | null,
      artist: obj.artist as string | null,
      album: obj.album as string | null,
      albumArtist: obj.album_artist as string | null,
      genre: obj.genre as string | null,
      year: obj.year as number | null,
      trackNumber: obj.track_number as number | null,
      discNumber: obj.disc_number as number | null,
      durationMs: obj.duration_ms as number | null,
      coverArtPath: obj.cover_art_path as string | null,
      fileModifiedAt: obj.file_modified_at as number | null,
      scannedAt: obj.scanned_at as number | null,
    };
  }

  // =========================================================================
  // Album Operations
  // =========================================================================

  /**
   * Upsert an album
   */
  async upsertAlbum(name: string, artist: string | null, year: number | null): Promise<Album> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const id = generateAlbumId(name, artist);

    this.db.run(`
      INSERT INTO albums (id, name, artist, year)
      VALUES (?, ?, ?, ?)
      ON CONFLICT(id) DO UPDATE SET
        year = COALESCE(excluded.year, albums.year)
    `, [id, name, artist, year]);

    return { id, name, artist, year, coverArtPath: null };
  }

  /**
   * Get all albums
   */
  async getAllAlbums(): Promise<Album[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec('SELECT * FROM albums ORDER BY artist, name');
    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => ({
      id: row[0] as string,
      name: row[1] as string,
      artist: row[2] as string | null,
      year: row[3] as number | null,
      coverArtPath: row[4] as string | null,
    }));
  }

  // =========================================================================
  // Artist Operations
  // =========================================================================

  /**
   * Upsert an artist
   */
  async upsertArtist(name: string): Promise<Artist> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const id = generateArtistId(name);

    this.db.run(`
      INSERT OR IGNORE INTO artists (id, name) VALUES (?, ?)
    `, [id, name]);

    return { id, name };
  }

  /**
   * Get all artists
   */
  async getAllArtists(): Promise<Artist[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec('SELECT * FROM artists ORDER BY name');
    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => ({
      id: row[0] as string,
      name: row[1] as string,
    }));
  }

  // =========================================================================
  // Playlist Operations
  // =========================================================================

  /**
   * Create a playlist
   */
  async createPlaylist(input: PlaylistInput): Promise<Playlist> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const id = generatePlaylistId();
    const now = Date.now();

    this.db.run(`
      INSERT INTO playlists (id, name, created_at, updated_at)
      VALUES (?, ?, ?, ?)
    `, [id, input.name, now, now]);

    return { id, name: input.name, createdAt: now, updatedAt: now };
  }

  /**
   * Get all playlists
   */
  async getAllPlaylists(): Promise<Playlist[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec('SELECT * FROM playlists ORDER BY name');
    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => ({
      id: row[0] as string,
      name: row[1] as string,
      createdAt: row[2] as number,
      updatedAt: row[3] as number,
    }));
  }

  /**
   * Delete a playlist
   */
  async deletePlaylist(id: string): Promise<void> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    this.db.run('DELETE FROM playlists WHERE id = ?', [id]);
    this.db.run('DELETE FROM playlist_tracks WHERE playlist_id = ?', [id]);
  }

  /**
   * Add a track to a playlist
   */
  async addTrackToPlaylist(playlistId: string, trackId: string): Promise<void> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    // Get current max position
    const result = this.db.exec(
      'SELECT COALESCE(MAX(position), -1) + 1 FROM playlist_tracks WHERE playlist_id = ?',
      [playlistId]
    );
    const position = result[0].values[0][0] as number;

    this.db.run(`
      INSERT OR IGNORE INTO playlist_tracks (playlist_id, track_id, position)
      VALUES (?, ?, ?)
    `, [playlistId, trackId, position]);

    // Update playlist timestamp
    this.db.run('UPDATE playlists SET updated_at = ? WHERE id = ?', [Date.now(), playlistId]);
  }

  /**
   * Get tracks in a playlist
   */
  async getPlaylistTracks(playlistId: string): Promise<Track[]> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    const result = this.db.exec(`
      SELECT t.* FROM tracks t
      JOIN playlist_tracks pt ON t.id = pt.track_id
      WHERE pt.playlist_id = ?
      ORDER BY pt.position
    `, [playlistId]);

    if (result.length === 0) {
      return [];
    }

    return result[0].values.map((row) => this.rowToTrack(result[0].columns, row));
  }

  /**
   * Remove a track from a playlist
   */
  async removeTrackFromPlaylist(playlistId: string, trackId: string): Promise<void> {
    if (!this.db) {
      throw new Error('Database not initialized');
    }

    this.db.run('DELETE FROM playlist_tracks WHERE playlist_id = ? AND track_id = ?', [playlistId, trackId]);
    this.db.run('UPDATE playlists SET updated_at = ? WHERE id = ?', [Date.now(), playlistId]);
  }
}
