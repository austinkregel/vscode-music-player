/**
 * End-to-end tests with real daemon
 * 
 * These tests spawn the actual daemon process and test the full flow
 */

import * as assert from 'assert';
import * as path from 'path';
import { spawn, ChildProcess } from 'child_process';
import { IPCClient } from '../../daemon/client';

// Skip these tests if daemon binary is not available
const DAEMON_PATH = path.join(__dirname, '../../../../bin/musicd');

suite('E2E with Daemon', function () {
  this.timeout(30000); // Increase timeout for daemon operations

  let daemon: ChildProcess | null = null;
  let client: IPCClient;

  async function waitForSocket(maxWaitMs: number = 5000): Promise<boolean> {
    const startTime = Date.now();
    
    while (Date.now() - startTime < maxWaitMs) {
      try {
        await client.connect();
        return true;
      } catch {
        await new Promise((resolve) => setTimeout(resolve, 200));
      }
    }
    
    return false;
  }

  suiteSetup(async function () {
    // Check if daemon binary exists
    const fs = await import('fs');
    if (!fs.existsSync(DAEMON_PATH)) {
      console.log('Daemon binary not found, skipping E2E tests');
      this.skip();
      return;
    }

    // Start daemon in test mode
    daemon = spawn(DAEMON_PATH, ['--test-mode', '--verbose'], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    daemon.stdout?.on('data', (data) => {
      console.log(`[daemon] ${data.toString().trim()}`);
    });

    daemon.stderr?.on('data', (data) => {
      console.error(`[daemon error] ${data.toString().trim()}`);
    });

    client = new IPCClient();

    // Wait for daemon to be ready
    const connected = await waitForSocket();
    if (!connected) {
      throw new Error('Failed to connect to daemon');
    }
  });

  suiteTeardown(async () => {
    if (client) {
      client.disconnect();
    }

    if (daemon && !daemon.killed) {
      daemon.kill('SIGTERM');
      
      // Wait for process to exit
      await new Promise<void>((resolve) => {
        const timeout = setTimeout(() => {
          daemon?.kill('SIGKILL');
          resolve();
        }, 3000);

        daemon?.once('exit', () => {
          clearTimeout(timeout);
          resolve();
        });
      });
    }
  });

  test('Client should connect to daemon', () => {
    assert.ok(client.isConnected());
  });

  test('Pairing flow should complete', async () => {
    const result = await client.pair('Test Client');
    
    assert.ok(result.token);
    assert.ok(result.clientId);
    assert.strictEqual(result.token.length, 64); // 256-bit hex
  });

  test('Status command should work after pairing', async () => {
    const status = await client.getStatus();
    
    assert.ok(status);
    assert.ok(['stopped', 'playing', 'paused'].includes(status.state));
    assert.strictEqual(typeof status.position, 'number');
    assert.strictEqual(typeof status.volume, 'number');
  });

  test('Volume command should work', async () => {
    const status = await client.setVolume(0.5);
    
    assert.ok(status);
    assert.strictEqual(status.volume, 0.5);
  });

  test('Queue command should work', async () => {
    const status = await client.queue([{ path: '/test/track1.mp3' }, { path: '/test/track2.mp3' }]);
    
    assert.ok(status);
    assert.strictEqual(status.queueSize, 2);
  });
});
