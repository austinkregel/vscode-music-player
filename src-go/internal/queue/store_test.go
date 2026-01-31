package queue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoadSaveRoundtrip(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "queue_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create manager with some data (without shuffle to test basic roundtrip)
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"})
	m.Next() // index 0
	m.Next() // index 1
	m.SetRepeat(RepeatAll)

	// Create store and save
	store := NewStore(tmpDir, m)
	if err := store.Save(); err != nil {
		t.Fatalf("Failed to save: %v", err)
	}

	// Verify file exists
	queueFile := filepath.Join(tmpDir, "queue.json")
	if _, err := os.Stat(queueFile); os.IsNotExist(err) {
		t.Fatal("Queue file was not created")
	}

	// Create new manager and load
	m2 := NewManager()
	store2 := NewStore(tmpDir, m2)
	if err := store2.Load(); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Verify state
	idx, size := m2.Position()
	if size != 3 {
		t.Errorf("Expected size 3, got %d", size)
	}
	if idx != 1 {
		t.Errorf("Expected index 1, got %d", idx)
	}
	if m2.GetRepeat() != RepeatAll {
		t.Error("Expected RepeatAll mode")
	}
}

func TestStoreLoadSaveWithShuffle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "queue_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"})
	m.Next()           // index 0
	m.Next()           // index 1 (on /path/2.mp3)
	m.SetShuffle(true) // index becomes 0 (current track moved to position 0 in shuffle)

	// Get the current track before saving
	currentPath, _ := m.Current()

	store := NewStore(tmpDir, m)
	if err := store.Save(); err != nil {
		t.Fatalf("Failed to save: %v", err)
	}

	// Load into new manager
	m2 := NewManager()
	store2 := NewStore(tmpDir, m2)
	if err := store2.Load(); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Verify shuffle state preserved
	if !m2.GetShuffle() {
		t.Error("Expected shuffle enabled")
	}

	// Verify current track is the same
	loadedPath, _ := m2.Current()
	if loadedPath != currentPath {
		t.Errorf("Expected current track %s, got %s", currentPath, loadedPath)
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "queue_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager()
	store := NewStore(tmpDir, m)

	// Load with no file should not error
	if err := store.Load(); err != nil {
		t.Errorf("Load with missing file should not error, got: %v", err)
	}

	// Manager should be empty
	_, size := m.Position()
	if size != 0 {
		t.Errorf("Expected empty queue, got size %d", size)
	}
}

func TestStoreLoadCorruptFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "queue_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write corrupt file
	queueFile := filepath.Join(tmpDir, "queue.json")
	if err := os.WriteFile(queueFile, []byte("not valid json"), 0600); err != nil {
		t.Fatalf("Failed to write corrupt file: %v", err)
	}

	m := NewManager()
	store := NewStore(tmpDir, m)

	// Load should return error
	if err := store.Load(); err == nil {
		t.Error("Load with corrupt file should return error")
	}
}

func TestStoreSaveWithMetadata(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "queue_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager()
	m.SetWithMetadata([]QueueItem{
		{Path: "/path/1.mp3", Metadata: &TrackMetadata{Title: "Track 1", Artist: "Artist 1"}},
		{Path: "/path/2.mp3", Metadata: &TrackMetadata{Title: "Track 2", Artist: "Artist 2"}},
	})
	m.Next()

	store := NewStore(tmpDir, m)
	if err := store.Save(); err != nil {
		t.Fatalf("Failed to save: %v", err)
	}

	// Load into new manager
	m2 := NewManager()
	store2 := NewStore(tmpDir, m2)
	if err := store2.Load(); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	items := m2.GetItems()
	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}

	if items[0].Metadata == nil || items[0].Metadata.Title != "Track 1" {
		t.Error("Expected metadata to be preserved")
	}
}
