/**
 * Search tests
 */

import * as assert from 'assert';
import { LibrarySearch } from '../../library/search';
import { MetadataCache } from '../../metadata/cache';
import { createTempDbPath, cleanupTestDb } from '../mocks/database';

describe('LibrarySearch', () => {
  let search: LibrarySearch;
  let cache: MetadataCache;
  let dbPath: string;

  beforeEach(async () => {
    dbPath = createTempDbPath();
    cache = new MetadataCache(dbPath);
    await cache.initialize();
    search = new LibrarySearch(cache);

    // Add test data
    await cache.upsertTrack({
      path: '/music/radiohead-creep.mp3',
      title: 'Creep',
      artist: 'Radiohead',
      album: 'Pablo Honey',
    });
    await cache.upsertTrack({
      path: '/music/beatles-yesterday.mp3',
      title: 'Yesterday',
      artist: 'The Beatles',
      album: 'Help!',
    });
    await cache.upsertTrack({
      path: '/music/radiohead-karma.mp3',
      title: 'Karma Police',
      artist: 'Radiohead',
      album: 'OK Computer',
    });
  });

  afterEach(() => {
    cache.close();
    cleanupTestDb(dbPath);
  });

  describe('searchTracks', () => {
    it('should find tracks by title', async () => {
      const results = await search.searchTracks('Creep');
      assert.strictEqual(results.length, 1);
      assert.strictEqual(results[0].title, 'Creep');
    });

    it('should find tracks by artist', async () => {
      const results = await search.searchTracks('Radiohead');
      assert.strictEqual(results.length, 2);
    });

    it('should find tracks by album', async () => {
      const results = await search.searchTracks('Pablo');
      assert.strictEqual(results.length, 1);
    });

    it('should return empty for no matches', async () => {
      const results = await search.searchTracks('Nonexistent');
      assert.strictEqual(results.length, 0);
    });
  });

  describe('searchAll', () => {
    it('should search across tracks, albums, and artists', async () => {
      const results = await search.searchAll('Radiohead');

      // Should find:
      // - 2 tracks by Radiohead
      // - 1 artist named Radiohead
      const trackResults = results.filter((r) => r.type === 'track');
      const artistResults = results.filter((r) => r.type === 'artist');

      assert.strictEqual(trackResults.length, 2);
      assert.strictEqual(artistResults.length, 1);
    });

    it('should find albums by name', async () => {
      const results = await search.searchAll('Computer');

      const albumResults = results.filter((r) => r.type === 'album');
      assert.ok(albumResults.length >= 1);
    });

    it('should include description and detail in results', async () => {
      const results = await search.searchAll('Creep');

      const trackResult = results.find((r) => r.type === 'track');
      assert.ok(trackResult);
      assert.strictEqual(trackResult.description, 'Radiohead');
      assert.strictEqual(trackResult.detail, 'Pablo Honey');
    });
  });
});
