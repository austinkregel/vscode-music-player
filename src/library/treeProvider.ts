/**
 * Library tree view provider
 * Provides a browsable tree of Artists → Albums → Tracks
 * Backed by the LibraryCache for persistence
 */

import * as vscode from 'vscode';
import * as path from 'path';
import type { ArtistNFO, AlbumNFO, ScanFileInfo } from '../types';
import type { LibraryCache, CachedPlaylist } from './cache';

/**
 * Tree item types
 */
export type LibraryItemType = 'artist' | 'album' | 'track' | 'playlist' | 'playlistTrack' | 'category' | 'control' | 'nowPlaying';

/**
 * Base tree item for the library
 */
export class LibraryItem extends vscode.TreeItem {
  constructor(
    public readonly label: string,
    public readonly itemType: LibraryItemType,
    public readonly collapsibleState: vscode.TreeItemCollapsibleState,
    public readonly itemData?: ArtistNFO | AlbumNFO | ScanFileInfo | CachedPlaylist | string
  ) {
    super(label, collapsibleState);
    this.contextValue = itemType;
    this.setIcon();
  }

  private setIcon(): void {
    switch (this.itemType) {
      case 'artist':
        this.iconPath = new vscode.ThemeIcon('account');
        break;
      case 'album':
        this.iconPath = new vscode.ThemeIcon('library');
        break;
      case 'track':
        this.iconPath = new vscode.ThemeIcon('file-media');
        break;
      case 'playlist':
        this.iconPath = new vscode.ThemeIcon('list-unordered');
        break;
      case 'playlistTrack':
        this.iconPath = new vscode.ThemeIcon('file-media');
        break;
      case 'category':
        this.iconPath = new vscode.ThemeIcon('folder');
        break;
    }
  }
}

/**
 * Tree data provider for the music library
 */
export class LibraryTreeProvider implements vscode.TreeDataProvider<LibraryItem> {
  private _onDidChangeTreeData = new vscode.EventEmitter<LibraryItem | undefined>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private cache: LibraryCache | null = null;
  private viewMode: 'artists' | 'albums' | 'tracks' = 'artists';
  private treeView: vscode.TreeView<LibraryItem> | null = null;
  private pendingRevealArtist: string | null = null;

  constructor() {}

  /**
   * Set the tree view reference (needed for reveal functionality)
   */
  setTreeView(treeView: vscode.TreeView<LibraryItem>): void {
    this.treeView = treeView;
  }

  /**
   * Reveal an artist in the tree view
   */
  revealArtist(artistName: string): void {
    if (!this.cache) {
      return;
    }

    // Make sure we're in artists view
    if (this.viewMode !== 'artists') {
      this.setViewMode('artists');
    }

    // Find the artist item
    const artists = this.cache.getArtists();
    const tracks = this.cache.getTracks();
    
    // Build list of artist names from both NFO and track metadata
    const artistNames = new Set<string>();
    for (const artist of artists) {
      artistNames.add(artist.name);
    }
    for (const track of tracks) {
      if (track.metadata?.artist) {
        artistNames.add(track.metadata.artist);
      }
    }

    // Find matching artist (case-insensitive)
    const matchedArtist = Array.from(artistNames).find(
      name => name.toLowerCase() === artistName.toLowerCase()
    );

    if (matchedArtist && this.treeView) {
      // Create a temporary LibraryItem to reveal
      const artistItem = new LibraryItem(
        matchedArtist,
        'artist',
        vscode.TreeItemCollapsibleState.Collapsed,
        matchedArtist
      );

      // Reveal the item
      this.treeView.reveal(artistItem, { select: true, focus: true, expand: true })
        .then(() => {
          // Successfully revealed
        }, () => {
          // Reveal failed - just show a message
          vscode.window.showInformationMessage(`Artist: ${matchedArtist}`);
        });
    } else {
      vscode.window.showInformationMessage(`Artist "${artistName}" not found in library`);
    }
  }

  /**
   * Get parent of an element (needed for reveal to work)
   */
  getParent(element: LibraryItem): LibraryItem | null {
    // For top-level items (artists, albums in albums view, tracks in tracks view), parent is null
    if (element.itemType === 'artist') {
      return null;
    }
    
    // For albums under an artist, parent is the artist
    if (element.itemType === 'album' && this.viewMode === 'artists') {
      const albumData = element.itemData as AlbumNFO | undefined;
      if (albumData?.artist) {
        return new LibraryItem(
          albumData.artist,
          'artist',
          vscode.TreeItemCollapsibleState.Collapsed,
          albumData.artist
        );
      }
    }

    // For tracks under an album, parent is the album
    if (element.itemType === 'track') {
      const trackData = element.itemData as ScanFileInfo | undefined;
      if (trackData?.metadata?.album) {
        // This is simplified - in practice we'd need to find the exact album
        return null;
      }
    }

    return null;
  }

  /**
   * Set the cache instance
   */
  setCache(cache: LibraryCache): void {
    this.cache = cache;
    
    // Listen for cache changes
    cache.onChange(() => {
      this.refresh();
    });

    this.refresh();
  }

  /**
   * Set view mode (artists, albums, tracks)
   */
  setViewMode(mode: 'artists' | 'albums' | 'tracks'): void {
    this.viewMode = mode;
    this.refresh();
  }

  /**
   * Refresh the tree view
   */
  refresh(): void {
    this._onDidChangeTreeData.fire(undefined);
  }

  /**
   * Get tree item
   */
  getTreeItem(element: LibraryItem): vscode.TreeItem {
    return element;
  }

  /**
   * Get children of an element
   */
  getChildren(element?: LibraryItem): Thenable<LibraryItem[]> {
    if (!this.cache) {
      return Promise.resolve([]);
    }

    if (!element) {
      // Root level - show based on view mode
      return Promise.resolve(this.getRootItems());
    }

    // Child items
    switch (element.itemType) {
      case 'artist':
        return Promise.resolve(this.getAlbumsForArtist(element));
      case 'album':
        return Promise.resolve(this.getTracksForAlbum(element));
      default:
        return Promise.resolve([]);
    }
  }

  private getRootItems(): LibraryItem[] {
    if (!this.cache || !this.cache.hasData()) {
      return [];
    }

    switch (this.viewMode) {
      case 'artists':
        return this.getArtistItems();
      case 'albums':
        return this.getAlbumItems();
      case 'tracks':
        return this.getTrackItems();
      default:
        return [];
    }
  }

  private getArtistItems(): LibraryItem[] {
    if (!this.cache) return [];

    const artists = this.cache.getArtists();

    if (artists.length === 0) {
      // Fallback: derive artists from album metadata or file paths
      const albums = this.cache.getAlbums();
      const tracks = this.cache.getTracks();
      const artistSet = new Set<string>();
      
      for (const album of albums) {
        if (album.artist) {
          artistSet.add(album.artist);
        }
      }

      // If no albums, derive from file paths
      if (artistSet.size === 0) {
        for (const track of tracks) {
          const parts = track.path.split(path.sep);
          // Assume structure: .../Artist/Album/Track.mp3
          if (parts.length >= 3) {
            artistSet.add(parts[parts.length - 3]);
          }
        }
      }

      return Array.from(artistSet)
        .sort()
        .map(name => new LibraryItem(
          name,
          'artist',
          vscode.TreeItemCollapsibleState.Collapsed,
          name // Store artist name as data
        ));
    }

    return artists
      .sort((a, b) => a.name.localeCompare(b.name))
      .map(artist => {
        const item = new LibraryItem(
          artist.name,
          'artist',
          vscode.TreeItemCollapsibleState.Collapsed,
          artist
        );
        if (artist.rating) {
          item.description = `⭐ ${artist.rating}`;
        }
        return item;
      });
  }

  private getAlbumItems(): LibraryItem[] {
    if (!this.cache) return [];

    const albums = this.cache.getAlbums();

    if (albums.length === 0) {
      // Derive from file paths
      const tracks = this.cache.getTracks();
      const albumSet = new Map<string, { name: string; path: string }>();
      
      for (const track of tracks) {
        const dir = path.dirname(track.path);
        const albumName = path.basename(dir);
        if (!albumSet.has(dir)) {
          albumSet.set(dir, { name: albumName, path: dir });
        }
      }

      return Array.from(albumSet.values())
        .sort((a, b) => a.name.localeCompare(b.name))
        .map(album => {
          const item = new LibraryItem(
            album.name,
            'album',
            vscode.TreeItemCollapsibleState.Collapsed,
            album.path
          );
          return item;
        });
    }

    return albums
      .sort((a, b) => a.title.localeCompare(b.title))
      .map(album => {
        const item = new LibraryItem(
          album.title,
          'album',
          vscode.TreeItemCollapsibleState.Collapsed,
          album
        );
        const parts = [];
        if (album.artist) parts.push(album.artist);
        if (album.year) parts.push(`(${album.year})`);
        item.description = parts.join(' ');
        return item;
      });
  }

  private getTrackItems(): LibraryItem[] {
    if (!this.cache) return [];

    const tracks = this.cache.getTracks();

    return tracks
      .sort((a, b) => a.path.localeCompare(b.path))
      .map(track => {
        const fileName = path.basename(track.path, path.extname(track.path));
        const item = new LibraryItem(
          fileName,
          'track',
          vscode.TreeItemCollapsibleState.None,
          track
        );
        item.command = {
          command: 'local-media.playFile',
          title: 'Play',
          arguments: [track.path],
        };
        return item;
      });
  }

  private getAlbumsForArtist(artistItem: LibraryItem): LibraryItem[] {
    if (!this.cache) return [];

    const artistName = typeof artistItem.itemData === 'string' 
      ? artistItem.itemData 
      : (artistItem.itemData as ArtistNFO)?.name || artistItem.label;

    const albums = this.cache.getAlbumsForArtist(artistName as string);

    if (albums.length > 0) {
      return albums
        .sort((a, b) => (a.year || 0) - (b.year || 0))
        .map(album => {
          const item = new LibraryItem(
            album.title,
            'album',
            vscode.TreeItemCollapsibleState.Collapsed,
            album
          );
          if (album.year) {
            item.description = `(${album.year})`;
          }
          return item;
        });
    }

    // Fallback: derive from file paths
    const tracks = this.cache.getTracks();
    const albumSet = new Map<string, string>();
    
    for (const track of tracks) {
      const parts = track.path.split(path.sep);
      if (parts.length >= 3 && parts[parts.length - 3] === artistName) {
        const albumPath = path.dirname(track.path);
        const albumName = parts[parts.length - 2];
        albumSet.set(albumPath, albumName);
      }
    }

    return Array.from(albumSet.entries())
      .sort(([, a], [, b]) => a.localeCompare(b))
      .map(([albumPath, albumName]) => {
        const item = new LibraryItem(
          albumName,
          'album',
          vscode.TreeItemCollapsibleState.Collapsed,
          albumPath
        );
        return item;
      });
  }

  private getTracksForAlbum(albumItem: LibraryItem): LibraryItem[] {
    if (!this.cache) return [];

    let albumPath: string;

    if (typeof albumItem.itemData === 'string') {
      albumPath = albumItem.itemData;
    } else if ((albumItem.itemData as AlbumNFO)?.albumPath) {
      albumPath = (albumItem.itemData as AlbumNFO).albumPath;
    } else {
      // Try to find by matching album name
      const tracks = this.cache.getTracks();
      const albumName = albumItem.label;
      
      const matchingTracks = tracks.filter(t => {
        const dir = path.basename(path.dirname(t.path));
        return dir === albumName || dir.includes(albumName as string);
      });

      return matchingTracks
        .sort((a, b) => a.path.localeCompare(b.path))
        .map(track => this.createTrackItem(track));
    }

    const tracks = this.cache.getTracksForAlbum(albumPath);

    return tracks
      .sort((a, b) => a.path.localeCompare(b.path))
      .map(track => this.createTrackItem(track));
  }

  private createTrackItem(track: ScanFileInfo): LibraryItem {
    // Use metadata if available, otherwise fall back to filename
    let label: string;
    let description: string | undefined;

    if (track.metadata?.title) {
      label = track.metadata.title;
      if (track.metadata.artist) {
        description = track.metadata.artist;
      }
    } else {
      // Fallback to filename
      label = path.basename(track.path, path.extname(track.path));
    }

    const item = new LibraryItem(
      label,
      'track',
      vscode.TreeItemCollapsibleState.None,
      track
    );
    
    if (description) {
      item.description = description;
    }

    // Show duration in tooltip
    if (track.metadata?.duration) {
      const mins = Math.floor(track.metadata.duration / 60000);
      const secs = Math.floor((track.metadata.duration % 60000) / 1000);
      item.tooltip = `${label}\n${track.metadata.artist || ''}\n${track.metadata.album || ''}\n${mins}:${secs.toString().padStart(2, '0')}`;
    }

    // Set command to play on double-click
    item.command = {
      command: 'local-media.playFile',
      title: 'Play',
      arguments: [track.path],
    };
    item.resourceUri = vscode.Uri.file(track.path);
    return item;
  }
}

/**
 * Playlist tree provider
 */
export class PlaylistTreeProvider implements vscode.TreeDataProvider<LibraryItem> {
  private _onDidChangeTreeData = new vscode.EventEmitter<LibraryItem | undefined>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private cache: LibraryCache | null = null;

  constructor() {}

  setCache(cache: LibraryCache): void {
    this.cache = cache;
    cache.onChange(() => this.refresh());
    this.refresh();
  }

  refresh(): void {
    this._onDidChangeTreeData.fire(undefined);
  }

  getTreeItem(element: LibraryItem): vscode.TreeItem {
    return element;
  }

  getChildren(element?: LibraryItem): Thenable<LibraryItem[]> {
    if (!this.cache) return Promise.resolve([]);

    if (!element) {
      // Root level - show playlists
      const playlists = this.cache.getPlaylists();
      
      return Promise.resolve(
        playlists.map(playlist => {
          const item = new LibraryItem(
            playlist.name,
            'playlist',
            vscode.TreeItemCollapsibleState.Collapsed,
            playlist
          );
          item.description = `${playlist.tracks.length} tracks`;
          return item;
        })
      );
    }

    // Playlist tracks
    if (element.itemType === 'playlist') {
      const playlist = element.itemData as CachedPlaylist;
      
      return Promise.resolve(
        playlist.tracks.map((trackPath, index) => {
          const fileName = path.basename(trackPath, path.extname(trackPath));
          const item = new LibraryItem(
            `${index + 1}. ${fileName}`,
            'playlistTrack',
            vscode.TreeItemCollapsibleState.None,
            trackPath
          );
          // Play entire playlist starting from this track
          item.command = {
            command: 'local-media.playPlaylistFromIndex',
            title: 'Play',
            arguments: [playlist.tracks, index],
          };
          return item;
        })
      );
    }

    return Promise.resolve([]);
  }
}

interface QueueTrackInfo {
  path: string;
  title?: string;
  artist?: string;
  album?: string;
}

/**
 * Queue tree provider - shows the current playback queue
 */
export class QueueTreeProvider implements vscode.TreeDataProvider<LibraryItem> {
  private _onDidChangeTreeData = new vscode.EventEmitter<LibraryItem | undefined>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private queue: QueueTrackInfo[] = [];
  private currentIndex: number = -1;
  private isPlaying: boolean = false;

  refresh(): void {
    this._onDidChangeTreeData.fire(undefined);
  }

  setQueue(tracks: QueueTrackInfo[], currentIndex: number = 0): void {
    this.queue = tracks;
    this.currentIndex = currentIndex;
    this.refresh();
  }

  setPlaybackState(isPlaying: boolean): void {
    this.isPlaying = isPlaying;
    this.refresh();
  }

  setCurrentTrack(info: { title?: string; artist?: string; position?: number; duration?: number; path?: string }): void {
    // Update metadata of current track if we have one
    if (this.currentIndex >= 0 && this.currentIndex < this.queue.length) {
      if (info.title) {
        this.queue[this.currentIndex].title = info.title;
      }
      if (info.artist) {
        this.queue[this.currentIndex].artist = info.artist;
      }
    } else if (info.path) {
      // Current index is out of bounds - try to find the track in queue
      const existingIndex = this.queue.findIndex(t => t.path === info.path);
      if (existingIndex >= 0) {
        // Track exists - update its metadata and set as current
        this.currentIndex = existingIndex;
        if (info.title) {
          this.queue[existingIndex].title = info.title;
        }
        if (info.artist) {
          this.queue[existingIndex].artist = info.artist;
        }
      } else if (this.queue.length === 0) {
        // Queue is truly empty - add the track
        this.queue = [{
          path: info.path,
          title: info.title,
          artist: info.artist,
        }];
        this.currentIndex = 0;
      }
    }
    this.refresh();
  }

  setCurrentIndex(index: number): void {
    this.currentIndex = index;
    this.refresh();
  }

  appendTrack(track: QueueTrackInfo): void {
    this.queue.push(track);
    this.refresh();
  }

  appendTracks(tracks: QueueTrackInfo[]): void {
    this.queue.push(...tracks);
    this.refresh();
  }

  /**
   * Find a track in the queue by path
   * Returns the index or -1 if not found
   */
  findTrackIndex(path: string): number {
    return this.queue.findIndex(t => t.path === path);
  }

  /**
   * Get the current queue
   */
  getQueue(): QueueTrackInfo[] {
    return [...this.queue];
  }

  /**
   * Check if queue has any items
   */
  hasItems(): boolean {
    return this.queue.length > 0;
  }

  clearQueue(): void {
    this.queue = [];
    this.currentIndex = -1;
    this.refresh();
  }

  getTreeItem(element: LibraryItem): vscode.TreeItem {
    return element;
  }

  getChildren(element?: LibraryItem): Thenable<LibraryItem[]> {
    if (element) return Promise.resolve([]);

    const items: LibraryItem[] = [];

    if (this.queue.length === 0) {
      const emptyItem = new LibraryItem(
        'Queue is empty',
        'category',
        vscode.TreeItemCollapsibleState.None
      );
      emptyItem.iconPath = new vscode.ThemeIcon('info');
      emptyItem.description = 'Double-click a track to play';
      items.push(emptyItem);
      return Promise.resolve(items);
    }

    // Add queue tracks
    this.queue.forEach((track, index) => {
      // Use metadata if available, otherwise fallback to filename
      const label = track.title || path.basename(track.path, path.extname(track.path));
      const isCurrent = index === this.currentIndex;
      const item = new LibraryItem(
        label,
        'track',
        vscode.TreeItemCollapsibleState.None,
        track.path
      );
      
      // All items show artist if available, current one gets play icon
      item.description = track.artist || '';
      if (isCurrent) {
        item.iconPath = new vscode.ThemeIcon(this.isPlaying ? 'play' : 'debug-pause');
      } else {
        item.iconPath = new vscode.ThemeIcon('circle-outline');
      }
      
      // Double-click to play
      item.command = {
        command: 'local-media.playFile',
        title: 'Play',
        arguments: [track.path],
      };
      items.push(item);
    });

    return Promise.resolve(items);
  }
}
