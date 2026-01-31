/**
 * Queue manager
 * Manages the play queue on the extension side
 */

import type { QueueItem, RepeatMode, TrackMetadata, Track } from '../types';
import { IPCClient } from '../daemon/client';

/**
 * Play queue manager
 */
export class QueueManager {
  private client: IPCClient;
  private items: QueueItem[] = [];
  private currentIndex: number = -1;
  private shuffle: boolean = false;
  private repeat: RepeatMode = 'off';
  private shuffleOrder: number[] = [];

  constructor(client: IPCClient) {
    this.client = client;
  }

  /**
   * Set the queue with new items
   */
  async setQueue(tracks: Track[]): Promise<void> {
    this.items = tracks.map((t) => ({
      path: t.path,
      metadata: {
        title: t.title || undefined,
        artist: t.artist || undefined,
        album: t.album || undefined,
        duration: t.durationMs || undefined,
      },
    }));

    this.currentIndex = -1;
    this.generateShuffleOrder();

    // Send to daemon
    const queueItems = this.items.map((i) => ({ path: i.path, metadata: i.metadata }));
    await this.client.queue(queueItems, false);
  }

  /**
   * Add tracks to the queue
   */
  async addToQueue(tracks: Track[]): Promise<void> {
    const newItems: QueueItem[] = tracks.map((t) => ({
      path: t.path,
      metadata: {
        title: t.title || undefined,
        artist: t.artist || undefined,
        album: t.album || undefined,
        duration: t.durationMs || undefined,
      },
    }));

    this.items.push(...newItems);
    this.generateShuffleOrder();

    // Append to daemon queue
    const queueItems = newItems.map((i) => ({ path: i.path, metadata: i.metadata }));
    await this.client.queue(queueItems, true);
  }

  /**
   * Clear the queue
   */
  async clear(): Promise<void> {
    this.items = [];
    this.currentIndex = -1;
    this.shuffleOrder = [];

    await this.client.stop();
  }

  /**
   * Play a track from the queue
   */
  async playIndex(index: number): Promise<void> {
    if (index < 0 || index >= this.items.length) {
      throw new Error('Invalid index');
    }

    this.currentIndex = index;
    const item = this.items[this.getActualIndex(index)];

    await this.client.play(item.path, item.metadata);
  }

  /**
   * Play the next track
   */
  async next(): Promise<boolean> {
    if (this.items.length === 0) {
      return false;
    }

    let nextIndex = this.currentIndex + 1;

    if (this.repeat === 'one') {
      // Stay on same track
      nextIndex = this.currentIndex;
    } else if (nextIndex >= this.items.length) {
      if (this.repeat === 'all') {
        nextIndex = 0;
      } else {
        return false; // End of queue
      }
    }

    await this.playIndex(nextIndex);
    return true;
  }

  /**
   * Play the previous track
   */
  async previous(): Promise<boolean> {
    if (this.items.length === 0) {
      return false;
    }

    let prevIndex = this.currentIndex - 1;

    if (this.repeat === 'one') {
      prevIndex = this.currentIndex;
    } else if (prevIndex < 0) {
      if (this.repeat === 'all') {
        prevIndex = this.items.length - 1;
      } else {
        return false; // Beginning of queue
      }
    }

    await this.playIndex(prevIndex);
    return true;
  }

  /**
   * Get the current queue
   */
  getQueue(): QueueItem[] {
    return [...this.items];
  }

  /**
   * Get the current index
   */
  getCurrentIndex(): number {
    return this.currentIndex;
  }

  /**
   * Get the current item
   */
  getCurrentItem(): QueueItem | null {
    if (this.currentIndex < 0 || this.currentIndex >= this.items.length) {
      return null;
    }
    return this.items[this.getActualIndex(this.currentIndex)];
  }

  /**
   * Set shuffle mode
   */
  setShuffle(enabled: boolean): void {
    this.shuffle = enabled;
    if (enabled) {
      this.generateShuffleOrder();
    }
  }

  /**
   * Get shuffle mode
   */
  getShuffle(): boolean {
    return this.shuffle;
  }

  /**
   * Set repeat mode
   */
  setRepeat(mode: RepeatMode): void {
    this.repeat = mode;
  }

  /**
   * Get repeat mode
   */
  getRepeat(): RepeatMode {
    return this.repeat;
  }

  /**
   * Get the actual index (considering shuffle)
   */
  private getActualIndex(displayIndex: number): number {
    if (!this.shuffle || this.shuffleOrder.length === 0) {
      return displayIndex;
    }
    return this.shuffleOrder[displayIndex] ?? displayIndex;
  }

  /**
   * Generate a shuffled order
   */
  private generateShuffleOrder(): void {
    this.shuffleOrder = [];
    for (let i = 0; i < this.items.length; i++) {
      this.shuffleOrder.push(i);
    }

    // Fisher-Yates shuffle
    for (let i = this.shuffleOrder.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [this.shuffleOrder[i], this.shuffleOrder[j]] = [this.shuffleOrder[j], this.shuffleOrder[i]];
    }
  }

  /**
   * Remove a track from the queue
   */
  removeAt(index: number): void {
    if (index < 0 || index >= this.items.length) {
      return;
    }

    this.items.splice(index, 1);

    if (index < this.currentIndex) {
      this.currentIndex--;
    } else if (index === this.currentIndex) {
      // Current track was removed
      if (this.currentIndex >= this.items.length) {
        this.currentIndex = this.items.length - 1;
      }
    }

    this.generateShuffleOrder();
  }

  /**
   * Move a track in the queue
   */
  move(fromIndex: number, toIndex: number): void {
    if (fromIndex < 0 || fromIndex >= this.items.length ||
        toIndex < 0 || toIndex >= this.items.length) {
      return;
    }

    const [item] = this.items.splice(fromIndex, 1);
    this.items.splice(toIndex, 0, item);

    // Adjust current index
    if (fromIndex === this.currentIndex) {
      this.currentIndex = toIndex;
    } else if (fromIndex < this.currentIndex && toIndex >= this.currentIndex) {
      this.currentIndex--;
    } else if (fromIndex > this.currentIndex && toIndex <= this.currentIndex) {
      this.currentIndex++;
    }

    this.generateShuffleOrder();
  }
}
