# Local Media Player

A VS Code extension for playing local music files with native OS media integration, powered by a headless Go daemon.

## Features

- **Native Audio Playback** - High-quality audio playback using FFmpeg, supporting all common formats (MP3, FLAC, OGG, WAV, AAC, and more)
- **OS Media Integration** - Control playback from your OS media controls:
  - Linux: MPRIS D-Bus integration (works with GNOME, KDE, etc.)
  - macOS: Now Playing integration
  - Windows: System Media Transport Controls
- **Library Management** - Scan and browse your music library by artists, albums, or tracks
- **Queue & Playlists** - Full queue management with shuffle and repeat modes, plus persistent playlists
- **Status Bar Controls** - Quick access to playback controls and now-playing info directly in VS Code
- **Metadata Support** - Reads tags from audio files and NFO metadata files (artist.nfo, album.nfo)

## Architecture

The extension consists of two components:

1. **VS Code Extension** (TypeScript) - Provides the UI, library browsing, and playlist management
2. **musicd Daemon** (Go) - Handles audio playback, OS media session integration, and IPC communication

Communication between the extension and daemon uses a Unix socket (or named pipe on Windows) with a JSON-based protocol.

## Requirements

- **FFmpeg** - Required for audio decoding. Install via your package manager:
  ```bash
  # Ubuntu/Debian
  sudo apt install ffmpeg

  # macOS
  brew install ffmpeg

  # Windows (via Chocolatey)
  choco install ffmpeg
  ```

- **Linux only**: ALSA development libraries for building the daemon:
  ```bash
  sudo apt install libasound2-dev
  ```

## Installation

### From VS Code Marketplace

Search for "Local Media Player" in the VS Code Extensions view.

### From Source

```bash
# Clone the repository
git clone https://github.com/austinkregel/vscode-music-player
cd vscode-music-player

# Install dependencies
make deps

# Build everything (daemon + extension)
make build

# Or build just for your current platform
make build-daemon-local
make build-extension
```

## Getting Started

1. Open VS Code and access the Music panel in the Activity Bar
2. Click "Set Library Folder" to configure your music library path
3. Click "Scan Library" to index your music files
4. Browse your library and double-click a track to start playback

The daemon starts automatically when you first play a track. A pairing prompt may appear for security authorization.

## Extension Settings

| Setting | Description | Default |
|---------|-------------|---------|
| `localMedia.libraryPath` | Path to your music library (e.g., NFS mount) | `""` |
| `localMedia.autoStartDaemon` | Automatically start the media daemon when connecting | `true` |
| `localMedia.daemonPath` | Custom path to the musicd daemon binary (leave empty to use bundled) | `""` |

## Commands

All commands are available via the Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`):

| Command | Description |
|---------|-------------|
| `Local Media: Play/Pause` | Toggle playback |
| `Local Media: Next Track` | Skip to next track |
| `Local Media: Previous Track` | Skip to previous track |
| `Local Media: Stop` | Stop playback |
| `Local Media: Set Library Folder` | Configure music library path |
| `Local Media: Scan Music Library` | Re-scan library for new files |
| `Local Media: Connect to Daemon` | Manually connect to the daemon |
| `Local Media: Disconnect from Daemon` | Disconnect from the daemon |
| `Local Media: Create Playlist` | Create a new playlist |

## Daemon Configuration

The musicd daemon stores its configuration in `~/.config/musicd/`. Configuration options include:

- **libraryPaths** - Multiple library paths to scan
- **sampleRate** - Audio output sample rate (default: 44100)
- **bufferSizeMs** - Audio buffer size in milliseconds
- **defaultVolume** - Default volume level (0.0 - 1.0)
- **rememberQueue** - Persist queue across restarts
- **rememberPosition** - Resume playback position on restart

## Building for Different Platforms

The project supports cross-compilation for multiple platforms:

```bash
# Build for all platforms (requires native toolchains)
make build-daemon

# Build Linux binaries using Docker (cross-platform friendly)
make build-daemon-docker-all

# Package the extension
make package
```

### Supported Platforms

| Platform | Architecture | Notes |
|----------|--------------|-------|
| Linux | amd64, arm64 | Requires ALSA |
| macOS | amd64 (Intel), arm64 (Apple Silicon) | Uses Core Audio |
| Windows | amd64 | Uses Windows Audio Session API |

## Development

```bash
# Setup development environment
make setup

# Run daemon in test mode (auto-approves pairing)
make dev-daemon

# Watch mode for extension development
make watch

# Run all tests
make test

# Lint code
make lint
```

## Security

The daemon uses a token-based authentication system:

1. When a client first connects, it must pair with the daemon
2. The user approves the pairing request via a system notification
3. A token is issued and stored securely in VS Code's secret storage
4. Subsequent connections use the stored token for authentication

In test mode (`-test-mode` flag), pairing is auto-approved for development purposes.

## Troubleshooting

### Daemon won't start

- Ensure FFmpeg is installed and available in PATH
- Check the "Local Media" output channel in VS Code for error messages
- Try running the daemon manually: `./bin/musicd -verbose`

### No audio output

- Check your system's audio output settings
- Ensure the audio file format is supported by FFmpeg
- On Linux, verify ALSA is configured correctly

### Media controls not working

- Linux: Ensure D-Bus is running and MPRIS is supported by your desktop environment
- macOS: Grant accessibility permissions if prompted
- Windows: Check that no other application is blocking media key access

## License

MIT

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.
