// Package types provides shared type definitions used across the musicd daemon.
package types

// TrackMetadata contains metadata for a track
type TrackMetadata struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int64  `json:"duration,omitempty"` // milliseconds
	ArtPath  string `json:"artPath,omitempty"`
}

// QueueItem represents an item in the playback queue
type QueueItem struct {
	Path     string         `json:"path"`
	Metadata *TrackMetadata `json:"metadata,omitempty"`
}

// RepeatMode represents the repeat behavior
type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatOne
	RepeatAll
)

// String returns the string representation of the repeat mode
func (r RepeatMode) String() string {
	switch r {
	case RepeatOne:
		return "one"
	case RepeatAll:
		return "all"
	default:
		return "off"
	}
}

// ParseRepeatMode parses a string into a RepeatMode
func ParseRepeatMode(s string) RepeatMode {
	switch s {
	case "one":
		return RepeatOne
	case "all":
		return RepeatAll
	default:
		return RepeatOff
	}
}
