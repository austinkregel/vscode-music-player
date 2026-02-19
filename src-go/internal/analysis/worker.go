package analysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// AnalysisStatus represents the current state of background analysis
type AnalysisStatus struct {
	Status       string `json:"status"` // "idle", "running", "paused", "complete"
	TotalTracks  int    `json:"totalTracks"`
	Analyzed     int    `json:"analyzed"`
	InProgress   int    `json:"inProgress"`
	Failed       int    `json:"failed"`
	Message      string `json:"message"`
	StartedAt    int64  `json:"startedAt,omitempty"`
	EstimatedEnd int64  `json:"estimatedEnd,omitempty"`
}

// AnalysisResult contains the result of analyzing a single track
type AnalysisResult struct {
	TrackPath string
	Features  *AudioFeatures
	FileHash  string
	Error     error
}

// TrackInfo contains information needed to analyze a track
type TrackInfo struct {
	Path     string
	FileHash string // Used to detect if file changed
}

// Worker performs background audio analysis
type Worker struct {
	mu sync.Mutex

	// Configuration
	maxWorkers   int
	throttleMs   int64 // Sleep between tracks during playback
	idleThrottle int64 // Sleep between tracks when idle

	// State
	status      AnalysisStatus
	ctx         context.Context
	cancel      context.CancelFunc
	isRunning   bool
	isPaused    bool
	pauseChan   chan struct{}
	resumeChan  chan struct{}

	// External state
	isPlayingFunc func() bool // Function to check if audio is playing

	// FFmpeg path
	ffmpegPath string
	nicePath   string

	// Feature extractor
	extractor *FeatureExtractor

	// Results callback
	onResult func(AnalysisResult)

	// Counts
	analyzedCount  int64
	failedCount    int64
	inProgressCount int64
}

// WorkerConfig contains configuration for the analysis worker
type WorkerConfig struct {
	MaxWorkers    int           // Maximum concurrent analysis workers (0 = NumCPU - 1)
	ThrottleMs    int64         // Sleep ms between tracks during playback
	IdleThrottle  int64         // Sleep ms between tracks when idle
	IsPlayingFunc func() bool   // Function to check playback state
	OnResult      func(AnalysisResult) // Callback when analysis completes
}

// NewWorker creates a new background analysis worker
func NewWorker(cfg WorkerConfig) (*Worker, error) {
	// Find FFmpeg
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %w", err)
	}

	// Find nice command
	nicePath, _ := exec.LookPath("nice")

	// Set defaults
	maxWorkers := cfg.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU() - 1
		if maxWorkers < 1 {
			maxWorkers = 1
		}
	}

	throttleMs := cfg.ThrottleMs
	if throttleMs <= 0 {
		throttleMs = 100 // 100ms pause during playback
	}

	idleThrottle := cfg.IdleThrottle
	if idleThrottle <= 0 {
		idleThrottle = 10 // 10ms pause when idle
	}

	return &Worker{
		maxWorkers:    maxWorkers,
		throttleMs:    throttleMs,
		idleThrottle:  idleThrottle,
		isPlayingFunc: cfg.IsPlayingFunc,
		ffmpegPath:    ffmpegPath,
		nicePath:      nicePath,
		extractor:     NewFeatureExtractor(44100),
		onResult:      cfg.OnResult,
		status:        AnalysisStatus{Status: "idle"},
		pauseChan:     make(chan struct{}),
		resumeChan:    make(chan struct{}),
	}, nil
}

// Start begins background analysis of the given tracks
func (w *Worker) Start(ctx context.Context, tracks []TrackInfo) error {
	w.mu.Lock()
	if w.isRunning {
		w.mu.Unlock()
		return fmt.Errorf("analysis already running")
	}

	w.ctx, w.cancel = context.WithCancel(ctx)
	w.isRunning = true
	w.isPaused = false
	atomic.StoreInt64(&w.analyzedCount, 0)
	atomic.StoreInt64(&w.failedCount, 0)
	atomic.StoreInt64(&w.inProgressCount, 0)

	w.status = AnalysisStatus{
		Status:      "running",
		TotalTracks: len(tracks),
		StartedAt:   time.Now().Unix(),
	}
	w.mu.Unlock()

	go w.run(tracks)
	return nil
}

// Stop stops the background analysis
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.isRunning = false
	w.status.Status = "idle"
	w.status.Message = "Analysis stopped"
}

// Pause pauses the background analysis
func (w *Worker) Pause() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isRunning || w.isPaused {
		return
	}

	w.isPaused = true
	w.status.Status = "paused"
	close(w.pauseChan)
	w.pauseChan = make(chan struct{})
}

// Resume resumes paused analysis
func (w *Worker) Resume() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isRunning || !w.isPaused {
		return
	}

	w.isPaused = false
	w.status.Status = "running"
	close(w.resumeChan)
	w.resumeChan = make(chan struct{})
}

// GetStatus returns the current analysis status
func (w *Worker) GetStatus() AnalysisStatus {
	w.mu.Lock()
	defer w.mu.Unlock()

	status := w.status
	status.Analyzed = int(atomic.LoadInt64(&w.analyzedCount))
	status.Failed = int(atomic.LoadInt64(&w.failedCount))
	status.InProgress = int(atomic.LoadInt64(&w.inProgressCount))

	return status
}

// IsRunning returns whether analysis is currently running
func (w *Worker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.isRunning
}

// run executes the background analysis
func (w *Worker) run(tracks []TrackInfo) {
	defer func() {
		w.mu.Lock()
		w.isRunning = false
		if w.status.Status == "running" {
			w.status.Status = "complete"
			w.status.Message = fmt.Sprintf("Analysis complete: %d tracks analyzed, %d failed",
				atomic.LoadInt64(&w.analyzedCount), atomic.LoadInt64(&w.failedCount))
		}
		w.mu.Unlock()
		log.Printf("[ANALYSIS] Worker finished: %d analyzed, %d failed",
			atomic.LoadInt64(&w.analyzedCount), atomic.LoadInt64(&w.failedCount))
	}()

	log.Printf("[ANALYSIS] Starting analysis of %d tracks with %d workers", len(tracks), w.maxWorkers)

	// Create job channel
	jobs := make(chan TrackInfo, len(tracks))
	for _, track := range tracks {
		jobs <- track
	}
	close(jobs)

	// Determine active workers based on playback state
	activeWorkers := w.getActiveWorkerCount()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < activeWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			w.worker(workerID, jobs)
		}(i)
	}

	wg.Wait()
}

// getActiveWorkerCount returns the number of workers to use
func (w *Worker) getActiveWorkerCount() int {
	if w.isPlayingFunc != nil && w.isPlayingFunc() {
		// During playback, use only 1 worker
		return 1
	}
	return w.maxWorkers
}

// getThrottle returns the current throttle delay
func (w *Worker) getThrottle() time.Duration {
	if w.isPlayingFunc != nil && w.isPlayingFunc() {
		return time.Duration(w.throttleMs) * time.Millisecond
	}
	return time.Duration(w.idleThrottle) * time.Millisecond
}

// worker processes tracks from the job channel
func (w *Worker) worker(id int, jobs <-chan TrackInfo) {
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		// Check for pause
		w.mu.Lock()
		isPaused := w.isPaused
		resumeChan := w.resumeChan
		w.mu.Unlock()

		if isPaused {
			select {
			case <-w.ctx.Done():
				return
			case <-resumeChan:
				// Resumed
			}
		}

		// Get next job
		track, ok := <-jobs
		if !ok {
			return // No more jobs
		}

		atomic.AddInt64(&w.inProgressCount, 1)

		// Analyze the track
		result := w.analyzeTrack(track)

		atomic.AddInt64(&w.inProgressCount, -1)
		if result.Error != nil {
			atomic.AddInt64(&w.failedCount, 1)
			log.Printf("[ANALYSIS] Worker %d: Failed %s: %v", id, track.Path, result.Error)
		} else {
			atomic.AddInt64(&w.analyzedCount, 1)
		}

		// Call result callback
		if w.onResult != nil {
			w.onResult(result)
		}

		// Throttle
		throttle := w.getThrottle()
		if throttle > 0 {
			time.Sleep(throttle)
		}
	}
}

// analyzeTrack analyzes a single audio track
func (w *Worker) analyzeTrack(track TrackInfo) AnalysisResult {
	result := AnalysisResult{
		TrackPath: track.Path,
	}

	// Check if file exists
	fileInfo, err := os.Stat(track.Path)
	if err != nil {
		result.Error = fmt.Errorf("file not found: %w", err)
		return result
	}

	// Compute file hash for change detection
	result.FileHash = computeFileHash(track.Path, fileInfo.Size())

	// Decode audio to PCM using FFmpeg
	pcmData, err := w.decodeAudioToPCM(track.Path)
	if err != nil {
		result.Error = fmt.Errorf("decode failed: %w", err)
		return result
	}

	if len(pcmData) < 4096 {
		result.Error = fmt.Errorf("audio too short")
		return result
	}

	// Extract features
	features := w.extractor.ProcessPCM(pcmData, 2) // Stereo
	result.Features = features

	return result
}

// decodeAudioToPCM decodes audio file to raw PCM data
func (w *Worker) decodeAudioToPCM(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build FFmpeg command
	// Output: signed 16-bit little-endian, stereo, 44100Hz
	ffmpegArgs := []string{
		"-i", path,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", "2",
		"-ar", "44100",
		"-",
	}

	var cmd *exec.Cmd
	if w.nicePath != "" {
		// Run at low priority (nice level 19)
		args := append([]string{"-n", "19", w.ffmpegPath}, ffmpegArgs...)
		cmd = exec.CommandContext(ctx, w.nicePath, args...)
	} else {
		cmd = exec.CommandContext(ctx, w.ffmpegPath, ffmpegArgs...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Read all output (limit to ~10 minutes of audio = ~500MB max)
	// For analysis, we only need a representative sample
	maxBytes := 44100 * 2 * 2 * 600 // 10 minutes of stereo 16-bit @ 44100Hz
	var buf bytes.Buffer
	buf.Grow(1024 * 1024) // Pre-allocate 1MB

	limited := io.LimitReader(stdout, int64(maxBytes))
	_, err = io.Copy(&buf, limited)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read output: %w", err)
	}

	cmd.Wait()
	return buf.Bytes(), nil
}

// computeFileHash computes a hash for change detection
// Uses size + first and last 64KB of file
func computeFileHash(path string, size int64) string {
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s:%d", path, size)))

	f, err := os.Open(path)
	if err != nil {
		return hex.EncodeToString(hasher.Sum(nil))[:16]
	}
	defer f.Close()

	// Read first 64KB
	buf := make([]byte, 65536)
	n, _ := f.Read(buf)
	hasher.Write(buf[:n])

	// Read last 64KB
	if size > 65536 {
		f.Seek(-65536, io.SeekEnd)
		n, _ = f.Read(buf)
		hasher.Write(buf[:n])
	}

	return hex.EncodeToString(hasher.Sum(nil))[:16]
}
