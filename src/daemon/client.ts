/**
 * IPC Client for communicating with the musicd daemon
 * Uses Unix domain sockets on Linux/macOS and named pipes on Windows
 */

import * as net from 'net';
import * as path from 'path';
import * as os from 'os';
import { EventEmitter } from 'events';
import type {
  Response,
  StatusResponse,
  AudioDataResponse,
  PairResponse,
  TrackMetadata,
  CommandType,
  ConfigRequest,
  ConfigResponse,
  ScanResponse,
  ScanStatusResponse,
  QueueItem,
  PushMessage,
} from '../types';
import {
  encodeCommand,
  decodeResponse,
  parseMessage,
  createPairRequest,
  createPlayRequest,
  createQueueRequest,
  createSeekRequest,
  createVolumeRequest,
  isPairResponse,
  isStatusResponse,
  isAudioDataResponse,
  isConfigResponse,
  isScanResponse,
  isScanStatusResponse,
} from './protocol';

export interface IPCClientOptions {
  socketPath?: string;
  connectTimeout?: number;
  requestTimeout?: number;
}

export interface IPCClientEvents {
  connected: () => void;
  disconnected: () => void;
  error: (error: Error) => void;
  status: (status: StatusResponse) => void;
  audioData: (data: AudioDataResponse) => void;
}

/**
 * IPC Client for daemon communication
 */
export class IPCClient extends EventEmitter {
  private socket: net.Socket | null = null;
  private socketPath: string;
  private token: string | null = null;
  private buffer: string = '';
  private pendingRequests: Map<number, {
    resolve: (value: Response) => void;
    reject: (reason: Error) => void;
    timeout: NodeJS.Timeout;
  }> = new Map();
  private requestCounter: number = 0;
  private connectTimeout: number;
  private requestTimeout: number;
  private reconnectTimer: NodeJS.Timeout | null = null;
  
  // Request queue to serialize all requests (protocol doesn't support concurrent requests)
  private requestQueue: Array<{
    cmd: CommandType;
    data?: unknown;
    resolve: (value: Response) => void;
    reject: (reason: Error) => void;
  }> = [];
  private isProcessingRequest: boolean = false;

  constructor(options: IPCClientOptions = {}) {
    super();
    this.socketPath = options.socketPath || this.getDefaultSocketPath();
    this.connectTimeout = options.connectTimeout || 5000;
    this.requestTimeout = options.requestTimeout || 10000;
  }

  /**
   * Get the default socket path based on platform
   */
  private getDefaultSocketPath(): string {
    if (process.platform === 'win32') {
      return `\\\\.\\pipe\\musicd-${os.userInfo().username}`;
    }
    return `/tmp/musicd-${process.getuid?.() || os.userInfo().uid}.sock`;
  }

  /**
   * Set the authentication token
   */
  setToken(token: string): void {
    this.token = token;
  }

  /**
   * Get the current token
   */
  getToken(): string | null {
    return this.token;
  }

  /**
   * Check if connected to daemon
   */
  isConnected(): boolean {
    return this.socket !== null && !this.socket.destroyed;
  }

  /**
   * Connect to the daemon
   */
  async connect(): Promise<void> {
    if (this.isConnected()) {
      return;
    }

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        if (this.socket) {
          this.socket.destroy();
          this.socket = null;
        }
        reject(new Error('Connection timeout'));
      }, this.connectTimeout);

      this.socket = net.createConnection(this.socketPath);

      this.socket.on('connect', () => {
        clearTimeout(timeout);
        this.emit('connected');
        resolve();
      });

      this.socket.on('error', (err) => {
        clearTimeout(timeout);
        this.emit('error', err);
        if (!this.isConnected()) {
          reject(err);
        }
      });

      this.socket.on('data', (data) => {
        this.handleData(data);
      });

      this.socket.on('close', () => {
        this.socket = null;
        this.emit('disconnected');
      });
    });
  }

  /**
   * Disconnect from the daemon
   */
  disconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    if (this.socket) {
      this.socket.destroy();
      this.socket = null;
    }

    // Reject all pending requests
    for (const [id, pending] of this.pendingRequests) {
      clearTimeout(pending.timeout);
      pending.reject(new Error('Disconnected'));
      this.pendingRequests.delete(id);
    }

    // Reject all queued requests
    for (const request of this.requestQueue) {
      request.reject(new Error('Disconnected'));
    }
    this.requestQueue = [];
    this.isProcessingRequest = false;
  }

  /**
   * Handle incoming data from socket
   */
  private handleData(data: Buffer): void {
    this.buffer += data.toString('utf-8');

    // Process all complete messages in buffer
    let result = parseMessage(this.buffer);
    while (result.message !== null) {
      this.handleMessage(result.message);
      this.buffer = result.remaining;
      result = parseMessage(this.buffer);
    }
  }

  /**
   * Handle a complete message
   */
  private handleMessage(message: string): void {
    // Try to parse as JSON first
    let parsed: unknown;
    try {
      parsed = JSON.parse(message);
    } catch {
      return; // Invalid JSON, ignore
    }

    // Check if it's a push message (has 'type' field) vs response (has 'success' field)
    if (parsed && typeof parsed === 'object' && 'type' in parsed) {
      this.handlePushMessage(parsed as PushMessage);
      return;
    }

    // Handle as request/response
    const response = decodeResponse(message);

    // Handle responses sequentially (FIFO queue)
    const entry = this.pendingRequests.entries().next().value;
    if (entry) {
      const [id, pending] = entry as [number, typeof entry[1]];
      clearTimeout(pending.timeout);
      this.pendingRequests.delete(id);
      pending.resolve(response);
    }
  }

  /**
   * Handle a push message from the server
   */
  private handlePushMessage(msg: PushMessage): void {
    if (msg.type === 'audioData' && isAudioDataResponse(msg.data)) {
      this.emit('audioData', msg.data);
    }
  }

  /**
   * Send a command to the daemon (queued to prevent response mismatch)
   */
  private async send(cmd: CommandType, data?: unknown): Promise<Response> {
    if (!this.isConnected()) {
      throw new Error('Not connected to daemon');
    }

    // Queue the request and process serially
    return new Promise((resolve, reject) => {
      this.requestQueue.push({ cmd, data, resolve, reject });
      this.processNextRequest();
    });
  }

  /**
   * Process the next request in the queue
   */
  private processNextRequest(): void {
    if (this.isProcessingRequest || this.requestQueue.length === 0) {
      return;
    }

    const request = this.requestQueue.shift()!;
    this.isProcessingRequest = true;

    const id = ++this.requestCounter;
    
    const timeout = setTimeout(() => {
      this.pendingRequests.delete(id);
      this.isProcessingRequest = false;
      request.reject(new Error('Request timeout'));
      this.processNextRequest();
    }, this.requestTimeout);

    this.pendingRequests.set(id, { 
      resolve: (response) => {
        this.isProcessingRequest = false;
        request.resolve(response);
        this.processNextRequest();
      }, 
      reject: (error) => {
        this.isProcessingRequest = false;
        request.reject(error);
        this.processNextRequest();
      }, 
      timeout 
    });

    const message = encodeCommand(request.cmd, request.data, this.token || undefined) + '\n';
    this.socket!.write(message, (err) => {
      if (err) {
        clearTimeout(timeout);
        this.pendingRequests.delete(id);
        this.isProcessingRequest = false;
        request.reject(err);
        this.processNextRequest();
      }
    });
  }

  // =========================================================================
  // Public API Methods
  // =========================================================================

  /**
   * Pair with the daemon
   */
  async pair(clientName: string): Promise<PairResponse> {
    const response = await this.send('pair', createPairRequest(clientName));
    
    if (!response.success) {
      throw new Error(response.error || 'Pairing failed');
    }

    if (!isPairResponse(response.data)) {
      throw new Error('Invalid pair response');
    }

    // Store the token
    this.token = response.data.token;

    return response.data;
  }

  /**
   * Play a file
   */
  async play(filePath: string, metadata?: TrackMetadata): Promise<StatusResponse> {
    const response = await this.send('play', createPlayRequest(filePath, metadata));
    
    if (!response.success) {
      throw new Error(response.error || 'Play failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Pause playback
   */
  async pause(): Promise<StatusResponse> {
    const response = await this.send('pause');
    
    if (!response.success) {
      throw new Error(response.error || 'Pause failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Resume playback
   */
  async resume(): Promise<StatusResponse> {
    const response = await this.send('resume');
    
    if (!response.success) {
      throw new Error(response.error || 'Resume failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Stop playback
   */
  async stop(): Promise<StatusResponse> {
    const response = await this.send('stop');
    
    if (!response.success) {
      throw new Error(response.error || 'Stop failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Skip to next track
   */
  async next(): Promise<StatusResponse> {
    const response = await this.send('next');
    
    if (!response.success) {
      throw new Error(response.error || 'Next failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Go to previous track
   */
  async prev(): Promise<StatusResponse> {
    const response = await this.send('prev');
    
    if (!response.success) {
      throw new Error(response.error || 'Previous failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Set or append to the queue
   */
  async queue(items: QueueItem[], append: boolean = false): Promise<StatusResponse> {
    const response = await this.send('queue', createQueueRequest(items, append));
    
    if (!response.success) {
      throw new Error(response.error || 'Queue failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Seek to a position
   */
  async seek(positionMs: number): Promise<StatusResponse> {
    const response = await this.send('seek', createSeekRequest(positionMs));
    
    if (!response.success) {
      throw new Error(response.error || 'Seek failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Set volume level
   */
  async setVolume(level: number): Promise<StatusResponse> {
    const response = await this.send('volume', createVolumeRequest(level));
    
    if (!response.success) {
      throw new Error(response.error || 'Volume failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Get current status
   */
  async getStatus(): Promise<StatusResponse> {
    const response = await this.send('status');
    
    if (!response.success) {
      throw new Error(response.error || 'Status failed');
    }

    if (!isStatusResponse(response.data)) {
      throw new Error('Invalid status response');
    }

    return response.data;
  }

  /**
   * Get real-time audio frequency data for visualization (polling mode)
   * Returns 64 frequency bands (0-255 each), logarithmically distributed 20Hz-20kHz
   * @deprecated Use subscribeAudioData() for real-time streaming instead
   */
  async getAudioData(): Promise<AudioDataResponse> {
    const response = await this.send('getAudioData');
    
    if (!response.success) {
      throw new Error(response.error || 'Get audio data failed');
    }

    if (!isAudioDataResponse(response.data)) {
      throw new Error('Invalid audio data response');
    }

    return response.data;
  }

  /**
   * Subscribe to real-time audio data streaming (~60fps)
   * Audio data will be emitted via the 'audioData' event
   */
  async subscribeAudioData(): Promise<void> {
    const response = await this.send('subscribeAudioData');
    
    if (!response.success) {
      throw new Error(response.error || 'Subscribe failed');
    }
  }

  /**
   * Unsubscribe from audio data streaming
   */
  async unsubscribeAudioData(): Promise<void> {
    const response = await this.send('unsubscribeAudioData');
    
    if (!response.success) {
      throw new Error(response.error || 'Unsubscribe failed');
    }
  }

  /**
   * Get daemon configuration
   */
  async getConfig(): Promise<ConfigResponse> {
    const response = await this.send('getConfig');
    
    if (!response.success) {
      throw new Error(response.error || 'Get config failed');
    }

    if (!isConfigResponse(response.data)) {
      throw new Error('Invalid config response');
    }

    return response.data;
  }

  /**
   * Update daemon configuration
   */
  async setConfig(config: ConfigRequest): Promise<ConfigResponse> {
    const response = await this.send('setConfig', config);
    
    if (!response.success) {
      throw new Error(response.error || 'Set config failed');
    }

    if (!isConfigResponse(response.data)) {
      throw new Error('Invalid config response');
    }

    return response.data;
  }

  /**
   * Start a library scan (async - returns immediately)
   */
  async scanLibrary(): Promise<ScanStatusResponse> {
    const response = await this.send('scanLibrary');
    
    if (!response.success) {
      throw new Error(response.error || 'Scan library failed');
    }

    if (!isScanStatusResponse(response.data)) {
      throw new Error('Invalid scan status response');
    }

    return response.data;
  }

  /**
   * Get the current scan status and results
   */
  async getScanStatus(): Promise<ScanStatusResponse> {
    const response = await this.send('getScanStatus');
    
    if (!response.success) {
      throw new Error(response.error || 'Get scan status failed');
    }

    if (!isScanStatusResponse(response.data)) {
      throw new Error('Invalid scan status response');
    }

    return response.data;
  }

  /**
   * Scan library and wait for completion (polls until done)
   */
  async scanLibraryAndWait(
    onProgress?: (progress: number, message: string) => void
  ): Promise<ScanResponse> {
    // Start the scan
    await this.scanLibrary();

    // Poll for completion
    while (true) {
      await new Promise(resolve => setTimeout(resolve, 500)); // Poll every 500ms

      const status = await this.getScanStatus();
      
      if (onProgress) {
        onProgress(status.progress, status.message || '');
      }

      if (status.status === 'complete') {
        if (!status.results) {
          throw new Error('Scan completed but no results returned');
        }
        return status.results;
      }

      if (status.status === 'error') {
        throw new Error(status.message || 'Scan failed');
      }

      if (status.status === 'idle') {
        throw new Error('Scan stopped unexpectedly');
      }
    }
  }
}

export default IPCClient;
