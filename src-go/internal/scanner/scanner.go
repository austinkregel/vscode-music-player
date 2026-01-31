// Package scanner provides library scanning functionality.
// It walks configured library paths and finds audio files.
package scanner

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SupportedExtensions are the audio file extensions we recognize
var SupportedExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".m4a":  true,
	".aac":  true,
	".ogg":  true,
	".wav":  true,
	".wma":  true,
	".alac": true,
	".opus": true,
}

// TrackMetadata contains extracted audio metadata
type TrackMetadata struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int64  `json:"duration,omitempty"` // milliseconds
}

// FileInfo represents basic info about an audio file
type FileInfo struct {
	Path       string         `json:"path"`
	Size       int64          `json:"size"`
	ModifiedAt int64          `json:"modifiedAt"` // Unix timestamp
	Metadata   *TrackMetadata `json:"metadata,omitempty"`
}

// ScanResult is the result of a library scan
type ScanResult struct {
	LibraryPath string     `json:"libraryPath"`
	Files       []FileInfo `json:"files"`
	TotalFiles  int        `json:"totalFiles"`
	ScanTimeMs  int64      `json:"scanTimeMs"`
	Error       string     `json:"error,omitempty"`
}

// ScanStatus represents the current scan state
type ScanStatus struct {
	Status   string // "idle", "scanning", "complete", "error"
	Progress int    // 0-100
	Message  string
}

// Scanner handles library scanning
type Scanner struct {
	mu           sync.Mutex
	isRunning    bool
	cancel       context.CancelFunc
	status       ScanStatus
	lastResults  []ScanResult
	lastMetadata *LibraryMetadata
	ffprobePath  string
	nicePath     string // Path to 'nice' command for low-priority execution
}

// NewScanner creates a new scanner
func NewScanner() *Scanner {
	// Find ffprobe in PATH
	ffprobePath, _ := exec.LookPath("ffprobe")
	
	// Find nice command for low-priority execution (Linux/macOS)
	nicePath, _ := exec.LookPath("nice")
	
	return &Scanner{
		status:      ScanStatus{Status: "idle"},
		ffprobePath: ffprobePath,
		nicePath:    nicePath,
	}
}

// extractMetadata uses ffprobe to extract track metadata
// Runs at low priority using 'nice' when available to avoid hogging CPU
func (s *Scanner) extractMetadata(path string) *TrackMetadata {
	if s.ffprobePath == "" {
		return nil
	}

	ffprobeArgs := []string{
		"-v", "error",
		"-show_entries", "format=duration:format_tags=title,artist,album:stream_tags=title,artist,album",
		"-of", "json",
		path,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if s.nicePath != "" {
		// Run ffprobe at low priority (nice level 19 = lowest priority)
		args := append([]string{"-n", "19", s.ffprobePath}, ffprobeArgs...)
		cmd = exec.CommandContext(ctx, s.nicePath, args...)
	} else {
		cmd = exec.CommandContext(ctx, s.ffprobePath, ffprobeArgs...)
	}
	
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var result struct {
		Format struct {
			Duration string `json:"duration"`
			Tags     struct {
				Title  string `json:"title"`
				Artist string `json:"artist"`
				Album  string `json:"album"`
			} `json:"tags"`
		} `json:"format"`
		Streams []struct {
			Tags struct {
				Title  string `json:"title"`
				Artist string `json:"artist"`
				Album  string `json:"album"`
			} `json:"tags"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil
	}

	meta := &TrackMetadata{}

	// Get from format tags first
	if result.Format.Tags.Title != "" {
		meta.Title = result.Format.Tags.Title
	}
	if result.Format.Tags.Artist != "" {
		meta.Artist = result.Format.Tags.Artist
	}
	if result.Format.Tags.Album != "" {
		meta.Album = result.Format.Tags.Album
	}

	// Override with stream tags if available
	if len(result.Streams) > 0 {
		if result.Streams[0].Tags.Title != "" && meta.Title == "" {
			meta.Title = result.Streams[0].Tags.Title
		}
		if result.Streams[0].Tags.Artist != "" && meta.Artist == "" {
			meta.Artist = result.Streams[0].Tags.Artist
		}
		if result.Streams[0].Tags.Album != "" && meta.Album == "" {
			meta.Album = result.Streams[0].Tags.Album
		}
	}

	// Parse duration
	if result.Format.Duration != "" {
		if durationSec, err := strconv.ParseFloat(result.Format.Duration, 64); err == nil {
			meta.Duration = int64(durationSec * 1000)
		}
	}

	// Fallback to filename if no title
	if meta.Title == "" {
		fileName := filepath.Base(path)
		meta.Title = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	}

	return meta
}

// GetStatus returns the current scan status
func (s *Scanner) GetStatus() ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// GetLastResults returns the last scan results
func (s *Scanner) GetLastResults() ([]ScanResult, *LibraryMetadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastResults, s.lastMetadata
}

// ClearResults clears the last scan results (after they've been fetched)
func (s *Scanner) ClearResults() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastResults = nil
	s.lastMetadata = nil
	if s.status.Status == "complete" {
		s.status.Status = "idle"
	}
}

// IsRunning returns whether a scan is in progress
func (s *Scanner) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}

// Stop stops any running scan
func (s *Scanner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.isRunning = false
}

// ScanPaths scans the given library paths for audio files (synchronous)
func (s *Scanner) ScanPaths(ctx context.Context, paths []string) []ScanResult {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return []ScanResult{{Error: "scan already in progress"}}
	}
	s.isRunning = true
	s.status = ScanStatus{Status: "scanning", Progress: 0, Message: "Starting scan..."}
	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isRunning = false
		s.cancel = nil
		s.mu.Unlock()
	}()

	results := make([]ScanResult, 0, len(paths))
	totalPaths := len(paths)

	for i, path := range paths {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.status = ScanStatus{Status: "idle", Message: "Scan cancelled"}
			s.mu.Unlock()
			return results
		default:
		}

		// Update progress
		progress := (i * 100) / totalPaths
		s.mu.Lock()
		s.status = ScanStatus{Status: "scanning", Progress: progress, Message: "Scanning: " + path}
		s.mu.Unlock()

		result := s.scanPath(ctx, path)
		results = append(results, result)
	}

	return results
}

// ScanPathsAsync starts a background scan and returns immediately
func (s *Scanner) ScanPathsAsync(ctx context.Context, paths []string, metadataScan bool) bool {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return false
	}
	s.isRunning = true
	s.status = ScanStatus{Status: "scanning", Progress: 0, Message: "Starting scan..."}
	s.lastResults = nil
	s.lastMetadata = nil
	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.isRunning = false
			s.cancel = nil
			s.mu.Unlock()
		}()

		log.Printf("[SCANNER] Async scan starting for %d paths", len(paths))
		results := make([]ScanResult, 0, len(paths))
		totalPaths := len(paths)
		lastLoggedProgress := -5 // Track last logged progress for 5% intervals

		for i, path := range paths {
			select {
			case <-ctx.Done():
				log.Printf("[SCANNER] Scan cancelled")
				s.mu.Lock()
				s.status = ScanStatus{Status: "idle", Message: "Scan cancelled"}
				s.mu.Unlock()
				return
			default:
			}

			// Update progress
			progress := ((i * 100) / totalPaths) / 2 // First half is file scanning
			s.mu.Lock()
			s.status = ScanStatus{Status: "scanning", Progress: progress, Message: "Scanning files: " + path}
			s.mu.Unlock()
			
			// Log every 5% progress
			if progress >= lastLoggedProgress+5 {
				log.Printf("[SCANNER] Progress %d%%: Scanning files", progress)
				lastLoggedProgress = progress
			}

			result := s.scanPath(ctx, path)
			results = append(results, result)
			log.Printf("[SCANNER] Found %d files in %s", result.TotalFiles, path)
		}

		// Scan for metadata if requested
		var metadata *LibraryMetadata
		if metadataScan {
			log.Printf("[SCANNER] Progress 50%%: Starting NFO metadata scan")
			lastLoggedProgress = 45 // Reset for second phase
			allArtists := []ArtistInfo{}
			allAlbums := []AlbumInfo{}
			allArtwork := make(map[string][]string)

			for i, libPath := range paths {
				progress := 50 + ((i * 50) / totalPaths) // Second half is metadata
				s.mu.Lock()
				s.status = ScanStatus{Status: "scanning", Progress: progress, Message: "Reading metadata: " + libPath}
				s.mu.Unlock()
				
				// Log every 5% progress
				if progress >= lastLoggedProgress+5 {
					log.Printf("[SCANNER] Progress %d%%: Reading NFO metadata", progress)
					lastLoggedProgress = progress
				}

				libMeta, err := s.ScanMetadata(libPath)
				if err != nil {
					log.Printf("[SCANNER] NFO scan error for %s: %v", libPath, err)
					continue
				}

				allArtists = append(allArtists, libMeta.Artists...)
				allAlbums = append(allAlbums, libMeta.Albums...)
				for path, art := range libMeta.Artwork {
					allArtwork[path] = art
				}
			}

			if len(allArtists) > 0 || len(allAlbums) > 0 || len(allArtwork) > 0 {
				metadata = &LibraryMetadata{
					Artists: allArtists,
					Albums:  allAlbums,
					Artwork: allArtwork,
				}
			}
			log.Printf("[SCANNER] NFO metadata complete: %d artists, %d albums", len(allArtists), len(allAlbums))
		}

		// Calculate total files
		totalFiles := 0
		for _, r := range results {
			totalFiles += r.TotalFiles
		}

		// Store results
		s.mu.Lock()
		s.lastResults = results
		s.lastMetadata = metadata
		s.status = ScanStatus{Status: "complete", Progress: 100, Message: "Scan complete"}
		s.mu.Unlock()

		log.Printf("[SCANNER] Async scan complete: %d total files from %d library paths", totalFiles, len(paths))
	}()

	return true
}

// scanPath scans a single library path
func (s *Scanner) scanPath(ctx context.Context, libraryPath string) ScanResult {
	start := time.Now()
	result := ScanResult{
		LibraryPath: libraryPath,
		Files:       []FileInfo{},
	}

	// Check if path exists
	info, err := os.Stat(libraryPath)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	if !info.IsDir() {
		result.Error = "path is not a directory"
		return result
	}

	// Collect file paths first
	var filePaths []string
	var fileSizes []int64
	var fileModTimes []int64

	// Walk the directory tree
	err = filepath.WalkDir(libraryPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip directories
		if d.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") && path != libraryPath {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if it's an audio file
		ext := strings.ToLower(filepath.Ext(path))
		if !SupportedExtensions[ext] {
			return nil
		}

		// Get file info
		fileInfo, err := d.Info()
		if err != nil {
			return nil // Skip files we can't stat
		}

		filePaths = append(filePaths, path)
		fileSizes = append(fileSizes, fileInfo.Size())
		fileModTimes = append(fileModTimes, fileInfo.ModTime().Unix())

		return nil
	})

	if err != nil && err != context.Canceled {
		result.Error = err.Error()
	}

	log.Printf("[SCANNER] Discovered %d audio files in %s, extracting metadata...", len(filePaths), libraryPath)

	// Extract metadata in parallel
	type indexedFile struct {
		index int
		file  FileInfo
	}

	// Use 4 workers to avoid overwhelming the system
	// Each worker also runs ffprobe at low priority via 'nice'
	numWorkers := 4
	jobs := make(chan int, len(filePaths))
	results := make(chan indexedFile, len(filePaths))

	// Progress tracking for metadata extraction
	var processedCount int64
	totalToProcess := len(filePaths)
	lastLoggedPercent := int64(-5)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				fi := FileInfo{
					Path:       filePaths[i],
					Size:       fileSizes[i],
					ModifiedAt: fileModTimes[i],
					Metadata:   s.extractMetadata(filePaths[i]),
				}
				results <- indexedFile{index: i, file: fi}

				// Log progress every 5%
				count := atomic.AddInt64(&processedCount, 1)
				if totalToProcess > 0 {
					percent := (count * 100) / int64(totalToProcess)
					if percent >= atomic.LoadInt64(&lastLoggedPercent)+5 {
						atomic.StoreInt64(&lastLoggedPercent, percent)
						log.Printf("[SCANNER] Extracting metadata: %d%% (%d/%d files)", percent, count, totalToProcess)
					}
				}
			}
		}()
	}

	// Send jobs
	for i := range filePaths {
		jobs <- i
	}
	close(jobs)

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Build result array in order
	fileInfos := make([]FileInfo, len(filePaths))
	for r := range results {
		fileInfos[r.index] = r.file
	}

	result.Files = fileInfos
	result.TotalFiles = len(result.Files)
	result.ScanTimeMs = time.Since(start).Milliseconds()

	log.Printf("[SCANNER] Scanned %d files in %dms from %s", result.TotalFiles, result.ScanTimeMs, libraryPath)

	return result
}

// ScanPathsStreaming scans paths and sends results via a channel
// This is useful for large libraries where you want incremental updates
func (s *Scanner) ScanPathsStreaming(ctx context.Context, paths []string, results chan<- FileInfo) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return nil
	}
	s.isRunning = true
	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isRunning = false
		s.cancel = nil
		s.mu.Unlock()
		close(results)
	}()

	for _, libraryPath := range paths {
		// Check if path exists
		info, err := os.Stat(libraryPath)
		if err != nil || !info.IsDir() {
			continue
		}

		// Walk the directory tree
		err = filepath.WalkDir(libraryPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && path != libraryPath {
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !SupportedExtensions[ext] {
				return nil
			}

			fileInfo, err := d.Info()
			if err != nil {
				return nil
			}

			select {
			case results <- FileInfo{
				Path:       path,
				Size:       fileInfo.Size(),
				ModifiedAt: fileInfo.ModTime().Unix(),
			}:
			case <-ctx.Done():
				return ctx.Err()
			}

			return nil
		})

		if err == context.Canceled {
			return err
		}
	}

	return nil
}
