/**
 * Shared TypeScript types for the local-media extension
 */

// ============================================================================
// IPC Protocol Types
// ============================================================================

export type CommandType =
  | 'pair'
  | 'play'
  | 'pause'
  | 'resume'
  | 'stop'
  | 'next'
  | 'prev'
  | 'queue'
  | 'seek'
  | 'volume'
  | 'status'
  | 'getConfig'
  | 'setConfig'
  | 'scanLibrary'
  | 'getScanStatus'
  | 'getAudioData'
  | 'subscribeAudioData'
  | 'unsubscribeAudioData';

export interface Request {
  cmd: CommandType;
  token?: string;
  data?: unknown;
}

export interface Response<T = unknown> {
  success: boolean;
  error?: string;
  data?: T;
}

// Push message from server (no request needed)
export interface PushMessage<T = unknown> {
  type: string;
  data?: T;
}

export interface PairRequest {
  clientName: string;
}

export interface PairResponse {
  token: string;
  clientId: string;
  requiresApproval: boolean;
}

export interface PlayRequest {
  path: string;
  metadata?: TrackMetadata;
}

export interface QueueItem {
  path: string;
  metadata?: TrackMetadata;
}

export interface QueueRequest {
  items: QueueItem[];
  append: boolean;
}

export interface SeekRequest {
  position: number; // milliseconds
}

export interface VolumeRequest {
  level: number; // 0.0 - 1.0
}

export interface StatusResponse {
  state: PlaybackState;
  path?: string;
  position: number;
  duration: number;
  volume: number;
  metadata?: TrackMetadata;
  queueIndex: number;
  queueSize: number;
}

/**
 * Real-time audio frequency data for visualization
 */
export interface AudioDataResponse {
  /** Frequency band magnitudes (0-255), 128 bands logarithmically distributed 20Hz-20kHz */
  bands: number[];
  /** Playback position in milliseconds when these samples were analyzed */
  position: number;
  /** Unix timestamp (ms) when the audio data was captured */
  timestamp: number;
}

// ============================================================================
// Playback Types
// ============================================================================

export type PlaybackState = 'stopped' | 'playing' | 'paused';

export interface TrackMetadata {
  title?: string;
  artist?: string;
  album?: string;
  duration?: number; // milliseconds
  artPath?: string;
}

// ============================================================================
// Library Types
// ============================================================================

export interface Track {
  id: string;
  path: string;
  title?: string;
  artist?: string;
  album?: string;
  albumArtist?: string;
  genre?: string;
  year?: number;
  trackNumber?: number;
  discNumber?: number;
  durationMs?: number;
  coverArtPath?: string;
  fileModifiedAt?: number;
  scannedAt?: number;
}

export interface Album {
  id: string;
  name: string;
  artist?: string;
  year?: number;
  coverArtPath?: string;
}

export interface Artist {
  id: string;
  name: string;
}

// ============================================================================
// Playlist Types
// ============================================================================

export interface Playlist {
  id: string;
  name: string;
  createdAt: number;
  updatedAt: number;
}

export interface PlaylistTrack {
  playlistId: string;
  trackId: string;
  position: number;
}

// ============================================================================
// Queue Types
// ============================================================================

export interface QueueItem {
  path: string;
  metadata?: TrackMetadata;
}

export type RepeatMode = 'off' | 'one' | 'all';

export interface QueueState {
  items: QueueItem[];
  currentIndex: number;
  shuffle: boolean;
  repeat: RepeatMode;
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface ExtensionConfig {
  libraryPath: string;
  daemonPath?: string;
  autoStartDaemon: boolean;
}

// ============================================================================
// Daemon Configuration Types
// ============================================================================

export interface ConfigRequest {
  libraryPaths?: string[];
  sampleRate?: number;
  bufferSizeMs?: number;
  defaultVolume?: number;
  resumeOnStart?: boolean;
  rememberQueue?: boolean;
  rememberPosition?: boolean;
}

export interface ConfigResponse {
  configPath: string;
  libraryPaths: string[];
  sampleRate: number;
  bufferSizeMs: number;
  defaultVolume: number;
  resumeOnStart: boolean;
  rememberQueue: boolean;
  rememberPosition: boolean;
}

// ============================================================================
// Library Scan Types
// ============================================================================

export interface ScanFileInfo {
  path: string;
  size: number;
  modifiedAt: number;
  metadata?: TrackMetadata;
}

export interface ScanResult {
  libraryPath: string;
  files: ScanFileInfo[];
  totalFiles: number;
  scanTimeMs: number;
  error?: string;
}

export interface ScanResponse {
  results: ScanResult[];
  totalFiles: number;
  metadata?: ScanMetadata;
}

export interface ScanStatusResponse {
  status: 'idle' | 'scanning' | 'complete' | 'error';
  progress: number;
  message?: string;
  results?: ScanResponse;
}

// ============================================================================
// NFO Metadata Types (pre-processed library metadata)
// ============================================================================

export interface ScanMetadata {
  artists: ArtistNFO[];
  albums: AlbumNFO[];
  artwork: Record<string, string[]>;
}

export interface ArtistNFO {
  name: string;
  sortName?: string;
  musicBrainzId?: string;
  rating?: number;
  biography?: string;
  genres?: string[];
  styles?: string[];
  path: string;
}

export interface AlbumNFO {
  title: string;
  artist?: string;
  musicBrainzAlbumId?: string;
  year?: number;
  rating?: number;
  genres?: string[];
  label?: string;
  path: string;
  albumPath: string;
}
