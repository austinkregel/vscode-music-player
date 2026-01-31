// Package ipc handles inter-process communication between the daemon and clients.
package ipc

import (
	"encoding/json"
	"fmt"
)

// CommandType represents the type of command
type CommandType string

const (
	CmdPair          CommandType = "pair"
	CmdPlay          CommandType = "play"
	CmdPause         CommandType = "pause"
	CmdResume        CommandType = "resume"
	CmdStop          CommandType = "stop"
	CmdNext          CommandType = "next"
	CmdPrev          CommandType = "prev"
	CmdQueue         CommandType = "queue"
	CmdSeek          CommandType = "seek"
	CmdVolume        CommandType = "volume"
	CmdStatus        CommandType = "status"
	CmdGetConfig     CommandType = "getConfig"
	CmdSetConfig     CommandType = "setConfig"
	CmdScanLibrary   CommandType = "scanLibrary"
	CmdGetScanStatus CommandType = "getScanStatus"

	// Queue management commands
	CmdGetQueue     CommandType = "getQueue"
	CmdSetRepeat    CommandType = "setRepeat"
	CmdSetShuffle   CommandType = "setShuffle"
	CmdQueueJump    CommandType = "queueJump"
	CmdQueueRemove  CommandType = "queueRemove"
	CmdQueueMove    CommandType = "queueMove"

	// Audio visualization
	CmdGetAudioData        CommandType = "getAudioData"
	CmdSubscribeAudioData  CommandType = "subscribeAudioData"
	CmdUnsubscribeAudioData CommandType = "unsubscribeAudioData"
)

// PushMessage represents a server-initiated message (no request needed)
type PushMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Request represents a client request
type Request struct {
	Cmd   CommandType     `json:"cmd"`
	Token string          `json:"token,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Response represents a server response
type Response struct {
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// PairRequest is the data for a pair command
type PairRequest struct {
	ClientName string `json:"clientName"`
}

// PairResponse is the response to a pair command
type PairResponse struct {
	Token      string `json:"token"`
	ClientID   string `json:"clientId"`
	RequiresApproval bool `json:"requiresApproval"`
}

// PlayRequest is the data for a play command
type PlayRequest struct {
	Path     string         `json:"path"`
	Metadata *TrackMetadata `json:"metadata,omitempty"`
}

// TrackMetadata contains track metadata for display
type TrackMetadata struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int64  `json:"duration,omitempty"` // milliseconds
	ArtPath  string `json:"artPath,omitempty"`
}

// QueueRequest is the data for a queue command
// QueueItem represents an item in the queue request
type QueueItem struct {
	Path     string         `json:"path"`
	Metadata *TrackMetadata `json:"metadata,omitempty"`
}

type QueueRequest struct {
	Items  []QueueItem `json:"items"`
	Append bool        `json:"append"`
}

// SeekRequest is the data for a seek command
type SeekRequest struct {
	Position int64 `json:"position"` // milliseconds
}

// VolumeRequest is the data for a volume command
type VolumeRequest struct {
	Level float64 `json:"level"` // 0.0 - 1.0
}

// ConfigRequest is the data for a setConfig command
type ConfigRequest struct {
	LibraryPaths     *[]string `json:"libraryPaths,omitempty"`
	SampleRate       *int      `json:"sampleRate,omitempty"`
	BufferSizeMs     *int      `json:"bufferSizeMs,omitempty"`
	DefaultVolume    *float64  `json:"defaultVolume,omitempty"`
	ResumeOnStart    *bool     `json:"resumeOnStart,omitempty"`
	RememberQueue    *bool     `json:"rememberQueue,omitempty"`
	RememberPosition *bool     `json:"rememberPosition,omitempty"`
}

// ConfigResponse is the response to a getConfig command
type ConfigResponse struct {
	ConfigPath       string   `json:"configPath"`
	LibraryPaths     []string `json:"libraryPaths"`
	SampleRate       int      `json:"sampleRate"`
	BufferSizeMs     int      `json:"bufferSizeMs"`
	DefaultVolume    float64  `json:"defaultVolume"`
	ResumeOnStart    bool     `json:"resumeOnStart"`
	RememberQueue    bool     `json:"rememberQueue"`
	RememberPosition bool     `json:"rememberPosition"`
}

// ScanFileMetadata contains extracted metadata for a scanned file
type ScanFileMetadata struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int64  `json:"duration,omitempty"` // milliseconds
}

// ScanFileInfo represents a scanned audio file
type ScanFileInfo struct {
	Path       string            `json:"path"`
	Size       int64             `json:"size"`
	ModifiedAt int64             `json:"modifiedAt"`
	Metadata   *ScanFileMetadata `json:"metadata,omitempty"`
}

// ScanResult is the result from scanning a library path
type ScanResult struct {
	LibraryPath string         `json:"libraryPath"`
	Files       []ScanFileInfo `json:"files"`
	TotalFiles  int            `json:"totalFiles"`
	ScanTimeMs  int64          `json:"scanTimeMs"`
	Error       string         `json:"error,omitempty"`
}

// ScanResponse is the response to a scanLibrary command
type ScanResponse struct {
	Results    []ScanResult    `json:"results"`
	TotalFiles int             `json:"totalFiles"`
	Metadata   *ScanMetadata   `json:"metadata,omitempty"`
}

// ScanStatusResponse is the response to getScanStatus command
type ScanStatusResponse struct {
	Status     string          `json:"status"` // "idle", "scanning", "complete", "error"
	Progress   int             `json:"progress"` // 0-100
	Message    string          `json:"message,omitempty"`
	Results    *ScanResponse   `json:"results,omitempty"` // Only set when status is "complete"
}

// ScanMetadata contains pre-processed metadata from NFO files
type ScanMetadata struct {
	Artists []ArtistNFO          `json:"artists"`
	Albums  []AlbumNFO           `json:"albums"`
	Artwork map[string][]string  `json:"artwork"`
}

// ArtistNFO represents metadata from an artist.nfo file
type ArtistNFO struct {
	Name          string   `json:"name"`
	SortName      string   `json:"sortName,omitempty"`
	MusicBrainzID string   `json:"musicBrainzId,omitempty"`
	Rating        float64  `json:"rating,omitempty"`
	Biography     string   `json:"biography,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Styles        []string `json:"styles,omitempty"`
	Path          string   `json:"path"`
}

// AlbumNFO represents metadata from an album.nfo file
type AlbumNFO struct {
	Title             string   `json:"title"`
	Artist            string   `json:"artist,omitempty"`
	MusicBrainzAlbumID string  `json:"musicBrainzAlbumId,omitempty"`
	Year              int      `json:"year,omitempty"`
	Rating            float64  `json:"rating,omitempty"`
	Genres            []string `json:"genres,omitempty"`
	Label             string   `json:"label,omitempty"`
	Path              string   `json:"path"`
	AlbumPath         string   `json:"albumPath"`
}

// StatusResponse is the response to a status command
type StatusResponse struct {
	State      string         `json:"state"`
	Path       string         `json:"path,omitempty"`
	Position   int64          `json:"position"`
	Duration   int64          `json:"duration"`
	Volume     float64        `json:"volume"`
	Metadata   *TrackMetadata `json:"metadata,omitempty"`
	QueueIndex int            `json:"queueIndex"`
	QueueSize  int            `json:"queueSize"`
	RepeatMode string         `json:"repeatMode"` // "off", "one", "all"
	Shuffle    bool           `json:"shuffle"`
}

// GetQueueResponse is the response to a getQueue command
type GetQueueResponse struct {
	Items      []QueueItem `json:"items"`
	Index      int         `json:"index"`
	RepeatMode string      `json:"repeatMode"`
	Shuffle    bool        `json:"shuffle"`
}

// SetRepeatRequest is the data for a setRepeat command
type SetRepeatRequest struct {
	Mode string `json:"mode"` // "off", "one", "all"
}

// SetShuffleRequest is the data for a setShuffle command
type SetShuffleRequest struct {
	Enabled bool `json:"enabled"`
}

// QueueJumpRequest is the data for a queueJump command
type QueueJumpRequest struct {
	Index int `json:"index"`
}

// QueueRemoveRequest is the data for a queueRemove command
type QueueRemoveRequest struct {
	Index int `json:"index"`
}

// QueueMoveRequest is the data for a queueMove command
type QueueMoveRequest struct {
	FromIndex int `json:"fromIndex"`
	ToIndex   int `json:"toIndex"`
}

// AudioDataResponse contains real-time frequency data for visualization
type AudioDataResponse struct {
	// Bands contains frequency band magnitudes (0-255), similar to Web Audio API
	// 128 bands, logarithmically distributed from 20Hz to 20kHz
	// Note: Using []int instead of []uint8 because Go's json package base64-encodes []byte/[]uint8
	Bands []int `json:"bands"`
	// Position is the playback position in milliseconds when these samples were analyzed
	// This allows the UI to sync visualization with actual audio playback
	Position int64 `json:"position"`
	// Timestamp is when the audio data was captured (Unix ms)
	Timestamp int64 `json:"timestamp"`
}

// EncodeRequest encodes a request to JSON
func EncodeRequest(req *Request) ([]byte, error) {
	return json.Marshal(req)
}

// DecodeRequest decodes a request from JSON
func DecodeRequest(data []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to decode request: %w", err)
	}
	return &req, nil
}

// EncodeResponse encodes a response to JSON
func EncodeResponse(resp *Response) ([]byte, error) {
	return json.Marshal(resp)
}

// DecodeResponse decodes a response from JSON
func DecodeResponse(data []byte) (*Response, error) {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &resp, nil
}

// NewSuccessResponse creates a successful response
func NewSuccessResponse(data interface{}) (*Response, error) {
	var rawData json.RawMessage
	if data != nil {
		var err error
		rawData, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}
	return &Response{
		Success: true,
		Data:    rawData,
	}, nil
}

// NewErrorResponse creates an error response
func NewErrorResponse(err string) *Response {
	return &Response{
		Success: false,
		Error:   err,
	}
}

// NewPushMessage creates a push message for streaming data
func NewPushMessage(msgType string, data interface{}) ([]byte, error) {
	var rawData json.RawMessage
	if data != nil {
		var err error
		rawData, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}
	msg := PushMessage{
		Type: msgType,
		Data: rawData,
	}
	return json.Marshal(msg)
}
