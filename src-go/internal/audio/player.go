// Package audio handles audio decoding and playback using FFmpeg and Oto.
package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/austinkregel/local-media/musicd/internal/media"
)

// FindAlbumArt looks for album art in the track's directory or parent directory.
// It checks for common art filenames: folder.jpg, cover.jpg, album.jpg, etc.
// Returns the path to the art file if found, or empty string if not found.
func FindAlbumArt(trackPath string) string {
	if trackPath == "" {
		return ""
	}

	dir := filepath.Dir(trackPath)

	// Common album art filenames to check
	artFilenames := []string{
		"folder.jpg", "folder.png",
		"cover.jpg", "cover.png",
		"album.jpg", "album.png",
		"front.jpg", "front.png",
		"Folder.jpg", "Folder.png",
		"Cover.jpg", "Cover.png",
	}

	// Check current directory (album folder)
	for _, name := range artFilenames {
		artPath := filepath.Join(dir, name)
		if _, err := os.Stat(artPath); err == nil {
			return artPath
		}
	}

	// Check parent directory (artist folder) for folder.jpg
	parentDir := filepath.Dir(dir)
	for _, name := range []string{"folder.jpg", "folder.png", "Folder.jpg", "Folder.png"} {
		artPath := filepath.Join(parentDir, name)
		if _, err := os.Stat(artPath); err == nil {
			return artPath
		}
	}

	return ""
}

// PlaybackState represents the current state of the player
type PlaybackState string

const (
	StateStopped PlaybackState = "stopped"
	StatePlaying PlaybackState = "playing"
	StatePaused  PlaybackState = "paused"
)

// TrackMetadata contains metadata to display in OS media sessions
type TrackMetadata struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int64  `json:"duration,omitempty"` // milliseconds
	ArtPath  string `json:"artPath,omitempty"`
}

// Status represents the current playback status
type Status struct {
	State      PlaybackState  `json:"state"`
	Path       string         `json:"path,omitempty"`
	Position   int64          `json:"position"`   // milliseconds
	Duration   int64          `json:"duration"`   // milliseconds
	Volume     float64        `json:"volume"`     // 0.0 - 1.0
	Metadata   *TrackMetadata `json:"metadata,omitempty"`
}

// TrackEndCallback is called when a track finishes playing naturally
type TrackEndCallback func(path string)

// QueueCallback is called for next/previous track requests (from OS media controls)
type QueueCallback func()

// ShuffleCallback is called when shuffle state changes from OS media controls
type ShuffleCallback func(enabled bool)

// LoopCallback is called when loop/repeat mode changes from OS media controls
type LoopCallback func(status media.LoopStatus)

// Player handles audio playback
type Player struct {
	mu           sync.RWMutex
	playbackMu   sync.Mutex    // Serializes all play/stop operations - ensures single song at a time
	state        PlaybackState
	currentPath  string
	position     int64
	duration     int64
	volume       float64
	metadata     *TrackMetadata
	mediaSession media.Session

	// Session tracking - ensures only one playback at a time
	sessionID    uint64        // Incremented on each new playback
	sessionDone  chan struct{} // Closed when current session ends

	// Playback control
	stopChan     chan struct{}
	pauseChan    chan struct{}
	resumeChan   chan struct{}
	cancelFunc   context.CancelFunc // Cancel function for current playback
	wasManualStop bool              // True if playback was stopped manually (not track end)

	// Callbacks
	onTrackEnd TrackEndCallback
	onNext     QueueCallback
	onPrevious QueueCallback
	onShuffle  ShuffleCallback
	onLoop     LoopCallback

	// Audio output
	output Output

	// Decoder
	decoder Decoder
}

// Output is the interface for audio output backends
type Output interface {
	io.WriteCloser
	SampleRate() int
	Channels() int
}

// Decoder is the interface for audio decoders
type Decoder interface {
	Decode(ctx context.Context, path string, output Output) error
	Duration(path string) (time.Duration, error)
	Close() error
}

// NewPlayer creates a new audio player
func NewPlayer(mediaSession media.Session) (*Player, error) {
	output, err := NewOtoOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create audio output: %w", err)
	}

	decoder, err := NewFFmpegDecoder()
	if err != nil {
		output.Close()
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	return &Player{
		state:        StateStopped,
		volume:       1.0,
		mediaSession: mediaSession,
		output:       output,
		decoder:      decoder,
		stopChan:     make(chan struct{}),
		pauseChan:    make(chan struct{}),
		resumeChan:   make(chan struct{}),
	}, nil
}

// SetOnTrackEnd sets a callback to be called when a track finishes playing naturally
func (p *Player) SetOnTrackEnd(callback TrackEndCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onTrackEnd = callback
}

// SetOnNext sets a callback for next track requests (from OS media controls)
func (p *Player) SetOnNext(callback QueueCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onNext = callback
}

// SetOnPrevious sets a callback for previous track requests (from OS media controls)
func (p *Player) SetOnPrevious(callback QueueCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPrevious = callback
}

// SetOnShuffle sets a callback for shuffle toggle requests (from OS media controls)
func (p *Player) SetOnShuffle(callback ShuffleCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onShuffle = callback
}

// SetOnLoop sets a callback for loop/repeat mode changes (from OS media controls)
func (p *Player) SetOnLoop(callback LoopCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onLoop = callback
}

// Play starts playback of the specified file
func (p *Player) Play(ctx context.Context, path string, metadata *TrackMetadata) error {
	// Serialize all play operations - only one Play() can run at a time
	p.playbackMu.Lock()
	defer p.playbackMu.Unlock()

	p.mu.Lock()

	// Stop any current playback and WAIT for it to finish
	if p.state == StatePlaying || p.state == StatePaused {
		p.stopPlaybackLocked()
		oldDone := p.sessionDone
		p.mu.Unlock()

		// Wait for old playback goroutine to fully exit
		if oldDone != nil {
			<-oldDone
		}

		p.mu.Lock()
	}

	// Create new session
	p.sessionID++
	p.sessionDone = make(chan struct{})
	currentSession := p.sessionID
	doneChan := p.sessionDone

	p.currentPath = path
	p.position = 0
	p.state = StatePlaying
	p.metadata = metadata
	p.wasManualStop = false // Reset - this playback wasn't manually stopped

	// Get duration first (quick ffprobe call)
	var duration time.Duration
	if metadata != nil && metadata.Duration > 0 {
		duration = time.Duration(metadata.Duration) * time.Millisecond
	} else {
		var err error
		duration, err = p.decoder.Duration(path)
		if err != nil {
			p.mu.Unlock()
			return fmt.Errorf("failed to get duration: %w", err)
		}
	}
	p.duration = duration.Milliseconds()

	// Extract full metadata asynchronously if not provided
	if metadata == nil || (metadata.Title == "" && metadata.Artist == "") {
		go func(playerPath string, sessID uint64) {
			if ffmpegDecoder, ok := p.decoder.(*FFmpegDecoder); ok {
				if fileMeta, err := ffmpegDecoder.Metadata(playerPath); err == nil {
					log.Printf("[PLAYER] Extracted metadata: %s - %s (%s)", fileMeta.Artist, fileMeta.Title, fileMeta.Album)

					// Find album art
					artPath := FindAlbumArt(playerPath)
					if artPath != "" {
						log.Printf("[PLAYER] Found album art: %s", artPath)
					}

					p.mu.Lock()
					// Only update if we're still playing the same file AND same session
					if p.currentPath == playerPath && p.sessionID == sessID {
						p.metadata = &TrackMetadata{
							Title:    fileMeta.Title,
							Artist:   fileMeta.Artist,
							Album:    fileMeta.Album,
							Duration: fileMeta.Duration.Milliseconds(),
							ArtPath:  artPath,
						}
						// Update media session with new metadata
						if p.mediaSession != nil {
							p.mediaSession.UpdateMetadata(media.Metadata{
								Title:    fileMeta.Title,
								Artist:   fileMeta.Artist,
								Album:    fileMeta.Album,
								Duration: fileMeta.Duration,
								ArtPath:  artPath,
							})
						}
					}
					p.mu.Unlock()
				} else {
					log.Printf("[PLAYER] Failed to extract metadata: %v", err)
				}
			}
		}(path, currentSession)
	}

	// Update media session
	if p.mediaSession != nil {
		// Try to find album art if not provided in metadata
		artPath := ""
		if metadata != nil {
			artPath = metadata.ArtPath
		}
		if artPath == "" {
			artPath = FindAlbumArt(path)
			if artPath != "" {
				log.Printf("[PLAYER] Found album art: %s", artPath)
			}
		}

		var title, artist, album string
		if metadata != nil {
			title = metadata.Title
			artist = metadata.Artist
			album = metadata.Album
		}

		p.mediaSession.UpdateMetadata(media.Metadata{
			Title:    title,
			Artist:   artist,
			Album:    album,
			Duration: duration,
			ArtPath:  artPath,
		})
		p.mediaSession.UpdatePlaybackState(media.StatePlaying, time.Duration(p.position)*time.Millisecond)
	}

	p.stopChan = make(chan struct{})

	// Create a cancellable context for this playback session
	playbackCtx, cancel := context.WithCancel(context.Background())
	p.cancelFunc = cancel

	p.mu.Unlock()

	// Start decoding in background - goroutine closes doneChan when it exits
	go func() {
		defer close(doneChan)
		p.playbackLoop(playbackCtx, path, currentSession)
	}()

	return nil
}

func (p *Player) playbackLoop(ctx context.Context, path string, sessionID uint64) {
	log.Printf("[PLAYER] Starting playback (session %d): %s", sessionID, path)

	// Verify we're still the active session at start
	p.mu.RLock()
	if p.sessionID != sessionID {
		p.mu.RUnlock()
		log.Printf("[PLAYER] Session %d superseded, exiting immediately", sessionID)
		return
	}
	p.mu.RUnlock()

	// Track elapsed time accounting for pauses
	var elapsedBeforePause time.Duration
	playStartTime := time.Now()

	// Start a goroutine to update position while playing
	positionDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		wasPlaying := true
		lastMediaUpdate := time.Now()

		for {
			select {
			case <-positionDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.mu.Lock()
				// Check if we're still the active session
				if p.sessionID != sessionID {
					p.mu.Unlock()
					return
				}
				if p.state == StatePlaying {
					if !wasPlaying {
						// Just resumed - reset start time
						playStartTime = time.Now()
						wasPlaying = true
						// Update media session on state change
						if p.mediaSession != nil {
							p.mediaSession.UpdatePlaybackState(media.StatePlaying, time.Duration(p.position)*time.Millisecond)
						}
						lastMediaUpdate = time.Now()
					}
					p.position = (elapsedBeforePause + time.Since(playStartTime)).Milliseconds()
					// Check if we've reached the end
					if p.position >= p.duration {
						p.position = p.duration
					}
					// Only update media session every 5 seconds (for Rate-based tracking)
					if time.Since(lastMediaUpdate) >= 5*time.Second {
						if p.mediaSession != nil {
							p.mediaSession.UpdatePlaybackState(media.StatePlaying, time.Duration(p.position)*time.Millisecond)
						}
						lastMediaUpdate = time.Now()
					}
				} else if p.state == StatePaused && wasPlaying {
					// Just paused - save elapsed time
					elapsedBeforePause += time.Since(playStartTime)
					wasPlaying = false
				}
				p.mu.Unlock()
			}
		}
	}()

	err := p.decoder.Decode(ctx, path, p.output)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("[PLAYER] Decode error: %v", err)
	} else {
		log.Printf("[PLAYER] Decode complete, audio buffered: %s", path)
	}

	// Check if we're still the active session before waiting for playback
	p.mu.RLock()
	if p.sessionID != sessionID {
		p.mu.RUnlock()
		close(positionDone)
		log.Printf("[PLAYER] Session %d superseded after decode, exiting", sessionID)
		return
	}
	remainingMs := p.duration - p.position
	p.mu.RUnlock()

	// Wait for the audio to actually finish playing
	// The buffer needs time to drain through the audio output
	if remainingMs > 0 && err == nil {
		log.Printf("[PLAYER] Waiting for audio playback to complete (%dms remaining)", remainingMs)
		select {
		case <-ctx.Done():
			log.Printf("[PLAYER] Playback cancelled")
		case <-time.After(time.Duration(remainingMs+500) * time.Millisecond):
			log.Printf("[PLAYER] Playback finished: %s", path)
		}
	}

	close(positionDone)

	p.mu.Lock()

	// Only update state if we're still the active session
	if p.sessionID == sessionID && p.currentPath == path {
		wasManual := p.wasManualStop
		callback := p.onTrackEnd

		p.state = StateStopped
		p.currentPath = ""
		p.position = 0

		if p.mediaSession != nil {
			p.mediaSession.UpdatePlaybackState(media.StateStopped, 0)
		}

		p.mu.Unlock()

		// If track ended naturally (not manually stopped), call the callback
		if !wasManual && callback != nil {
			log.Printf("[PLAYER] Track ended naturally, calling onTrackEnd callback")
			callback(path)
		}
	} else {
		p.mu.Unlock()
	}
}

// playbackLoopFrom is like playbackLoop but starts from a specific position (for seeking)
func (p *Player) playbackLoopFrom(ctx context.Context, path string, startMs int64, sessionID uint64) {
	log.Printf("[PLAYER] Starting playback from %dms (session %d): %s", startMs, sessionID, path)

	// Verify we're still the active session at start
	p.mu.RLock()
	if p.sessionID != sessionID {
		p.mu.RUnlock()
		log.Printf("[PLAYER] Session %d superseded, exiting immediately", sessionID)
		return
	}
	p.mu.RUnlock()

	// Track elapsed time accounting for pauses, starting from seek position
	elapsedBeforePause := time.Duration(startMs) * time.Millisecond
	playStartTime := time.Now()

	// Start a goroutine to update position while playing
	positionDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		wasPlaying := true
		lastMediaUpdate := time.Now()

		for {
			select {
			case <-positionDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.mu.Lock()
				// Check if we're still the active session
				if p.sessionID != sessionID {
					p.mu.Unlock()
					return
				}
				if p.state == StatePlaying {
					if !wasPlaying {
						playStartTime = time.Now()
						wasPlaying = true
						if p.mediaSession != nil {
							p.mediaSession.UpdatePlaybackState(media.StatePlaying, time.Duration(p.position)*time.Millisecond)
						}
						lastMediaUpdate = time.Now()
					}
					p.position = (elapsedBeforePause + time.Since(playStartTime)).Milliseconds()
					if p.position >= p.duration {
						p.position = p.duration
					}
					if time.Since(lastMediaUpdate) >= 5*time.Second {
						if p.mediaSession != nil {
							p.mediaSession.UpdatePlaybackState(media.StatePlaying, time.Duration(p.position)*time.Millisecond)
						}
						lastMediaUpdate = time.Now()
					}
				} else if p.state == StatePaused && wasPlaying {
					elapsedBeforePause += time.Since(playStartTime)
					wasPlaying = false
				}
				p.mu.Unlock()
			}
		}
	}()

	// Decode from the specified start position
	ffmpegDecoder, ok := p.decoder.(*FFmpegDecoder)
	var err error
	if ok {
		err = ffmpegDecoder.DecodeFrom(ctx, path, p.output, startMs)
	} else {
		// Fallback to regular decode (loses seek position)
		err = p.decoder.Decode(ctx, path, p.output)
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("[PLAYER] Decode error: %v", err)
	} else {
		log.Printf("[PLAYER] Decode complete, audio buffered: %s", path)
	}

	// Check if we're still the active session before waiting for playback
	p.mu.RLock()
	if p.sessionID != sessionID {
		p.mu.RUnlock()
		close(positionDone)
		log.Printf("[PLAYER] Session %d superseded after decode, exiting", sessionID)
		return
	}
	remainingMs := p.duration - p.position
	p.mu.RUnlock()

	// Wait for the audio to actually finish playing
	if remainingMs > 0 && err == nil {
		log.Printf("[PLAYER] Waiting for audio playback to complete (%dms remaining)", remainingMs)
		select {
		case <-ctx.Done():
			log.Printf("[PLAYER] Playback cancelled")
		case <-time.After(time.Duration(remainingMs+500) * time.Millisecond):
			log.Printf("[PLAYER] Playback finished: %s", path)
		}
	}

	close(positionDone)

	p.mu.Lock()

	// Only update state if we're still the active session
	if p.sessionID == sessionID && p.currentPath == path {
		wasManual := p.wasManualStop
		callback := p.onTrackEnd

		p.state = StateStopped
		p.currentPath = ""
		p.position = 0

		if p.mediaSession != nil {
			p.mediaSession.UpdatePlaybackState(media.StateStopped, 0)
		}

		p.mu.Unlock()

		if !wasManual && callback != nil {
			log.Printf("[PLAYER] Track ended naturally, calling onTrackEnd callback")
			callback(path)
		}
	} else {
		p.mu.Unlock()
	}
}

// Pause pauses playback (idempotent - no error if already paused or stopped)
func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Idempotent: already paused or stopped is not an error
	if p.state == StatePaused || p.state == StateStopped {
		return nil
	}

	if p.state != StatePlaying {
		return nil // Not in a state where pause makes sense
	}

	p.state = StatePaused

	// Actually pause the audio output
	if otoOutput, ok := p.output.(*OtoOutput); ok {
		otoOutput.Pause()
	}

	if p.mediaSession != nil {
		p.mediaSession.UpdatePlaybackState(media.StatePaused, time.Duration(p.position)*time.Millisecond)
	}

	log.Printf("[PLAYER] Paused at position %dms", p.position)

	return nil
}

// Resume resumes playback (idempotent - no error if already playing or stopped)
func (p *Player) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Idempotent: already playing is not an error
	if p.state == StatePlaying {
		return nil
	}

	// Nothing to resume if stopped
	if p.state == StateStopped {
		return nil
	}

	if p.state != StatePaused {
		return nil // Not in a state where resume makes sense
	}

	p.state = StatePlaying

	// Actually resume the audio output
	if otoOutput, ok := p.output.(*OtoOutput); ok {
		otoOutput.Resume()
	}

	if p.mediaSession != nil {
		p.mediaSession.UpdatePlaybackState(media.StatePlaying, time.Duration(p.position)*time.Millisecond)
	}

	log.Printf("[PLAYER] Resumed at position %dms", p.position)

	return nil
}

// Stop stops playback
func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == StateStopped {
		return nil
	}

	p.stopPlaybackLocked()
	return nil
}

func (p *Player) stopPlaybackLocked() {
	p.state = StateStopped
	p.wasManualStop = true // Mark this as a manual stop

	// First, cancel the context to stop FFmpeg immediately
	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}

	// Brief pause to let FFmpeg process the cancellation
	time.Sleep(10 * time.Millisecond)

	// Now stop the audio output and clear buffer
	if otoOutput, ok := p.output.(*OtoOutput); ok {
		otoOutput.Stop()
	}

	// Signal the playback goroutine (in case it's waiting on something else)
	select {
	case p.stopChan <- struct{}{}:
	default:
	}

	if p.mediaSession != nil {
		p.mediaSession.UpdatePlaybackState(media.StateStopped, 0)
	}

	log.Printf("[PLAYER] Stopped playback")

	p.currentPath = ""
	p.position = 0
	p.metadata = nil
}

// Seek seeks to the specified position in milliseconds
func (p *Player) Seek(positionMs int64) error {
	p.mu.Lock()
	
	if p.state == StateStopped {
		p.mu.Unlock()
		return errors.New("not playing")
	}

	// Clamp to valid range
	if positionMs < 0 {
		positionMs = 0
	}
	if positionMs > p.duration {
		positionMs = p.duration
	}

	// Save current state
	path := p.currentPath
	metadata := p.metadata
	wasPlaying := p.state == StatePlaying
	
	log.Printf("[PLAYER] Seeking to %dms in %s", positionMs, path)
	
	// Stop current playback (marks as manual stop)
	p.stopPlaybackLocked()
	p.mu.Unlock()

	// Restart from new position
	if wasPlaying {
		return p.PlayFrom(context.Background(), path, metadata, positionMs)
	}
	
	return nil
}

// PlayFrom starts playback from a specific position (for seeking)
func (p *Player) PlayFrom(ctx context.Context, path string, metadata *TrackMetadata, startMs int64) error {
	// Serialize all play operations - only one Play() can run at a time
	p.playbackMu.Lock()
	defer p.playbackMu.Unlock()

	p.mu.Lock()

	// Stop any current playback and WAIT for it to finish
	if p.state == StatePlaying || p.state == StatePaused {
		p.stopPlaybackLocked()
		oldDone := p.sessionDone
		p.mu.Unlock()

		// Wait for old playback goroutine to fully exit
		if oldDone != nil {
			<-oldDone
		}

		p.mu.Lock()
	}

	// Create new session
	p.sessionID++
	p.sessionDone = make(chan struct{})
	currentSession := p.sessionID
	doneChan := p.sessionDone

	p.currentPath = path
	p.position = startMs
	p.state = StatePlaying
	p.metadata = metadata
	p.wasManualStop = false

	// Get duration if not provided in metadata
	var duration time.Duration
	if metadata != nil && metadata.Duration > 0 {
		duration = time.Duration(metadata.Duration) * time.Millisecond
	} else {
		var err error
		duration, err = p.decoder.Duration(path)
		if err != nil {
			p.mu.Unlock()
			return fmt.Errorf("failed to get duration: %w", err)
		}
	}
	p.duration = duration.Milliseconds()

	// Update media session
	if p.mediaSession != nil && metadata != nil {
		p.mediaSession.UpdateMetadata(media.Metadata{
			Title:    metadata.Title,
			Artist:   metadata.Artist,
			Album:    metadata.Album,
			Duration: duration,
			ArtPath:  metadata.ArtPath,
		})
		p.mediaSession.UpdatePlaybackState(media.StatePlaying, time.Duration(startMs)*time.Millisecond)
	}

	p.stopChan = make(chan struct{})

	playbackCtx, cancel := context.WithCancel(context.Background())
	p.cancelFunc = cancel

	p.mu.Unlock()

	// Start decoding from the specified position - goroutine closes doneChan when it exits
	go func() {
		defer close(doneChan)
		p.playbackLoopFrom(playbackCtx, path, startMs, currentSession)
	}()

	return nil
}

// SetVolume sets the playback volume (0.0 - 1.0)
func (p *Player) SetVolume(volume float64) error {
	if volume < 0 || volume > 1 {
		return errors.New("volume must be between 0.0 and 1.0")
	}

	p.mu.Lock()
	p.volume = volume

	// Apply volume to the audio output
	if otoOutput, ok := p.output.(*OtoOutput); ok {
		otoOutput.SetVolume(volume)
	}

	p.mu.Unlock()

	return nil
}

// Status returns the current playback status
func (p *Player) Status() Status {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return Status{
		State:    p.state,
		Path:     p.currentPath,
		Position: p.position,
		Duration: p.duration,
		Volume:   p.volume,
		Metadata: p.metadata,
	}
}

// GetAudioBands returns current frequency bands for visualization (64 bands, 0-255 each)
func (p *Player) GetAudioBands() []uint8 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if otoOutput, ok := p.output.(*OtoOutput); ok {
		return otoOutput.GetAudioBands()
	}
	return make([]uint8, 64)
}

// SetAudioCallback registers a callback for real-time audio data push
// The callback is called immediately when new audio analysis is ready (no polling)
func (p *Player) SetAudioCallback(cb AudioDataCallback) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if otoOutput, ok := p.output.(*OtoOutput); ok {
		otoOutput.SetAudioCallback(cb)
	}
}

// Close releases all resources
func (p *Player) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != StateStopped {
		p.stopPlaybackLocked()
	}

	var errs []error

	if p.decoder != nil {
		if err := p.decoder.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if p.output != nil {
		if err := p.output.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}

	return nil
}

// UpdateShuffle updates the shuffle state in the OS media session
func (p *Player) UpdateShuffle(enabled bool) error {
	p.mu.RLock()
	session := p.mediaSession
	p.mu.RUnlock()

	if session != nil {
		return session.UpdateShuffle(enabled)
	}
	return nil
}

// UpdateLoopStatus updates the loop/repeat mode in the OS media session
func (p *Player) UpdateLoopStatus(status media.LoopStatus) error {
	p.mu.RLock()
	session := p.mediaSession
	p.mu.RUnlock()

	if session != nil {
		return session.UpdateLoopStatus(status)
	}
	return nil
}

func stateToMediaState(state PlaybackState) media.PlaybackState {
	switch state {
	case StatePlaying:
		return media.StatePlaying
	case StatePaused:
		return media.StatePaused
	default:
		return media.StateStopped
	}
}

// OnCommand implements media.CommandHandler for MPRIS/OS media control integration
func (p *Player) OnCommand(cmd media.Command, data interface{}) error {
	// Don't log frequent Seek commands (OS syncing position)
	if cmd != media.CmdSeek {
		log.Printf("[PLAYER] Received OS media command: %s", cmd)
	}

	switch cmd {
	case media.CmdPlay:
		p.mu.Lock()
		state := p.state
		p.mu.Unlock()
		if state == StatePaused {
			return p.Resume()
		}
		// If stopped, we can't play without a path
		return nil

	case media.CmdPause:
		return p.Pause()

	case media.CmdPlayPause:
		p.mu.Lock()
		state := p.state
		p.mu.Unlock()
		if state == StatePlaying {
			return p.Pause()
		} else if state == StatePaused {
			return p.Resume()
		}
		return nil

	case media.CmdStop:
		return p.Stop()

	case media.CmdNext:
		p.mu.RLock()
		callback := p.onNext
		p.mu.RUnlock()
		if callback != nil {
			callback()
		}
		return nil

	case media.CmdPrevious:
		p.mu.RLock()
		callback := p.onPrevious
		p.mu.RUnlock()
		if callback != nil {
			callback()
		}
		return nil

	case media.CmdSeek:
		if pos, ok := data.(time.Duration); ok {
			log.Printf("[PLAYER] Seeking to %v", pos)
			return p.Seek(pos.Milliseconds())
		}
		return nil

	case media.CmdSetShuffle:
		if enabled, ok := data.(bool); ok {
			log.Printf("[PLAYER] Shuffle toggled from OS: %v", enabled)
			p.mu.RLock()
			callback := p.onShuffle
			p.mu.RUnlock()
			if callback != nil {
				callback(enabled)
			}
		}
		return nil

	case media.CmdSetLoopStatus:
		if status, ok := data.(media.LoopStatus); ok {
			log.Printf("[PLAYER] Loop status changed from OS: %s", status)
			p.mu.RLock()
			callback := p.onLoop
			p.mu.RUnlock()
			if callback != nil {
				callback(status)
			}
		}
		return nil
	}

	return nil
}
