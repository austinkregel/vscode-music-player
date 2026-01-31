/**
 * Library browser tree view
 * Provides a tree view for browsing the music library
 */

import * as vscode from 'vscode';
import { MetadataCache } from '../metadata/cache';
import type { Track, Album, Artist } from '../metadata/models';

/**
 * Tree item types
 */
export type LibraryItemType = 'root' | 'artists' | 'albums' | 'tracks' | 'artist' | 'album' | 'track';

/**
 * Tree item data
 */
export interface LibraryItemData {
  type: LibraryItemType;
  id?: string;
  name: string;
  path?: string;
}

/**
 * Library tree item
 */
export class LibraryTreeItem extends vscode.TreeItem {
  constructor(
    public readonly data: LibraryItemData,
    public readonly collapsibleState: vscode.TreeItemCollapsibleState
  ) {
    super(data.name, collapsibleState);

    this.contextValue = data.type;

    switch (data.type) {
      case 'artists':
        this.iconPath = new vscode.ThemeIcon('person');
        break;
      case 'albums':
        this.iconPath = new vscode.ThemeIcon('library');
        break;
      case 'tracks':
        this.iconPath = new vscode.ThemeIcon('list-flat');
        break;
      case 'artist':
        this.iconPath = new vscode.ThemeIcon('account');
        break;
      case 'album':
        this.iconPath = new vscode.ThemeIcon('folder');
        break;
      case 'track':
        this.iconPath = new vscode.ThemeIcon('file-media');
        this.command = {
          command: 'local-media.playTrack',
          title: 'Play',
          arguments: [data.path],
        };
        break;
    }
  }
}

/**
 * Library tree data provider
 */
export class LibraryTreeDataProvider implements vscode.TreeDataProvider<LibraryTreeItem> {
  private _onDidChangeTreeData = new vscode.EventEmitter<LibraryTreeItem | undefined | null | void>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private cache: MetadataCache;

  constructor(cache: MetadataCache) {
    this.cache = cache;
  }

  /**
   * Refresh the tree view
   */
  refresh(): void {
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(element: LibraryTreeItem): vscode.TreeItem {
    return element;
  }

  async getChildren(element?: LibraryTreeItem): Promise<LibraryTreeItem[]> {
    if (!element) {
      // Root level - show categories
      return [
        new LibraryTreeItem(
          { type: 'artists', name: 'Artists' },
          vscode.TreeItemCollapsibleState.Collapsed
        ),
        new LibraryTreeItem(
          { type: 'albums', name: 'Albums' },
          vscode.TreeItemCollapsibleState.Collapsed
        ),
        new LibraryTreeItem(
          { type: 'tracks', name: 'All Tracks' },
          vscode.TreeItemCollapsibleState.Collapsed
        ),
      ];
    }

    switch (element.data.type) {
      case 'artists':
        return this.getArtists();
      case 'albums':
        return this.getAlbums();
      case 'tracks':
        return this.getAllTracks();
      case 'artist':
        return this.getArtistAlbums(element.data.name);
      case 'album':
        return this.getAlbumTracks(element.data.name);
      default:
        return [];
    }
  }

  private async getArtists(): Promise<LibraryTreeItem[]> {
    const artists = await this.cache.getAllArtists();
    return artists.map(
      (artist) =>
        new LibraryTreeItem(
          { type: 'artist', id: artist.id, name: artist.name },
          vscode.TreeItemCollapsibleState.Collapsed
        )
    );
  }

  private async getAlbums(): Promise<LibraryTreeItem[]> {
    const albums = await this.cache.getAllAlbums();
    return albums.map(
      (album) =>
        new LibraryTreeItem(
          { type: 'album', id: album.id, name: album.name },
          vscode.TreeItemCollapsibleState.Collapsed
        )
    );
  }

  private async getAllTracks(): Promise<LibraryTreeItem[]> {
    const tracks = await this.cache.getAllTracks();
    return tracks.map((track) => this.trackToTreeItem(track));
  }

  private async getArtistAlbums(artistName: string): Promise<LibraryTreeItem[]> {
    const tracks = await this.cache.getTracksByArtist(artistName);
    
    // Get unique albums
    const albumSet = new Set<string>();
    const albums: string[] = [];
    for (const track of tracks) {
      if (track.album && !albumSet.has(track.album)) {
        albumSet.add(track.album);
        albums.push(track.album);
      }
    }

    return albums.map(
      (album) =>
        new LibraryTreeItem(
          { type: 'album', name: album },
          vscode.TreeItemCollapsibleState.Collapsed
        )
    );
  }

  private async getAlbumTracks(albumName: string): Promise<LibraryTreeItem[]> {
    const tracks = await this.cache.getTracksByAlbum(albumName);
    return tracks.map((track) => this.trackToTreeItem(track));
  }

  private trackToTreeItem(track: Track): LibraryTreeItem {
    const name = track.title || track.path.split('/').pop() || 'Unknown';
    return new LibraryTreeItem(
      { type: 'track', id: track.id, name, path: track.path },
      vscode.TreeItemCollapsibleState.None
    );
  }
}
