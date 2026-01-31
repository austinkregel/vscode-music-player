/**
 * Player control buttons in the status bar
 */

import * as vscode from 'vscode';

/**
 * Player control buttons displayed in the status bar
 */
export class PlayerControls {
  private prevButton: vscode.StatusBarItem;
  private playPauseButton: vscode.StatusBarItem;
  private nextButton: vscode.StatusBarItem;
  private isPlaying: boolean = false;
  private isVisible: boolean = false;

  constructor() {
    // Previous track button (rightmost of controls, so lowest priority on right side)
    this.prevButton = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      103 // Higher priority = more right
    );
    this.prevButton.text = '$(chevron-left)';
    this.prevButton.tooltip = 'Previous Track';
    this.prevButton.command = 'local-media.previous';

    // Play/Pause button
    this.playPauseButton = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      102
    );
    this.playPauseButton.command = 'local-media.playPause';
    this.updatePlayPauseButton();

    // Next track button (leftmost of controls on right side)
    this.nextButton = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Right,
      101
    );
    this.nextButton.text = '$(chevron-right)';
    this.nextButton.tooltip = 'Next Track';
    this.nextButton.command = 'local-media.next';
  }

  /**
   * Update the play/pause button state
   */
  setPlaying(isPlaying: boolean): void {
    this.isPlaying = isPlaying;
    this.updatePlayPauseButton();
  }

  private updatePlayPauseButton(): void {
    if (this.isPlaying) {
      this.playPauseButton.text = '$(debug-pause)';
      this.playPauseButton.tooltip = 'Pause';
    } else {
      this.playPauseButton.text = '$(play)';
      this.playPauseButton.tooltip = 'Play';
    }
  }

  /**
   * Show the player controls
   */
  show(): void {
    if (!this.isVisible) {
      this.prevButton.show();
      this.playPauseButton.show();
      this.nextButton.show();
      this.isVisible = true;
    }
  }

  /**
   * Hide the player controls
   */
  hide(): void {
    if (this.isVisible) {
      this.prevButton.hide();
      this.playPauseButton.hide();
      this.nextButton.hide();
      this.isVisible = false;
    }
  }

  /**
   * Dispose of resources
   */
  dispose(): void {
    this.prevButton.dispose();
    this.playPauseButton.dispose();
    this.nextButton.dispose();
  }
}
