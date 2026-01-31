/**
 * Playlist manager tests
 */

import * as assert from 'assert';
import { PlaylistManager } from '../../playlist/manager';
import { MetadataCache } from '../../metadata/cache';
import { createTempDbPath, cleanupTestDb } from '../mocks/database';

describe('PlaylistManager', () => {
  let manager: PlaylistManager;
  let cache: MetadataCache;
  let dbPath: string;

  beforeEach(async () => {
    dbPath = createTempDbPath();
    cache = new MetadataCache(dbPath);
    await cache.initialize();
    manager = new PlaylistManager(cache);

    // Add some test tracks
    await cache.upsertTrack({ path: '/track1.mp3', title: 'Track 1' });
    await cache.upsertTrack({ path: '/track2.mp3', title: 'Track 2' });
    await cache.upsertTrack({ path: '/track3.mp3', title: 'Track 3' });
  });

  afterEach(() => {
    cache.close();
    cleanupTestDb(dbPath);
  });

  async function getTrackIds(): Promise<string[]> {
    const tracks = await cache.getAllTracks();
    return tracks.map((t) => t.id);
  }

  it('should create a playlist', async () => {
    const playlist = await manager.create('My Playlist');
    assert.ok(playlist.id);
    assert.strictEqual(playlist.name, 'My Playlist');
  });

  it('should get all playlists', async () => {
    await manager.create('Playlist 1');
    await manager.create('Playlist 2');

    const playlists = await manager.getAll();
    assert.strictEqual(playlists.length, 2);
  });

  it('should delete a playlist', async () => {
    const playlist = await manager.create('To Delete');
    await manager.delete(playlist.id);

    const playlists = await manager.getAll();
    assert.strictEqual(playlists.length, 0);
  });

  it('should add tracks to playlist', async () => {
    const trackIds = await getTrackIds();
    const playlist = await manager.create('Test');

    await manager.addTrack(playlist.id, trackIds[0]);
    await manager.addTrack(playlist.id, trackIds[1]);

    const tracks = await manager.getTracks(playlist.id);
    assert.strictEqual(tracks.length, 2);
  });

  it('should add multiple tracks at once', async () => {
    const trackIds = await getTrackIds();
    const playlist = await manager.create('Test');

    await manager.addTracks(playlist.id, trackIds);

    const tracks = await manager.getTracks(playlist.id);
    assert.strictEqual(tracks.length, 3);
  });

  it('should remove track from playlist', async () => {
    const trackIds = await getTrackIds();
    const playlist = await manager.create('Test');

    await manager.addTracks(playlist.id, trackIds);
    await manager.removeTrack(playlist.id, trackIds[1]);

    const tracks = await manager.getTracks(playlist.id);
    assert.strictEqual(tracks.length, 2);
  });

  it('should reorder tracks', async () => {
    const trackIds = await getTrackIds();
    const playlist = await manager.create('Test');

    await manager.addTrack(playlist.id, trackIds[0]); // position 0
    await manager.addTrack(playlist.id, trackIds[1]); // position 1
    await manager.addTrack(playlist.id, trackIds[2]); // position 2

    // Move track from position 2 to position 0
    await manager.reorder(playlist.id, 2, 0);

    const tracks = await manager.getTracks(playlist.id);
    assert.strictEqual(tracks[0].id, trackIds[2]);
    assert.strictEqual(tracks[1].id, trackIds[0]);
    assert.strictEqual(tracks[2].id, trackIds[1]);
  });

  it('should get track paths for a playlist', async () => {
    const trackIds = await getTrackIds();
    const playlist = await manager.create('Test');

    await manager.addTracks(playlist.id, trackIds);

    const paths = await manager.getTrackPaths(playlist.id);
    assert.strictEqual(paths.length, 3);
    assert.ok(paths.includes('/track1.mp3'));
    assert.ok(paths.includes('/track2.mp3'));
    assert.ok(paths.includes('/track3.mp3'));
  });

  it('should duplicate a playlist', async () => {
    const trackIds = await getTrackIds();
    const original = await manager.create('Original');
    await manager.addTracks(original.id, trackIds);

    const duplicate = await manager.duplicate(original.id, 'Copy of Original');

    assert.notStrictEqual(duplicate.id, original.id);
    assert.strictEqual(duplicate.name, 'Copy of Original');

    const dupTracks = await manager.getTracks(duplicate.id);
    assert.strictEqual(dupTracks.length, 3);
  });
});
