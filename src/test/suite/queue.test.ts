/**
 * Queue manager tests
 */

import * as assert from 'assert';
import { QueueManager } from '../../queue/manager';
import { MockIPCClient } from '../mocks/ipcClient';
import type { Track } from '../../types';

describe('QueueManager', () => {
  let manager: QueueManager;
  let mockClient: MockIPCClient;

  const testTracks: Track[] = [
    { id: '1', path: '/track1.mp3', title: 'Track 1', artist: 'Artist A' },
    { id: '2', path: '/track2.mp3', title: 'Track 2', artist: 'Artist B' },
    { id: '3', path: '/track3.mp3', title: 'Track 3', artist: 'Artist A' },
  ];

  beforeEach(async () => {
    mockClient = new MockIPCClient();
    await mockClient.connect();
    // Cast to any to use the mock with the queue manager
    manager = new QueueManager(mockClient as any);
  });

  describe('setQueue', () => {
    it('should set the queue with tracks', async () => {
      await manager.setQueue(testTracks);
      const queue = manager.getQueue();
      assert.strictEqual(queue.length, 3);
    });

    it('should reset index when setting queue', async () => {
      await manager.setQueue(testTracks);
      assert.strictEqual(manager.getCurrentIndex(), -1);
    });
  });

  describe('addToQueue', () => {
    it('should add tracks to existing queue', async () => {
      await manager.setQueue(testTracks.slice(0, 2));
      await manager.addToQueue([testTracks[2]]);

      const queue = manager.getQueue();
      assert.strictEqual(queue.length, 3);
    });
  });

  describe('clear', () => {
    it('should clear the queue', async () => {
      await manager.setQueue(testTracks);
      await manager.clear();

      assert.strictEqual(manager.getQueue().length, 0);
      assert.strictEqual(manager.getCurrentIndex(), -1);
    });
  });

  describe('playIndex', () => {
    it('should play track at index', async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(1);

      assert.strictEqual(manager.getCurrentIndex(), 1);
    });

    it('should throw for invalid index', async () => {
      await manager.setQueue(testTracks);

      await assert.rejects(() => manager.playIndex(10));
      await assert.rejects(() => manager.playIndex(-1));
    });
  });

  describe('next/previous', () => {
    beforeEach(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(0);
    });

    it('should move to next track', async () => {
      const result = await manager.next();
      assert.strictEqual(result, true);
      assert.strictEqual(manager.getCurrentIndex(), 1);
    });

    it('should return false at end of queue', async () => {
      await manager.playIndex(2);
      const result = await manager.next();
      assert.strictEqual(result, false);
    });

    it('should move to previous track', async () => {
      await manager.playIndex(1);
      const result = await manager.previous();
      assert.strictEqual(result, true);
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    it('should return false at beginning of queue', async () => {
      const result = await manager.previous();
      assert.strictEqual(result, false);
    });
  });

  describe('repeat modes', () => {
    beforeEach(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(0);
    });

    it('should repeat one track', async () => {
      manager.setRepeat('one');
      await manager.next();
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    it('should repeat all tracks', async () => {
      manager.setRepeat('all');
      await manager.playIndex(2);
      await manager.next();
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    it('should wrap to end with repeat all on previous', async () => {
      manager.setRepeat('all');
      await manager.previous();
      assert.strictEqual(manager.getCurrentIndex(), 2);
    });
  });

  describe('shuffle', () => {
    it('should toggle shuffle mode', () => {
      assert.strictEqual(manager.getShuffle(), false);

      manager.setShuffle(true);
      assert.strictEqual(manager.getShuffle(), true);

      manager.setShuffle(false);
      assert.strictEqual(manager.getShuffle(), false);
    });
  });

  describe('removeAt', () => {
    beforeEach(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(1);
    });

    it('should remove track and adjust index if before current', () => {
      manager.removeAt(0);
      assert.strictEqual(manager.getQueue().length, 2);
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    it('should remove track without adjusting if after current', () => {
      manager.removeAt(2);
      assert.strictEqual(manager.getQueue().length, 2);
      assert.strictEqual(manager.getCurrentIndex(), 1);
    });
  });

  describe('move', () => {
    beforeEach(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(1);
    });

    it('should move track and adjust index', () => {
      manager.move(0, 2); // Move first track to end
      const queue = manager.getQueue();

      assert.strictEqual(queue[0].path, '/track2.mp3');
      assert.strictEqual(queue[2].path, '/track1.mp3');
    });
  });

  describe('getCurrentItem', () => {
    it('should return null when no track is playing', () => {
      assert.strictEqual(manager.getCurrentItem(), null);
    });

    it('should return current item', async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(0);

      const item = manager.getCurrentItem();
      assert.ok(item);
      assert.strictEqual(item.path, '/track1.mp3');
    });
  });
});
