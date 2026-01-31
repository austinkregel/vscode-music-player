/**
 * Daemon lifecycle management
 * Handles starting, stopping, and health checking the musicd daemon
 */

import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import { spawn, ChildProcess } from 'child_process';
import { IPCClient } from './client';

export interface DaemonStatus {
  running: boolean;
  connected: boolean;
  pid?: number;
  version?: string;
}

/**
 * Manages the musicd daemon lifecycle
 */
export class DaemonManager {
  private daemon: ChildProcess | null = null;
  private client: IPCClient;
  private outputChannel: vscode.OutputChannel;
  private extensionPath: string;
  private statusCheckInterval: NodeJS.Timeout | null = null;

  constructor(
    client: IPCClient,
    outputChannel: vscode.OutputChannel,
    extensionPath: string
  ) {
    this.client = client;
    this.outputChannel = outputChannel;
    this.extensionPath = extensionPath;
  }

  /**
   * Get the path to the daemon binary for the current platform
   */
  private getDaemonPath(): string {
    const binDir = path.join(this.extensionPath, 'bin');
    let binaryName = 'musicd';

    switch (process.platform) {
      case 'win32':
        binaryName = 'musicd-windows-amd64.exe';
        break;
      case 'darwin':
        binaryName = process.arch === 'arm64' 
          ? 'musicd-darwin-arm64' 
          : 'musicd-darwin-amd64';
        break;
      case 'linux':
        binaryName = process.arch === 'arm64'
          ? 'musicd-linux-arm64'
          : 'musicd-linux-amd64';
        break;
    }

    return path.join(binDir, binaryName);
  }

  /**
   * Check if the daemon binary exists
   */
  hasDaemonBinary(): boolean {
    const daemonPath = this.getDaemonPath();
    return fs.existsSync(daemonPath);
  }

  /**
   * Start the daemon
   */
  async start(): Promise<void> {
    if (this.daemon && !this.daemon.killed) {
      this.outputChannel.appendLine('Daemon already running');
      return;
    }

    const daemonPath = this.getDaemonPath();

    if (!fs.existsSync(daemonPath)) {
      throw new Error(`Daemon binary not found: ${daemonPath}`);
    }

    this.outputChannel.appendLine(`Starting daemon: ${daemonPath}`);

    this.daemon = spawn(daemonPath, ['--verbose'], {
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    // Log daemon output
    this.daemon.stdout?.on('data', (data) => {
      this.outputChannel.appendLine(`[musicd] ${data.toString().trim()}`);
    });

    this.daemon.stderr?.on('data', (data) => {
      this.outputChannel.appendLine(`[musicd error] ${data.toString().trim()}`);
    });

    this.daemon.on('error', (err) => {
      this.outputChannel.appendLine(`Daemon error: ${err.message}`);
    });

    this.daemon.on('exit', (code, signal) => {
      this.outputChannel.appendLine(
        `Daemon exited with code ${code}, signal ${signal}`
      );
      this.daemon = null;
    });

    // Wait for daemon to start
    await this.waitForDaemon();
  }

  /**
   * Wait for daemon to be ready
   */
  private async waitForDaemon(maxAttempts: number = 10): Promise<void> {
    for (let i = 0; i < maxAttempts; i++) {
      try {
        await this.client.connect();
        this.outputChannel.appendLine('Connected to daemon');
        return;
      } catch (err) {
        if (i < maxAttempts - 1) {
          await this.delay(500);
        }
      }
    }

    throw new Error('Failed to connect to daemon after multiple attempts');
  }

  /**
   * Stop the daemon
   */
  async stop(): Promise<void> {
    if (this.statusCheckInterval) {
      clearInterval(this.statusCheckInterval);
      this.statusCheckInterval = null;
    }

    this.client.disconnect();

    if (this.daemon && !this.daemon.killed) {
      this.daemon.kill('SIGTERM');
      
      // Wait for graceful shutdown
      await this.waitForExit(5000);

      if (this.daemon && !this.daemon.killed) {
        this.daemon.kill('SIGKILL');
      }
    }

    this.daemon = null;
    this.outputChannel.appendLine('Daemon stopped');
  }

  /**
   * Wait for daemon to exit
   */
  private waitForExit(timeout: number): Promise<void> {
    return new Promise((resolve) => {
      if (!this.daemon) {
        resolve();
        return;
      }

      const timer = setTimeout(() => {
        resolve();
      }, timeout);

      this.daemon.once('exit', () => {
        clearTimeout(timer);
        resolve();
      });
    });
  }

  /**
   * Restart the daemon
   */
  async restart(): Promise<void> {
    await this.stop();
    await this.start();
  }

  /**
   * Get daemon status
   */
  async getStatus(): Promise<DaemonStatus> {
    const running = this.daemon !== null && !this.daemon.killed;
    const connected = this.client.isConnected();

    return {
      running,
      connected,
      pid: this.daemon?.pid,
    };
  }

  /**
   * Ensure daemon is running and connected
   */
  async ensureRunning(): Promise<void> {
    if (!this.client.isConnected()) {
      try {
        await this.client.connect();
      } catch {
        // Try to start the daemon
        await this.start();
      }
    }
  }

  /**
   * Start periodic status checking
   */
  startStatusChecking(intervalMs: number = 5000): void {
    if (this.statusCheckInterval) {
      return;
    }

    this.statusCheckInterval = setInterval(async () => {
      try {
        if (this.client.isConnected()) {
          await this.client.getStatus();
        }
      } catch (err) {
        this.outputChannel.appendLine(`Status check failed: ${err}`);
      }
    }, intervalMs);
  }

  /**
   * Dispose of resources
   */
  dispose(): void {
    if (this.statusCheckInterval) {
      clearInterval(this.statusCheckInterval);
    }
    this.stop().catch(() => {});
  }

  private delay(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
