//go:build windows

package media

import (
	"log"
	"time"
)

// WindowsSession implements Windows System Media Transport Controls
// Full SMTC integration requires either:
// 1. Using go-ole to interact with WinRT APIs (github.com/go-ole/go-ole)
// 2. A C++ helper library with cgo for WinRT SystemMediaTransportControls
// 3. Direct syscalls to Windows APIs
//
// For now, this is a logging stub that shows what would be displayed.
// To implement fully:
// - Use go-ole to initialize COM and access WinRT APIs
// - Create SystemMediaTransportControls instance
// - Set DisplayUpdater with metadata
// - Handle ButtonPressed events for play/pause/next/previous
type WindowsSession struct {
	handler  CommandHandler
	metadata Metadata
	state    PlaybackState
	position time.Duration
}

// NewSession creates a new Windows media session
func NewSession() (Session, error) {
	log.Printf("[MEDIA-WIN] Windows SMTC session created (stub mode)")
	log.Printf("[MEDIA-WIN] To enable full SMTC, implement WinRT integration via go-ole")
	return &WindowsSession{
		state: StateStopped,
	}, nil
}

// UpdateMetadata updates the media metadata
func (s *WindowsSession) UpdateMetadata(metadata Metadata) error {
	s.metadata = metadata
	log.Printf("[MEDIA-WIN] SMTC metadata updated: %s - %s (%s)",
		metadata.Artist, metadata.Title, metadata.Album)
	// In a full implementation:
	// 1. Get SystemMediaTransportControls.DisplayUpdater
	// 2. Set Type to MediaPlaybackType.Music
	// 3. Set MusicProperties (Title, Artist, AlbumTitle)
	// 4. If metadata.ArtPath is set, create a thumbnail from the file
	// 5. Call Update() on DisplayUpdater
	return nil
}

// UpdatePlaybackState updates the playback state
func (s *WindowsSession) UpdatePlaybackState(state PlaybackState, position time.Duration) error {
	s.state = state
	s.position = position

	stateStr := "stopped"
	switch state {
	case StatePlaying:
		stateStr = "playing"
	case StatePaused:
		stateStr = "paused"
	}

	log.Printf("[MEDIA-WIN] SMTC state updated: %s at %v", stateStr, position)
	// In a full implementation:
	// 1. Set PlaybackStatus on SystemMediaTransportControls
	// 2. Use TimelineProperties for position/duration
	return nil
}

// UpdateShuffle updates the shuffle state
func (s *WindowsSession) UpdateShuffle(enabled bool) error {
	log.Printf("[MEDIA-WIN] SMTC shuffle: %v", enabled)
	// In a full implementation:
	// 1. Set IsShuffleEnabled on SystemMediaTransportControls
	// 2. Handle ShuffleEnabledChangeRequested event
	return nil
}

// UpdateLoopStatus updates the loop/repeat mode
func (s *WindowsSession) UpdateLoopStatus(status LoopStatus) error {
	log.Printf("[MEDIA-WIN] SMTC loop status: %s", status)
	// In a full implementation:
	// 1. Map LoopStatus to AutoRepeatMode enum (None, Track, List)
	// 2. Set AutoRepeatMode on SystemMediaTransportControls
	// 3. Handle AutoRepeatModeChangeRequested event
	return nil
}

// SetCommandHandler sets the handler for media commands
func (s *WindowsSession) SetCommandHandler(handler CommandHandler) {
	s.handler = handler
	log.Printf("[MEDIA-WIN] Command handler registered")
	// In a full implementation:
	// 1. Register for ButtonPressed event on SystemMediaTransportControls
	// 2. Map button types to media commands
	// 3. Call handler.OnCommand() for each button press
}

// Close releases resources
func (s *WindowsSession) Close() error {
	log.Printf("[MEDIA-WIN] SMTC session closed")
	return nil
}
