package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FileMetadata contains metadata extracted from an audio file
type FileMetadata struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
}

// FFmpegDecoder uses FFmpeg for audio decoding
type FFmpegDecoder struct {
	ffmpegPath  string
	ffprobePath string
}

// NewFFmpegDecoder creates a new FFmpeg-based decoder
func NewFFmpegDecoder() (*FFmpegDecoder, error) {
	// Find ffmpeg and ffprobe in PATH
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe not found in PATH: %w", err)
	}

	return &FFmpegDecoder{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
	}, nil
}

// Decode decodes an audio file and writes PCM data to the output
func (d *FFmpegDecoder) Decode(ctx context.Context, path string, output Output) error {
	return d.DecodeFrom(ctx, path, output, 0)
}

// DecodeFrom decodes an audio file starting from the specified position
func (d *FFmpegDecoder) DecodeFrom(ctx context.Context, path string, output Output, startMs int64) error {
	// Build ffmpeg command to decode to raw PCM
	// Output format: signed 16-bit little-endian, stereo, 44100Hz
	args := []string{}

	// Add seek position if not starting from beginning
	if startMs > 0 {
		startSec := float64(startMs) / 1000.0
		args = append(args, "-ss", fmt.Sprintf("%.3f", startSec))
	}

	args = append(args,
		"-i", path,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", fmt.Sprintf("%d", output.Channels()),
		"-ar", fmt.Sprintf("%d", output.SampleRate()),
		"-",
	)

	cmd := exec.CommandContext(ctx, d.ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Ensure process is killed and reaped on any exit path
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait() // Reap zombie process
		}
	}()

	// Read decoded audio and write to output
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := stdout.Read(buf)
		if n > 0 {
			if _, writeErr := output.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write to output: %w", writeErr)
			}
		}
		if err != nil {
			break
		}
	}

	return cmd.Wait()
}

// Duration returns the duration of an audio file
func (d *FFmpegDecoder) Duration(path string) (time.Duration, error) {
	// Use ffprobe to get duration
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	}

	cmd := exec.Command(d.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	durationSec, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return time.Duration(durationSec * float64(time.Second)), nil
}

// Metadata extracts metadata from an audio file using ffprobe
func (d *FFmpegDecoder) Metadata(path string) (*FileMetadata, error) {
	// Use ffprobe to get metadata in JSON format
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		path,
	}

	cmd := exec.Command(d.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	// Parse JSON output
	var probeResult struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &probeResult); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	meta := &FileMetadata{}

	// Extract tags (case-insensitive lookup)
	tags := probeResult.Format.Tags
	for key, value := range tags {
		switch strings.ToLower(key) {
		case "title":
			meta.Title = value
		case "artist":
			meta.Artist = value
		case "album":
			meta.Album = value
		case "album_artist":
			if meta.Artist == "" {
				meta.Artist = value
			}
		}
	}

	// Parse duration
	if probeResult.Format.Duration != "" {
		if durationSec, err := strconv.ParseFloat(probeResult.Format.Duration, 64); err == nil {
			meta.Duration = time.Duration(durationSec * float64(time.Second))
		}
	}

	// Fallback to filename if no title
	if meta.Title == "" {
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		meta.Title = strings.TrimSuffix(base, ext)
	}

	return meta, nil
}

// Close releases decoder resources
func (d *FFmpegDecoder) Close() error {
	return nil
}
