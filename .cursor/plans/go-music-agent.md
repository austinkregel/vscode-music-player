# VS Code Native Music Player – Architecture & Design

## Overview

This project implements a **secure, native-integrated music playback system** controlled from VS Code but executed by a **lightweight headless Go daemon**. The daemon is responsible for audio playback and OS-level media integration, while the VS Code extension provides UI, library browsing, and workflow-friendly controls.

Key goals:

* Native OS media controls (media keys, lock screen, system overlays)
* Secure, explicit client authorization
* Simple, low-overhead daemon
* Tight VS Code integration (status bar, commands)
* Local playback from an NFS-mounted music library (Plex as organizer, not player)

---

## High-Level Architecture

```
VS Code Extension (UI / Commands)
        │
        │  IPC (local, authenticated)
        ▼
Go Media Daemon (headless, autostart)
        │
        ├─ Audio decoding & playback
        ├─ OS native media session APIs
        ├─ Playback state & queue
        └─ Secure client authorization
        │
        ▼
NFS-mounted Music Library (MP3 / M4A / FLAC)
```

### Responsibilities

**VS Code Extension**

* Library browsing & search
* Playlist / queue management
* Status bar "Now Playing"
* Command palette & keybindings
* Pairing UX and token storage

**Go Daemon**

* Audio playback
* Media session integration (OS-level)
* Playlist execution
* IPC server
* Client authentication & authorization

---

## Audio Playback

### Formats

* MP3
* M4A (AAC; may require ffmpeg)
* FLAC

### Go Libraries (recommended)

* Metadata: `github.com/dhowden/tag`
* MP3: `github.com/hajimehoshi/go-mp3`
* FLAC: `github.com/mewkiz/flac`
* Output: `github.com/hajimehoshi/oto`

> Note: AAC/M4A decoding is the hardest part; ffmpeg can be an optional dependency if required.

---

## OS Media Integration

The daemon integrates directly with native OS media frameworks.

### Windows

* Windows Media Session API
* Media keys
* Lock screen / system overlay

### macOS

* MPNowPlayingInfoCenter
* Remote command events
* Requires small Objective-C bridge via cgo

### Linux

* MPRIS over DBus
* Widely supported and straightforward

---

## IPC Design

### Transport (Local Only)

* **Preferred:** Unix domain socket / Windows named pipe
* User-only permissions
* No TCP ports exposed

### Message Style

* JSON messages
* Stateless commands
* Stateful responses

Example:

```json
{ "cmd": "play", "trackId": "abc123" }
```

---

## Security Model

### Threat Model

Defends against:

* Unauthorized local processes
* Accidental or malicious localhost access
* Unapproved clients

Does not attempt to defend against:

* Root/admin compromise
* Full user account compromise

---

## Client Authorization & Pairing

### Core Principles

1. No unauthenticated control
2. Explicit user approval
3. Per-client identity
4. Capability-based permissions
5. Revocable access

### Pairing Flow

1. Client connects and sends a pairing request
2. Daemon prompts user for approval (OS notification / tray / CLI)
3. Daemon issues a random capability token
4. Client stores token securely (VS Code SecretStorage)
5. All future commands require the token

### Token Handling

* 256-bit random tokens
* Stored hashed on disk by daemon
* Capabilities attached (e.g. playback, read-only)
* Revocable and rotatable

---

## Hardening Measures

* IPC restricted to local user
* Token required for all commands
* Rate limiting on auth failures
* Path validation (stay within music root)
* Optional UID verification on Unix sockets

---

## VS Code Status Bar Integration

The VS Code extension displays a live "Now Playing" indicator.

### Features

* Track title and artist
* Play / pause icon
* Clickable controls
* Opens full player UI on click

Example:

```
▶ Radiohead – Everything In Its Right Place
```

### Behavior

* Updates on playback state changes
* Truncates long titles gracefully
* Hidden when daemon is unavailable

---

## Startup & Lifecycle

### Daemon

* Autostarts on user login
* Idle when paused
* Runs independently of VS Code

### VS Code Extension

* Connects to daemon on activation
* Prompts to pair if untrusted
* Does not block editor startup

---

## Plex Relationship

* Plex acts as organizer and metadata source
* Playback is local via NFS-mounted library
* Optional future integration:

  * Read Plex playlists
  * Sync play counts / ratings

---

## Design Philosophy

* Keep the daemon small and boring
* Push UI and workflow logic into VS Code
* Prefer explicit security over convenience
* Avoid cross-platform hacks where possible

This separation provides a robust, native-feeling experience without overloading either side of the system.
