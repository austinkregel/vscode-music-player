// Package main is the entry point for the musicd daemon.
// musicd is a headless audio playback daemon that integrates with OS media sessions
// and communicates with clients (like the VS Code extension) via IPC.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/austinkregel/local-media/musicd/internal/audio"
	"github.com/austinkregel/local-media/musicd/internal/auth"
	"github.com/austinkregel/local-media/musicd/internal/config"
	"github.com/austinkregel/local-media/musicd/internal/ipc"
	"github.com/austinkregel/local-media/musicd/internal/media"
	"github.com/austinkregel/local-media/musicd/internal/queue"
)

// Version is set at build time via ldflags
var Version = "dev"

// Config holds daemon configuration
type Config struct {
	SocketPath string
	ConfigDir  string
	TestMode   bool
	Verbose    bool
}

func main() {
	cfg := parseFlags()

	if cfg.Verbose {
		log.Printf("musicd version %s starting...", Version)
	}

	// Create context that cancels on interrupt signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	if err := run(ctx, cfg); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.SocketPath, "socket", "", "IPC socket path (default: auto-generated based on UID)")
	flag.StringVar(&cfg.ConfigDir, "config", "", "Configuration directory (default: ~/.config/musicd)")
	flag.BoolVar(&cfg.TestMode, "test-mode", false, "Run in test mode (auto-approve pairing)")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	// Set defaults
	if cfg.ConfigDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home directory: %v", err)
		}
		cfg.ConfigDir = homeDir + "/.config/musicd"
	}

	if cfg.SocketPath == "" {
		cfg.SocketPath = fmt.Sprintf("/tmp/musicd-%d.sock", os.Getuid())
	}

	return cfg
}

func run(ctx context.Context, cfg *Config) error {
	// Ensure config directory exists
	if err := os.MkdirAll(cfg.ConfigDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Initialize config manager
	configMgr := config.NewManager(cfg.ConfigDir)
	if err := configMgr.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize components
	authStore, err := auth.NewStore(cfg.ConfigDir + "/clients.json")
	if err != nil {
		return fmt.Errorf("failed to initialize auth store: %w", err)
	}

	authManager := auth.NewManager(authStore, cfg.TestMode)

	// Initialize media session (platform-specific)
	mediaSession, err := media.NewSession()
	if err != nil {
		log.Printf("[MEDIA] Warning: failed to initialize media session: %v", err)
		log.Printf("[MEDIA] Continuing without OS media integration")
		// Continue without media session - not fatal
		mediaSession = media.NewNoOpSession()
	} else {
		log.Printf("[MEDIA] Media session initialized successfully")
	}

	// Initialize audio player
	player, err := audio.NewPlayer(mediaSession)
	if err != nil {
		return fmt.Errorf("failed to initialize audio player: %w", err)
	}
	defer player.Close()

	// Connect media session commands to the player
	mediaSession.SetCommandHandler(player)
	log.Printf("[MEDIA] Connected media session commands to player")

	// Initialize queue manager
	queueMgr := queue.NewManager()

	// Initialize queue persistence if configured
	daemonCfg := configMgr.Get()
	var queueStore *queue.Store
	if daemonCfg.Behavior.RememberQueue {
		queueStore = queue.NewStore(cfg.ConfigDir, queueMgr)

		// Load saved queue
		if err := queueStore.Load(); err != nil {
			log.Printf("[QUEUE] Warning: failed to load saved queue: %v", err)
		} else {
			idx, size := queueMgr.Position()
			if size > 0 {
				log.Printf("[QUEUE] Loaded saved queue: %d items, position %d", size, idx)
			}
		}

		// Set up auto-save on queue changes
		queueMgr.SetOnChange(func() {
			if err := queueStore.Save(); err != nil {
				log.Printf("[QUEUE] Warning: failed to save queue: %v", err)
			}
		})
	}

	// Initialize IPC server
	server, err := ipc.NewServer(cfg.SocketPath, authManager, configMgr, player, queueMgr, mediaSession)
	if err != nil {
		return fmt.Errorf("failed to initialize IPC server: %w", err)
	}

	// Start the IPC server
	log.Printf("Starting IPC server on %s", cfg.SocketPath)
	if err := server.Start(ctx); err != nil {
		// Save queue before exiting if persistence is enabled
		if queueStore != nil {
			if saveErr := queueStore.Save(); saveErr != nil {
				log.Printf("[QUEUE] Warning: failed to save queue on shutdown: %v", saveErr)
			} else {
				log.Printf("[QUEUE] Queue saved on shutdown")
			}
		}
		return fmt.Errorf("IPC server error: %w", err)
	}

	// Save queue on clean shutdown
	if queueStore != nil {
		if saveErr := queueStore.Save(); saveErr != nil {
			log.Printf("[QUEUE] Warning: failed to save queue on shutdown: %v", saveErr)
		} else {
			log.Printf("[QUEUE] Queue saved on shutdown")
		}
	}

	return nil
}
