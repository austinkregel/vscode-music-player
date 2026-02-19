// Package config handles daemon configuration file management.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the daemon configuration
type Config struct {
	// LibraryPaths is a list of directories containing music files
	LibraryPaths []string `json:"libraryPaths"`

	// DataDir is where to store data files (analysis, cache, etc.)
	DataDir string `json:"dataDir"`

	// Audio settings
	Audio AudioConfig `json:"audio"`

	// Behavior settings
	Behavior BehaviorConfig `json:"behavior"`
}

// AudioConfig contains audio-related settings
type AudioConfig struct {
	// SampleRate for audio output (default: 44100)
	SampleRate int `json:"sampleRate"`

	// BufferSize in milliseconds (default: 100)
	BufferSizeMs int `json:"bufferSizeMs"`

	// Volume level 0.0 - 1.0 (default: 1.0)
	DefaultVolume float64 `json:"defaultVolume"`
}

// BehaviorConfig contains behavior-related settings
type BehaviorConfig struct {
	// ResumeOnStart - resume last playing track on daemon start
	ResumeOnStart bool `json:"resumeOnStart"`

	// RememberQueue - persist queue across restarts
	RememberQueue bool `json:"rememberQueue"`

	// RememberPosition - remember playback position
	RememberPosition bool `json:"rememberPosition"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		LibraryPaths: []string{},
		Audio: AudioConfig{
			SampleRate:    44100,
			BufferSizeMs:  100,
			DefaultVolume: 1.0,
		},
		Behavior: BehaviorConfig{
			ResumeOnStart:    false,
			RememberQueue:    true,
			RememberPosition: true,
		},
	}
}

// Manager handles loading and saving configuration
type Manager struct {
	configDir  string
	configPath string
	config     *Config
}

// NewManager creates a new configuration manager
func NewManager(configDir string) *Manager {
	return &Manager{
		configDir:  configDir,
		configPath: filepath.Join(configDir, "config.json"),
		config:     DefaultConfig(),
	}
}

// Load reads the configuration from disk
func (m *Manager) Load() error {
	// Ensure config directory exists
	if err := os.MkdirAll(m.configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Check if config file exists
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		// Create default config
		m.config = DefaultConfig()
		return m.Save()
	}

	// Read existing config
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Parse JSON
	config := DefaultConfig() // Start with defaults
	if err := json.Unmarshal(data, config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	m.config = config
	return nil
}

// Save writes the configuration to disk
func (m *Manager) Save() error {
	// Ensure config directory exists
	if err := os.MkdirAll(m.configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(m.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Get returns the current configuration
func (m *Manager) Get() *Config {
	return m.config
}

// GetPath returns the config file path
func (m *Manager) GetPath() string {
	return m.configPath
}

// Update updates the configuration and saves it
func (m *Manager) Update(config *Config) error {
	m.config = config
	return m.Save()
}

// SetLibraryPaths updates the library paths
func (m *Manager) SetLibraryPaths(paths []string) error {
	m.config.LibraryPaths = paths
	return m.Save()
}

// AddLibraryPath adds a library path
func (m *Manager) AddLibraryPath(path string) error {
	// Check if already exists
	for _, p := range m.config.LibraryPaths {
		if p == path {
			return nil // Already exists
		}
	}

	m.config.LibraryPaths = append(m.config.LibraryPaths, path)
	return m.Save()
}

// RemoveLibraryPath removes a library path
func (m *Manager) RemoveLibraryPath(path string) error {
	paths := make([]string, 0, len(m.config.LibraryPaths))
	for _, p := range m.config.LibraryPaths {
		if p != path {
			paths = append(paths, p)
		}
	}
	m.config.LibraryPaths = paths
	return m.Save()
}
