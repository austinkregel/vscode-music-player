// Package media provides OS-level media session integration.
package media

import (
	"time"
)

// PlaybackState represents the playback state for media sessions
type PlaybackState int

const (
	StateStopped PlaybackState = iota
	StatePlaying
	StatePaused
)

// Metadata contains track metadata for media session display
type Metadata struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	ArtPath  string
}

// LoopStatus represents the loop/repeat mode for MPRIS
type LoopStatus string

const (
	LoopNone     LoopStatus = "None"
	LoopTrack    LoopStatus = "Track"
	LoopPlaylist LoopStatus = "Playlist"
)

// Session is the interface for OS media session integration
type Session interface {
	// UpdateMetadata updates the currently playing track metadata
	UpdateMetadata(metadata Metadata) error

	// UpdatePlaybackState updates the playback state and position
	UpdatePlaybackState(state PlaybackState, position time.Duration) error

	// UpdateShuffle updates the shuffle state
	UpdateShuffle(enabled bool) error

	// UpdateLoopStatus updates the repeat/loop mode
	UpdateLoopStatus(status LoopStatus) error

	// SetCommandHandler sets the handler for media commands (play, pause, etc.)
	SetCommandHandler(handler CommandHandler)

	// Close releases resources
	Close() error
}

// Command represents a media command from the OS
type Command int

const (
	CmdPlay Command = iota
	CmdPause
	CmdPlayPause
	CmdStop
	CmdNext
	CmdPrevious
	CmdSeek
	CmdSetShuffle
	CmdSetLoopStatus
)

// String returns the command name
func (c Command) String() string {
	switch c {
	case CmdPlay:
		return "Play"
	case CmdPause:
		return "Pause"
	case CmdPlayPause:
		return "PlayPause"
	case CmdStop:
		return "Stop"
	case CmdNext:
		return "Next"
	case CmdPrevious:
		return "Previous"
	case CmdSeek:
		return "Seek"
	case CmdSetShuffle:
		return "SetShuffle"
	case CmdSetLoopStatus:
		return "SetLoopStatus"
	default:
		return "Unknown"
	}
}

// CommandHandler handles media commands from the OS
type CommandHandler interface {
	OnCommand(cmd Command, data interface{}) error
}

// CommandHandlerFunc is a function adapter for CommandHandler
type CommandHandlerFunc func(cmd Command, data interface{}) error

func (f CommandHandlerFunc) OnCommand(cmd Command, data interface{}) error {
	return f(cmd, data)
}

// NoOpSession is a session that does nothing
// Used when media session integration is not available
type NoOpSession struct{}

// NewNoOpSession creates a new no-op session
func NewNoOpSession() *NoOpSession {
	return &NoOpSession{}
}

func (s *NoOpSession) UpdateMetadata(metadata Metadata) error {
	return nil
}

func (s *NoOpSession) UpdatePlaybackState(state PlaybackState, position time.Duration) error {
	return nil
}

func (s *NoOpSession) UpdateShuffle(enabled bool) error {
	return nil
}

func (s *NoOpSession) UpdateLoopStatus(status LoopStatus) error {
	return nil
}

func (s *NoOpSession) SetCommandHandler(handler CommandHandler) {
}

func (s *NoOpSession) Close() error {
	return nil
}
