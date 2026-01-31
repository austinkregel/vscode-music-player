/**
 * IPC Protocol implementation
 * Handles encoding/decoding of messages between extension and daemon
 */

import type {
  Request,
  Response,
  CommandType,
  PairRequest,
  PairResponse,
  PlayRequest,
  QueueRequest,
  QueueItem,
  SeekRequest,
  VolumeRequest,
  StatusResponse,
  TrackMetadata,
  ConfigResponse,
  ScanResponse,
  ScanStatusResponse,
  AudioDataResponse,
} from '../types';

/**
 * Encode a command into a request object
 */
export function encodeCommand(
  cmd: CommandType,
  data?: unknown,
  token?: string
): string {
  const request: Request = { cmd };

  if (token) {
    request.token = token;
  }

  if (data !== undefined) {
    request.data = data;
  }

  return JSON.stringify(request);
}

/**
 * Decode a response from JSON
 */
export function decodeResponse<T>(data: string): Response<T> {
  try {
    return JSON.parse(data) as Response<T>;
  } catch (err) {
    return {
      success: false,
      error: `Failed to parse response: ${err}`,
    };
  }
}

/**
 * Create a pair request
 */
export function createPairRequest(clientName: string): PairRequest {
  return { clientName };
}

/**
 * Create a play request
 */
export function createPlayRequest(
  path: string,
  metadata?: TrackMetadata
): PlayRequest {
  return { path, metadata };
}

/**
 * Create a queue request
 */
export function createQueueRequest(
  items: QueueItem[],
  append: boolean = false
): QueueRequest {
  return { items, append };
}

/**
 * Create a seek request
 */
export function createSeekRequest(positionMs: number): SeekRequest {
  return { position: positionMs };
}

/**
 * Create a volume request
 */
export function createVolumeRequest(level: number): VolumeRequest {
  if (level < 0 || level > 1) {
    throw new Error('Volume level must be between 0.0 and 1.0');
  }
  return { level };
}

/**
 * Type guard for StatusResponse
 */
export function isStatusResponse(data: unknown): data is StatusResponse {
  if (!data || typeof data !== 'object') {
    return false;
  }

  const obj = data as Record<string, unknown>;
  return (
    typeof obj.state === 'string' &&
    typeof obj.position === 'number' &&
    typeof obj.duration === 'number' &&
    typeof obj.volume === 'number' &&
    typeof obj.queueIndex === 'number' &&
    typeof obj.queueSize === 'number'
  );
}

/**
 * Type guard for AudioDataResponse
 */
export function isAudioDataResponse(data: unknown): data is AudioDataResponse {
  if (!data || typeof data !== 'object') {
    return false;
  }

  const obj = data as Record<string, unknown>;
  return Array.isArray(obj.bands);
}

/**
 * Type guard for PairResponse
 */
export function isPairResponse(data: unknown): data is PairResponse {
  if (!data || typeof data !== 'object') {
    return false;
  }

  const obj = data as Record<string, unknown>;
  return (
    typeof obj.token === 'string' &&
    typeof obj.clientId === 'string' &&
    typeof obj.requiresApproval === 'boolean'
  );
}

/**
 * Type guard for ConfigResponse
 */
export function isConfigResponse(data: unknown): data is ConfigResponse {
  if (!data || typeof data !== 'object') {
    return false;
  }

  const obj = data as Record<string, unknown>;
  return (
    typeof obj.configPath === 'string' &&
    Array.isArray(obj.libraryPaths) &&
    typeof obj.sampleRate === 'number'
  );
}

/**
 * Type guard for ScanResponse
 */
export function isScanResponse(data: unknown): data is ScanResponse {
  if (!data || typeof data !== 'object') {
    return false;
  }

  const obj = data as Record<string, unknown>;
  return (
    Array.isArray(obj.results) &&
    typeof obj.totalFiles === 'number'
  );
}

/**
 * Type guard for ScanStatusResponse
 */
export function isScanStatusResponse(data: unknown): data is ScanStatusResponse {
  if (!data || typeof data !== 'object') {
    return false;
  }

  const obj = data as Record<string, unknown>;
  return (
    typeof obj.status === 'string' &&
    typeof obj.progress === 'number'
  );
}

/**
 * Parse a line-delimited message from the socket
 */
export function parseMessage(buffer: string): { message: string | null; remaining: string } {
  const newlineIndex = buffer.indexOf('\n');
  
  if (newlineIndex === -1) {
    return { message: null, remaining: buffer };
  }

  const message = buffer.substring(0, newlineIndex);
  const remaining = buffer.substring(newlineIndex + 1);

  return { message, remaining };
}
