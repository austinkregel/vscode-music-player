/**
 * Command palette integration
 * Registers and handles playback commands
 */

import * as vscode from 'vscode';
import { IPCClient } from '../daemon/client';
import { AuthManager } from '../auth/pairing';
import { DaemonManager } from '../daemon/lifecycle';
import { StatusBarManager } from './statusBar';
import { PlayerWebviewPanel } from './webview/playerView';
import type { PlayerControls } from './playerControls';
import type { LibraryCache } from '../library/cache';
import type { QueueTreeProvider } from '../library/treeProvider';

/**
 * Command handler class
 */
export class CommandHandler {
  private client: IPCClient;
  private authManager: AuthManager;
  private daemonManager: DaemonManager;
  private statusBar: StatusBarManager;
  private extensionUri: vscode.Uri;
  private playerControls: PlayerControls | null = null;
  private libraryCache: LibraryCache | null = null;
  private queueProvider: QueueTreeProvider | null = null;
  private statusPollInterval: NodeJS.Timeout | null = null;
  private lastScanProgress: number = 0;

  constructor(
    client: IPCClient,
    authManager: AuthManager,
    daemonManager: DaemonManager,
    statusBar: StatusBarManager,
    extensionUri: vscode.Uri
  ) {
    this.client = client;
    this.authManager = authManager;
    this.daemonManager = daemonManager;
    this.statusBar = statusBar;
    this.extensionUri = extensionUri;
  }

  /**
   * Set the player controls instance
   */
  setPlayerControls(controls: PlayerControls): void {
    this.playerControls = controls;
  }

  /**
   * Set the library cache
   */
  setLibraryCache(cache: LibraryCache): void {
    this.libraryCache = cache;
  }

  /**
   * Set the queue tree provider
   */
  setQueueProvider(provider: QueueTreeProvider): void {
    this.queueProvider = provider;
  }

  /**
   * Register all commands
   */
  registerCommands(context: vscode.ExtensionContext): void {
    const commands: Array<[string, () => Promise<void>]> = [
      ['local-media.connect', () => this.connect()],
      ['local-media.disconnect', () => this.disconnect()],
      ['local-media.play', () => this.play()],
      ['local-media.pause', () => this.pause()],
      ['local-media.playPause', () => this.playPause()],
      ['local-media.stop', () => this.stop()],
      ['local-media.next', () => this.next()],
      ['local-media.previous', () => this.previous()],
      ['local-media.showPlayer', () => this.showPlayer()],
      ['local-media.scanLibrary', () => this.scanLibrary()],
      ['local-media.openSettings', () => this.openSettings()],
      ['local-media.setLibraryFolder', () => this.setLibraryFolder()],
      ['local-media.addLibraryFolder', () => this.addLibraryFolder()],
    ];

    for (const [command, handler] of commands) {
      context.subscriptions.push(
        vscode.commands.registerCommand(command, handler)
      );
    }

    // Register playFile command with argument
    context.subscriptions.push(
      vscode.commands.registerCommand('local-media.playFile', (filePath: string) => this.playFile(filePath))
    );
  }

  /**
   * Start polling for status updates
   */
  startStatusPolling(intervalMs: number = 1000): void {
    if (this.statusPollInterval) {
      return;
    }

    this.statusPollInterval = setInterval(async () => {
      await this.updateStatus();
    }, intervalMs);
  }

  /**
   * Stop polling for status updates
   */
  stopStatusPolling(): void {
    if (this.statusPollInterval) {
      clearInterval(this.statusPollInterval);
      this.statusPollInterval = null;
    }
  }

  /**
   * Update status bar with current playback status
   */
  private async updateStatus(): Promise<void> {
    if (!this.client.isConnected() || !this.authManager.isAuthenticated) {
      return;
    }

    try {
      const status = await this.client.getStatus();
      this.statusBar.updateNowPlaying(status);

      // Update player controls visibility and state
      if (this.playerControls) {
        if (status.state === 'playing' || status.state === 'paused') {
          this.playerControls.show();
          this.playerControls.setPlaying(status.state === 'playing');
        } else {
          this.playerControls.hide();
        }
      }

      // Update queue tree provider with current state
      if (this.queueProvider) {
        this.queueProvider.setPlaybackState(status.state === 'playing');
        this.queueProvider.setCurrentIndex(status.queueIndex);
        this.queueProvider.setCurrentTrack({
          title: status.metadata?.title,
          artist: status.metadata?.artist,
          position: status.position,
          duration: status.duration,
          path: status.path,
        });
      }
    } catch (err) {
      // Connection might be lost - update state
      this.statusBar.setDaemonState('error', 'Connection lost');
    }
  }

  /**
   * Ensure connected and authenticated before executing a command
   */
  private async ensureConnected(): Promise<boolean> {
    try {
      await this.daemonManager.ensureRunning();

      if (!this.authManager.isAuthenticated) {
        const success = await this.authManager.authenticate();
        if (!success) {
          return false;
        }
      }

      return true;
    } catch (err) {
      vscode.window.showErrorMessage(`Connection failed: ${err}`);
      return false;
    }
  }

  // =========================================================================
  // Command Implementations
  // =========================================================================

  async connect(): Promise<void> {
    try {
      this.statusBar.setDaemonState('connecting');
      
      await this.daemonManager.ensureRunning();
      const success = await this.authManager.authenticate();

      if (success) {
        this.statusBar.setDaemonState('connected');
        this.startStatusPolling();
        await this.updateStatus();
      } else {
        this.statusBar.setDaemonState('error', 'Authentication failed');
      }
    } catch (err) {
      this.statusBar.setDaemonState('error', String(err));
      vscode.window.showErrorMessage(`Failed to connect: ${err}`);
    }
  }

  async disconnect(): Promise<void> {
    this.stopStatusPolling();
    await this.authManager.disconnect();
    this.client.disconnect();
    this.statusBar.setDaemonState('disconnected');
    this.statusBar.hideNowPlaying();
    vscode.window.showInformationMessage('Disconnected from music daemon');
  }

  async play(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      // If we have a current track paused, resume it
      const status = this.statusBar.getStatus();
      if (status?.state === 'paused') {
        await this.client.resume();
      } else {
        // Otherwise, show a file picker
        const files = await vscode.window.showOpenDialog({
          canSelectFiles: true,
          canSelectMany: false,
          filters: {
            'Audio Files': ['mp3', 'flac', 'm4a', 'aac', 'ogg', 'wav'],
          },
          title: 'Select audio file to play',
        });

        if (files && files.length > 0) {
          await this.client.play(files[0].fsPath);
        }
      }

      await this.updateStatus();
    } catch (err) {
      vscode.window.showErrorMessage(`Play failed: ${err}`);
    }
  }

  async pause(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      await this.client.pause();
      await this.updateStatus();
    } catch (err) {
      vscode.window.showErrorMessage(`Pause failed: ${err}`);
    }
  }

  async playPause(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      const status = this.statusBar.getStatus();
      
      if (status?.state === 'playing') {
        await this.client.pause();
      } else if (status?.state === 'paused') {
        await this.client.resume();
      } else {
        // Nothing playing, trigger play command
        await this.play();
        return;
      }

      await this.updateStatus();
    } catch (err) {
      vscode.window.showErrorMessage(`Play/Pause failed: ${err}`);
    }
  }

  async stop(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      await this.client.stop();
      await this.updateStatus();
    } catch (err) {
      vscode.window.showErrorMessage(`Stop failed: ${err}`);
    }
  }

  async next(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      await this.client.next();
      await this.updateStatus();
    } catch (err) {
      vscode.window.showErrorMessage(`Next failed: ${err}`);
    }
  }

  async previous(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      // Get current position to decide behavior
      const status = await this.client.getStatus();
      const positionMs = status.position || 0;
      
      // If more than 5 seconds into the song, restart it
      // Otherwise, go to the previous track
      if (positionMs > 5000) {
        await this.client.seek(0);
      } else {
        await this.client.prev();
      }
      await this.updateStatus();
    } catch (err) {
      vscode.window.showErrorMessage(`Previous failed: ${err}`);
    }
  }

  async showPlayer(): Promise<void> {
    PlayerWebviewPanel.createOrShow(this.extensionUri, this.client);
  }

  /**
   * Play a specific file by path (used for double-click on tracks)
   */
  async playFile(filePath: string): Promise<void> {
    if (!filePath) {
      vscode.window.showErrorMessage('No file path provided');
      return;
    }

    if (!await this.ensureConnected()) {
      return;
    }

    try {
      await this.client.play(filePath);
      await this.updateStatus();
    } catch (err) {
      vscode.window.showErrorMessage(`Failed to play file: ${err}`);
    }
  }

  async scanLibrary(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      // Check if library paths are configured
      const config = await this.client.getConfig();
      
      if (!config.libraryPaths || config.libraryPaths.length === 0) {
        const result = await vscode.window.showWarningMessage(
          'No library folders configured. Would you like to add one now?',
          'Add Folder',
          'Cancel'
        );

        if (result === 'Add Folder') {
          await this.setLibraryFolder();
          return;
        }
        return;
      }

      // Show progress
      this.statusBar.setDaemonState('scanning');
      
      await vscode.window.withProgress(
        {
          location: vscode.ProgressLocation.Notification,
          title: 'Scanning music library...',
          cancellable: false,
        },
        async (progress) => {
          // Use the async scan with progress callback
          const scanResult = await this.client.scanLibraryAndWait((percent, message) => {
            progress.report({ 
              message: message || 'Scanning...', 
              increment: percent - (this.lastScanProgress || 0) 
            });
            this.lastScanProgress = percent;
            this.statusBar.setScanProgress(percent);
          });
          
          this.lastScanProgress = 0;
          
          // Restore connected state after scan
          this.statusBar.setDaemonState('connected');

          progress.report({ 
            message: `Found ${scanResult.totalFiles} audio files`,
            increment: 100 
          });

          // Show summary
          const paths = scanResult.results
            .map(r => `${r.libraryPath}: ${r.totalFiles} files (${r.scanTimeMs}ms)`)
            .join('\n');

          // Build summary message
          const meta = scanResult.metadata;
          const artistCount = meta?.artists?.length || 0;
          const albumCount = meta?.albums?.length || 0;
          const artworkDirs = meta?.artwork ? Object.keys(meta.artwork).length : 0;

          let summaryMsg = `Found ${scanResult.totalFiles} audio files`;
          if (artistCount > 0 || albumCount > 0) {
            summaryMsg += `, ${artistCount} artists, ${albumCount} albums`;
          }
          if (artworkDirs > 0) {
            summaryMsg += `, artwork in ${artworkDirs} folders`;
          }

          vscode.window.showInformationMessage(
            `Library scan complete! ${summaryMsg}.`,
            'Show Details'
          ).then(selection => {
            if (selection === 'Show Details') {
              const output = vscode.window.createOutputChannel('Local Media');
              output.clear();
              output.appendLine('Library Scan Results');
              output.appendLine('====================');
              output.appendLine('');
              
              for (const result of scanResult.results) {
                output.appendLine(`📁 ${result.libraryPath}`);
                output.appendLine(`   Files: ${result.totalFiles}`);
                output.appendLine(`   Scan time: ${result.scanTimeMs}ms`);
                if (result.error) {
                  output.appendLine(`   ⚠️ Error: ${result.error}`);
                }
                output.appendLine('');
              }
              
              output.appendLine(`Total: ${scanResult.totalFiles} audio files`);
              output.appendLine('');

              // Show NFO metadata if found
              if (meta) {
                if (meta.artists && meta.artists.length > 0) {
                  output.appendLine('');
                  output.appendLine('Artists (from NFO files)');
                  output.appendLine('------------------------');
                  for (const artist of meta.artists) {
                    const rating = artist.rating ? ` ⭐ ${artist.rating}` : '';
                    const mbid = artist.musicBrainzId ? ` [MB: ${artist.musicBrainzId.substring(0, 8)}...]` : '';
                    output.appendLine(`  🎤 ${artist.name}${rating}${mbid}`);
                    if (artist.genres && artist.genres.length > 0) {
                      output.appendLine(`     Genres: ${artist.genres.join(', ')}`);
                    }
                  }
                }

                if (meta.albums && meta.albums.length > 0) {
                  output.appendLine('');
                  output.appendLine('Albums (from NFO files)');
                  output.appendLine('-----------------------');
                  for (const album of meta.albums) {
                    const year = album.year ? ` (${album.year})` : '';
                    const rating = album.rating ? ` ⭐ ${album.rating}` : '';
                    output.appendLine(`  💿 ${album.title}${year}${rating}`);
                    if (album.artist) {
                      output.appendLine(`     by ${album.artist}`);
                    }
                    if (album.genres && album.genres.length > 0) {
                      output.appendLine(`     Genres: ${album.genres.join(', ')}`);
                    }
                  }
                }

                if (meta.artwork && Object.keys(meta.artwork).length > 0) {
                  output.appendLine('');
                  output.appendLine(`Artwork found in ${Object.keys(meta.artwork).length} directories`);
                }
              }

              output.show();
            }
          });

          // Update the library cache with scan results
          if (this.libraryCache) {
            const config = await this.client.getConfig();
            await this.libraryCache.updateFromScan(scanResult, config.libraryPaths);
          }
        }
      );
    } catch (err) {
      this.statusBar.setDaemonState('connected'); // Restore state on error
      vscode.window.showErrorMessage(`Library scan failed: ${err}`);
    }
  }

  async openSettings(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    try {
      const config = await this.client.getConfig();
      
      // Show the config in a quick pick with edit options
      const options = [
        {
          label: '$(folder) Library Folders',
          description: config.libraryPaths?.length 
            ? `${config.libraryPaths.length} folder(s)` 
            : 'No folders configured',
          action: 'libraryPaths',
        },
        {
          label: '$(settings-gear) Audio Settings',
          description: `Sample rate: ${config.sampleRate || 44100}Hz`,
          action: 'audio',
        },
        {
          label: '$(file) Open Config File',
          description: config.configPath || '~/.config/musicd/config.json',
          action: 'openFile',
        },
      ];

      const selected = await vscode.window.showQuickPick(options, {
        title: 'Daemon Settings',
        placeHolder: 'Select setting to configure',
      });

      if (!selected) {
        return;
      }

      switch (selected.action) {
        case 'libraryPaths':
          await this.manageLibraryFolders();
          break;
        case 'audio':
          await this.configureAudio();
          break;
        case 'openFile':
          if (config.configPath) {
            const doc = await vscode.workspace.openTextDocument(config.configPath);
            await vscode.window.showTextDocument(doc);
          }
          break;
      }
    } catch (err) {
      vscode.window.showErrorMessage(`Failed to get settings: ${err}`);
    }
  }

  async setLibraryFolder(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    const folders = await vscode.window.showOpenDialog({
      canSelectFiles: false,
      canSelectFolders: true,
      canSelectMany: false,
      title: 'Select Music Library Folder',
      openLabel: 'Set as Library',
    });

    if (folders && folders.length > 0) {
      try {
        await this.client.setConfig({ libraryPaths: [folders[0].fsPath] });
        vscode.window.showInformationMessage(
          `Library folder set to: ${folders[0].fsPath}`
        );
      } catch (err) {
        vscode.window.showErrorMessage(`Failed to set library folder: ${err}`);
      }
    }
  }

  async addLibraryFolder(): Promise<void> {
    if (!await this.ensureConnected()) {
      return;
    }

    const folders = await vscode.window.showOpenDialog({
      canSelectFiles: false,
      canSelectFolders: true,
      canSelectMany: true,
      title: 'Add Music Library Folder(s)',
      openLabel: 'Add to Library',
    });

    if (folders && folders.length > 0) {
      try {
        // Get current config
        const config = await this.client.getConfig();
        const currentPaths = config.libraryPaths || [];
        
        // Add new paths
        const newPaths = folders.map(f => f.fsPath);
        const allPaths = [...new Set([...currentPaths, ...newPaths])];
        
        await this.client.setConfig({ libraryPaths: allPaths });
        vscode.window.showInformationMessage(
          `Added ${newPaths.length} folder(s) to library`
        );
      } catch (err) {
        vscode.window.showErrorMessage(`Failed to add library folders: ${err}`);
      }
    }
  }

  private async manageLibraryFolders(): Promise<void> {
    const config = await this.client.getConfig();
    const paths = config.libraryPaths || [];

    const options = [
      { label: '$(add) Add Folder...', action: 'add' },
      ...paths.map(p => ({
        label: '$(folder) ' + p,
        description: '$(trash) Click to remove',
        action: 'remove',
        path: p,
      })),
    ];

    const selected = await vscode.window.showQuickPick(options, {
      title: 'Library Folders',
      placeHolder: paths.length ? 'Select folder to remove or add new' : 'No folders configured',
    });

    if (!selected) {
      return;
    }

    if (selected.action === 'add') {
      await this.addLibraryFolder();
    } else if (selected.action === 'remove' && 'path' in selected) {
      const confirm = await vscode.window.showWarningMessage(
        `Remove "${selected.path}" from library?`,
        'Remove',
        'Cancel'
      );

      if (confirm === 'Remove') {
        const newPaths = paths.filter(p => p !== selected.path);
        await this.client.setConfig({ libraryPaths: newPaths });
        vscode.window.showInformationMessage('Folder removed from library');
      }
    }
  }

  private async configureAudio(): Promise<void> {
    const config = await this.client.getConfig();

    const sampleRates = ['44100', '48000', '96000', '192000'];
    const currentRate = String(config.sampleRate || 44100);

    const selected = await vscode.window.showQuickPick(
      sampleRates.map(r => ({
        label: r + ' Hz',
        picked: r === currentRate,
      })),
      {
        title: 'Sample Rate',
        placeHolder: `Current: ${currentRate} Hz`,
      }
    );

    if (selected) {
      const newRate = parseInt(selected.label, 10);
      await this.client.setConfig({ sampleRate: newRate });
      vscode.window.showInformationMessage(
        `Sample rate set to ${newRate} Hz (restart daemon to apply)`
      );
    }
  }

  /**
   * Dispose of resources
   */
  dispose(): void {
    this.stopStatusPolling();
  }
}
