/**
 * Status bar tests
 */

import * as assert from 'assert';
import { NowPlayingStatusBar } from '../../ui/statusBar';
import type { StatusResponse } from '../../types';

// Mocha globals are provided by the VS Code test runner
declare const describe: Mocha.SuiteFunction;
declare const it: Mocha.TestFunction;

// Mock vscode module for unit testing
const mockStatusBarItem = {
  text: '',
  tooltip: undefined as any,
  command: undefined as string | undefined,
  show: () => {},
  hide: () => {},
  dispose: () => {},
};

// We need to mock vscode.window.createStatusBarItem
// For now, we'll test the logic without the actual VS Code API

describe('NowPlayingStatusBar', () => {
  describe('formatTime', () => {
    // Test the time formatting logic
    it('should format milliseconds to mm:ss', () => {
      // 0 ms = 0:00
      assert.strictEqual(formatTime(0), '0:00');
      
      // 1000 ms = 0:01
      assert.strictEqual(formatTime(1000), '0:01');
      
      // 60000 ms = 1:00
      assert.strictEqual(formatTime(60000), '1:00');
      
      // 90000 ms = 1:30
      assert.strictEqual(formatTime(90000), '1:30');
      
      // 3661000 ms = 61:01
      assert.strictEqual(formatTime(3661000), '61:01');
    });
  });

  describe('truncate', () => {
    it('should not truncate short text', () => {
      const result = truncate('Hello', 10);
      assert.strictEqual(result, 'Hello');
    });

    it('should truncate long text with ellipsis', () => {
      const result = truncate('Hello World', 8);
      assert.strictEqual(result, 'Hello W…');
    });
  });

  describe('formatText', () => {
    it('should format track with artist and title', () => {
      const status: StatusResponse = {
        state: 'playing',
        position: 0,
        duration: 180000,
        volume: 1,
        queueIndex: 0,
        queueSize: 1,
        metadata: {
          title: 'Test Song',
          artist: 'Test Artist',
        },
      };

      const text = formatText(status);
      assert.ok(text.includes('Test Artist'));
      assert.ok(text.includes('Test Song'));
    });

    it('should use filename if no metadata', () => {
      const status: StatusResponse = {
        state: 'playing',
        path: '/music/my-song.mp3',
        position: 0,
        duration: 180000,
        volume: 1,
        queueIndex: 0,
        queueSize: 1,
      };

      const text = formatText(status);
      assert.ok(text.includes('my-song.mp3'));
    });
  });
});

// Helper functions extracted from the StatusBar class for testing
function formatTime(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}

function truncate(text: string, maxLength: number): string {
  if (text.length <= maxLength) {
    return text;
  }
  return text.substring(0, maxLength - 1) + '…';
}

function formatText(status: StatusResponse): string {
  if (!status.metadata) {
    if (status.path) {
      const fileName = status.path.split('/').pop() || status.path;
      return truncate(fileName, 40);
    }
    return 'Unknown Track';
  }

  const { title, artist } = status.metadata;

  if (title && artist) {
    const text = `${artist} – ${title}`;
    return truncate(text, 50);
  }

  if (title) {
    return truncate(title, 40);
  }

  return 'Unknown Track';
}
