package queue

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	idx, size := m.Position()
	if idx != -1 {
		t.Errorf("Expected index -1, got %d", idx)
	}
	if size != 0 {
		t.Errorf("Expected size 0, got %d", size)
	}
}

func TestSet(t *testing.T) {
	m := NewManager()

	paths := []string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"}
	m.Set(paths)

	idx, size := m.Position()
	if idx != -1 {
		t.Errorf("Expected index -1 after Set, got %d", idx)
	}
	if size != 3 {
		t.Errorf("Expected size 3, got %d", size)
	}

	items := m.GetItems()
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
}

func TestAppend(t *testing.T) {
	m := NewManager()

	m.Set([]string{"/path/1.mp3"})
	m.Append([]string{"/path/2.mp3", "/path/3.mp3"})

	_, size := m.Position()
	if size != 3 {
		t.Errorf("Expected size 3, got %d", size)
	}
}

func TestNext(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"})

	// First Next should return first track
	path, _ := m.Next()
	if path != "/path/1.mp3" {
		t.Errorf("Expected /path/1.mp3, got %s", path)
	}

	idx, _ := m.Position()
	if idx != 0 {
		t.Errorf("Expected index 0, got %d", idx)
	}

	// Second Next
	path, _ = m.Next()
	if path != "/path/2.mp3" {
		t.Errorf("Expected /path/2.mp3, got %s", path)
	}

	// Third Next
	path, _ = m.Next()
	if path != "/path/3.mp3" {
		t.Errorf("Expected /path/3.mp3, got %s", path)
	}

	// Fourth Next should return empty (end of queue)
	path, _ = m.Next()
	if path != "" {
		t.Errorf("Expected empty path at end of queue, got %s", path)
	}
}

func TestPrev(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"})

	// Move to end
	m.Next() // 0
	m.Next() // 1
	m.Next() // 2

	// Prev should go back
	path, _ := m.Prev()
	if path != "/path/2.mp3" {
		t.Errorf("Expected /path/2.mp3, got %s", path)
	}

	path, _ = m.Prev()
	if path != "/path/1.mp3" {
		t.Errorf("Expected /path/1.mp3, got %s", path)
	}

	// Prev at beginning should return empty
	path, _ = m.Prev()
	if path != "" {
		t.Errorf("Expected empty path at beginning, got %s", path)
	}
}

func TestCurrent(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3"})

	// Before any Next, Current should return empty
	path, _ := m.Current()
	if path != "" {
		t.Errorf("Expected empty path before navigation, got %s", path)
	}

	m.Next()
	path, _ = m.Current()
	if path != "/path/1.mp3" {
		t.Errorf("Expected /path/1.mp3, got %s", path)
	}
}

func TestSetIndex(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"})

	// Set to valid index
	if !m.SetIndex(1) {
		t.Error("SetIndex(1) should succeed")
	}

	path, _ := m.Current()
	if path != "/path/2.mp3" {
		t.Errorf("Expected /path/2.mp3, got %s", path)
	}

	// Set to invalid index
	if m.SetIndex(-1) {
		t.Error("SetIndex(-1) should fail")
	}

	if m.SetIndex(10) {
		t.Error("SetIndex(10) should fail")
	}
}

func TestClear(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3"})
	m.Next()

	m.Clear()

	idx, size := m.Position()
	if idx != -1 {
		t.Errorf("Expected index -1 after Clear, got %d", idx)
	}
	if size != 0 {
		t.Errorf("Expected size 0 after Clear, got %d", size)
	}
}

func TestRepeatAll(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3"})
	m.SetRepeat(RepeatAll)

	m.Next()            // 0
	m.Next()            // 1
	path, _ := m.Next() // Should wrap to 0

	if path != "/path/1.mp3" {
		t.Errorf("Expected /path/1.mp3 with RepeatAll, got %s", path)
	}
}

func TestRepeatOne(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3"})
	m.SetRepeat(RepeatOne)

	m.Next() // 0

	// Next should return same track
	path, _ := m.Next()
	if path != "/path/1.mp3" {
		t.Errorf("Expected /path/1.mp3 with RepeatOne, got %s", path)
	}
}

func TestRemove(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"})
	m.Next() // index 0
	m.Next() // index 1

	// Remove track before current - index should adjust
	m.Remove(0)

	idx, size := m.Position()
	if idx != 0 {
		t.Errorf("Expected index 0 after remove, got %d", idx)
	}
	if size != 2 {
		t.Errorf("Expected size 2 after remove, got %d", size)
	}

	path, _ := m.Current()
	if path != "/path/2.mp3" {
		t.Errorf("Expected /path/2.mp3, got %s", path)
	}
}

func TestInsert(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/3.mp3"})
	m.Next() // index 0

	// Insert at index 1
	m.Insert(1, "/path/2.mp3", nil)

	_, size := m.Position()
	if size != 3 {
		t.Errorf("Expected size 3 after insert, got %d", size)
	}

	items := m.GetItems()
	if items[1].Path != "/path/2.mp3" {
		t.Errorf("Expected /path/2.mp3 at index 1, got %s", items[1].Path)
	}
}

func TestSetWithMetadata(t *testing.T) {
	m := NewManager()

	items := []QueueItem{
		{Path: "/path/1.mp3", Metadata: &TrackMetadata{Title: "Track 1"}},
		{Path: "/path/2.mp3", Metadata: &TrackMetadata{Title: "Track 2"}},
	}
	m.SetWithMetadata(items)

	m.Next()
	path, meta := m.Current()

	if path != "/path/1.mp3" {
		t.Errorf("Expected /path/1.mp3, got %s", path)
	}

	if meta == nil {
		t.Fatal("Expected metadata, got nil")
	}

	if meta.Title != "Track 1" {
		t.Errorf("Expected title 'Track 1', got '%s'", meta.Title)
	}
}

func TestShuffleGetSet(t *testing.T) {
	m := NewManager()

	if m.GetShuffle() {
		t.Error("Shuffle should be off by default")
	}

	m.SetShuffle(true)
	if !m.GetShuffle() {
		t.Error("Shuffle should be on after SetShuffle(true)")
	}

	m.SetShuffle(false)
	if m.GetShuffle() {
		t.Error("Shuffle should be off after SetShuffle(false)")
	}
}

func TestRepeatGetSet(t *testing.T) {
	m := NewManager()

	if m.GetRepeat() != RepeatOff {
		t.Error("Repeat should be off by default")
	}

	m.SetRepeat(RepeatOne)
	if m.GetRepeat() != RepeatOne {
		t.Error("Repeat should be RepeatOne")
	}

	m.SetRepeat(RepeatAll)
	if m.GetRepeat() != RepeatAll {
		t.Error("Repeat should be RepeatAll")
	}
}

func TestShuffleOrder(t *testing.T) {
	m := NewManager()
	paths := []string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3", "/path/4.mp3", "/path/5.mp3"}
	m.Set(paths)

	// Enable shuffle
	m.SetShuffle(true)

	// Collect all paths by navigating through
	visited := make(map[string]bool)
	for i := 0; i < len(paths); i++ {
		path, _ := m.Next()
		if path == "" {
			t.Fatalf("Got empty path after %d Next() calls", i+1)
		}
		visited[path] = true
	}

	// Verify all tracks are reachable
	if len(visited) != len(paths) {
		t.Errorf("Expected %d unique paths, got %d", len(paths), len(visited))
	}
}

func TestShuffleMaintainsCurrentTrack(t *testing.T) {
	m := NewManager()
	paths := []string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3", "/path/4.mp3"}
	m.Set(paths)

	// Navigate to second track
	m.Next() // index 0
	m.Next() // index 1

	currentPath, _ := m.Current()
	if currentPath != "/path/2.mp3" {
		t.Fatalf("Expected /path/2.mp3 before shuffle, got %s", currentPath)
	}

	// Enable shuffle - current track should remain the same
	m.SetShuffle(true)

	afterShufflePath, _ := m.Current()
	if afterShufflePath != "/path/2.mp3" {
		t.Errorf("Expected current track to stay as /path/2.mp3, got %s", afterShufflePath)
	}
}

func TestShuffleDisableMaintainsCurrentTrack(t *testing.T) {
	m := NewManager()
	paths := []string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3", "/path/4.mp3"}
	m.Set(paths)

	// Navigate and enable shuffle
	m.Next() // index 0
	m.SetShuffle(true)
	m.Next() // random next

	currentPath, _ := m.Current()
	if currentPath == "" {
		t.Fatal("Expected a current path")
	}

	// Disable shuffle
	m.SetShuffle(false)

	afterPath, _ := m.Current()
	if afterPath != currentPath {
		t.Errorf("Expected current track to remain %s, got %s", currentPath, afterPath)
	}
}

func TestMove(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3", "/path/3.mp3"})
	m.Next() // index 0

	// Move item from index 2 to index 0
	if !m.Move(2, 0) {
		t.Error("Move should succeed")
	}

	items := m.GetItems()
	if items[0].Path != "/path/3.mp3" {
		t.Errorf("Expected /path/3.mp3 at index 0, got %s", items[0].Path)
	}
	if items[1].Path != "/path/1.mp3" {
		t.Errorf("Expected /path/1.mp3 at index 1, got %s", items[1].Path)
	}
	if items[2].Path != "/path/2.mp3" {
		t.Errorf("Expected /path/2.mp3 at index 2, got %s", items[2].Path)
	}
}

func TestMoveInvalidIndex(t *testing.T) {
	m := NewManager()
	m.Set([]string{"/path/1.mp3", "/path/2.mp3"})

	if m.Move(-1, 0) {
		t.Error("Move with negative from index should fail")
	}
	if m.Move(0, 5) {
		t.Error("Move with out-of-bounds to index should fail")
	}
}

func TestOnChange(t *testing.T) {
	m := NewManager()

	callCount := 0
	m.SetOnChange(func() {
		callCount++
	})

	m.Set([]string{"/path/1.mp3"})
	if callCount != 1 {
		t.Errorf("Expected 1 onChange call after Set, got %d", callCount)
	}

	m.Next()
	if callCount != 2 {
		t.Errorf("Expected 2 onChange calls after Next, got %d", callCount)
	}

	m.SetRepeat(RepeatAll)
	if callCount != 3 {
		t.Errorf("Expected 3 onChange calls after SetRepeat, got %d", callCount)
	}
}
