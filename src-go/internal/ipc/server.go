package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/austinkregel/local-media/musicd/internal/audio"
	"github.com/austinkregel/local-media/musicd/internal/auth"
	"github.com/austinkregel/local-media/musicd/internal/config"
	"github.com/austinkregel/local-media/musicd/internal/media"
	"github.com/austinkregel/local-media/musicd/internal/queue"
	"github.com/austinkregel/local-media/musicd/internal/scanner"
)

// Server handles IPC communication with clients
type Server struct {
	socketPath      string
	authManager     *auth.Manager
	configMgr       *config.Manager
	player          *audio.Player
	queueMgr        *queue.Manager
	mediaSession    media.Session
	libScanner      *scanner.Scanner
	listener        net.Listener
	mu              sync.Mutex
	clients         map[net.Conn]struct{}
	advancingTrack  sync.Mutex // Prevents concurrent next/prev track calls
	audioLogCounter int        // For throttled audio debug logging

	// Audio data streaming (callback-based, no polling)
	audioSubsMu sync.RWMutex
	audioSubs   map[net.Conn]bool // Clients subscribed to audio data
}

// NewServer creates a new IPC server
func NewServer(
	socketPath string,
	authManager *auth.Manager,
	configMgr *config.Manager,
	player *audio.Player,
	queueMgr *queue.Manager,
	mediaSession media.Session,
) (*Server, error) {
	s := &Server{
		socketPath:   socketPath,
		authManager:  authManager,
		configMgr:    configMgr,
		player:       player,
		queueMgr:     queueMgr,
		mediaSession: mediaSession,
		libScanner:   scanner.NewScanner(),
		clients:      make(map[net.Conn]struct{}),
		audioSubs:    make(map[net.Conn]bool),
	}
	
	// Register callback for real-time audio data push (no polling!)
	player.SetAudioCallback(func(bands []uint8) {
		s.pushAudioDataImmediate(bands)
	})
	
	// Set up callbacks for queue management
	player.SetOnTrackEnd(func(finishedPath string) {
		log.Printf("[QUEUE] Track ended: %s, advancing to next", finishedPath)
		s.playNextTrack()
	})
	
	player.SetOnNext(func() {
		log.Printf("[QUEUE] Next track requested via OS media controls")
		s.playNextTrack()
	})
	
	player.SetOnPrevious(func() {
		log.Printf("[QUEUE] Previous track requested via OS media controls")
		s.playPrevTrack()
	})

	player.SetOnShuffle(func(enabled bool) {
		log.Printf("[QUEUE] Shuffle toggled via OS media controls: %v", enabled)
		s.queueMgr.SetShuffle(enabled)
		// State is already updated in the media session by the OS
	})

	player.SetOnLoop(func(status media.LoopStatus) {
		log.Printf("[QUEUE] Loop status changed via OS media controls: %s", status)
		// Map media.LoopStatus to queue's RepeatMode
		var repeatMode queue.RepeatMode
		switch status {
		case media.LoopNone:
			repeatMode = queue.RepeatOff
		case media.LoopTrack:
			repeatMode = queue.RepeatOne
		case media.LoopPlaylist:
			repeatMode = queue.RepeatAll
		}
		s.queueMgr.SetRepeat(repeatMode)
		// State is already updated in the media session by the OS
	})

	return s, nil
}

// playNextTrack advances to the next track in the queue and starts playing
func (s *Server) playNextTrack() {
	// Serialize track advancement to prevent concurrent calls from causing issues
	s.advancingTrack.Lock()
	defer s.advancingTrack.Unlock()

	nextPath, nextMeta := s.queueMgr.Next()
	if nextPath == "" {
		log.Printf("[QUEUE] No more tracks in queue")
		return
	}

	log.Printf("[QUEUE] Playing next track: %s", nextPath)
	if err := s.player.Play(context.Background(), nextPath, (*audio.TrackMetadata)(nextMeta)); err != nil {
		log.Printf("[QUEUE] Failed to play next track: %v", err)
	}
}

// playPrevTrack goes to the previous track in the queue and starts playing
func (s *Server) playPrevTrack() {
	// Serialize track advancement to prevent concurrent calls from causing issues
	s.advancingTrack.Lock()
	defer s.advancingTrack.Unlock()

	prevPath, prevMeta := s.queueMgr.Prev()
	if prevPath == "" {
		log.Printf("[QUEUE] No previous track in queue")
		return
	}

	log.Printf("[QUEUE] Playing previous track: %s", prevPath)
	if err := s.player.Play(context.Background(), prevPath, (*audio.TrackMetadata)(prevMeta)); err != nil {
		log.Printf("[QUEUE] Failed to play previous track: %v", err)
	}
}

// Start starts the IPC server
func (s *Server) Start(ctx context.Context) error {
	// Remove existing socket file if it exists
	if err := os.RemoveAll(s.socketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	log.Printf("[IPC] Creating socket at %s", s.socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	s.listener = listener

	// Set socket permissions (user-only)
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	log.Printf("[IPC] Server listening, waiting for connections...")

	// Accept connections in background
	go s.acceptLoop(ctx)

	// Audio data is now pushed via callback (no timer-based streaming)

	// Wait for context cancellation
	<-ctx.Done()

	log.Printf("[IPC] Shutting down server...")

	// Cleanup
	s.mu.Lock()
	clientCount := len(s.clients)
	for conn := range s.clients {
		conn.Close()
	}
	s.mu.Unlock()

	log.Printf("[IPC] Closed %d client connections", clientCount)

	listener.Close()
	os.RemoveAll(s.socketPath)

	log.Printf("[IPC] Server stopped")

	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("[IPC] Accept error: %v", err)
				continue
			}
		}

		remoteAddr := conn.RemoteAddr().String()
		log.Printf("[IPC] New client connection from %s", remoteAddr)

		s.mu.Lock()
		s.clients[conn] = struct{}{}
		clientCount := len(s.clients)
		s.mu.Unlock()

		log.Printf("[IPC] Active clients: %d", clientCount)

		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	
	defer func() {
		log.Printf("[IPC] Client disconnected: %s", remoteAddr)
		conn.Close()
		s.mu.Lock()
		delete(s.clients, conn)
		clientCount := len(s.clients)
		s.mu.Unlock()
		// Remove from audio subscribers
		s.audioSubsMu.Lock()
		delete(s.audioSubs, conn)
		s.audioSubsMu.Unlock()
		log.Printf("[IPC] Active clients: %d", clientCount)
	}()

	reader := bufio.NewReader(conn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read line (newline-delimited JSON)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("[IPC] Read error from %s: %v", remoteAddr, err)
			}
			return
		}

		// Parse request
		req, err := DecodeRequest(line)
		if err != nil {
			log.Printf("[IPC] Invalid request format from %s: %v", remoteAddr, err)
			s.sendError(conn, "invalid request format")
			continue
		}

		// Skip verbose logging for frequent polling commands
		isPollingCmd := req.Cmd == CmdStatus || req.Cmd == CmdGetScanStatus || req.Cmd == CmdGetAudioData

		if !isPollingCmd {
			log.Printf("[IPC] Command: %s", req.Cmd)
		}

		// Handle request (pass conn for subscription commands)
		resp := s.handleRequest(ctx, conn, req)

		if !isPollingCmd {
			if resp.Success {
				log.Printf("[IPC] Response: success")
			} else {
				log.Printf("[IPC] Response: error=%q", resp.Error)
			}
		}

		// Send response
		if err := s.sendResponse(conn, resp); err != nil {
			log.Printf("[IPC] Send error to %s: %v", remoteAddr, err)
			return
		}
	}
}

func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func (s *Server) handleRequest(ctx context.Context, conn net.Conn, req *Request) *Response {
	// Pair command doesn't require authentication
	if req.Cmd == CmdPair {
		return s.handlePair(req)
	}

	// All other commands require authentication
	if !s.authManager.ValidateToken(req.Token) {
		return NewErrorResponse("unauthorized")
	}

	switch req.Cmd {
	case CmdPlay:
		return s.handlePlay(ctx, req)
	case CmdPause:
		return s.handlePause()
	case CmdResume:
		return s.handleResume()
	case CmdStop:
		return s.handleStop()
	case CmdNext:
		return s.handleNext(ctx)
	case CmdPrev:
		return s.handlePrev(ctx)
	case CmdQueue:
		return s.handleQueue(req)
	case CmdSeek:
		return s.handleSeek(req)
	case CmdVolume:
		return s.handleVolume(req)
	case CmdStatus:
		return s.handleStatus()
	case CmdGetConfig:
		return s.handleGetConfig()
	case CmdSetConfig:
		return s.handleSetConfig(req)
	case CmdScanLibrary:
		return s.handleScanLibrary(ctx)
	case CmdGetScanStatus:
		return s.handleGetScanStatus()
	case CmdGetQueue:
		return s.handleGetQueue()
	case CmdSetRepeat:
		return s.handleSetRepeat(req)
	case CmdSetShuffle:
		return s.handleSetShuffle(req)
	case CmdQueueJump:
		return s.handleQueueJump(ctx, req)
	case CmdQueueRemove:
		return s.handleQueueRemove(req)
	case CmdQueueMove:
		return s.handleQueueMove(req)
	case CmdGetAudioData:
		return s.handleGetAudioData()
	case CmdSubscribeAudioData:
		return s.handleSubscribeAudioData(conn)
	case CmdUnsubscribeAudioData:
		return s.handleUnsubscribeAudioData(conn)
	default:
		return NewErrorResponse("unknown command")
	}
}

func (s *Server) handlePair(req *Request) *Response {
	var pairReq PairRequest
	if req.Data != nil {
		if err := json.Unmarshal(req.Data, &pairReq); err != nil {
			return NewErrorResponse("invalid pair request")
		}
	}

	log.Printf("[AUTH] Pairing request from client: %q", pairReq.ClientName)

	token, clientID, requiresApproval, err := s.authManager.Pair(pairReq.ClientName)
	if err != nil {
		log.Printf("[AUTH] Pairing failed: %v", err)
		return NewErrorResponse(err.Error())
	}

	log.Printf("[AUTH] Paired client %s (ID: %s, approval required: %v)", pairReq.ClientName, clientID, requiresApproval)

	resp, err := NewSuccessResponse(PairResponse{
		Token:            token,
		ClientID:         clientID,
		RequiresApproval: requiresApproval,
	})
	if err != nil {
		return NewErrorResponse("internal error")
	}

	return resp
}

func (s *Server) handlePlay(ctx context.Context, req *Request) *Response {
	var playReq PlayRequest
	if err := json.Unmarshal(req.Data, &playReq); err != nil {
		log.Printf("[PLAYER] Invalid play request: %v", err)
		return NewErrorResponse("invalid play request")
	}

	if playReq.Path == "" {
		log.Printf("[PLAYER] Play request missing path")
		return NewErrorResponse("path is required")
	}

	log.Printf("[PLAYER] Play request: %s", playReq.Path)

	// Check if this path is in the queue
	queueItems := s.queueMgr.GetItems()
	foundInQueue := false
	for i, item := range queueItems {
		if item.Path == playReq.Path {
			s.queueMgr.SetIndex(i)
			log.Printf("[QUEUE] Found track in queue at index %d", i)
			foundInQueue = true
			break
		}
	}

	// If not in queue, add it as a single-track queue
	if !foundInQueue {
		s.queueMgr.Set([]string{playReq.Path})
		s.queueMgr.SetIndex(0)
		log.Printf("[QUEUE] Added single track to queue")
	}

	// Convert metadata
	var metadata *audio.TrackMetadata
	if playReq.Metadata != nil {
		metadata = &audio.TrackMetadata{
			Title:    playReq.Metadata.Title,
			Artist:   playReq.Metadata.Artist,
			Album:    playReq.Metadata.Album,
			Duration: playReq.Metadata.Duration,
			ArtPath:  playReq.Metadata.ArtPath,
		}
		log.Printf("[PLAYER] Metadata: %s - %s (%s)", metadata.Artist, metadata.Title, metadata.Album)
	}

	if err := s.player.Play(ctx, playReq.Path, metadata); err != nil {
		log.Printf("[PLAYER] Play failed: %v", err)
		return NewErrorResponse(err.Error())
	}

	log.Printf("[PLAYER] Now playing: %s", playReq.Path)
	return s.handleStatus()
}

func (s *Server) handlePause() *Response {
	log.Printf("[PLAYER] Pause requested")
	if err := s.player.Pause(); err != nil {
		log.Printf("[PLAYER] Pause failed: %v", err)
		return NewErrorResponse(err.Error())
	}
	log.Printf("[PLAYER] Paused")
	return s.handleStatus()
}

func (s *Server) handleResume() *Response {
	log.Printf("[PLAYER] Resume requested")
	if err := s.player.Resume(); err != nil {
		log.Printf("[PLAYER] Resume failed: %v", err)
		return NewErrorResponse(err.Error())
	}
	log.Printf("[PLAYER] Resumed")
	return s.handleStatus()
}

func (s *Server) handleStop() *Response {
	log.Printf("[PLAYER] Stop requested")
	if err := s.player.Stop(); err != nil {
		log.Printf("[PLAYER] Stop failed: %v", err)
		return NewErrorResponse(err.Error())
	}
	log.Printf("[PLAYER] Stopped")
	return s.handleStatus()
}

func (s *Server) handleNext(ctx context.Context) *Response {
	log.Printf("[PLAYER] Next track requested")
	path, metadata := s.queueMgr.Next()
	if path == "" {
		log.Printf("[PLAYER] No next track in queue")
		return NewErrorResponse("no next track")
	}
	log.Printf("[PLAYER] Next track: %s", path)

	var audioMeta *audio.TrackMetadata
	if metadata != nil {
		audioMeta = &audio.TrackMetadata{
			Title:    metadata.Title,
			Artist:   metadata.Artist,
			Album:    metadata.Album,
			Duration: metadata.Duration,
			ArtPath:  metadata.ArtPath,
		}
	}

	if err := s.player.Play(ctx, path, audioMeta); err != nil {
		return NewErrorResponse(err.Error())
	}

	return s.handleStatus()
}

func (s *Server) handlePrev(ctx context.Context) *Response {
	log.Printf("[PLAYER] Previous track requested")
	path, metadata := s.queueMgr.Prev()
	if path == "" {
		log.Printf("[PLAYER] No previous track in queue")
		return NewErrorResponse("no previous track")
	}
	log.Printf("[PLAYER] Previous track: %s", path)

	var audioMeta *audio.TrackMetadata
	if metadata != nil {
		audioMeta = &audio.TrackMetadata{
			Title:    metadata.Title,
			Artist:   metadata.Artist,
			Album:    metadata.Album,
			Duration: metadata.Duration,
			ArtPath:  metadata.ArtPath,
		}
	}

	if err := s.player.Play(ctx, path, audioMeta); err != nil {
		return NewErrorResponse(err.Error())
	}

	return s.handleStatus()
}

func (s *Server) handleQueue(req *Request) *Response {
	var queueReq QueueRequest
	if err := json.Unmarshal(req.Data, &queueReq); err != nil {
		return NewErrorResponse("invalid queue request")
	}

	log.Printf("[QUEUE] Queue request: %d items, append=%v", len(queueReq.Items), queueReq.Append)

	// Convert to queue items
	var queueItems []queue.QueueItem
	for _, item := range queueReq.Items {
		qi := queue.QueueItem{Path: item.Path}
		if item.Metadata != nil {
			qi.Metadata = &queue.TrackMetadata{
				Title:    item.Metadata.Title,
				Artist:   item.Metadata.Artist,
				Album:    item.Metadata.Album,
				Duration: item.Metadata.Duration,
			}
		}
		queueItems = append(queueItems, qi)
	}

	if queueReq.Append {
		s.queueMgr.AppendWithMetadata(queueItems)
		log.Printf("[QUEUE] Appended %d tracks to queue", len(queueItems))
	} else {
		s.queueMgr.SetWithMetadata(queueItems)
		log.Printf("[QUEUE] Set queue to %d tracks", len(queueItems))
	}

	idx, size := s.queueMgr.Position()
	log.Printf("[QUEUE] Queue position: %d/%d", idx, size)

	return s.handleStatus()
}

func (s *Server) handleSeek(req *Request) *Response {
	var seekReq SeekRequest
	if err := json.Unmarshal(req.Data, &seekReq); err != nil {
		return NewErrorResponse("invalid seek request")
	}

	log.Printf("[PLAYER] Seek to position: %dms", seekReq.Position)
	if err := s.player.Seek(seekReq.Position); err != nil {
		log.Printf("[PLAYER] Seek failed: %v", err)
		return NewErrorResponse(err.Error())
	}

	return s.handleStatus()
}

func (s *Server) handleVolume(req *Request) *Response {
	var volReq VolumeRequest
	if err := json.Unmarshal(req.Data, &volReq); err != nil {
		return NewErrorResponse("invalid volume request")
	}

	log.Printf("[PLAYER] Set volume to: %.2f", volReq.Level)
	if err := s.player.SetVolume(volReq.Level); err != nil {
		log.Printf("[PLAYER] Volume change failed: %v", err)
		return NewErrorResponse(err.Error())
	}

	return s.handleStatus()
}

func (s *Server) handleStatus() *Response {
	status := s.player.Status()
	queueIdx, queueSize := s.queueMgr.Position()

	var metadata *TrackMetadata
	if status.Metadata != nil {
		metadata = &TrackMetadata{
			Title:    status.Metadata.Title,
			Artist:   status.Metadata.Artist,
			Album:    status.Metadata.Album,
			Duration: status.Metadata.Duration,
			ArtPath:  status.Metadata.ArtPath,
		}
	}

	// Get repeat mode as string
	repeatMode := "off"
	switch s.queueMgr.GetRepeat() {
	case queue.RepeatOne:
		repeatMode = "one"
	case queue.RepeatAll:
		repeatMode = "all"
	}

	statusResp := StatusResponse{
		State:      string(status.State),
		Path:       status.Path,
		Position:   status.Position,
		Duration:   status.Duration,
		Volume:     status.Volume,
		Metadata:   metadata,
		QueueIndex: queueIdx,
		QueueSize:  queueSize,
		RepeatMode: repeatMode,
		Shuffle:    s.queueMgr.GetShuffle(),
	}

	// Log status details if playing or paused
	if status.State != "stopped" {
		log.Printf("[PLAYER] Status: state=%s pos=%dms dur=%dms path=%s",
			status.State, status.Position, status.Duration, truncateForLog(status.Path, 50))
	}

	resp, err := NewSuccessResponse(statusResp)
	if err != nil {
		return NewErrorResponse("internal error")
	}

	return resp
}

func (s *Server) handleGetAudioData() *Response {
	bandsU8 := s.player.GetAudioBands()
	
	// Convert []uint8 to []int for JSON (Go base64-encodes []uint8)
	bands := make([]int, len(bandsU8))
	for i, b := range bandsU8 {
		bands[i] = int(b)
	}

	// Debug: log every 30th request (~1 per second at 30fps)
	s.audioLogCounter++
	if s.audioLogCounter%30 == 0 {
		var sum, nonZero int
		for _, b := range bands {
			sum += b
			if b > 0 {
				nonZero++
			}
		}
		log.Printf("[AUDIO] Bands: %d non-zero, sum=%d, sample=[%d,%d,%d,%d,%d,%d,%d,%d]",
			nonZero, sum, bands[0], bands[4], bands[8], bands[16], bands[24], bands[32], bands[48], bands[63])
	}

	resp, err := NewSuccessResponse(AudioDataResponse{
		Bands: bands,
	})
	if err != nil {
		return NewErrorResponse("internal error")
	}

	return resp
}

func (s *Server) handleGetConfig() *Response {
	log.Printf("[CONFIG] Get config requested")
	cfg := s.configMgr.Get()

	resp, err := NewSuccessResponse(ConfigResponse{
		ConfigPath:       s.configMgr.GetPath(),
		LibraryPaths:     cfg.LibraryPaths,
		SampleRate:       cfg.Audio.SampleRate,
		BufferSizeMs:     cfg.Audio.BufferSizeMs,
		DefaultVolume:    cfg.Audio.DefaultVolume,
		ResumeOnStart:    cfg.Behavior.ResumeOnStart,
		RememberQueue:    cfg.Behavior.RememberQueue,
		RememberPosition: cfg.Behavior.RememberPosition,
	})
	if err != nil {
		return NewErrorResponse("internal error")
	}

	return resp
}

func (s *Server) handleScanLibrary(ctx context.Context) *Response {
	cfg := s.configMgr.Get()

	if len(cfg.LibraryPaths) == 0 {
		log.Printf("[SCANNER] No library paths configured")
		return NewErrorResponse("no library paths configured")
	}

	// Check if scan is already running
	if s.libScanner.IsRunning() {
		log.Printf("[SCANNER] Scan already in progress")
		// Return current status instead of error
		return s.handleGetScanStatus()
	}

	log.Printf("[SCANNER] Starting async library scan for %d paths: %v", len(cfg.LibraryPaths), cfg.LibraryPaths)

	// Start async scan - returns immediately
	started := s.libScanner.ScanPathsAsync(ctx, cfg.LibraryPaths, true)
	if !started {
		return NewErrorResponse("failed to start scan")
	}

	// Return status response indicating scan has started
	resp, err := NewSuccessResponse(ScanStatusResponse{
		Status:   "scanning",
		Progress: 0,
		Message:  "Scan started",
	})
	if err != nil {
		return NewErrorResponse("internal error")
	}

	return resp
}

func (s *Server) handleGetScanStatus() *Response {
	status := s.libScanner.GetStatus()

	// If scan is complete, include the results
	var scanResp *ScanResponse
	if status.Status == "complete" {
		results, metadata := s.libScanner.GetLastResults()
		
		// Convert scanner results to IPC response format
		ipcResults := make([]ScanResult, 0, len(results))
		totalFiles := 0

		for _, sr := range results {
			files := make([]ScanFileInfo, 0, len(sr.Files))
			for _, f := range sr.Files {
				fileInfo := ScanFileInfo{
					Path:       f.Path,
					Size:       f.Size,
					ModifiedAt: f.ModifiedAt,
				}
				// Include metadata if available
				if f.Metadata != nil {
					fileInfo.Metadata = &ScanFileMetadata{
						Title:    f.Metadata.Title,
						Artist:   f.Metadata.Artist,
						Album:    f.Metadata.Album,
						Duration: f.Metadata.Duration,
					}
				}
				files = append(files, fileInfo)
			}

			ipcResults = append(ipcResults, ScanResult{
				LibraryPath: sr.LibraryPath,
				Files:       files,
				TotalFiles:  sr.TotalFiles,
				ScanTimeMs:  sr.ScanTimeMs,
				Error:       sr.Error,
			})

			totalFiles += sr.TotalFiles
		}

		// Convert metadata
		var ipcMetadata *ScanMetadata
		if metadata != nil {
			allArtists := []ArtistNFO{}
			allAlbums := []AlbumNFO{}

			for _, a := range metadata.Artists {
				allArtists = append(allArtists, ArtistNFO{
					Name:          a.Name,
					SortName:      a.SortName,
					MusicBrainzID: a.MusicBrainzID,
					Rating:        a.Rating,
					Biography:     a.Biography,
					Genres:        a.Genre,
					Styles:        a.Style,
					Path:          a.Path,
				})
			}

			for _, a := range metadata.Albums {
				allAlbums = append(allAlbums, AlbumNFO{
					Title:              a.Title,
					Artist:             a.Artist,
					MusicBrainzAlbumID: a.MusicBrainzAlbumID,
					Year:               a.Year,
					Rating:             a.Rating,
					Genres:             a.Genre,
					Label:              a.Label,
					Path:               a.Path,
					AlbumPath:          a.AlbumPath,
				})
			}

			if len(allArtists) > 0 || len(allAlbums) > 0 || len(metadata.Artwork) > 0 {
				ipcMetadata = &ScanMetadata{
					Artists: allArtists,
					Albums:  allAlbums,
					Artwork: metadata.Artwork,
				}
			}
		}

		scanResp = &ScanResponse{
			Results:    ipcResults,
			TotalFiles: totalFiles,
			Metadata:   ipcMetadata,
		}

		log.Printf("[SCANNER] Scan complete: %d files", totalFiles)

		// Clear results after fetching
		s.libScanner.ClearResults()
	}

	resp, err := NewSuccessResponse(ScanStatusResponse{
		Status:   status.Status,
		Progress: status.Progress,
		Message:  status.Message,
		Results:  scanResp,
	})
	if err != nil {
		return NewErrorResponse("internal error")
	}

	return resp
}

func (s *Server) handleSetConfig(req *Request) *Response {
	log.Printf("[CONFIG] Set config requested")
	var cfgReq ConfigRequest
	if err := json.Unmarshal(req.Data, &cfgReq); err != nil {
		return NewErrorResponse("invalid config request")
	}

	cfg := s.configMgr.Get()

	// Update fields if provided
	if cfgReq.LibraryPaths != nil {
		cfg.LibraryPaths = *cfgReq.LibraryPaths
	}
	if cfgReq.SampleRate != nil {
		cfg.Audio.SampleRate = *cfgReq.SampleRate
	}
	if cfgReq.BufferSizeMs != nil {
		cfg.Audio.BufferSizeMs = *cfgReq.BufferSizeMs
	}
	if cfgReq.DefaultVolume != nil {
		cfg.Audio.DefaultVolume = *cfgReq.DefaultVolume
	}
	if cfgReq.ResumeOnStart != nil {
		cfg.Behavior.ResumeOnStart = *cfgReq.ResumeOnStart
	}
	if cfgReq.RememberQueue != nil {
		cfg.Behavior.RememberQueue = *cfgReq.RememberQueue
	}
	if cfgReq.RememberPosition != nil {
		cfg.Behavior.RememberPosition = *cfgReq.RememberPosition
	}

	// Save the updated config
	if err := s.configMgr.Update(cfg); err != nil {
		log.Printf("[CONFIG] Failed to save config: %v", err)
		return NewErrorResponse(fmt.Sprintf("failed to save config: %v", err))
	}

	log.Printf("[CONFIG] Config updated and saved")
	return s.handleGetConfig()
}

func (s *Server) handleGetQueue() *Response {
	log.Printf("[QUEUE] Get queue requested")

	items := s.queueMgr.GetItems()
	idx, _ := s.queueMgr.Position()

	// Convert to IPC format
	ipcItems := make([]QueueItem, len(items))
	for i, item := range items {
		ipcItems[i] = QueueItem{Path: item.Path}
		if item.Metadata != nil {
			ipcItems[i].Metadata = &TrackMetadata{
				Title:    item.Metadata.Title,
				Artist:   item.Metadata.Artist,
				Album:    item.Metadata.Album,
				Duration: item.Metadata.Duration,
				ArtPath:  item.Metadata.ArtPath,
			}
		}
	}

	// Get repeat mode as string
	repeatMode := "off"
	switch s.queueMgr.GetRepeat() {
	case queue.RepeatOne:
		repeatMode = "one"
	case queue.RepeatAll:
		repeatMode = "all"
	}

	resp, err := NewSuccessResponse(GetQueueResponse{
		Items:      ipcItems,
		Index:      idx,
		RepeatMode: repeatMode,
		Shuffle:    s.queueMgr.GetShuffle(),
	})
	if err != nil {
		return NewErrorResponse("internal error")
	}
	return resp
}

func (s *Server) handleSetRepeat(req *Request) *Response {
	var repeatReq SetRepeatRequest
	if err := json.Unmarshal(req.Data, &repeatReq); err != nil {
		return NewErrorResponse("invalid setRepeat request")
	}

	log.Printf("[QUEUE] Set repeat mode to: %s", repeatReq.Mode)

	var mode queue.RepeatMode
	var loopStatus media.LoopStatus
	switch repeatReq.Mode {
	case "one":
		mode = queue.RepeatOne
		loopStatus = media.LoopTrack
	case "all":
		mode = queue.RepeatAll
		loopStatus = media.LoopPlaylist
	default:
		mode = queue.RepeatOff
		loopStatus = media.LoopNone
	}

	s.queueMgr.SetRepeat(mode)

	// Update OS media session
	if err := s.player.UpdateLoopStatus(loopStatus); err != nil {
		log.Printf("[QUEUE] Failed to update media session loop status: %v", err)
	}

	return s.handleStatus()
}

func (s *Server) handleSetShuffle(req *Request) *Response {
	var shuffleReq SetShuffleRequest
	if err := json.Unmarshal(req.Data, &shuffleReq); err != nil {
		return NewErrorResponse("invalid setShuffle request")
	}

	log.Printf("[QUEUE] Set shuffle to: %v", shuffleReq.Enabled)
	s.queueMgr.SetShuffle(shuffleReq.Enabled)

	// Update OS media session
	if err := s.player.UpdateShuffle(shuffleReq.Enabled); err != nil {
		log.Printf("[QUEUE] Failed to update media session shuffle: %v", err)
	}

	return s.handleStatus()
}

func (s *Server) handleQueueJump(ctx context.Context, req *Request) *Response {
	var jumpReq QueueJumpRequest
	if err := json.Unmarshal(req.Data, &jumpReq); err != nil {
		return NewErrorResponse("invalid queueJump request")
	}

	log.Printf("[QUEUE] Jump to index: %d", jumpReq.Index)

	if !s.queueMgr.SetIndex(jumpReq.Index) {
		return NewErrorResponse("invalid queue index")
	}

	// Get the current item and start playing it
	path, metadata := s.queueMgr.Current()
	if path == "" {
		return NewErrorResponse("no track at index")
	}

	var audioMeta *audio.TrackMetadata
	if metadata != nil {
		audioMeta = &audio.TrackMetadata{
			Title:    metadata.Title,
			Artist:   metadata.Artist,
			Album:    metadata.Album,
			Duration: metadata.Duration,
			ArtPath:  metadata.ArtPath,
		}
	}

	if err := s.player.Play(ctx, path, audioMeta); err != nil {
		return NewErrorResponse(err.Error())
	}

	return s.handleStatus()
}

func (s *Server) handleQueueRemove(req *Request) *Response {
	var removeReq QueueRemoveRequest
	if err := json.Unmarshal(req.Data, &removeReq); err != nil {
		return NewErrorResponse("invalid queueRemove request")
	}

	log.Printf("[QUEUE] Remove item at index: %d", removeReq.Index)

	if !s.queueMgr.Remove(removeReq.Index) {
		return NewErrorResponse("invalid queue index")
	}

	return s.handleStatus()
}

func (s *Server) handleQueueMove(req *Request) *Response {
	var moveReq QueueMoveRequest
	if err := json.Unmarshal(req.Data, &moveReq); err != nil {
		return NewErrorResponse("invalid queueMove request")
	}

	log.Printf("[QUEUE] Move item from %d to %d", moveReq.FromIndex, moveReq.ToIndex)

	if !s.queueMgr.Move(moveReq.FromIndex, moveReq.ToIndex) {
		return NewErrorResponse("invalid queue indices")
	}

	return s.handleStatus()
}

func (s *Server) sendResponse(conn net.Conn, resp *Response) error {
	data, err := EncodeResponse(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

func (s *Server) sendError(conn net.Conn, msg string) {
	s.sendResponse(conn, NewErrorResponse(msg))
}

// Audio data subscription handlers

func (s *Server) handleSubscribeAudioData(conn net.Conn) *Response {
	s.audioSubsMu.Lock()
	s.audioSubs[conn] = true
	count := len(s.audioSubs)
	s.audioSubsMu.Unlock()
	
	log.Printf("[AUDIO] Client subscribed to audio data (total: %d)", count)
	
	resp, _ := NewSuccessResponse(map[string]bool{"subscribed": true})
	return resp
}

func (s *Server) handleUnsubscribeAudioData(conn net.Conn) *Response {
	s.audioSubsMu.Lock()
	delete(s.audioSubs, conn)
	count := len(s.audioSubs)
	s.audioSubsMu.Unlock()
	
	log.Printf("[AUDIO] Client unsubscribed from audio data (remaining: %d)", count)
	
	resp, _ := NewSuccessResponse(map[string]bool{"subscribed": false})
	return resp
}

// pushAudioDataImmediate is called directly by the audio analyzer callback
// This provides true real-time push with zero latency (no polling/timer)
func (s *Server) pushAudioDataImmediate(bandsU8 []uint8) {
	s.audioSubsMu.RLock()
	if len(s.audioSubs) == 0 {
		s.audioSubsMu.RUnlock()
		return
	}
	
	// Copy subscriber list to avoid holding lock during I/O
	subs := make([]net.Conn, 0, len(s.audioSubs))
	for conn := range s.audioSubs {
		subs = append(subs, conn)
	}
	s.audioSubsMu.RUnlock()
	
	// Convert []uint8 to []int for JSON
	bands := make([]int, len(bandsU8))
	for i, b := range bandsU8 {
		bands[i] = int(b)
	}
	
	// Get current playback position for sync (Position is already in ms)
	status := s.player.Status()
	position := status.Position
	timestamp := time.Now().UnixMilli()
	
	// Create push message with position for sync
	msgBytes, err := NewPushMessage("audioData", AudioDataResponse{
		Bands:     bands,
		Position:  position,
		Timestamp: timestamp,
	})
	if err != nil {
		return
	}
	msgBytes = append(msgBytes, '\n')
	
	// Send to all subscribers immediately
	for _, conn := range subs {
		_, err := conn.Write(msgBytes)
		if err != nil {
			// Remove failed connection from subscribers
			s.audioSubsMu.Lock()
			delete(s.audioSubs, conn)
			s.audioSubsMu.Unlock()
		}
	}
}
