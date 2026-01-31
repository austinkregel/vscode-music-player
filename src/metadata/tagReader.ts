/**
 * Tag reader using music-metadata
 * Extracts ID3, Vorbis, and other metadata from audio files
 */

import type { TrackInput } from './models';

// We'll use dynamic import for music-metadata due to ESM compatibility
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mm: any = null;

async function getMusicMetadata(): Promise<any> {
  if (!mm) {
    mm = await import('music-metadata');
  }
  return mm;
}

/**
 * Supported audio file extensions
 */
export const SUPPORTED_EXTENSIONS = ['.mp3', '.flac', '.m4a', '.aac', '.ogg', '.opus', '.wav', '.wma'];

/**
 * Check if a file extension is a supported audio format
 */
export function isSupportedAudioFile(filePath: string): boolean {
  const ext = filePath.toLowerCase().substring(filePath.lastIndexOf('.'));
  return SUPPORTED_EXTENSIONS.includes(ext);
}

/**
 * Read metadata from an audio file
 */
export async function readTags(filePath: string): Promise<TrackInput> {
  const musicMetadata = await getMusicMetadata();
  const metadata = await musicMetadata.parseFile(filePath);
  const { common, format } = metadata;

  // Calculate duration in milliseconds
  const durationMs = format.duration ? Math.round(format.duration * 1000) : null;

  return {
    path: filePath,
    title: common.title || null,
    artist: common.artist || null,
    album: common.album || null,
    albumArtist: common.albumartist || null,
    genre: common.genre?.[0] || null,
    year: common.year || null,
    trackNumber: common.track?.no || null,
    discNumber: common.disk?.no || null,
    durationMs,
  };
}

/**
 * Extract cover art from an audio file
 * Returns the picture data if available
 */
export async function extractCoverArt(filePath: string): Promise<{
  data: Buffer;
  format: string;
} | null> {
  const musicMetadata = await getMusicMetadata();
  const metadata = await musicMetadata.parseFile(filePath);
  const picture = metadata.common.picture?.[0];

  if (!picture) {
    return null;
  }

  return {
    data: Buffer.from(picture.data),
    format: picture.format,
  };
}

/**
 * Get duration of an audio file in milliseconds
 */
export async function getDuration(filePath: string): Promise<number | null> {
  try {
    const musicMetadata = await getMusicMetadata();
    const metadata = await musicMetadata.parseFile(filePath);
    return metadata.format.duration ? Math.round(metadata.format.duration * 1000) : null;
  } catch {
    return null;
  }
}
