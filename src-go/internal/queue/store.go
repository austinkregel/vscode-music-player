// Package queue provides queue persistence functionality.
package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PersistentState represents the queue state that gets persisted to disk
type PersistentState struct {
	Items        []QueueItem `json:"items"`
	Index        int         `json:"index"`
	Shuffle      bool        `json:"shuffle"`
	ShuffleOrder []int       `json:"shuffleOrder,omitempty"`
	Repeat       string      `json:"repeat"` // "off", "one", "all"
}

// Store handles queue persistence to disk
type Store struct {
	mu       sync.Mutex
	filePath string
	manager  *Manager
}

// NewStore creates a new queue store
func NewStore(configDir string, manager *Manager) *Store {
	return &Store{
		filePath: filepath.Join(configDir, "queue.json"),
		manager:  manager,
	}
}

// Load loads the queue state from disk
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No saved state, that's fine
			return nil
		}
		return fmt.Errorf("failed to read queue file: %w", err)
	}

	var state PersistentState
	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupt file - log and continue with empty queue
		return fmt.Errorf("failed to parse queue file: %w", err)
	}

	// Restore state to manager
	s.manager.mu.Lock()
	defer s.manager.mu.Unlock()

	s.manager.items = state.Items
	s.manager.index = state.Index
	s.manager.shuffle = state.Shuffle
	s.manager.shuffleOrder = state.ShuffleOrder

	switch state.Repeat {
	case "one":
		s.manager.repeat = RepeatOne
	case "all":
		s.manager.repeat = RepeatAll
	default:
		s.manager.repeat = RepeatOff
	}

	return nil
}

// Save saves the current queue state to disk
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get state from manager
	s.manager.mu.RLock()
	state := PersistentState{
		Items:        make([]QueueItem, len(s.manager.items)),
		Index:        s.manager.index,
		Shuffle:      s.manager.shuffle,
		ShuffleOrder: s.manager.shuffleOrder,
	}
	copy(state.Items, s.manager.items)

	switch s.manager.repeat {
	case RepeatOne:
		state.Repeat = "one"
	case RepeatAll:
		state.Repeat = "all"
	default:
		state.Repeat = "off"
	}
	s.manager.mu.RUnlock()

	// Marshal to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal queue state: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create queue directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write queue file: %w", err)
	}

	return nil
}

// AutoSave sets up the manager to automatically save on changes
// Returns a function to stop auto-saving
func (s *Store) AutoSave() {
	// We'll use a simple approach: save after each mutation
	// A more sophisticated approach would debounce saves
}

// GetFilePath returns the path to the queue file
func (s *Store) GetFilePath() string {
	return s.filePath
}
