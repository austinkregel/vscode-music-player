package audio

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/hajimehoshi/oto/v2"
)

const (
	defaultSampleRate = 44100
	defaultChannels   = 2
	defaultBitDepth   = 2 // 16-bit = 2 bytes
	
	// Maximum buffer size to prevent visualization getting ahead of audio
	// 100ms at 44100Hz stereo 16-bit = 17640 bytes
	// This keeps visualization in sync with what the user hears
	maxBufferSize = 17640
)

// OtoOutput is an audio output using the Oto library
type OtoOutput struct {
	context    *oto.Context
	player     oto.Player // oto.Player is an interface, not a pointer
	sampleRate int
	channels   int
	mu         sync.Mutex
	cond       *sync.Cond // Condition variable for pause/resume synchronization
	buffer     *bytes.Buffer
	volume     float64 // 0.0 - 1.0
	paused     bool    // True when explicitly paused - prevents auto-resume on Write
	closed     bool    // True when output is closed - unblocks waiting goroutines
	analyzer   *AudioAnalyzer // Real-time FFT analyzer for visualization
}

// NewOtoOutput creates a new Oto-based audio output
func NewOtoOutput() (*OtoOutput, error) {
	return NewOtoOutputWithConfig(defaultSampleRate, defaultChannels)
}

// NewOtoOutputWithConfig creates a new Oto-based audio output with custom config
func NewOtoOutputWithConfig(sampleRate, channels int) (*OtoOutput, error) {
	// Create Oto context
	ctx, ready, err := oto.NewContext(sampleRate, channels, defaultBitDepth)
	if err != nil {
		return nil, fmt.Errorf("failed to create oto context: %w", err)
	}

	// Wait for context to be ready
	<-ready

	// Create a buffer for audio data
	buffer := &bytes.Buffer{}

	output := &OtoOutput{
		context:    ctx,
		sampleRate: sampleRate,
		channels:   channels,
		buffer:     buffer,
		volume:     1.0,
		analyzer:   NewAudioAnalyzer(sampleRate, channels),
	}
	output.cond = sync.NewCond(&output.mu)

	// Create player with the buffer as source
	output.player = ctx.NewPlayer(output)

	return output, nil
}

// Read implements io.Reader for the player to read from
func (o *OtoOutput) Read(p []byte) (n int, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Block while paused and not closed, waiting for Resume() or Close()
	for o.paused && !o.closed {
		o.cond.Wait()
	}

	// If closed, signal EOF to stop the player cleanly
	if o.closed {
		return 0, io.EOF
	}

	// If buffer is empty but not paused, return silence to keep stream alive
	if o.buffer.Len() == 0 {
		for i := range p {
			p[i] = 0
		}
		return len(p), nil
	}

	n, err = o.buffer.Read(p)
	if err != nil {
		return n, err
	}

	// Process samples through analyzer for visualization (before volume adjustment)
	if o.analyzer != nil && n > 0 {
		o.analyzer.ProcessSamples(p[:n])
	}

	// Apply volume scaling to 16-bit PCM samples
	if o.volume < 1.0 && n > 0 {
		o.applyVolume(p[:n])
	}

	return n, nil
}

// applyVolume scales 16-bit PCM samples by the current volume
func (o *OtoOutput) applyVolume(data []byte) {
	vol := o.volume
	if vol >= 1.0 {
		return
	}

	// Process 16-bit samples (2 bytes per sample, little-endian)
	for i := 0; i < len(data)-1; i += 2 {
		// Read 16-bit little-endian sample
		sample := int16(data[i]) | int16(data[i+1])<<8

		// Scale by volume
		scaled := int16(float64(sample) * vol)

		// Write back
		data[i] = byte(scaled)
		data[i+1] = byte(scaled >> 8)
	}
}

// SetVolume sets the playback volume (0.0 - 1.0)
func (o *OtoOutput) SetVolume(v float64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	o.volume = v
}

// GetVolume returns the current volume
func (o *OtoOutput) GetVolume() float64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.volume
}

// Write writes PCM audio data to the output buffer
// Blocks if buffer exceeds maxBufferSize to keep visualization in sync with audio
func (o *OtoOutput) Write(data []byte) (int, error) {
	// Wait until buffer has room - this throttles decoding to match playback
	for {
		o.mu.Lock()
		if o.buffer.Len() < maxBufferSize {
			break
		}
		o.mu.Unlock()
		// Buffer full, wait for playback to consume some
		time.Sleep(10 * time.Millisecond)
	}
	defer o.mu.Unlock()

	n, err := o.buffer.Write(data)
	if err != nil {
		return n, err
	}

	// Only auto-start player if not explicitly paused
	if o.player != nil && !o.player.IsPlaying() && !o.paused {
		o.player.Play()
	}

	return n, nil
}

// Pause pauses audio playback
func (o *OtoOutput) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.paused = true // Set flag BEFORE pausing to prevent race with Write
	if o.player != nil && o.player.IsPlaying() {
		o.player.Pause()
	}
}

// Resume resumes audio playback
func (o *OtoOutput) Resume() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.paused = false // Clear flag to allow playback
	o.cond.Broadcast() // Wake up any blocked Read() goroutines
	if o.player != nil && !o.player.IsPlaying() {
		o.player.Play()
	}
}

// Stop stops playback and clears the buffer
func (o *OtoOutput) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.paused = false // Reset paused flag so new playback can start
	if o.player != nil {
		o.player.Pause()
	}
	// Clear the buffer so old audio doesn't play when we start again
	o.buffer.Reset()
}

// IsPlaying returns whether audio is currently playing
func (o *OtoOutput) IsPlaying() bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.player != nil && o.player.IsPlaying()
}

// Close releases the audio output resources
func (o *OtoOutput) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.closed = true
	o.cond.Broadcast() // Wake up any blocked Read() goroutines so they can exit

	if o.player != nil {
		if err := o.player.Close(); err != nil {
			return err
		}
	}
	return nil
}

// SampleRate returns the sample rate
func (o *OtoOutput) SampleRate() int {
	return o.sampleRate
}

// Channels returns the number of channels
func (o *OtoOutput) Channels() int {
	return o.channels
}

// GetAudioBands returns the current frequency bands for visualization
func (o *OtoOutput) GetAudioBands() []uint8 {
	if o.analyzer != nil {
		return o.analyzer.GetBands()
	}
	return make([]uint8, 64)
}

// ResetAnalyzer resets the audio analyzer state
func (o *OtoOutput) ResetAnalyzer() {
	if o.analyzer != nil {
		o.analyzer.Reset()
	}
}

// SetAudioCallback registers a callback for real-time audio data push
// The callback is called immediately when new audio analysis data is ready
func (o *OtoOutput) SetAudioCallback(cb AudioDataCallback) {
	if o.analyzer != nil {
		o.analyzer.SetCallback(cb)
	}
}

// Ensure OtoOutput implements io.Reader
var _ io.Reader = (*OtoOutput)(nil)
