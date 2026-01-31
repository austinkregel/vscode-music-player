/**
 * VS Code Local Media Extension
 * 
 * A music player extension that integrates with a headless Go daemon
 * for native audio playback and OS media session integration.
 */

import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { IPCClient } from './daemon/client';
import { DaemonManager } from './daemon/lifecycle';
import { AuthManager } from './auth/pairing';
import { StatusBarManager } from './ui/statusBar';
import { CommandHandler } from './ui/commands';
import { PlayerControls } from './ui/playerControls';
import { LibraryTreeProvider, PlaylistTreeProvider, QueueTreeProvider } from './library/treeProvider';
import { LibraryCache } from './library/cache';

// Supported audio file extensions
const AUDIO_EXTENSIONS = ['.mp3', '.flac', '.wav', '.ogg', '.m4a', '.aac', '.wma', '.opus'];

// Global instances
let client: IPCClient;
let daemonManager: DaemonManager;
let authManager: AuthManager;
let statusBar: StatusBarManager;
let playerControls: PlayerControls;
let commandHandler: CommandHandler;
let libraryCache: LibraryCache;
let libraryTreeProvider: LibraryTreeProvider;
let playlistTreeProvider: PlaylistTreeProvider;
let queueTreeProvider: QueueTreeProvider;
let outputChannel: vscode.OutputChannel;

/**
 * Helper to safely register a command with error handling
 */
function safeRegisterCommand(
  context: vscode.ExtensionContext,
  commandId: string,
  handler: (...args: unknown[]) => unknown
): vscode.Disposable {
  outputChannel.appendLine(`Registering command: ${commandId}`);
  try {
    const disposable = vscode.commands.registerCommand(commandId, async (...args: unknown[]) => {
      try {
        return await handler(...args);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        outputChannel.appendLine(`[ERROR] Command ${commandId} failed: ${message}`);
        vscode.window.showErrorMessage(`Command failed: ${message}`);
      }
    });
    outputChannel.appendLine(`  ✓ Registered: ${commandId}`);
    return disposable;
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    outputChannel.appendLine(`  ✗ Failed to register ${commandId}: ${message}`);
    // Return a no-op disposable
    return { dispose: () => {} };
  }
}

/**
 * Extension activation
 */
export async function activate(context: vscode.ExtensionContext): Promise<void> {
  console.log('local-media extension is activating...');

  // Create output channel for logging FIRST
  outputChannel = vscode.window.createOutputChannel('Local Media');
  context.subscriptions.push(outputChannel);
  outputChannel.appendLine('='.repeat(50));
  outputChannel.appendLine(`Local Media extension activating at ${new Date().toISOString()}`);
  outputChannel.appendLine('='.repeat(50));

  try {
    // Initialize IPC client
    outputChannel.appendLine('Initializing IPC client...');
    client = new IPCClient();

    // Initialize library cache (persisted in globalState)
    outputChannel.appendLine('Initializing library cache...');
    libraryCache = new LibraryCache(context.globalState);
    
    // Log cache status
    const stats = libraryCache.getStats();
    outputChannel.appendLine(`Library cache loaded: ${stats.tracks} tracks, ${stats.artists} artists, ${stats.albums} albums`);

    // Initialize UI components
    outputChannel.appendLine('Initializing UI components...');
    statusBar = new StatusBarManager();
    statusBar.show();

    playerControls = new PlayerControls();

    // Initialize tree providers with cache
    outputChannel.appendLine('Initializing tree providers...');
    libraryTreeProvider = new LibraryTreeProvider();
    libraryTreeProvider.setCache(libraryCache);

    playlistTreeProvider = new PlaylistTreeProvider();
    playlistTreeProvider.setCache(libraryCache);

    queueTreeProvider = new QueueTreeProvider();

    // Register tree views
    outputChannel.appendLine('Registering tree views...');
    const libraryTreeView = vscode.window.createTreeView('localMedia.library', {
      treeDataProvider: libraryTreeProvider,
      showCollapseAll: true,
    });
    context.subscriptions.push(libraryTreeView);
    
    // Pass tree view reference to provider for reveal functionality
    libraryTreeProvider.setTreeView(libraryTreeView);

    // Track double-clicks on library tree
    let lastSelectedItem: string | undefined;
    let lastSelectedTime = 0;
    const DOUBLE_CLICK_THRESHOLD = 400; // ms

    libraryTreeView.onDidChangeSelection((e) => {
      if (e.selection.length === 0) return;
      
      const item = e.selection[0];
      if (item.itemType !== 'track') return;
      
      const trackPath = item.resourceUri?.fsPath;
      if (!trackPath) return;

      const now = Date.now();
      
      // Check for double-click (same item selected twice quickly)
      if (trackPath === lastSelectedItem && (now - lastSelectedTime) < DOUBLE_CLICK_THRESHOLD) {
        // Double-click detected - play the track
        vscode.commands.executeCommand('local-media.playFile', trackPath);
        lastSelectedItem = undefined;
        lastSelectedTime = 0;
      } else {
        // Single click - just track for potential double-click
        lastSelectedItem = trackPath;
        lastSelectedTime = now;
      }
    });

    const playlistTreeView = vscode.window.createTreeView('localMedia.playlists', {
      treeDataProvider: playlistTreeProvider,
      showCollapseAll: true,
    });
    context.subscriptions.push(playlistTreeView);

    const queueTreeView = vscode.window.createTreeView('localMedia.queue', {
      treeDataProvider: queueTreeProvider,
    });
    context.subscriptions.push(queueTreeView);

    // Set up client event handlers
    outputChannel.appendLine('Setting up client event handlers...');
    client.on('connected', () => {
      outputChannel.appendLine('Connected to daemon');
      statusBar.setDaemonState('connected');
    });

    client.on('disconnected', () => {
      outputChannel.appendLine('Disconnected from daemon');
      statusBar.setDaemonState('disconnected');
      statusBar.hideNowPlaying();
      playerControls.hide();
    });

    client.on('error', (err: Error) => {
      outputChannel.appendLine(`Client error: ${err.message}`);
      statusBar.setDaemonState('error', err.message);
    });

    // Initialize managers
    outputChannel.appendLine('Initializing managers...');
    daemonManager = new DaemonManager(client, outputChannel, context.extensionPath);
    outputChannel.appendLine(`  Extension path: ${context.extensionPath}`);
    outputChannel.appendLine(`  Daemon binary exists: ${daemonManager.hasDaemonBinary()}`);
    
    authManager = new AuthManager(client, context.secrets);
    commandHandler = new CommandHandler(client, authManager, daemonManager, statusBar, context.extensionUri);

    // Pass dependencies to command handler
    commandHandler.setLibraryCache(libraryCache);
    commandHandler.setPlayerControls(playerControls);
    commandHandler.setQueueProvider(queueTreeProvider);

    // Register disposables
    context.subscriptions.push({
      dispose: () => {
        commandHandler.dispose();
        playerControls.dispose();
        statusBar.dispose();
        daemonManager.dispose();
        client.disconnect();
      },
    });

    // Register commands
    outputChannel.appendLine('Registering core commands...');
    commandHandler.registerCommands(context);

    // Register library view commands
    outputChannel.appendLine('Registering library commands...');
    registerLibraryCommands(context);

    outputChannel.appendLine('='.repeat(50));
    outputChannel.appendLine('Extension activation complete!');
    outputChannel.appendLine('='.repeat(50));

    // Try to connect on startup (non-blocking)
    tryAutoConnect().catch((err) => {
      outputChannel.appendLine(`Auto-connect failed: ${err}`);
    });

    console.log('local-media extension activated');
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    const stack = err instanceof Error ? err.stack : '';
    console.error('local-media extension activation failed:', err);
    outputChannel.appendLine('='.repeat(50));
    outputChannel.appendLine(`ACTIVATION FAILED: ${message}`);
    outputChannel.appendLine(stack || '(no stack trace)');
    outputChannel.appendLine('='.repeat(50));
    outputChannel.show(); // Show the output channel so the user sees the error
    vscode.window.showErrorMessage(`Local Media extension failed to activate: ${message}`);
  }
}

/**
 * Register library-related commands
 */
function registerLibraryCommands(context: vscode.ExtensionContext): void {
  try {
    // View mode commands
    context.subscriptions.push(
      safeRegisterCommand(context, 'local-media.viewArtists', () => {
        libraryTreeProvider.setViewMode('artists');
      }),
      safeRegisterCommand(context, 'local-media.viewAlbums', () => {
        libraryTreeProvider.setViewMode('albums');
      }),
      safeRegisterCommand(context, 'local-media.viewTracks', () => {
        libraryTreeProvider.setViewMode('tracks');
      }),
      safeRegisterCommand(context, 'local-media.viewPlaylists', () => {
        // Playlists have their own view now
        vscode.commands.executeCommand('localMedia.playlists.focus');
      }),
      safeRegisterCommand(context, 'local-media.refreshLibrary', () => {
        libraryTreeProvider.refresh();
        playlistTreeProvider.refresh();
      }),

      // Go to artist command (from status bar click)
      safeRegisterCommand(context, 'local-media.goToArtist', async () => {
        const artist = statusBar.getCurrentArtist();
        if (!artist) {
          vscode.window.showInformationMessage('No artist information available');
          return;
        }
        
        // Switch to artists view and focus the library panel
        libraryTreeProvider.setViewMode('artists');
        await vscode.commands.executeCommand('localMedia.library.focus');
        
        // Try to reveal the artist in the tree
        // The tree provider will need to expose a method for this
        libraryTreeProvider.revealArtist(artist);
      }),

      // Playlist commands
      safeRegisterCommand(context, 'local-media.createPlaylist', async () => {
        const name = await vscode.window.showInputBox({
          prompt: 'Enter playlist name',
          placeHolder: 'My Playlist',
        });
        if (name) {
          await libraryCache.createPlaylist(name);
          vscode.window.showInformationMessage(`Created playlist: ${name}`);
        }
      }),

      safeRegisterCommand(context, 'local-media.renamePlaylist', async (item: unknown) => {
        const itemObj = item as Record<string, unknown> | undefined;
        // itemData contains the CachedPlaylist object with id and name
        const itemData = itemObj?.itemData as Record<string, unknown> | undefined;
        const playlistId = itemData?.id as string | undefined;
        const currentName = itemObj?.label as string | undefined;
        
        if (!playlistId) {
          vscode.window.showErrorMessage('Could not determine playlist');
          return;
        }

        const newName = await vscode.window.showInputBox({
          prompt: 'Enter new playlist name',
          value: currentName || '',
        });

        if (newName && newName !== currentName) {
          await libraryCache.renamePlaylist(playlistId, newName);
          playlistTreeProvider.refresh();
          vscode.window.showInformationMessage(`Renamed playlist to: ${newName}`);
        }
      }),

      safeRegisterCommand(context, 'local-media.deletePlaylist', async (item: unknown) => {
        const itemObj = item as Record<string, unknown> | undefined;
        // itemData contains the CachedPlaylist object with id and name
        const itemData = itemObj?.itemData as Record<string, unknown> | undefined;
        const playlistId = itemData?.id as string | undefined;
        const playlistName = itemObj?.label as string | undefined;
        
        if (!playlistId) {
          vscode.window.showErrorMessage('Could not determine playlist');
          return;
        }

        const confirm = await vscode.window.showWarningMessage(
          `Delete playlist "${playlistName || 'Unknown'}"?`,
          { modal: true },
          'Delete'
        );

        if (confirm === 'Delete') {
          await libraryCache.deletePlaylist(playlistId);
          playlistTreeProvider.refresh();
          vscode.window.showInformationMessage(`Deleted playlist: ${playlistName}`);
        }
      }),

      safeRegisterCommand(context, 'local-media.addToPlaylist', async (item: unknown) => {
        const playlists = libraryCache.getPlaylists();
        if (playlists.length === 0) {
          const create = await vscode.window.showInformationMessage(
            'No playlists exist. Create one?',
            'Create'
          );
          if (create) {
            vscode.commands.executeCommand('local-media.createPlaylist');
          }
          return;
        }

        const selected = await vscode.window.showQuickPick(
          playlists.map(p => ({ label: p.name, id: p.id })),
          { placeHolder: 'Select playlist' }
        );

        const itemObj = item as Record<string, unknown> | undefined;
        if (selected && itemObj?.itemData) {
          const itemPath = typeof itemObj.itemData === 'string' 
            ? itemObj.itemData 
            : (itemObj.itemData as Record<string, unknown>).path;
          
          if (typeof itemPath === 'string') {
            // Check if path is a directory (album folder)
            try {
              const stat = await fs.promises.stat(itemPath);
              if (stat.isDirectory()) {
                // Enumerate audio files in the directory
                const files = await fs.promises.readdir(itemPath);
                const audioFiles = files
                  .filter(f => AUDIO_EXTENSIONS.includes(path.extname(f).toLowerCase()))
                  .sort() // Sort alphabetically (usually by track number prefix)
                  .map(f => path.join(itemPath, f));
                
                if (audioFiles.length === 0) {
                  vscode.window.showWarningMessage('No audio files found in this folder');
                  return;
                }
                
                // Add all audio files to playlist
                for (const filePath of audioFiles) {
                  await libraryCache.addToPlaylist(selected.id, filePath);
                }
                vscode.window.showInformationMessage(`Added ${audioFiles.length} tracks to ${selected.label}`);
              } else {
                // Single file
                await libraryCache.addToPlaylist(selected.id, itemPath);
                vscode.window.showInformationMessage(`Added to ${selected.label}`);
              }
            } catch (err) {
              vscode.window.showErrorMessage(`Failed to add to playlist: ${err}`);
            }
          }
        }
      }),

      // Play file command (called from tree view or double-click)
      safeRegisterCommand(context, 'local-media.playFile', async (arg: unknown) => {
        if (!authManager.isAuthenticated) {
          vscode.window.showWarningMessage('Not connected to daemon. Please connect first.');
          return;
        }

        // Extract the file path - arg could be:
        // 1. A string (from double-click handler)
        // 2. A LibraryItem object (from tree view click)
        // 3. An object with itemData.path (from tree view with ScanFileInfo)
        let filePath: string | undefined;
        
        if (typeof arg === 'string') {
          filePath = arg;
        } else if (arg && typeof arg === 'object') {
          const obj = arg as Record<string, unknown>;
          // Check for itemData.path (LibraryItem with ScanFileInfo)
          if (obj.itemData && typeof obj.itemData === 'object') {
            const itemData = obj.itemData as Record<string, unknown>;
            if (typeof itemData.path === 'string') {
              filePath = itemData.path;
            }
          }
          // Check for resourceUri.fsPath (LibraryItem with resourceUri)
          if (!filePath && obj.resourceUri && typeof obj.resourceUri === 'object') {
            const uri = obj.resourceUri as Record<string, unknown>;
            if (typeof uri.fsPath === 'string') {
              filePath = uri.fsPath;
            }
          }
          // Check for direct path property
          if (!filePath && typeof obj.path === 'string') {
            filePath = obj.path;
          }
        }

        if (!filePath) {
          vscode.window.showErrorMessage('Could not determine file path to play');
          return;
        }

        // Check if path is a directory (album folder) - if so, get all audio files
        let filesToPlay: string[] = [];
        try {
          const stat = await fs.promises.stat(filePath);
          if (stat.isDirectory()) {
            const files = await fs.promises.readdir(filePath);
            filesToPlay = files
              .filter(f => AUDIO_EXTENSIONS.includes(path.extname(f).toLowerCase()))
              .sort()
              .map(f => path.join(filePath, f));
            
            if (filesToPlay.length === 0) {
              vscode.window.showWarningMessage('No audio files found in this folder');
              return;
            }
          } else {
            filesToPlay = [filePath];
          }
        } catch {
          filesToPlay = [filePath];
        }

        // Build track info for queue
        const trackInfos = filesToPlay.map(fp => {
          const track = libraryCache.getTrackByPath(fp);
          return {
            path: fp,
            title: track?.metadata?.title,
            artist: track?.metadata?.artist,
            album: track?.metadata?.album,
          };
        });

        // Check if first track is already in the queue
        const existingIndex = queueTreeProvider.findTrackIndex(filesToPlay[0]);
        
        if (existingIndex >= 0 && filesToPlay.length === 1) {
          // Single track in queue - just switch to it
          await client.play(filesToPlay[0]);
          queueTreeProvider.setCurrentIndex(existingIndex);
          queueTreeProvider.setPlaybackState(true);
        } else {
          // Play first track and queue all tracks
          await client.play(filesToPlay[0]);
          
          // If multiple tracks, queue the rest
          if (filesToPlay.length > 1) {
            await client.queue(filesToPlay.slice(1).map(p => ({ path: p })), true);
          }
          
          // Update queue provider
          queueTreeProvider.setQueue(trackInfos, 0);
          queueTreeProvider.setPlaybackState(true);
        }
        
        playerControls.show();
        playerControls.setPlaying(true);
      }),

      // Play playlist from a specific index (called from playlist track click)
      safeRegisterCommand(context, 'local-media.playPlaylistFromIndex', async (tracks: unknown, startIndex: unknown) => {
        if (!authManager.isAuthenticated) {
          vscode.window.showWarningMessage('Not connected to daemon. Please connect first.');
          return;
        }

        // Validate arguments
        if (!Array.isArray(tracks) || typeof startIndex !== 'number') {
          vscode.window.showErrorMessage('Invalid playlist arguments');
          return;
        }

        const trackPaths = tracks as string[];
        if (trackPaths.length === 0 || startIndex < 0 || startIndex >= trackPaths.length) {
          return;
        }

        // Build track info for queue display
        const trackInfos = trackPaths.map(fp => {
          const track = libraryCache.getTrackByPath(fp);
          return {
            path: fp,
            title: track?.metadata?.title,
            artist: track?.metadata?.artist,
            album: track?.metadata?.album,
          };
        });

        // Play the track at startIndex
        await client.play(trackPaths[startIndex]);

        // Queue all tracks (clear existing queue first)
        // The tracks before startIndex go after the tracks after startIndex
        const tracksAfter = trackPaths.slice(startIndex + 1);
        const tracksBefore = trackPaths.slice(0, startIndex);
        const queueOrder = [...tracksAfter, ...tracksBefore];

        if (queueOrder.length > 0) {
          await client.queue(queueOrder.map(p => ({ path: p })), true); // clear=true
        }

        // Update queue provider - show full playlist with current track highlighted
        queueTreeProvider.setQueue(trackInfos, startIndex);
        queueTreeProvider.setPlaybackState(true);

        playerControls.show();
        playerControls.setPlaying(true);
      }),

      // Queue track command (handles both files and folders)
      safeRegisterCommand(context, 'local-media.queueTrack', async (item: unknown) => {
        if (!authManager.isAuthenticated) {
          vscode.window.showWarningMessage('Not connected to daemon.');
          return;
        }

        const itemObj = item as Record<string, unknown> | undefined;
        const itemPath = typeof itemObj?.itemData === 'string'
          ? itemObj.itemData
          : (itemObj?.itemData as Record<string, unknown> | undefined)?.path;

        if (typeof itemPath === 'string') {
          // Check if path is a directory (album folder)
          let filesToQueue: string[] = [];
          try {
            const stat = await fs.promises.stat(itemPath);
            if (stat.isDirectory()) {
              const files = await fs.promises.readdir(itemPath);
              filesToQueue = files
                .filter(f => AUDIO_EXTENSIONS.includes(path.extname(f).toLowerCase()))
                .sort()
                .map(f => path.join(itemPath, f));
              
              if (filesToQueue.length === 0) {
                vscode.window.showWarningMessage('No audio files found in this folder');
                return;
              }
            } else {
              filesToQueue = [itemPath];
            }
          } catch {
            filesToQueue = [itemPath];
          }

          // Queue all files
          await client.queue(filesToQueue.map(p => ({ path: p })), true);
          
          // Append to UI queue
          for (const fp of filesToQueue) {
            const track = libraryCache.getTrackByPath(fp);
            queueTreeProvider.appendTrack({
              path: fp,
              title: track?.metadata?.title,
              artist: track?.metadata?.artist,
              album: track?.metadata?.album,
            });
          }
          
          const msg = filesToQueue.length === 1 
            ? 'Added to queue' 
            : `Added ${filesToQueue.length} tracks to queue`;
          vscode.window.showInformationMessage(msg);
        }
      }),

      // Play album command
      safeRegisterCommand(context, 'local-media.playAlbum', async (item: unknown) => {
        outputChannel.appendLine(`[playAlbum] Called with: ${JSON.stringify(item)}`);
        
        if (!authManager.isAuthenticated) {
          vscode.window.showWarningMessage('Not connected to daemon.');
          return;
        }

        const itemObj = item as Record<string, unknown> | undefined;
        const albumPath = typeof itemObj?.itemData === 'string'
          ? itemObj.itemData
          : (itemObj?.itemData as Record<string, unknown> | undefined)?.albumPath;

        outputChannel.appendLine(`[playAlbum] Album path: ${albumPath}`);

        if (typeof albumPath === 'string') {
          const tracks = libraryCache.getTracksForAlbum(albumPath);
          outputChannel.appendLine(`[playAlbum] Found ${tracks.length} tracks`);
          
          if (tracks.length > 0) {
            const queueItems = tracks.map(t => ({ path: t.path }));
            await client.queue(queueItems, false); // Replace queue
            await client.play(queueItems[0].path); // Start playing
            
            // Update queue provider with all tracks
            const queueTrackInfos = tracks.map(t => ({
              path: t.path,
              title: t.metadata?.title,
              artist: t.metadata?.artist,
              album: t.metadata?.album,
            }));
            queueTreeProvider.setQueue(queueTrackInfos, 0);
            queueTreeProvider.setPlaybackState(true);
            
            playerControls.show();
            playerControls.setPlaying(true);
            vscode.window.showInformationMessage(`Playing ${tracks.length} tracks`);
          } else {
            vscode.window.showWarningMessage('No tracks found for this album');
          }
        } else {
          vscode.window.showWarningMessage('Could not determine album path');
        }
      })
    );

    outputChannel.appendLine('Library commands registered successfully');
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    outputChannel.appendLine(`[ERROR] Failed to register library commands: ${message}`);
    vscode.window.showErrorMessage(`Failed to register library commands: ${message}`);
  }
}

/**
 * Try to automatically connect to the daemon on startup
 */
async function tryAutoConnect(): Promise<void> {
  // Check if we have a stored token
  if (await authManager.hasStoredToken()) {
    try {
      statusBar.setDaemonState('connecting');
      
      // Try to connect
      await client.connect();
      
      // Try to authenticate with stored token
      if (await authManager.tryStoredToken()) {
        outputChannel.appendLine('Auto-connected with stored token');
        statusBar.setDaemonState('connected');
        commandHandler.startStatusPolling();
      } else {
        statusBar.setDaemonState('disconnected');
      }
    } catch {
      // Daemon might not be running, that's OK
      statusBar.setDaemonState('disconnected');
      outputChannel.appendLine('Auto-connect skipped - daemon not available');
    }
  }
}

/**
 * Extension deactivation
 */
export function deactivate(): void {
  console.log('local-media extension is deactivating...');

  // Cleanup is handled by disposables registered in activate()
}
