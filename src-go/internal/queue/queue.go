// Package queue manages the playback queue.
package queue

import (
	"math/rand"
	"sync"
	"time"
)

// TrackMetadata contains metadata for a queued track
type TrackMetadata struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int64  `json:"duration,omitempty"`
	ArtPath  string `json:"artPath,omitempty"`
}

// QueueItem represents an item in the playback queue
type QueueItem struct {
	Path     string
	Metadata *TrackMetadata
}

// ChangeCallback is called when the queue state changes
type ChangeCallback func()

// SimilarityProvider is called to get similar tracks when continue mode is enabled
type SimilarityProvider func(trackPath string, exclude []string) string

// Manager manages the playback queue
type Manager struct {
	mu           sync.RWMutex
	items        []QueueItem
	index        int // Current position in items (or shuffleOrder if shuffled)
	shuffle      bool
	shuffleOrder []int // Shuffled indices into items
	repeat       RepeatMode
	rng          *rand.Rand
	onChange     ChangeCallback // Called when queue state changes

	// Continue mode settings
	continueMode       ContinueMode
	recentlyPlayed     []string // Track paths recently played (for exclusion)
	maxRecentlyPlayed  int      // Max items to keep in recentlyPlayed
	similarityProvider SimilarityProvider
}

// RepeatMode represents the repeat behavior
type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatOne
	RepeatAll
)

// ContinueMode represents what happens when the queue is exhausted
type ContinueMode int

const (
	ContinueOff     ContinueMode = iota
	ContinueSimilar              // Play similar to last track
	ContinueRandom               // Play random track from library
)

// NewManager creates a new queue manager
func NewManager() *Manager {
	return &Manager{
		items:             make([]QueueItem, 0),
		index:             -1,
		repeat:            RepeatOff,
		shuffleOrder:      make([]int, 0),
		rng:               rand.New(rand.NewSource(time.Now().UnixNano())),
		continueMode:      ContinueOff,
		recentlyPlayed:    make([]string, 0),
		maxRecentlyPlayed: 50,
	}
}

// SetOnChange sets a callback to be called when the queue state changes
func (m *Manager) SetOnChange(callback ChangeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = callback
}

// notifyChange calls the onChange callback if set (must be called without lock held)
func (m *Manager) notifyChange() {
	m.mu.RLock()
	callback := m.onChange
	m.mu.RUnlock()
	if callback != nil {
		callback()
	}
}

// Set replaces the entire queue with new paths
func (m *Manager) Set(paths []string) {
	m.mu.Lock()

	m.items = make([]QueueItem, len(paths))
	for i, path := range paths {
		m.items[i] = QueueItem{Path: path}
	}
	m.index = -1

	// Regenerate shuffle order if shuffle is enabled
	if m.shuffle {
		m.generateShuffleOrder()
	}

	m.mu.Unlock()
	m.notifyChange()
}

// SetWithMetadata replaces the queue with paths and metadata
func (m *Manager) SetWithMetadata(items []QueueItem) {
	m.mu.Lock()

	m.items = make([]QueueItem, len(items))
	copy(m.items, items)
	m.index = -1

	// Regenerate shuffle order if shuffle is enabled
	if m.shuffle {
		m.generateShuffleOrder()
	}

	m.mu.Unlock()
	m.notifyChange()
}

// Append adds paths to the end of the queue
func (m *Manager) Append(paths []string) {
	m.mu.Lock()

	for _, path := range paths {
		m.items = append(m.items, QueueItem{Path: path})
	}

	// Add new items to shuffle order if shuffle is enabled
	if m.shuffle {
		m.appendToShuffleOrder(len(paths))
	}

	m.mu.Unlock()
	m.notifyChange()
}

// AppendWithMetadata adds items with metadata to the queue
func (m *Manager) AppendWithMetadata(items []QueueItem) {
	m.mu.Lock()

	m.items = append(m.items, items...)

	// Add new items to shuffle order if shuffle is enabled
	if m.shuffle {
		m.appendToShuffleOrder(len(items))
	}

	m.mu.Unlock()
	m.notifyChange()
}

// appendToShuffleOrder adds new item indices to the shuffle order in random positions
func (m *Manager) appendToShuffleOrder(count int) {
	startIdx := len(m.items) - count
	for i := 0; i < count; i++ {
		newIdx := startIdx + i
		// Insert at random position after current index
		insertPos := m.index + 1 + m.rng.Intn(len(m.shuffleOrder)-m.index)
		if insertPos > len(m.shuffleOrder) {
			insertPos = len(m.shuffleOrder)
		}
		m.shuffleOrder = append(m.shuffleOrder[:insertPos], append([]int{newIdx}, m.shuffleOrder[insertPos:]...)...)
	}
}

// Clear clears the queue
func (m *Manager) Clear() {
	m.mu.Lock()

	m.items = make([]QueueItem, 0)
	m.shuffleOrder = make([]int, 0)
	m.index = -1

	m.mu.Unlock()
	m.notifyChange()
}

// Next moves to the next track and returns it
func (m *Manager) Next() (string, *TrackMetadata) {
	m.mu.Lock()

	if len(m.items) == 0 {
		m.mu.Unlock()
		return "", nil
	}

	// Handle repeat one - return current track
	if m.repeat == RepeatOne && m.index >= 0 {
		itemIdx := m.getItemIndex(m.index)
		if itemIdx >= 0 && itemIdx < len(m.items) {
			item := m.items[itemIdx]
			m.mu.Unlock()
			return item.Path, item.Metadata
		}
	}

	// Move to next position
	m.index++

	// Handle end of queue
	maxIndex := m.getMaxIndex()
	if m.index >= maxIndex {
		if m.repeat == RepeatAll {
			m.index = 0
			// Re-shuffle when looping back if shuffle is enabled
			if m.shuffle {
				m.generateShuffleOrder()
			}
		} else {
			m.index = maxIndex - 1
			m.mu.Unlock()
			return "", nil
		}
	}

	itemIdx := m.getItemIndex(m.index)
	if itemIdx < 0 || itemIdx >= len(m.items) {
		m.mu.Unlock()
		return "", nil
	}
	item := m.items[itemIdx]
	m.mu.Unlock()
	m.notifyChange()
	return item.Path, item.Metadata
}

// Prev moves to the previous track and returns it
func (m *Manager) Prev() (string, *TrackMetadata) {
	m.mu.Lock()

	if len(m.items) == 0 {
		m.mu.Unlock()
		return "", nil
	}

	// Handle repeat one - return current track
	if m.repeat == RepeatOne && m.index >= 0 {
		itemIdx := m.getItemIndex(m.index)
		if itemIdx >= 0 && itemIdx < len(m.items) {
			item := m.items[itemIdx]
			m.mu.Unlock()
			return item.Path, item.Metadata
		}
	}

	// Move to previous position
	m.index--

	// Handle beginning of queue
	if m.index < 0 {
		if m.repeat == RepeatAll {
			m.index = m.getMaxIndex() - 1
		} else {
			m.index = 0
			m.mu.Unlock()
			return "", nil
		}
	}

	itemIdx := m.getItemIndex(m.index)
	if itemIdx < 0 || itemIdx >= len(m.items) {
		m.mu.Unlock()
		return "", nil
	}
	item := m.items[itemIdx]
	m.mu.Unlock()
	m.notifyChange()
	return item.Path, item.Metadata
}

// getItemIndex returns the actual item index for the given position index
// If shuffle is enabled, it looks up the shuffled order
func (m *Manager) getItemIndex(posIndex int) int {
	if !m.shuffle || len(m.shuffleOrder) == 0 {
		return posIndex
	}
	if posIndex < 0 || posIndex >= len(m.shuffleOrder) {
		return -1
	}
	return m.shuffleOrder[posIndex]
}

// getMaxIndex returns the maximum valid index
func (m *Manager) getMaxIndex() int {
	if m.shuffle && len(m.shuffleOrder) > 0 {
		return len(m.shuffleOrder)
	}
	return len(m.items)
}

// generateShuffleOrder creates a new shuffled order of indices
func (m *Manager) generateShuffleOrder() {
	n := len(m.items)
	m.shuffleOrder = make([]int, n)
	for i := 0; i < n; i++ {
		m.shuffleOrder[i] = i
	}
	// Fisher-Yates shuffle
	for i := n - 1; i > 0; i-- {
		j := m.rng.Intn(i + 1)
		m.shuffleOrder[i], m.shuffleOrder[j] = m.shuffleOrder[j], m.shuffleOrder[i]
	}
}

// Current returns the current track
func (m *Manager) Current() (string, *TrackMetadata) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.index < 0 {
		return "", nil
	}

	itemIdx := m.getItemIndex(m.index)
	if itemIdx < 0 || itemIdx >= len(m.items) {
		return "", nil
	}

	item := m.items[itemIdx]
	return item.Path, item.Metadata
}

// SetIndex sets the current queue index
func (m *Manager) SetIndex(index int) bool {
	m.mu.Lock()

	if index < 0 || index >= len(m.items) {
		m.mu.Unlock()
		return false
	}

	m.index = index
	m.mu.Unlock()
	m.notifyChange()
	return true
}

// Position returns the current index and queue size
func (m *Manager) Position() (int, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.index, len(m.items)
}

// GetItems returns all items in the queue
func (m *Manager) GetItems() []QueueItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]QueueItem, len(m.items))
	copy(items, m.items)
	return items
}

// SetShuffle enables or disables shuffle mode
func (m *Manager) SetShuffle(enabled bool) {
	m.mu.Lock()

	wasEnabled := m.shuffle
	m.shuffle = enabled

	if enabled && !wasEnabled {
		// Just enabled shuffle - generate shuffle order
		m.generateShuffleOrder()

		// If we have a current track, find it in the shuffle order and move it to position 0
		// so the user continues from where they were
		if m.index >= 0 && m.index < len(m.items) {
			currentItemIdx := m.index
			for i, idx := range m.shuffleOrder {
				if idx == currentItemIdx {
					// Swap with position 0 so current track is first
					m.shuffleOrder[0], m.shuffleOrder[i] = m.shuffleOrder[i], m.shuffleOrder[0]
					break
				}
			}
			m.index = 0 // Now at position 0 in shuffle order
		}
	} else if !enabled && wasEnabled {
		// Just disabled shuffle - restore normal order
		// Find current item in shuffle order and set index to its actual position
		if m.index >= 0 && m.index < len(m.shuffleOrder) {
			m.index = m.shuffleOrder[m.index]
		}
		m.shuffleOrder = nil
	}

	m.mu.Unlock()
	m.notifyChange()
}

// GetShuffle returns whether shuffle is enabled
func (m *Manager) GetShuffle() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.shuffle
}

// SetRepeat sets the repeat mode
func (m *Manager) SetRepeat(mode RepeatMode) {
	m.mu.Lock()
	m.repeat = mode
	m.mu.Unlock()
	m.notifyChange()
}

// GetRepeat returns the current repeat mode
func (m *Manager) GetRepeat() RepeatMode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.repeat
}

// Remove removes an item at the specified index (actual item index, not shuffle position)
func (m *Manager) Remove(index int) bool {
	m.mu.Lock()

	if index < 0 || index >= len(m.items) {
		m.mu.Unlock()
		return false
	}

	m.items = append(m.items[:index], m.items[index+1:]...)

	// Update shuffle order if enabled
	if m.shuffle && len(m.shuffleOrder) > 0 {
		// Remove the index from shuffle order and adjust remaining indices
		newOrder := make([]int, 0, len(m.shuffleOrder)-1)
		removedPos := -1
		for i, idx := range m.shuffleOrder {
			if idx == index {
				removedPos = i
				continue
			}
			if idx > index {
				newOrder = append(newOrder, idx-1)
			} else {
				newOrder = append(newOrder, idx)
			}
		}
		m.shuffleOrder = newOrder

		// Adjust current position if the removed item was before current
		if removedPos >= 0 && removedPos < m.index {
			m.index--
		} else if removedPos == m.index && m.index >= len(m.shuffleOrder) {
			m.index = len(m.shuffleOrder) - 1
		}
	} else {
		// Non-shuffle mode: adjust current index if needed
		if index < m.index {
			m.index--
		} else if index == m.index {
			// Current track was removed, stay at same index (which is now the next track)
			if m.index >= len(m.items) {
				m.index = len(m.items) - 1
			}
		}
	}

	m.mu.Unlock()
	m.notifyChange()
	return true
}

// Insert inserts an item at the specified index (actual item index, not shuffle position)
func (m *Manager) Insert(index int, path string, metadata *TrackMetadata) bool {
	m.mu.Lock()

	if index < 0 || index > len(m.items) {
		m.mu.Unlock()
		return false
	}

	item := QueueItem{Path: path, Metadata: metadata}

	// Insert at index
	m.items = append(m.items[:index], append([]QueueItem{item}, m.items[index:]...)...)

	// Update shuffle order if enabled
	if m.shuffle && len(m.shuffleOrder) > 0 {
		// Adjust existing indices that are >= the insert index
		for i := range m.shuffleOrder {
			if m.shuffleOrder[i] >= index {
				m.shuffleOrder[i]++
			}
		}
		// Add the new index at a random position after current
		insertPos := m.index + 1 + m.rng.Intn(len(m.shuffleOrder)-m.index)
		if insertPos > len(m.shuffleOrder) {
			insertPos = len(m.shuffleOrder)
		}
		m.shuffleOrder = append(m.shuffleOrder[:insertPos], append([]int{index}, m.shuffleOrder[insertPos:]...)...)
	} else {
		// Non-shuffle mode: adjust current index if needed
		if index <= m.index {
			m.index++
		}
	}

	m.mu.Unlock()
	m.notifyChange()
	return true
}

// Move moves an item from one index to another
func (m *Manager) Move(fromIndex, toIndex int) bool {
	m.mu.Lock()

	if fromIndex < 0 || fromIndex >= len(m.items) {
		m.mu.Unlock()
		return false
	}
	if toIndex < 0 || toIndex >= len(m.items) {
		m.mu.Unlock()
		return false
	}
	if fromIndex == toIndex {
		m.mu.Unlock()
		return true
	}

	// Remove item at fromIndex
	item := m.items[fromIndex]
	m.items = append(m.items[:fromIndex], m.items[fromIndex+1:]...)

	// Insert at toIndex (adjusted if needed)
	if toIndex > fromIndex {
		toIndex--
	}
	m.items = append(m.items[:toIndex], append([]QueueItem{item}, m.items[toIndex:]...)...)

	// In shuffle mode, we don't change the shuffle order - just the underlying items
	// In non-shuffle mode, adjust current index
	if !m.shuffle {
		if m.index == fromIndex {
			m.index = toIndex
		} else if fromIndex < m.index && toIndex >= m.index {
			m.index--
		} else if fromIndex > m.index && toIndex <= m.index {
			m.index++
		}
	}

	m.mu.Unlock()
	m.notifyChange()
	return true
}

// SetContinueMode sets the queue continuation mode
func (m *Manager) SetContinueMode(mode ContinueMode) {
	m.mu.Lock()
	m.continueMode = mode
	m.mu.Unlock()
	m.notifyChange()
}

// GetContinueMode returns the current continue mode
func (m *Manager) GetContinueMode() ContinueMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.continueMode
}

// SetSimilarityProvider sets the function used to find similar tracks
func (m *Manager) SetSimilarityProvider(provider SimilarityProvider) {
	m.mu.Lock()
	m.similarityProvider = provider
	m.mu.Unlock()
}

// AddToRecentlyPlayed adds a track to the recently played list
func (m *Manager) AddToRecentlyPlayed(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already in list
	for _, p := range m.recentlyPlayed {
		if p == path {
			return
		}
	}

	m.recentlyPlayed = append(m.recentlyPlayed, path)

	// Trim to max size
	if len(m.recentlyPlayed) > m.maxRecentlyPlayed {
		m.recentlyPlayed = m.recentlyPlayed[len(m.recentlyPlayed)-m.maxRecentlyPlayed:]
	}
}

// GetRecentlyPlayed returns the list of recently played tracks
func (m *Manager) GetRecentlyPlayed() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, len(m.recentlyPlayed))
	copy(result, m.recentlyPlayed)
	return result
}

// ClearRecentlyPlayed clears the recently played list
func (m *Manager) ClearRecentlyPlayed() {
	m.mu.Lock()
	m.recentlyPlayed = make([]string, 0)
	m.mu.Unlock()
}

// SetMaxRecentlyPlayed sets the maximum number of tracks to keep in the history
func (m *Manager) SetMaxRecentlyPlayed(max int) {
	m.mu.Lock()
	m.maxRecentlyPlayed = max
	if len(m.recentlyPlayed) > max {
		m.recentlyPlayed = m.recentlyPlayed[len(m.recentlyPlayed)-max:]
	}
	m.mu.Unlock()
}

// TryGetSimilarTrack attempts to get a similar track when the queue is exhausted
// Returns empty string if no similar track found or continue mode is off
func (m *Manager) TryGetSimilarTrack() string {
	m.mu.RLock()
	mode := m.continueMode
	provider := m.similarityProvider
	var lastTrack string
	if m.index >= 0 && len(m.items) > 0 {
		itemIdx := m.getItemIndex(m.index)
		if itemIdx >= 0 && itemIdx < len(m.items) {
			lastTrack = m.items[itemIdx].Path
		}
	}
	exclude := make([]string, len(m.recentlyPlayed))
	copy(exclude, m.recentlyPlayed)
	m.mu.RUnlock()

	if mode != ContinueSimilar || provider == nil || lastTrack == "" {
		return ""
	}

	return provider(lastTrack, exclude)
}
