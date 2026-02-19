/**
 * Protocol tests
 * Tests for IPC message encoding/decoding
 */

import * as assert from 'assert';
import {
  encodeCommand,
  decodeResponse,
  createPairRequest,
  createPlayRequest,
  createQueueRequest,
  createSeekRequest,
  createVolumeRequest,
  isPairResponse,
  isStatusResponse,
  parseMessage,
} from '../../daemon/protocol';

// Mocha TDD globals are provided by the VS Code test runner
declare const suite: Mocha.SuiteFunction;
declare const test: Mocha.TestFunction;

suite('Protocol', () => {
  suite('encodeCommand', () => {
    test('should encode a simple command', () => {
      const msg = encodeCommand('play');
      const parsed = JSON.parse(msg);
      assert.strictEqual(parsed.cmd, 'play');
    });

    test('should encode command with data', () => {
      const msg = encodeCommand('play', { path: '/music/song.mp3' });
      const parsed = JSON.parse(msg);
      assert.strictEqual(parsed.cmd, 'play');
      assert.strictEqual(parsed.data.path, '/music/song.mp3');
    });

    test('should include auth token when provided', () => {
      const msg = encodeCommand('pause', {}, 'test-token-123');
      const parsed = JSON.parse(msg);
      assert.strictEqual(parsed.token, 'test-token-123');
    });

    test('should not include token when not provided', () => {
      const msg = encodeCommand('status');
      const parsed = JSON.parse(msg);
      assert.strictEqual(parsed.token, undefined);
    });
  });

  suite('decodeResponse', () => {
    test('should decode a success response', () => {
      const resp = decodeResponse('{"success":true,"data":{"state":"playing"}}');
      assert.strictEqual(resp.success, true);
      assert.deepStrictEqual(resp.data, { state: 'playing' });
    });

    test('should decode an error response', () => {
      const resp = decodeResponse('{"success":false,"error":"unauthorized"}');
      assert.strictEqual(resp.success, false);
      assert.strictEqual(resp.error, 'unauthorized');
    });

    test('should handle invalid JSON', () => {
      const resp = decodeResponse('not valid json');
      assert.strictEqual(resp.success, false);
      assert.ok(resp.error?.includes('Failed to parse'));
    });
  });

  suite('createPairRequest', () => {
    test('should create valid pair request', () => {
      const req = createPairRequest('VS Code');
      assert.strictEqual(req.clientName, 'VS Code');
    });
  });

  suite('createPlayRequest', () => {
    test('should create play request with path only', () => {
      const req = createPlayRequest('/music/song.mp3');
      assert.strictEqual(req.path, '/music/song.mp3');
      assert.strictEqual(req.metadata, undefined);
    });

    test('should create play request with metadata', () => {
      const req = createPlayRequest('/music/song.mp3', {
        title: 'Test Song',
        artist: 'Test Artist',
      });
      assert.strictEqual(req.path, '/music/song.mp3');
      assert.strictEqual(req.metadata?.title, 'Test Song');
    });
  });

  suite('createQueueRequest', () => {
    test('should create queue request with append=false', () => {
      const req = createQueueRequest([{ path: '/a.mp3' }, { path: '/b.mp3' }]);
      assert.deepStrictEqual(req.items, [{ path: '/a.mp3' }, { path: '/b.mp3' }]);
      assert.strictEqual(req.append, false);
    });

    test('should create queue request with append=true', () => {
      const req = createQueueRequest([{ path: '/c.mp3' }], true);
      assert.deepStrictEqual(req.items, [{ path: '/c.mp3' }]);
      assert.strictEqual(req.append, true);
    });
  });

  suite('createSeekRequest', () => {
    test('should create seek request', () => {
      const req = createSeekRequest(30000);
      assert.strictEqual(req.position, 30000);
    });
  });

  suite('createVolumeRequest', () => {
    test('should create volume request', () => {
      const req = createVolumeRequest(0.75);
      assert.strictEqual(req.level, 0.75);
    });

    test('should reject invalid volume levels', () => {
      assert.throws(() => createVolumeRequest(-0.1));
      assert.throws(() => createVolumeRequest(1.1));
    });

    test('should accept boundary values', () => {
      assert.doesNotThrow(() => createVolumeRequest(0));
      assert.doesNotThrow(() => createVolumeRequest(1));
    });
  });

  suite('isPairResponse', () => {
    test('should return true for valid pair response', () => {
      const data = {
        token: 'abc123',
        clientId: 'client1',
        requiresApproval: false,
      };
      assert.strictEqual(isPairResponse(data), true);
    });

    test('should return false for invalid data', () => {
      assert.strictEqual(isPairResponse(null), false);
      assert.strictEqual(isPairResponse({}), false);
      assert.strictEqual(isPairResponse({ token: 'abc' }), false);
    });
  });

  suite('isStatusResponse', () => {
    test('should return true for valid status response', () => {
      const data = {
        state: 'playing',
        position: 1000,
        duration: 180000,
        volume: 0.8,
        queueIndex: 0,
        queueSize: 10,
      };
      assert.strictEqual(isStatusResponse(data), true);
    });

    test('should return false for invalid data', () => {
      assert.strictEqual(isStatusResponse(null), false);
      assert.strictEqual(isStatusResponse({}), false);
      assert.strictEqual(isStatusResponse({ state: 'playing' }), false);
    });
  });

  suite('parseMessage', () => {
    test('should parse complete message', () => {
      const result = parseMessage('{"cmd":"play"}\n');
      assert.strictEqual(result.message, '{"cmd":"play"}');
      assert.strictEqual(result.remaining, '');
    });

    test('should return null for incomplete message', () => {
      const result = parseMessage('{"cmd":"pla');
      assert.strictEqual(result.message, null);
      assert.strictEqual(result.remaining, '{"cmd":"pla');
    });

    test('should handle multiple messages', () => {
      let buffer = '{"a":1}\n{"b":2}\n';
      
      let result = parseMessage(buffer);
      assert.strictEqual(result.message, '{"a":1}');
      
      result = parseMessage(result.remaining);
      assert.strictEqual(result.message, '{"b":2}');
      assert.strictEqual(result.remaining, '');
    });
  });
});
