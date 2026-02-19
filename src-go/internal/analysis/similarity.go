package analysis

import (
	"math"
	"sort"
)

const (
	// Feature extraction version
	FeatureVersion = 1

	// Default number of similar tracks to store per track
	DefaultTopK = 20

	// Minimum similarity threshold for edge storage
	MinSimilarityThreshold = 0.3
)

// FeatureWeights defines the importance of each feature group
type FeatureWeights struct {
	MFCC        float32 // Timbre (most important for "vibe")
	Tempo       float32 // Rhythm feel
	Spectral    float32 // Brightness/dynamics
	Energy      float32 // Loudness profile
	Bands       float32 // Bass/mid/treble balance
	Instruments float32 // Instrument presence
	Context     float32 // Playing style
}

// DefaultWeights returns the default feature weights
func DefaultWeights() FeatureWeights {
	return FeatureWeights{
		MFCC:        0.25,
		Tempo:       0.15,
		Spectral:    0.15,
		Energy:      0.10,
		Bands:       0.10,
		Instruments: 0.15,
		Context:     0.10,
	}
}

// SimilarityEngine computes and queries track similarity
type SimilarityEngine struct {
	store   *FeatureStore
	weights FeatureWeights
	topK    int
}

// NewSimilarityEngine creates a new similarity engine
func NewSimilarityEngine(store *FeatureStore) *SimilarityEngine {
	return &SimilarityEngine{
		store:   store,
		weights: DefaultWeights(),
		topK:    DefaultTopK,
	}
}

// SetWeights updates the feature weights
func (e *SimilarityEngine) SetWeights(w FeatureWeights) {
	e.weights = w
}

// ComputeSimilarity computes similarity between two feature sets
// Returns a value between 0 (different) and 1 (identical)
func (e *SimilarityEngine) ComputeSimilarity(a, b *AudioFeatures) float32 {
	if a == nil || b == nil {
		return 0
	}

	var totalDistance float32
	var totalWeight float32

	// 1. MFCC distance (normalized euclidean)
	mfccDist := e.mfccDistance(a, b)
	totalDistance += mfccDist * e.weights.MFCC
	totalWeight += e.weights.MFCC

	// 2. Tempo distance (normalized)
	tempoDist := e.tempoDistance(a.Tempo, b.Tempo)
	totalDistance += tempoDist * e.weights.Tempo
	totalWeight += e.weights.Tempo

	// 3. Spectral features distance
	spectralDist := e.spectralDistance(a, b)
	totalDistance += spectralDist * e.weights.Spectral
	totalWeight += e.weights.Spectral

	// 4. Energy distance
	energyDist := abs32(a.RMSEnergy - b.RMSEnergy)
	totalDistance += energyDist * e.weights.Energy
	totalWeight += e.weights.Energy

	// 5. Band ratio distance
	bandsDist := e.bandsDistance(a, b)
	totalDistance += bandsDist * e.weights.Bands
	totalWeight += e.weights.Bands

	// 6. Instrument profile distance
	instrDist := e.instrumentDistance(a, b)
	totalDistance += instrDist * e.weights.Instruments
	totalWeight += e.weights.Instruments

	// 7. Context distance
	contextDist := e.contextDistance(a, b)
	totalDistance += contextDist * e.weights.Context
	totalWeight += e.weights.Context

	// Convert distance to similarity
	if totalWeight == 0 {
		return 0
	}
	avgDistance := totalDistance / totalWeight
	similarity := 1 - avgDistance

	// Clamp to [0, 1]
	if similarity < 0 {
		return 0
	}
	if similarity > 1 {
		return 1
	}
	return similarity
}

// mfccDistance computes normalized distance between MFCC vectors
func (e *SimilarityEngine) mfccDistance(a, b *AudioFeatures) float32 {
	var sumSq float32
	for i := 0; i < numMFCC; i++ {
		diff := a.MFCC[i] - b.MFCC[i]
		sumSq += diff * diff
	}
	// Normalize to 0-1 range (typical MFCC values are -20 to 20)
	dist := float32(math.Sqrt(float64(sumSq))) / float32(numMFCC*20)
	if dist > 1 {
		dist = 1
	}
	return dist
}

// tempoDistance computes normalized tempo distance
func (e *SimilarityEngine) tempoDistance(a, b float32) float32 {
	// Handle double/half tempo as similar
	ratio := a / b
	if ratio > 1 {
		ratio = b / a
	}

	// Check for double/half tempo relationship
	if ratio > 0.45 && ratio < 0.55 {
		ratio = ratio * 2 // Treat half tempo as similar
	}

	// Convert ratio to distance (1.0 = same tempo)
	dist := float32(1.0) - ratio
	if dist < 0 {
		dist = 0
	}
	if dist > 1 {
		dist = 1
	}
	return dist
}

// spectralDistance computes distance between spectral features
func (e *SimilarityEngine) spectralDistance(a, b *AudioFeatures) float32 {
	centroidDiff := abs32(a.SpectralCentroid - b.SpectralCentroid)
	rolloffDiff := abs32(a.SpectralRolloff - b.SpectralRolloff)
	fluxDiff := abs32(a.SpectralFlux - b.SpectralFlux) / 10 // Normalize
	zcrDiff := abs32(a.ZeroCrossing - b.ZeroCrossing)

	return (centroidDiff + rolloffDiff + fluxDiff + zcrDiff) / 4
}

// bandsDistance computes distance between band ratios
func (e *SimilarityEngine) bandsDistance(a, b *AudioFeatures) float32 {
	bassDiff := abs32(a.BassRatio - b.BassRatio)
	midDiff := abs32(a.MidRatio - b.MidRatio)
	trebleDiff := abs32(a.TrebleRatio - b.TrebleRatio)
	return (bassDiff + midDiff + trebleDiff) / 3
}

// instrumentDistance computes distance between instrument profiles
func (e *SimilarityEngine) instrumentDistance(a, b *AudioFeatures) float32 {
	ai := a.Instruments
	bi := b.Instruments

	var sumDiff float32
	sumDiff += abs32(ai.BrassLike - bi.BrassLike)
	sumDiff += abs32(ai.StringLike - bi.StringLike)
	sumDiff += abs32(ai.WoodwindLike - bi.WoodwindLike)
	sumDiff += abs32(ai.Percussive - bi.Percussive)
	sumDiff += abs32(ai.SynthPad - bi.SynthPad)
	sumDiff += abs32(ai.VocalPresence - bi.VocalPresence)

	return sumDiff / 6
}

// contextDistance computes distance between contextual features
func (e *SimilarityEngine) contextDistance(a, b *AudioFeatures) float32 {
	ai := a.Instruments
	bi := b.Instruments

	var sumDiff float32
	sumDiff += abs32(ai.ArticulationStyle - bi.ArticulationStyle)
	sumDiff += abs32(ai.EnsembleSize - bi.EnsembleSize)
	sumDiff += abs32(ai.PlayingIntensity - bi.PlayingIntensity)
	sumDiff += abs32(a.AttackSharpness - b.AttackSharpness)
	sumDiff += abs32(a.HarmonicDensity - b.HarmonicDensity)
	sumDiff += abs32(a.RhythmComplexity - b.RhythmComplexity)
	sumDiff += abs32(a.DynamicRange - b.DynamicRange)

	return sumDiff / 7
}

// FindSimilar finds the most similar tracks to a given track
func (e *SimilarityEngine) FindSimilar(trackPath string, count int, exclude []string) []SimilarityEdge {
	// Check cached edges first
	edges := e.store.GetSimilarTracks(trackPath, count*2)

	// Filter excluded tracks
	excludeSet := make(map[string]bool)
	for _, p := range exclude {
		excludeSet[p] = true
	}

	var result []SimilarityEdge
	for _, edge := range edges {
		if !excludeSet[edge.TargetPath] {
			result = append(result, edge)
			if len(result) >= count {
				break
			}
		}
	}

	return result
}

// BuildGraph builds the similarity graph for all analyzed tracks
func (e *SimilarityEngine) BuildGraph() {
	allFeatures := e.store.GetAllFeatures()

	// Get all paths
	paths := make([]string, 0, len(allFeatures))
	for path := range allFeatures {
		paths = append(paths, path)
	}

	// Compute pairwise similarities
	for i, pathA := range paths {
		featuresA := allFeatures[pathA].Features
		var edges []SimilarityEdge

		for j, pathB := range paths {
			if i == j {
				continue
			}

			featuresB := allFeatures[pathB].Features
			similarity := e.ComputeSimilarity(featuresA, featuresB)

			if similarity >= MinSimilarityThreshold {
				edges = append(edges, SimilarityEdge{
					TargetPath: pathB,
					Weight:     similarity,
				})
			}
		}

		// Sort by similarity (highest first)
		sort.Slice(edges, func(a, b int) bool {
			return edges[a].Weight > edges[b].Weight
		})

		// Keep top K
		if len(edges) > e.topK {
			edges = edges[:e.topK]
		}

		e.store.StoreSimilarityEdges(pathA, edges)
	}
}

// ExplainSimilarity returns a breakdown of why two tracks are similar
func (e *SimilarityEngine) ExplainSimilarity(trackA, trackB string) map[string]float32 {
	fa, okA := e.store.GetFeatures(trackA)
	fb, okB := e.store.GetFeatures(trackB)

	if !okA || !okB {
		return nil
	}

	a := fa.Features
	b := fb.Features

	return map[string]float32{
		"overall":     e.ComputeSimilarity(a, b),
		"mfcc":        1 - e.mfccDistance(a, b),
		"tempo":       1 - e.tempoDistance(a.Tempo, b.Tempo),
		"spectral":    1 - e.spectralDistance(a, b),
		"energy":      1 - abs32(a.RMSEnergy-b.RMSEnergy),
		"bands":       1 - e.bandsDistance(a, b),
		"instruments": 1 - e.instrumentDistance(a, b),
		"context":     1 - e.contextDistance(a, b),
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
