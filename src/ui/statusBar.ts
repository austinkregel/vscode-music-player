/**
 * Status bar integration
 * Shows daemon state and "Now Playing" information in the VS Code status bar
 */

import * as vscode from 'vscode';
import type { StatusResponse, PlaybackState } from '../types';

/**
 * Daemon connection state
 */
export type DaemonState = 
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'playing'
  | 'paused'
  | 'scanning'
  | 'error';

/**
 * Status bar manager for the music player
 * Provides multiple status bar items:
 * 1. Daemon state indicator (always visible when extension is active)
 * 2. Song title (visible when playing)
 * 3. Artist name (visible when playing, clickable to navigate to artist)
 * 4. Progress bar (visible when playing)
 * 5. Time display (visible when playing)
 */
export class StatusBarManager {
  private daemonStatusItem: vscode.StatusBarItem;
  private titleItem: vscode.StatusBarItem;
  private artistItem: vscode.StatusBarItem;
  private progressItem: vscode.StatusBarItem;
  private timeItem: vscode.StatusBarItem;
  private currentStatus: StatusResponse | null = null;
  private daemonState: DaemonState = 'disconnected';
  private scanProgress: number = 0;
  private errorMessage: string = '';
  private currentArtist: string = '';

  constructor() {
    // Daemon status - always visible, on the right side (lower priority = further right)
    this.daemonStatusItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      50
    );
    this.daemonStatusItem.command = 'local-media.connect';

    // Time display - rightmost of the now playing items
    this.timeItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      103
    );
    this.timeItem.command = 'local-media.showPlayer';

    // Progress bar
    this.progressItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      104
    );
    this.progressItem.command = 'local-media.showPlayer';

    // Artist name - clickable to navigate to artist in library
    this.artistItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      105
    );
    this.artistItem.command = 'local-media.goToArtist';

    // Song title - leftmost of the now playing items
    this.titleItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      106
    );
    this.titleItem.command = 'local-media.showPlayer';

    // Initialize with disconnected state
    this.updateDaemonStatus();
    this.daemonStatusItem.show();
  }

  /**
   * Get the current artist name (for navigation command)
   */
  getCurrentArtist(): string {
    return this.currentArtist;
  }

  /**
   * Set the daemon connection state
   */
  setDaemonState(state: DaemonState, errorMessage?: string): void {
    this.daemonState = state;
    this.errorMessage = errorMessage || '';
    this.updateDaemonStatus();
  }

  /**
   * Set scanning progress (0-100)
   */
  setScanProgress(progress: number): void {
    this.scanProgress = progress;
    if (this.daemonState === 'scanning') {
      this.updateDaemonStatus();
    }
  }

  /**
   * Update daemon status indicator
   */
  private updateDaemonStatus(): void {
    const { icon, text, tooltip, color } = this.getDaemonStatusDisplay();
    
    // Only show text if there is any (when playing/connected, just show icon)
    this.daemonStatusItem.text = text ? `${icon} ${text}` : icon;
    this.daemonStatusItem.tooltip = tooltip;
    this.daemonStatusItem.color = color;
    this.daemonStatusItem.backgroundColor = this.getBackgroundColor();

    // Update command based on state
    if (this.daemonState === 'disconnected' || this.daemonState === 'error') {
      this.daemonStatusItem.command = 'local-media.connect';
    } else {
      this.daemonStatusItem.command = 'local-media.openSettings';
    }
  }

  /**
   * Get display properties for current daemon state
   */
  private getDaemonStatusDisplay(): {
    icon: string;
    text: string;
    tooltip: string;
    color: string | vscode.ThemeColor | undefined;
  } {
    switch (this.daemonState) {
      case 'disconnected':
        return {
          icon: '$(circle-outline)',
          text: 'Music',
          tooltip: 'Click to connect to music daemon',
          color: undefined,
        };

      case 'connecting':
        return {
          icon: '$(sync~spin)',
          text: 'Connecting...',
          tooltip: 'Connecting to music daemon...',
          color: undefined,
        };

      case 'connected':
        return {
          icon: '$(circle-filled)',
          text: '',
          tooltip: 'Connected\nClick for settings',
          color: new vscode.ThemeColor('charts.green'),
        };

      case 'playing':
        return {
          icon: '$(circle-filled)',
          text: '',
          tooltip: 'Playing\nClick for settings',
          color: new vscode.ThemeColor('charts.green'),
        };

      case 'paused':
        return {
          icon: '$(circle-filled)',
          text: '',
          tooltip: 'Paused\nClick for settings',
          color: new vscode.ThemeColor('charts.yellow'),
        };

      case 'scanning':
        const progressText = this.scanProgress > 0 
          ? ` (${this.scanProgress}%)` 
          : '';
        return {
          icon: '$(sync~spin)',
          text: `Scanning${progressText}`,
          tooltip: 'Scanning music library...',
          color: new vscode.ThemeColor('charts.blue'),
        };

      case 'error':
        return {
          icon: '$(error)',
          text: 'Music',
          tooltip: `Error: ${this.errorMessage}\nClick to reconnect`,
          color: new vscode.ThemeColor('charts.red'),
        };

      default:
        return {
          icon: '$(circle-outline)',
          text: 'Music',
          tooltip: 'Music player',
          color: undefined,
        };
    }
  }

  /**
   * Get background color for error state
   */
  private getBackgroundColor(): vscode.ThemeColor | undefined {
    if (this.daemonState === 'error') {
      return new vscode.ThemeColor('statusBarItem.errorBackground');
    }
    return undefined;
  }

  /**
   * Update the now playing status
   */
  updateNowPlaying(status: StatusResponse): void {
    this.currentStatus = status;

    // Update daemon state based on playback
    if (status.state === 'playing') {
      this.setDaemonState('playing');
    } else if (status.state === 'paused') {
      this.setDaemonState('paused');
    } else if (this.daemonState === 'playing' || this.daemonState === 'paused') {
      this.setDaemonState('connected');
    }

    // Hide now playing items if stopped
    if (status.state === 'stopped') {
      this.hideNowPlaying();
      return;
    }

    // Extract metadata
    const title = status.metadata?.title || this.getFilenameFromPath(status.path);
    const artist = status.metadata?.artist || '';
    const album = status.metadata?.album || '';
    
    // Store current artist for navigation
    this.currentArtist = artist;

    // Title item
    this.titleItem.text = `$(music) ${this.truncate(title, 30)}`;
    this.titleItem.tooltip = this.formatTitleTooltip(title, album, status);
    this.titleItem.show();

    // Artist item (only show if we have an artist)
    if (artist) {
      this.artistItem.text = `$(account) ${this.truncate(artist, 20)}`;
      this.artistItem.tooltip = new vscode.MarkdownString(
        `**${artist}**\n\nClick to show artist in library`
      );
      this.artistItem.show();
    } else {
      this.artistItem.hide();
    }

    // Progress bar
    const progressBar = this.formatProgressBar(status.position, status.duration, 12);
    this.progressItem.text = progressBar;
    this.progressItem.tooltip = `Track ${status.queueIndex + 1} of ${status.queueSize}`;
    this.progressItem.show();

    // Time display
    const position = this.formatTime(status.position);
    const duration = this.formatTime(status.duration);
    this.timeItem.text = `${position}/${duration}`;
    this.timeItem.tooltip = `${position} of ${duration}`;
    this.timeItem.show();
  }

  /**
   * Extract filename from path
   */
  private getFilenameFromPath(path?: string): string {
    if (!path) return 'Unknown Track';
    const fileName = path.split('/').pop() || path;
    // Remove extension
    const lastDot = fileName.lastIndexOf('.');
    return lastDot > 0 ? fileName.substring(0, lastDot) : fileName;
  }

  /**
   * Format the title tooltip
   */
  private formatTitleTooltip(title: string, album: string, status: StatusResponse): vscode.MarkdownString {
    const md = new vscode.MarkdownString();
    md.isTrusted = true;
    md.appendMarkdown(`**${title}**\n\n`);
    if (album) {
      md.appendMarkdown(`*${album}*\n\n`);
    }
    md.appendMarkdown(`Click to show player`);
    return md;
  }

  /**
   * Get the icon for the current playback state
   */
  private getPlaybackIcon(state: PlaybackState): string {
    switch (state) {
      case 'playing':
        return '$(play)';
      case 'paused':
        return '$(debug-pause)';
      default:
        return '$(primitive-square)';
    }
  }

  /**
   * Format a progress bar
   * @param current Current position in ms
   * @param total Total duration in ms
   * @param width Width of the bar in characters
   */
  private formatProgressBar(current: number, total: number, width: number): string {
    if (total <= 0) {
      return '░'.repeat(width);
    }

    const progress = Math.min(current / total, 1);
    const filledCount = Math.floor(progress * width);
    const emptyCount = width - filledCount;

    // Use block characters for a nice look
    // █ = filled, ░ = empty, ▒ = current position indicator
    const filled = '█'.repeat(Math.max(0, filledCount - 1));
    const current_char = filledCount > 0 ? '▓' : '';
    const empty = '░'.repeat(emptyCount);

    return filled + current_char + empty;
  }
  /**
   * Format time in mm:ss
   */
  private formatTime(ms: number): string {
    const totalSeconds = Math.floor(ms / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${minutes}:${seconds.toString().padStart(2, '0')}`;
  }

  /**
   * Truncate text to specified length
   */
  private truncate(text: string, maxLength: number): string {
    if (text.length <= maxLength) {
      return text;
    }
    return text.substring(0, maxLength - 1) + '…';
  }

  /**
   * Get the current playback status
   */
  getStatus(): StatusResponse | null {
    return this.currentStatus;
  }

  /**
   * Get current daemon state
   */
  getDaemonState(): DaemonState {
    return this.daemonState;
  }

  /**
   * Hide now playing items (but keep daemon status visible)
   */
  hideNowPlaying(): void {
    this.titleItem.hide();
    this.artistItem.hide();
    this.progressItem.hide();
    this.timeItem.hide();
    this.currentArtist = '';
  }

  /**
   * Show the daemon status bar (called on extension activation)
   */
  show(): void {
    this.daemonStatusItem.show();
  }

  /**
   * Hide all status bar items
   */
  hide(): void {
    this.daemonStatusItem.hide();
    this.titleItem.hide();
    this.artistItem.hide();
    this.progressItem.hide();
    this.timeItem.hide();
  }

  /**
   * Dispose of resources
   */
  dispose(): void {
    this.daemonStatusItem.dispose();
    this.titleItem.dispose();
    this.artistItem.dispose();
    this.progressItem.dispose();
    this.timeItem.dispose();
  }
}

// Legacy export for backward compatibility
export { StatusBarManager as NowPlayingStatusBar };
