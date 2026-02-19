/**
 * Queue manager tests
 */

import * as assert from 'assert';
import { QueueManager } from '../../queue/manager';
import { MockIPCClient } from '../mocks/ipcClient';
import type { Track } from '../../types';

// Mocha TDD globals are provided by the VS Code test runner
declare const suite: Mocha.SuiteFunction;
declare const test: Mocha.TestFunction;
declare const setup: Mocha.HookFunction;

suite('QueueManager', () => {
  let manager: QueueManager;
  let mockClient: MockIPCClient;

  const testTracks: Track[] = [
    { id: '1', path: '/track1.mp3', title: 'Track 1', artist: 'Artist A' },
    { id: '2', path: '/track2.mp3', title: 'Track 2', artist: 'Artist B' },
    { id: '3', path: '/track3.mp3', title: 'Track 3', artist: 'Artist A' },
  ];

  setup(async () => {
    mockClient = new MockIPCClient();
    await mockClient.connect();
    // Cast to any to use the mock with the queue manager
    manager = new QueueManager(mockClient as any);
  });

  suite('setQueue', () => {
    test('should set the queue with tracks', async () => {
      await manager.setQueue(testTracks);
      const queue = manager.getQueue();
      assert.strictEqual(queue.length, 3);
    });

    test('should reset index when setting queue', async () => {
      await manager.setQueue(testTracks);
      assert.strictEqual(manager.getCurrentIndex(), -1);
    });
  });

  suite('addToQueue', () => {
    test('should add tracks to existing queue', async () => {
      await manager.setQueue(testTracks.slice(0, 2));
      await manager.addToQueue([testTracks[2]]);

      const queue = manager.getQueue();
      assert.strictEqual(queue.length, 3);
    });
  });

  suite('clear', () => {
    test('should clear the queue', async () => {
      await manager.setQueue(testTracks);
      await manager.clear();

      assert.strictEqual(manager.getQueue().length, 0);
      assert.strictEqual(manager.getCurrentIndex(), -1);
    });
  });

  suite('playIndex', () => {
    test('should play track at index', async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(1);

      assert.strictEqual(manager.getCurrentIndex(), 1);
    });

    test('should throw for invalid index', async () => {
      await manager.setQueue(testTracks);

      await assert.rejects(() => manager.playIndex(10));
      await assert.rejects(() => manager.playIndex(-1));
    });
  });

  suite('next/previous', () => {
    setup(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(0);
    });

    test('should move to next track', async () => {
      const result = await manager.next();
      assert.strictEqual(result, true);
      assert.strictEqual(manager.getCurrentIndex(), 1);
    });

    test('should return false at end of queue', async () => {
      await manager.playIndex(2);
      const result = await manager.next();
      assert.strictEqual(result, false);
    });

    test('should move to previous track', async () => {
      await manager.playIndex(1);
      const result = await manager.previous();
      assert.strictEqual(result, true);
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    test('should return false at beginning of queue', async () => {
      const result = await manager.previous();
      assert.strictEqual(result, false);
    });
  });

  suite('repeat modes', () => {
    setup(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(0);
    });

    test('should repeat one track', async () => {
      manager.setRepeat('one');
      await manager.next();
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    test('should repeat all tracks', async () => {
      manager.setRepeat('all');
      await manager.playIndex(2);
      await manager.next();
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    test('should wrap to end with repeat all on previous', async () => {
      manager.setRepeat('all');
      await manager.previous();
      assert.strictEqual(manager.getCurrentIndex(), 2);
    });
  });

  suite('shuffle', () => {
    test('should toggle shuffle mode', () => {
      assert.strictEqual(manager.getShuffle(), false);

      manager.setShuffle(true);
      assert.strictEqual(manager.getShuffle(), true);

      manager.setShuffle(false);
      assert.strictEqual(manager.getShuffle(), false);
    });
  });

  suite('removeAt', () => {
    setup(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(1);
    });

    test('should remove track and adjust index if before current', () => {
      manager.removeAt(0);
      assert.strictEqual(manager.getQueue().length, 2);
      assert.strictEqual(manager.getCurrentIndex(), 0);
    });

    test('should remove track without adjusting if after current', () => {
      manager.removeAt(2);
      assert.strictEqual(manager.getQueue().length, 2);
      assert.strictEqual(manager.getCurrentIndex(), 1);
    });
  });

  suite('move', () => {
    setup(async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(1);
    });

    test('should move track and adjust index', () => {
      manager.move(0, 2); // Move first track to end
      const queue = manager.getQueue();

      assert.strictEqual(queue[0].path, '/track2.mp3');
      assert.strictEqual(queue[2].path, '/track1.mp3');
    });
  });

  suite('getCurrentItem', () => {
    test('should return null when no track is playing', () => {
      assert.strictEqual(manager.getCurrentItem(), null);
    });

    test('should return current item', async () => {
      await manager.setQueue(testTracks);
      await manager.playIndex(0);

      const item = manager.getCurrentItem();
      assert.ok(item);
      assert.strictEqual(item.path, '/track1.mp3');
    });
  });
});
