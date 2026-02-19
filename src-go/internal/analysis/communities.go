package analysis

import (
	"fmt"
	"math"
	"sort"
)

// CommunityDetector implements the Louvain algorithm for community detection
type CommunityDetector struct {
	store  *FeatureStore
	engine *SimilarityEngine
}

// NewCommunityDetector creates a new community detector
func NewCommunityDetector(store *FeatureStore, engine *SimilarityEngine) *CommunityDetector {
	return &CommunityDetector{
		store:  store,
		engine: engine,
	}
}

// DetectCommunities runs the Louvain algorithm to find communities
func (d *CommunityDetector) DetectCommunities() []CommunityInfo {
	allFeatures := d.store.GetAllFeatures()
	if len(allFeatures) < 2 {
		return nil
	}

	// Build node list
	nodes := make([]string, 0, len(allFeatures))
	for path := range allFeatures {
		nodes = append(nodes, path)
	}

	// Initialize: each node in its own community
	community := make(map[string]int)
	for i, node := range nodes {
		community[node] = i
	}

	// Build adjacency map from stored edges
	adjacency := make(map[string][]SimilarityEdge)
	for _, node := range nodes {
		adjacency[node] = d.store.GetSimilarTracks(node, DefaultTopK)
	}

	// Compute total edge weight
	var totalWeight float64
	for _, edges := range adjacency {
		for _, e := range edges {
			totalWeight += float64(e.Weight)
		}
	}
	totalWeight /= 2 // Each edge counted twice

	// Run Louvain phases
	improved := true
	iteration := 0
	maxIterations := 10

	for improved && iteration < maxIterations {
		improved = false
		iteration++

		// Phase 1: Local moving
		for _, node := range nodes {
			currentCommunity := community[node]
			bestCommunity := currentCommunity
			bestGain := 0.0

			// Compute current modularity contribution
			nodeDegree := d.computeNodeDegree(node, adjacency)

			// Find neighboring communities
			neighborCommunities := make(map[int]bool)
			for _, edge := range adjacency[node] {
				neighborCommunities[community[edge.TargetPath]] = true
			}

			// Try moving to each neighboring community
			for comm := range neighborCommunities {
				if comm == currentCommunity {
					continue
				}

				gain := d.computeModularityGain(node, comm, community, adjacency, nodeDegree, totalWeight)
				if gain > bestGain {
					bestGain = gain
					bestCommunity = comm
				}
			}

			if bestCommunity != currentCommunity {
				community[node] = bestCommunity
				improved = true
			}
		}
	}

	// Renumber communities to be sequential
	communityMap := make(map[int]int)
	nextID := 0
	for _, comm := range community {
		if _, exists := communityMap[comm]; !exists {
			communityMap[comm] = nextID
			nextID++
		}
	}
	for node := range community {
		community[node] = communityMap[community[node]]
	}

	// Store community assignments
	for node, comm := range community {
		centrality := d.computeCentrality(node, comm, community, adjacency)
		bridgeScore := d.computeBridgeScore(node, community, adjacency)

		d.store.StoreCommunity(node, &TrackCommunity{
			CommunityID: comm,
			Centrality:  centrality,
			BridgeScore: bridgeScore,
		})
	}

	// Generate community info
	communityInfo := d.generateCommunityInfo(nodes, community, allFeatures)
	d.store.StoreCommunityInfo(communityInfo)

	return communityInfo
}

// computeNodeDegree computes the sum of edge weights for a node
func (d *CommunityDetector) computeNodeDegree(node string, adjacency map[string][]SimilarityEdge) float64 {
	var degree float64
	for _, edge := range adjacency[node] {
		degree += float64(edge.Weight)
	}
	return degree
}

// computeModularityGain computes the gain from moving a node to a community
func (d *CommunityDetector) computeModularityGain(
	node string,
	targetComm int,
	community map[string]int,
	adjacency map[string][]SimilarityEdge,
	nodeDegree float64,
	totalWeight float64,
) float64 {
	// Sum of weights to target community
	var sumIn float64
	for _, edge := range adjacency[node] {
		if community[edge.TargetPath] == targetComm {
			sumIn += float64(edge.Weight)
		}
	}

	// Sum of degrees in target community
	var commDegree float64
	for n, c := range community {
		if c == targetComm {
			commDegree += d.computeNodeDegree(n, adjacency)
		}
	}

	// Modularity gain formula
	if totalWeight == 0 {
		return 0
	}
	m2 := 2 * totalWeight
	gain := sumIn/totalWeight - (commDegree*nodeDegree)/(m2*m2)
	return gain
}

// computeCentrality computes how central a node is within its community
func (d *CommunityDetector) computeCentrality(
	node string,
	comm int,
	community map[string]int,
	adjacency map[string][]SimilarityEdge,
) float32 {
	// Count edges to same community vs total
	var sameComm, total float64
	for _, edge := range adjacency[node] {
		total += float64(edge.Weight)
		if community[edge.TargetPath] == comm {
			sameComm += float64(edge.Weight)
		}
	}

	if total == 0 {
		return 0
	}
	return float32(sameComm / total)
}

// computeBridgeScore computes how much a node bridges different communities
func (d *CommunityDetector) computeBridgeScore(
	node string,
	community map[string]int,
	adjacency map[string][]SimilarityEdge,
) float32 {
	// Count connections to different communities
	commConnections := make(map[int]float64)
	var total float64

	for _, edge := range adjacency[node] {
		comm := community[edge.TargetPath]
		commConnections[comm] += float64(edge.Weight)
		total += float64(edge.Weight)
	}

	if total == 0 || len(commConnections) <= 1 {
		return 0
	}

	// Bridge score = entropy of community distribution
	var entropy float64
	for _, weight := range commConnections {
		p := weight / total
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	// Normalize to 0-1 (max entropy = log2(numCommunities))
	maxEntropy := math.Log2(float64(len(commConnections)))
	if maxEntropy == 0 {
		return 0
	}
	return float32(entropy / maxEntropy)
}

// generateCommunityInfo generates descriptive info for each community
func (d *CommunityDetector) generateCommunityInfo(
	nodes []string,
	community map[string]int,
	allFeatures map[string]*StoredFeatures,
) []CommunityInfo {
	// Group tracks by community
	commTracks := make(map[int][]string)
	for _, node := range nodes {
		comm := community[node]
		commTracks[comm] = append(commTracks[comm], node)
	}

	// Generate info for each community
	var result []CommunityInfo
	for commID, tracks := range commTracks {
		if len(tracks) == 0 {
			continue
		}

		// Compute average features
		avgFeatures := d.computeAverageFeatures(tracks, allFeatures)

		// Generate descriptive name
		name := d.generateCommunityName(avgFeatures, len(tracks))

		// Get top distinguishing features
		topFeatures := d.getTopFeatures(avgFeatures)

		result = append(result, CommunityInfo{
			ID:          commID,
			Name:        name,
			TrackCount:  len(tracks),
			TopFeatures: topFeatures,
		})
	}

	// Sort by track count
	sort.Slice(result, func(i, j int) bool {
		return result[i].TrackCount > result[j].TrackCount
	})

	return result
}

// computeAverageFeatures computes average features for a set of tracks
func (d *CommunityDetector) computeAverageFeatures(
	tracks []string,
	allFeatures map[string]*StoredFeatures,
) *AudioFeatures {
	if len(tracks) == 0 {
		return &AudioFeatures{}
	}

	avg := &AudioFeatures{}
	count := float32(0)

	for _, path := range tracks {
		sf, ok := allFeatures[path]
		if !ok || sf.Features == nil {
			continue
		}
		f := sf.Features
		count++

		// Accumulate scalar features
		avg.SpectralCentroid += f.SpectralCentroid
		avg.SpectralRolloff += f.SpectralRolloff
		avg.SpectralFlux += f.SpectralFlux
		avg.ZeroCrossing += f.ZeroCrossing
		avg.RMSEnergy += f.RMSEnergy
		avg.Tempo += f.Tempo
		avg.BassRatio += f.BassRatio
		avg.MidRatio += f.MidRatio
		avg.TrebleRatio += f.TrebleRatio
		avg.AttackSharpness += f.AttackSharpness
		avg.HarmonicDensity += f.HarmonicDensity
		avg.RhythmComplexity += f.RhythmComplexity
		avg.DynamicRange += f.DynamicRange

		// Instrument profile
		avg.Instruments.BrassLike += f.Instruments.BrassLike
		avg.Instruments.StringLike += f.Instruments.StringLike
		avg.Instruments.WoodwindLike += f.Instruments.WoodwindLike
		avg.Instruments.Percussive += f.Instruments.Percussive
		avg.Instruments.SynthPad += f.Instruments.SynthPad
		avg.Instruments.VocalPresence += f.Instruments.VocalPresence
		avg.Instruments.ArticulationStyle += f.Instruments.ArticulationStyle
		avg.Instruments.EnsembleSize += f.Instruments.EnsembleSize
		avg.Instruments.PlayingIntensity += f.Instruments.PlayingIntensity

		// MFCCs
		for i := 0; i < numMFCC; i++ {
			avg.MFCC[i] += f.MFCC[i]
		}
	}

	if count == 0 {
		return avg
	}

	// Divide by count
	avg.SpectralCentroid /= count
	avg.SpectralRolloff /= count
	avg.SpectralFlux /= count
	avg.ZeroCrossing /= count
	avg.RMSEnergy /= count
	avg.Tempo /= count
	avg.BassRatio /= count
	avg.MidRatio /= count
	avg.TrebleRatio /= count
	avg.AttackSharpness /= count
	avg.HarmonicDensity /= count
	avg.RhythmComplexity /= count
	avg.DynamicRange /= count

	avg.Instruments.BrassLike /= count
	avg.Instruments.StringLike /= count
	avg.Instruments.WoodwindLike /= count
	avg.Instruments.Percussive /= count
	avg.Instruments.SynthPad /= count
	avg.Instruments.VocalPresence /= count
	avg.Instruments.ArticulationStyle /= count
	avg.Instruments.EnsembleSize /= count
	avg.Instruments.PlayingIntensity /= count

	for i := 0; i < numMFCC; i++ {
		avg.MFCC[i] /= count
	}

	return avg
}

// generateCommunityName generates a descriptive name based on features
func (d *CommunityDetector) generateCommunityName(f *AudioFeatures, trackCount int) string {
	var parts []string

	// Tempo descriptor
	if f.Tempo > 140 {
		parts = append(parts, "High-tempo")
	} else if f.Tempo < 80 {
		parts = append(parts, "Slow")
	}

	// Energy descriptor
	if f.RMSEnergy > 0.5 {
		parts = append(parts, "energetic")
	} else if f.RMSEnergy < 0.2 {
		parts = append(parts, "mellow")
	}

	// Instrument descriptors
	dominantInstr := ""
	maxInstr := float32(0.3) // Minimum threshold

	if f.Instruments.BrassLike > maxInstr {
		dominantInstr = "brass"
		maxInstr = f.Instruments.BrassLike
	}
	if f.Instruments.StringLike > maxInstr {
		dominantInstr = "strings"
		maxInstr = f.Instruments.StringLike
	}
	if f.Instruments.Percussive > maxInstr {
		dominantInstr = "percussion"
		maxInstr = f.Instruments.Percussive
	}
	if f.Instruments.SynthPad > maxInstr {
		dominantInstr = "synth"
		maxInstr = f.Instruments.SynthPad
	}
	if f.Instruments.VocalPresence > maxInstr {
		dominantInstr = "vocal"
	}

	if dominantInstr != "" {
		parts = append(parts, dominantInstr)
	}

	// Frequency balance
	if f.BassRatio > 0.4 {
		parts = append(parts, "bass-heavy")
	} else if f.TrebleRatio > 0.3 {
		parts = append(parts, "bright")
	}

	// Build name
	if len(parts) == 0 {
		return fmt.Sprintf("Community %d", trackCount)
	}

	// Combine parts
	name := ""
	for i, part := range parts {
		if i == 0 {
			name = part
		} else if i == 1 {
			name = name + " " + part
		} else {
			name = name + ", " + part
		}
	}

	return name
}

// getTopFeatures returns the most distinguishing features
func (d *CommunityDetector) getTopFeatures(f *AudioFeatures) []string {
	type featureScore struct {
		name  string
		value float32
	}

	features := []featureScore{
		{"high-tempo", f.Tempo / 200},
		{"low-tempo", 1 - f.Tempo/200},
		{"brass", f.Instruments.BrassLike},
		{"strings", f.Instruments.StringLike},
		{"percussion", f.Instruments.Percussive},
		{"synth", f.Instruments.SynthPad},
		{"vocals", f.Instruments.VocalPresence},
		{"bass-heavy", f.BassRatio},
		{"bright", f.TrebleRatio},
		{"dynamic", f.DynamicRange},
		{"complex-rhythm", f.RhythmComplexity},
	}

	// Sort by value
	sort.Slice(features, func(i, j int) bool {
		return features[i].value > features[j].value
	})

	// Return top 3
	var result []string
	for i := 0; i < 3 && i < len(features); i++ {
		if features[i].value > 0.3 { // Only include significant features
			result = append(result, features[i].name)
		}
	}

	return result
}
