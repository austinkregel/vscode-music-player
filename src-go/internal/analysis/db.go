package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FeatureStore stores audio features and similarity data
type FeatureStore struct {
	mu       sync.RWMutex
	dataPath string

	// In-memory cache
	features    map[string]*StoredFeatures
	edges       map[string][]SimilarityEdge
	communities map[string]*TrackCommunity
	communityInfo []CommunityInfo
}

// StoredFeatures contains features with metadata
type StoredFeatures struct {
	Features   *AudioFeatures `json:"features"`
	Version    int            `json:"version"`
	AnalyzedAt int64          `json:"analyzedAt"`
	FileHash   string         `json:"fileHash"`
}

// SimilarityEdge represents a similarity connection
type SimilarityEdge struct {
	TargetPath string  `json:"targetPath"`
	Weight     float32 `json:"weight"`
}

// TrackCommunity contains community assignment for a track
type TrackCommunity struct {
	CommunityID int     `json:"communityId"`
	Centrality  float32 `json:"centrality"`
	BridgeScore float32 `json:"bridgeScore"`
}

// CommunityInfo contains information about a detected community
type CommunityInfo struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	TrackCount  int      `json:"trackCount"`
	TopFeatures []string `json:"topFeatures"`
}

// NewFeatureStore creates a new feature store
func NewFeatureStore(dataDir string) (*FeatureStore, error) {
	dataPath := filepath.Join(dataDir, "audio_analysis.json")

	store := &FeatureStore{
		dataPath:    dataPath,
		features:    make(map[string]*StoredFeatures),
		edges:       make(map[string][]SimilarityEdge),
		communities: make(map[string]*TrackCommunity),
	}

	// Load existing data
	if err := store.load(); err != nil {
		// Not an error if file doesn't exist
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load store: %w", err)
		}
	}

	return store, nil
}

// load reads stored data from disk
func (s *FeatureStore) load() error {
	data, err := os.ReadFile(s.dataPath)
	if err != nil {
		return err
	}

	var stored struct {
		Features    map[string]*StoredFeatures   `json:"features"`
		Edges       map[string][]SimilarityEdge  `json:"edges"`
		Communities map[string]*TrackCommunity   `json:"communities"`
		CommunityInfo []CommunityInfo            `json:"communityInfo"`
	}

	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	s.features = stored.Features
	s.edges = stored.Edges
	s.communities = stored.Communities
	s.communityInfo = stored.CommunityInfo

	if s.features == nil {
		s.features = make(map[string]*StoredFeatures)
	}
	if s.edges == nil {
		s.edges = make(map[string][]SimilarityEdge)
	}
	if s.communities == nil {
		s.communities = make(map[string]*TrackCommunity)
	}

	return nil
}

// Save writes data to disk
func (s *FeatureStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored := struct {
		Features    map[string]*StoredFeatures   `json:"features"`
		Edges       map[string][]SimilarityEdge  `json:"edges"`
		Communities map[string]*TrackCommunity   `json:"communities"`
		CommunityInfo []CommunityInfo            `json:"communityInfo"`
	}{
		Features:    s.features,
		Edges:       s.edges,
		Communities: s.communities,
		CommunityInfo: s.communityInfo,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.dataPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(s.dataPath, data, 0600); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// StoreFeatures stores features for a track
func (s *FeatureStore) StoreFeatures(trackPath string, features *AudioFeatures, version int, fileHash string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.features[trackPath] = &StoredFeatures{
		Features:   features,
		Version:    version,
		AnalyzedAt: unixNow(),
		FileHash:   fileHash,
	}
}

// GetFeatures retrieves features for a track
func (s *FeatureStore) GetFeatures(trackPath string) (*StoredFeatures, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, ok := s.features[trackPath]
	return f, ok
}

// HasFeatures checks if a track has stored features
func (s *FeatureStore) HasFeatures(trackPath string, minVersion int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, ok := s.features[trackPath]
	return ok && f.Version >= minVersion
}

// GetAllFeatures returns all stored features
func (s *FeatureStore) GetAllFeatures() map[string]*StoredFeatures {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*StoredFeatures, len(s.features))
	for k, v := range s.features {
		result[k] = v
	}
	return result
}

// GetAnalyzedCount returns the number of analyzed tracks
func (s *FeatureStore) GetAnalyzedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.features)
}

// StoreSimilarityEdges stores similarity edges for a track
func (s *FeatureStore) StoreSimilarityEdges(trackPath string, edges []SimilarityEdge) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edges[trackPath] = edges
}

// GetSimilarTracks returns similar tracks
func (s *FeatureStore) GetSimilarTracks(trackPath string, limit int) []SimilarityEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()

	edges, ok := s.edges[trackPath]
	if !ok {
		return nil
	}

	if len(edges) <= limit {
		return edges
	}
	return edges[:limit]
}

// StoreCommunity stores community assignment for a track
func (s *FeatureStore) StoreCommunity(trackPath string, community *TrackCommunity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.communities[trackPath] = community
}

// GetCommunity returns community assignment for a track
func (s *FeatureStore) GetCommunity(trackPath string) (*TrackCommunity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.communities[trackPath]
	return c, ok
}

// StoreCommunityInfo stores information about all communities
func (s *FeatureStore) StoreCommunityInfo(info []CommunityInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.communityInfo = info
}

// GetCommunities returns all community information
func (s *FeatureStore) GetCommunities() []CommunityInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.communityInfo
}

// GetTracksInCommunity returns all tracks in a community
func (s *FeatureStore) GetTracksInCommunity(communityID int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tracks []string
	for path, c := range s.communities {
		if c.CommunityID == communityID {
			tracks = append(tracks, path)
		}
	}
	return tracks
}

// GetBridgeTracks returns tracks with high bridge scores
func (s *FeatureStore) GetBridgeTracks(minScore float32) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tracks []string
	for path, c := range s.communities {
		if c.BridgeScore >= minScore {
			tracks = append(tracks, path)
		}
	}
	return tracks
}

// ClearAll clears all stored data
func (s *FeatureStore) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.features = make(map[string]*StoredFeatures)
	s.edges = make(map[string][]SimilarityEdge)
	s.communities = make(map[string]*TrackCommunity)
	s.communityInfo = nil
}

func unixNow() int64 {
	return time.Now().Unix()
}
