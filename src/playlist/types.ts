/**
 * Playlist-related types
 */

export interface PlaylistExport {
  name: string;
  tracks: PlaylistTrackExport[];
  createdAt: string;
  exportedAt: string;
}

export interface PlaylistTrackExport {
  path: string;
  title?: string;
  artist?: string;
  album?: string;
}
