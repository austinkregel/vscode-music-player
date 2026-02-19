/**
 * Metadata cache tests
 */

import * as assert from 'assert';
import { MetadataCache } from '../../metadata/cache';
import { createTestCache, createTempDbPath, cleanupTestDb } from '../mocks/database';

// Mocha TDD globals are provided by the VS Code test runner
declare const suite: Mocha.SuiteFunction;
declare const test: Mocha.TestFunction;
declare const setup: Mocha.HookFunction;
declare const teardown: Mocha.HookFunction;

suite('MetadataCache', () => {
  let cache: MetadataCache;
  let dbPath: string;

  setup(async () => {
    dbPath = createTempDbPath();
    cache = new MetadataCache(dbPath);
    await cache.initialize();
  });

  teardown(() => {
    cache.close();
    cleanupTestDb(dbPath);
  });

  suite('tracks', () => {
    test('should insert and retrieve a track', async () => {
      await cache.upsertTrack({
        path: '/music/track.mp3',
        title: 'Test Song',
        artist: 'Test Artist',
        album: 'Test Album',
      });

      const track = await cache.getTrackByPath('/music/track.mp3');
      assert.ok(track);
      assert.strictEqual(track.title, 'Test Song');
      assert.strictEqual(track.artist, 'Test Artist');
      assert.strictEqual(track.album, 'Test Album');
    });

    test('should update existing track on rescan', async () => {
      await cache.upsertTrack({ path: '/music/track.mp3', title: 'Old Title' });
      await cache.upsertTrack({ path: '/music/track.mp3', title: 'New Title' });

      const track = await cache.getTrackByPath('/music/track.mp3');
      assert.ok(track);
      assert.strictEqual(track.title, 'New Title');
    });

    test('should get track by ID', async () => {
      await cache.upsertTrack({
        path: '/music/unique-track.mp3',
        title: 'Unique Track',
      });

      const allTracks = await cache.getAllTracks();
      assert.strictEqual(allTracks.length, 1);

      const track = await cache.getTrackById(allTracks[0].id);
      assert.ok(track);
      assert.strictEqual(track.title, 'Unique Track');
    });

    test('should return null for non-existent track', async () => {
      const track = await cache.getTrackByPath('/nonexistent.mp3');
      assert.strictEqual(track, null);
    });

    test('should delete a track', async () => {
      await cache.upsertTrack({ path: '/music/to-delete.mp3', title: 'Delete Me' });

      const allTracks = await cache.getAllTracks();
      assert.strictEqual(allTracks.length, 1);

      await cache.deleteTrack(allTracks[0].id);

      const count = await cache.getTrackCount();
      assert.strictEqual(count, 0);
    });

    test('should get tracks by album', async () => {
      await cache.upsertTrack({ path: '/1.mp3', album: 'Album A', trackNumber: 1 });
      await cache.upsertTrack({ path: '/2.mp3', album: 'Album A', trackNumber: 2 });
      await cache.upsertTrack({ path: '/3.mp3', album: 'Album B', trackNumber: 1 });

      const tracks = await cache.getTracksByAlbum('Album A');
      assert.strictEqual(tracks.length, 2);
    });

    test('should get tracks by artist', async () => {
      await cache.upsertTrack({ path: '/1.mp3', artist: 'Artist X' });
      await cache.upsertTrack({ path: '/2.mp3', artist: 'Artist X' });
      await cache.upsertTrack({ path: '/3.mp3', artist: 'Artist Y' });

      const tracks = await cache.getTracksByArtist('Artist X');
      assert.strictEqual(tracks.length, 2);
    });
  });

  suite('search', () => {
    setup(async () => {
      await cache.upsertTrack({
        path: '/a.mp3',
        title: 'Hello World',
        artist: 'Radiohead',
        album: 'OK Computer',
      });
      await cache.upsertTrack({
        path: '/b.mp3',
        title: 'Goodbye',
        artist: 'The Beatles',
        album: 'Abbey Road',
      });
    });

    test('should find tracks by title', async () => {
      const results = await cache.search('Hello');
      assert.strictEqual(results.length, 1);
      assert.strictEqual(results[0].title, 'Hello World');
    });

    test('should find tracks by artist', async () => {
      const results = await cache.search('Radiohead');
      assert.strictEqual(results.length, 1);
    });

    test('should find tracks by album', async () => {
      const results = await cache.search('Computer');
      assert.strictEqual(results.length, 1);
    });

    test('should be case insensitive', async () => {
      const results = await cache.search('hello');
      assert.strictEqual(results.length, 1);
    });

    test('should return empty for no matches', async () => {
      const results = await cache.search('Nonexistent');
      assert.strictEqual(results.length, 0);
    });
  });

  suite('albums', () => {
    test('should auto-create albums when tracks are added', async () => {
      await cache.upsertTrack({
        path: '/music/song.mp3',
        album: 'Great Album',
        artist: 'Great Artist',
        year: 2024,
      });

      const albums = await cache.getAllAlbums();
      assert.strictEqual(albums.length, 1);
      assert.strictEqual(albums[0].name, 'Great Album');
    });
  });

  suite('artists', () => {
    test('should auto-create artists when tracks are added', async () => {
      await cache.upsertTrack({
        path: '/music/song.mp3',
        artist: 'Awesome Artist',
      });

      const artists = await cache.getAllArtists();
      assert.strictEqual(artists.length, 1);
      assert.strictEqual(artists[0].name, 'Awesome Artist');
    });
  });

  suite('playlists', () => {
    test('should create a playlist', async () => {
      const playlist = await cache.createPlaylist({ name: 'My Playlist' });
      assert.ok(playlist.id);
      assert.strictEqual(playlist.name, 'My Playlist');
      assert.ok(playlist.createdAt > 0);
    });

    test('should get all playlists', async () => {
      await cache.createPlaylist({ name: 'Playlist 1' });
      await cache.createPlaylist({ name: 'Playlist 2' });

      const playlists = await cache.getAllPlaylists();
      assert.strictEqual(playlists.length, 2);
    });

    test('should delete a playlist', async () => {
      const playlist = await cache.createPlaylist({ name: 'To Delete' });
      await cache.deletePlaylist(playlist.id);

      const playlists = await cache.getAllPlaylists();
      assert.strictEqual(playlists.length, 0);
    });

    test('should add and get tracks in playlist', async () => {
      // Create track
      await cache.upsertTrack({ path: '/song.mp3', title: 'Test Song' });
      const tracks = await cache.getAllTracks();
      const trackId = tracks[0].id;

      // Create playlist and add track
      const playlist = await cache.createPlaylist({ name: 'Test Playlist' });
      await cache.addTrackToPlaylist(playlist.id, trackId);

      // Get playlist tracks
      const playlistTracks = await cache.getPlaylistTracks(playlist.id);
      assert.strictEqual(playlistTracks.length, 1);
      assert.strictEqual(playlistTracks[0].id, trackId);
    });

    test('should remove track from playlist', async () => {
      await cache.upsertTrack({ path: '/song.mp3', title: 'Test Song' });
      const tracks = await cache.getAllTracks();
      const trackId = tracks[0].id;

      const playlist = await cache.createPlaylist({ name: 'Test Playlist' });
      await cache.addTrackToPlaylist(playlist.id, trackId);
      await cache.removeTrackFromPlaylist(playlist.id, trackId);

      const playlistTracks = await cache.getPlaylistTracks(playlist.id);
      assert.strictEqual(playlistTracks.length, 0);
    });
  });

  suite('persistence', () => {
    test('should persist data across saves', async () => {
      await cache.upsertTrack({ path: '/persist.mp3', title: 'Persist Me' });
      await cache.save();
      cache.close();

      // Reopen
      const cache2 = new MetadataCache(dbPath);
      await cache2.initialize();

      const track = await cache2.getTrackByPath('/persist.mp3');
      assert.ok(track);
      assert.strictEqual(track.title, 'Persist Me');

      cache2.close();
    });
  });
});
