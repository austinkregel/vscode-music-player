/**
 * Mock IPC Client for testing
 */

import { EventEmitter } from 'events';
import type { StatusResponse, PairResponse, TrackMetadata } from '../../types';

export class MockIPCClient extends EventEmitter {
  private connected = false;
  private token: string | null = null;
  private responses: Map<string, unknown> = new Map();
  private currentStatus: StatusResponse = {
    state: 'stopped',
    position: 0,
    duration: 0,
    volume: 1.0,
    queueIndex: -1,
    queueSize: 0,
  };

  async connect(): Promise<void> {
    this.connected = true;
    this.emit('connected');
  }

  disconnect(): void {
    this.connected = false;
    this.emit('disconnected');
  }

  isConnected(): boolean {
    return this.connected;
  }

  setToken(token: string): void {
    this.token = token;
  }

  getToken(): string | null {
    return this.token;
  }

  // Mock response setters
  setResponse(command: string, response: unknown): void {
    this.responses.set(command, response);
  }

  setStatus(status: Partial<StatusResponse>): void {
    this.currentStatus = { ...this.currentStatus, ...status };
  }

  // Simulate events
  simulateEvent(event: string, data: unknown): void {
    this.emit(event, data);
  }

  // API methods
  async pair(clientName: string): Promise<PairResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    return {
      token: 'mock-token-' + Date.now(),
      clientId: 'mock-client-id',
      requiresApproval: false,
    };
  }

  async play(path: string, metadata?: TrackMetadata): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    this.currentStatus = {
      ...this.currentStatus,
      state: 'playing',
      path,
      metadata,
    };

    return this.currentStatus;
  }

  async pause(): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    this.currentStatus.state = 'paused';
    return this.currentStatus;
  }

  async resume(): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    this.currentStatus.state = 'playing';
    return this.currentStatus;
  }

  async stop(): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    this.currentStatus = {
      ...this.currentStatus,
      state: 'stopped',
      path: undefined,
      position: 0,
    };

    return this.currentStatus;
  }

  async next(): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    return this.currentStatus;
  }

  async prev(): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    return this.currentStatus;
  }

  async queue(paths: string[], append: boolean): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    this.currentStatus.queueSize = append
      ? this.currentStatus.queueSize + paths.length
      : paths.length;

    return this.currentStatus;
  }

  async seek(position: number): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    this.currentStatus.position = position;
    return this.currentStatus;
  }

  async setVolume(level: number): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    this.currentStatus.volume = level;
    return this.currentStatus;
  }

  async getStatus(): Promise<StatusResponse> {
    if (!this.connected) {
      throw new Error('Not connected');
    }

    return this.currentStatus;
  }
}
