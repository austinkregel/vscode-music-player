package analysis

import (
	"encoding/binary"
	"math"
	"sync"

	"gonum.org/v1/gonum/dsp/fourier"
)

const (
	// FFT size for analysis - 2048 gives good frequency resolution
	analysisFFTSize = 2048
	// Number of MFCC coefficients
	numMFCC = 13
	// Number of mel filterbank channels
	numMelFilters = 26
	// Sample rate for analysis
	analysisSampleRate = 44100
	// Analysis window hop size (samples)
	hopSize = 1024
)

// AudioFeatures contains extracted audio features for a track
type AudioFeatures struct {
	// Core spectral features
	MFCC             [numMFCC]float32 // Mel-frequency cepstral coefficients
	MFCCStdDev       [numMFCC]float32 // Standard deviation of MFCCs
	SpectralCentroid float32          // Brightness (Hz normalized)
	SpectralRolloff  float32          // High-freq content threshold
	SpectralFlux     float32          // Rate of spectral change
	ZeroCrossing     float32          // Percussiveness indicator
	RMSEnergy        float32          // Overall loudness (normalized)
	Tempo            float32          // BPM
	BassRatio        float32          // Low frequency energy ratio
	MidRatio         float32          // Mid frequency energy ratio
	TrebleRatio      float32          // High frequency energy ratio

	// Instrument family signatures
	Instruments InstrumentProfile

	// Contextual features
	AttackSharpness  float32 // Staccato vs legato (0-1)
	HarmonicDensity  float32 // Sparse vs full arrangement (0-1)
	RhythmComplexity float32 // Syncopation level (0-1)
	DynamicRange     float32 // Compression vs dynamics (0-1)
}

// InstrumentProfile contains instrument family presence scores
type InstrumentProfile struct {
	BrassLike         float32 // Trumpet, trombone, sax
	StringLike        float32 // Violin, cello, guitar
	WoodwindLike      float32 // Flute, clarinet
	Percussive        float32 // Drums, percussion
	SynthPad          float32 // Synthesizers, pads
	VocalPresence     float32 // Human voice
	ArticulationStyle float32 // 0=legato, 1=staccato
	EnsembleSize      float32 // 0=solo, 1=full ensemble
	PlayingIntensity  float32 // 0=soft, 1=aggressive
}

// FeatureExtractor extracts audio features from PCM data
type FeatureExtractor struct {
	mu sync.Mutex

	fft                *fourier.FFT
	window             []float64
	melFilters         [][]float64
	instrumentDetector *InstrumentDetector

	// Accumulator for windowed analysis
	frameCount         int
	mfccAccum          [][]float64
	centroidAccum      []float64
	rolloffAccum       []float64
	fluxAccum          []float64
	zcrAccum           []float64
	rmsAccum           []float64
	bandEnergyAccum    [][]float64 // [frame][3] for bass/mid/treble
	attackAccum        []float64
	prevSpectrum       []float64
	onsetStrengths     []float64
	instrumentAccum    []InstrumentProfile

	sampleRate int
}

// NewFeatureExtractor creates a new feature extractor
func NewFeatureExtractor(sampleRate int) *FeatureExtractor {
	if sampleRate == 0 {
		sampleRate = analysisSampleRate
	}

	// Create Hanning window
	window := make([]float64, analysisFFTSize)
	for i := range window {
		window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(analysisFFTSize-1)))
	}

	fe := &FeatureExtractor{
		fft:                fourier.NewFFT(analysisFFTSize),
		window:             window,
		melFilters:         createMelFilterbank(numMelFilters, analysisFFTSize, sampleRate),
		instrumentDetector: NewInstrumentDetector(sampleRate, analysisFFTSize),
		prevSpectrum:       make([]float64, analysisFFTSize/2),
		sampleRate:         sampleRate,
	}

	fe.reset()
	return fe
}

// reset clears accumulators for a new track
func (fe *FeatureExtractor) reset() {
	fe.frameCount = 0
	fe.mfccAccum = nil
	fe.centroidAccum = nil
	fe.rolloffAccum = nil
	fe.fluxAccum = nil
	fe.zcrAccum = nil
	fe.rmsAccum = nil
	fe.bandEnergyAccum = nil
	fe.attackAccum = nil
	fe.onsetStrengths = nil
	fe.instrumentAccum = nil
	for i := range fe.prevSpectrum {
		fe.prevSpectrum[i] = 0
	}
}

// ProcessAudio extracts features from complete audio data (mono float64 samples)
func (fe *FeatureExtractor) ProcessAudio(samples []float64) *AudioFeatures {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	fe.reset()

	// Process in overlapping windows
	numFrames := (len(samples) - analysisFFTSize) / hopSize
	if numFrames < 1 {
		return &AudioFeatures{}
	}

	for i := 0; i < numFrames; i++ {
		start := i * hopSize
		end := start + analysisFFTSize
		if end > len(samples) {
			break
		}
		fe.processFrame(samples[start:end])
	}

	return fe.computeFinalFeatures()
}

// ProcessPCM extracts features from 16-bit stereo PCM data
func (fe *FeatureExtractor) ProcessPCM(data []byte, channels int) *AudioFeatures {
	// Convert to mono float64
	samples := pcmToMono(data, channels)
	return fe.ProcessAudio(samples)
}

// processFrame analyzes a single FFT frame
func (fe *FeatureExtractor) processFrame(frame []float64) {
	if len(frame) != analysisFFTSize {
		return
	}

	// Apply window
	windowed := make([]float64, analysisFFTSize)
	for i := 0; i < analysisFFTSize; i++ {
		windowed[i] = frame[i] * fe.window[i]
	}

	// Compute FFT
	coeffs := fe.fft.Coefficients(nil, windowed)

	// Compute magnitude spectrum
	spectrum := make([]float64, analysisFFTSize/2)
	for i := 0; i < len(spectrum); i++ {
		real := real(coeffs[i])
		imag := imag(coeffs[i])
		spectrum[i] = math.Sqrt(real*real + imag*imag)
	}

	// Compute features for this frame
	fe.computeFrameFeatures(frame, spectrum)
	fe.frameCount++
}

// computeFrameFeatures extracts features from a single frame
func (fe *FeatureExtractor) computeFrameFeatures(frame, spectrum []float64) {
	// 1. MFCCs
	mfcc := fe.computeMFCC(spectrum)
	fe.mfccAccum = append(fe.mfccAccum, mfcc)

	// 2. Spectral Centroid
	centroid := fe.computeSpectralCentroid(spectrum)
	fe.centroidAccum = append(fe.centroidAccum, centroid)

	// 3. Spectral Rolloff
	rolloff := fe.computeSpectralRolloff(spectrum, 0.85)
	fe.rolloffAccum = append(fe.rolloffAccum, rolloff)

	// 4. Spectral Flux
	flux := fe.computeSpectralFlux(spectrum)
	fe.fluxAccum = append(fe.fluxAccum, flux)

	// 5. Zero Crossing Rate
	zcr := fe.computeZCR(frame)
	fe.zcrAccum = append(fe.zcrAccum, zcr)

	// 6. RMS Energy
	rms := fe.computeRMS(frame)
	fe.rmsAccum = append(fe.rmsAccum, rms)

	// 7. Band Energy Ratios
	bandEnergies := fe.computeBandEnergies(spectrum)
	fe.bandEnergyAccum = append(fe.bandEnergyAccum, bandEnergies)

	// 8. Attack Sharpness (transient detection)
	attack := fe.computeAttackSharpness(spectrum)
	fe.attackAccum = append(fe.attackAccum, attack)

	// 9. Instrument Detection
	instrumentProfile := fe.instrumentDetector.DetectInstruments(spectrum, fe.prevSpectrum, zcr, rms)
	fe.instrumentAccum = append(fe.instrumentAccum, instrumentProfile)

	// Track onset strength for tempo detection
	if flux > 0 {
		fe.onsetStrengths = append(fe.onsetStrengths, flux)
	}

	// Update previous spectrum for next frame's flux calculation
	copy(fe.prevSpectrum, spectrum)
}

// computeMFCC computes Mel-frequency cepstral coefficients
func (fe *FeatureExtractor) computeMFCC(spectrum []float64) []float64 {
	// Apply mel filterbank
	melEnergies := make([]float64, numMelFilters)
	for i := 0; i < numMelFilters; i++ {
		for j := 0; j < len(spectrum) && j < len(fe.melFilters[i]); j++ {
			melEnergies[i] += spectrum[j] * spectrum[j] * fe.melFilters[i][j]
		}
		// Apply log compression
		if melEnergies[i] < 1e-10 {
			melEnergies[i] = 1e-10
		}
		melEnergies[i] = math.Log(melEnergies[i])
	}

	// Apply DCT to get MFCCs
	mfcc := make([]float64, numMFCC)
	for i := 0; i < numMFCC; i++ {
		for j := 0; j < numMelFilters; j++ {
			mfcc[i] += melEnergies[j] * math.Cos(math.Pi*float64(i)*(float64(j)+0.5)/float64(numMelFilters))
		}
	}

	return mfcc
}

// computeSpectralCentroid computes the "center of mass" of the spectrum
func (fe *FeatureExtractor) computeSpectralCentroid(spectrum []float64) float64 {
	var weightedSum, sum float64
	freqPerBin := float64(fe.sampleRate) / float64(analysisFFTSize)

	for i, mag := range spectrum {
		freq := float64(i) * freqPerBin
		weightedSum += freq * mag
		sum += mag
	}

	if sum == 0 {
		return 0
	}
	return weightedSum / sum
}

// computeSpectralRolloff computes the frequency below which rolloffPercent of energy is contained
func (fe *FeatureExtractor) computeSpectralRolloff(spectrum []float64, rolloffPercent float64) float64 {
	var totalEnergy float64
	for _, mag := range spectrum {
		totalEnergy += mag * mag
	}

	threshold := totalEnergy * rolloffPercent
	var cumEnergy float64
	freqPerBin := float64(fe.sampleRate) / float64(analysisFFTSize)

	for i, mag := range spectrum {
		cumEnergy += mag * mag
		if cumEnergy >= threshold {
			return float64(i) * freqPerBin
		}
	}
	return float64(len(spectrum)) * freqPerBin
}

// computeSpectralFlux computes the rate of spectral change
func (fe *FeatureExtractor) computeSpectralFlux(spectrum []float64) float64 {
	var flux float64
	for i := 0; i < len(spectrum) && i < len(fe.prevSpectrum); i++ {
		diff := spectrum[i] - fe.prevSpectrum[i]
		if diff > 0 {
			flux += diff * diff
		}
	}
	return math.Sqrt(flux)
}

// computeZCR computes zero crossing rate
func (fe *FeatureExtractor) computeZCR(frame []float64) float64 {
	var crossings int
	for i := 1; i < len(frame); i++ {
		if (frame[i] >= 0 && frame[i-1] < 0) || (frame[i] < 0 && frame[i-1] >= 0) {
			crossings++
		}
	}
	return float64(crossings) / float64(len(frame))
}

// computeRMS computes root mean square energy
func (fe *FeatureExtractor) computeRMS(frame []float64) float64 {
	var sum float64
	for _, s := range frame {
		sum += s * s
	}
	return math.Sqrt(sum / float64(len(frame)))
}

// computeBandEnergies computes energy in bass/mid/treble bands
func (fe *FeatureExtractor) computeBandEnergies(spectrum []float64) []float64 {
	freqPerBin := float64(fe.sampleRate) / float64(analysisFFTSize)

	// Band boundaries (Hz)
	const (
		bassMax   = 250.0
		midMax    = 4000.0
		trebleMax = 20000.0
	)

	var bassEnergy, midEnergy, trebleEnergy, totalEnergy float64

	for i, mag := range spectrum {
		freq := float64(i) * freqPerBin
		energy := mag * mag
		totalEnergy += energy

		switch {
		case freq < bassMax:
			bassEnergy += energy
		case freq < midMax:
			midEnergy += energy
		case freq < trebleMax:
			trebleEnergy += energy
		}
	}

	if totalEnergy == 0 {
		return []float64{0, 0, 0}
	}

	return []float64{
		bassEnergy / totalEnergy,
		midEnergy / totalEnergy,
		trebleEnergy / totalEnergy,
	}
}

// computeAttackSharpness estimates transient sharpness
func (fe *FeatureExtractor) computeAttackSharpness(spectrum []float64) float64 {
	// High spectral flux relative to energy indicates sharp attack
	var energy float64
	for _, mag := range spectrum {
		energy += mag * mag
	}
	flux := fe.computeSpectralFlux(spectrum)
	if energy == 0 {
		return 0
	}
	// Normalize
	return math.Min(flux/math.Sqrt(energy), 1.0)
}

// computeFinalFeatures aggregates frame features into final track features
func (fe *FeatureExtractor) computeFinalFeatures() *AudioFeatures {
	if fe.frameCount == 0 {
		return &AudioFeatures{}
	}

	features := &AudioFeatures{}

	// Average MFCCs and compute std dev
	for i := 0; i < numMFCC; i++ {
		var sum, sumSq float64
		for _, mfcc := range fe.mfccAccum {
			if i < len(mfcc) {
				sum += mfcc[i]
				sumSq += mfcc[i] * mfcc[i]
			}
		}
		mean := sum / float64(len(fe.mfccAccum))
		variance := sumSq/float64(len(fe.mfccAccum)) - mean*mean
		features.MFCC[i] = float32(mean)
		if variance > 0 {
			features.MFCCStdDev[i] = float32(math.Sqrt(variance))
		}
	}

	// Average other features
	features.SpectralCentroid = float32(average(fe.centroidAccum) / 20000.0) // Normalize to 0-1
	features.SpectralRolloff = float32(average(fe.rolloffAccum) / 20000.0)
	features.SpectralFlux = float32(average(fe.fluxAccum))
	features.ZeroCrossing = float32(average(fe.zcrAccum))
	features.RMSEnergy = float32(average(fe.rmsAccum))

	// Band energies
	var bassSum, midSum, trebleSum float64
	for _, be := range fe.bandEnergyAccum {
		if len(be) >= 3 {
			bassSum += be[0]
			midSum += be[1]
			trebleSum += be[2]
		}
	}
	n := float64(len(fe.bandEnergyAccum))
	if n > 0 {
		features.BassRatio = float32(bassSum / n)
		features.MidRatio = float32(midSum / n)
		features.TrebleRatio = float32(trebleSum / n)
	}

	// Contextual features
	features.AttackSharpness = float32(average(fe.attackAccum))
	features.DynamicRange = float32(computeDynamicRange(fe.rmsAccum))
	features.RhythmComplexity = float32(computeRhythmComplexity(fe.onsetStrengths))

	// Tempo estimation
	features.Tempo = float32(fe.estimateTempo())

	// Harmonic density (approximated by MFCC variance)
	var mfccVarSum float64
	for i := 1; i < numMFCC; i++ {
		mfccVarSum += float64(features.MFCCStdDev[i])
	}
	features.HarmonicDensity = float32(math.Min(mfccVarSum/10.0, 1.0))

	// Aggregate instrument profiles
	if len(fe.instrumentAccum) > 0 {
		features.Instruments = fe.aggregateInstrumentProfiles()
	}

	return features
}

// aggregateInstrumentProfiles averages instrument profiles across all frames
func (fe *FeatureExtractor) aggregateInstrumentProfiles() InstrumentProfile {
	if len(fe.instrumentAccum) == 0 {
		return InstrumentProfile{}
	}

	n := float32(len(fe.instrumentAccum))
	var result InstrumentProfile

	for _, p := range fe.instrumentAccum {
		result.BrassLike += p.BrassLike
		result.StringLike += p.StringLike
		result.WoodwindLike += p.WoodwindLike
		result.Percussive += p.Percussive
		result.SynthPad += p.SynthPad
		result.VocalPresence += p.VocalPresence
		result.ArticulationStyle += p.ArticulationStyle
		result.EnsembleSize += p.EnsembleSize
		result.PlayingIntensity += p.PlayingIntensity
	}

	result.BrassLike /= n
	result.StringLike /= n
	result.WoodwindLike /= n
	result.Percussive /= n
	result.SynthPad /= n
	result.VocalPresence /= n
	result.ArticulationStyle /= n
	result.EnsembleSize /= n
	result.PlayingIntensity /= n

	return result
}

// estimateTempo estimates BPM from onset strengths
func (fe *FeatureExtractor) estimateTempo() float64 {
	if len(fe.onsetStrengths) < 10 {
		return 120.0 // Default
	}

	// Simple autocorrelation-based tempo estimation
	hopDuration := float64(hopSize) / float64(fe.sampleRate)
	minLag := int(60.0 / 200.0 / hopDuration) // 200 BPM max
	maxLag := int(60.0 / 60.0 / hopDuration)  // 60 BPM min

	if maxLag >= len(fe.onsetStrengths) {
		maxLag = len(fe.onsetStrengths) - 1
	}
	if minLag < 1 {
		minLag = 1
	}

	bestLag := minLag
	bestCorr := 0.0

	for lag := minLag; lag <= maxLag; lag++ {
		var corr float64
		for i := 0; i < len(fe.onsetStrengths)-lag; i++ {
			corr += fe.onsetStrengths[i] * fe.onsetStrengths[i+lag]
		}
		if corr > bestCorr {
			bestCorr = corr
			bestLag = lag
		}
	}

	// Convert lag to BPM
	bpm := 60.0 / (float64(bestLag) * hopDuration)
	// Constrain to reasonable range
	if bpm < 60 {
		bpm = 60
	}
	if bpm > 200 {
		bpm = 200
	}
	return bpm
}

// ToBytes serializes features to compact binary format
func (f *AudioFeatures) ToBytes() []byte {
	buf := make([]byte, 200) // Approximate size
	offset := 0

	// Write MFCCs
	for i := 0; i < numMFCC; i++ {
		binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(f.MFCC[i]))
		offset += 4
	}
	for i := 0; i < numMFCC; i++ {
		binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(f.MFCCStdDev[i]))
		offset += 4
	}

	// Write scalar features
	scalars := []float32{
		f.SpectralCentroid, f.SpectralRolloff, f.SpectralFlux, f.ZeroCrossing,
		f.RMSEnergy, f.Tempo, f.BassRatio, f.MidRatio, f.TrebleRatio,
		f.AttackSharpness, f.HarmonicDensity, f.RhythmComplexity, f.DynamicRange,
	}
	for _, s := range scalars {
		binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(s))
		offset += 4
	}

	// Write instrument profile
	instruments := []float32{
		f.Instruments.BrassLike, f.Instruments.StringLike, f.Instruments.WoodwindLike,
		f.Instruments.Percussive, f.Instruments.SynthPad, f.Instruments.VocalPresence,
		f.Instruments.ArticulationStyle, f.Instruments.EnsembleSize, f.Instruments.PlayingIntensity,
	}
	for _, s := range instruments {
		binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(s))
		offset += 4
	}

	return buf[:offset]
}

// FromBytes deserializes features from binary format
func (f *AudioFeatures) FromBytes(data []byte) error {
	if len(data) < 200 {
		return nil // Not enough data
	}
	offset := 0

	// Read MFCCs
	for i := 0; i < numMFCC; i++ {
		f.MFCC[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
	}
	for i := 0; i < numMFCC; i++ {
		f.MFCCStdDev[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
	}

	// Read scalar features
	f.SpectralCentroid = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.SpectralRolloff = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.SpectralFlux = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.ZeroCrossing = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.RMSEnergy = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Tempo = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.BassRatio = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.MidRatio = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.TrebleRatio = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.AttackSharpness = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.HarmonicDensity = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.RhythmComplexity = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.DynamicRange = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// Read instrument profile
	f.Instruments.BrassLike = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.StringLike = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.WoodwindLike = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.Percussive = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.SynthPad = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.VocalPresence = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.ArticulationStyle = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.EnsembleSize = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	f.Instruments.PlayingIntensity = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))

	return nil
}

// Helper functions

func createMelFilterbank(numFilters, fftSize, sampleRate int) [][]float64 {
	// Convert Hz to Mel scale
	hzToMel := func(hz float64) float64 {
		return 2595 * math.Log10(1+hz/700)
	}
	melToHz := func(mel float64) float64 {
		return 700 * (math.Pow(10, mel/2595) - 1)
	}

	nyquist := float64(sampleRate) / 2
	lowMel := hzToMel(20)     // 20 Hz
	highMel := hzToMel(nyquist)

	// Create mel-spaced center frequencies
	melPoints := make([]float64, numFilters+2)
	for i := range melPoints {
		melPoints[i] = lowMel + float64(i)*(highMel-lowMel)/float64(numFilters+1)
	}

	// Convert back to Hz
	hzPoints := make([]float64, numFilters+2)
	for i := range hzPoints {
		hzPoints[i] = melToHz(melPoints[i])
	}

	// Convert to FFT bin indices
	binPoints := make([]int, numFilters+2)
	for i := range binPoints {
		binPoints[i] = int(math.Floor(hzPoints[i] * float64(fftSize) / float64(sampleRate)))
	}

	// Create triangular filters
	filters := make([][]float64, numFilters)
	for i := 0; i < numFilters; i++ {
		filters[i] = make([]float64, fftSize/2)

		for j := binPoints[i]; j < binPoints[i+1] && j < fftSize/2; j++ {
			if binPoints[i+1] != binPoints[i] {
				filters[i][j] = float64(j-binPoints[i]) / float64(binPoints[i+1]-binPoints[i])
			}
		}
		for j := binPoints[i+1]; j < binPoints[i+2] && j < fftSize/2; j++ {
			if binPoints[i+2] != binPoints[i+1] {
				filters[i][j] = float64(binPoints[i+2]-j) / float64(binPoints[i+2]-binPoints[i+1])
			}
		}
	}

	return filters
}

func pcmToMono(data []byte, channels int) []float64 {
	bytesPerSample := 2
	samplesPerFrame := channels
	numSamples := len(data) / (bytesPerSample * samplesPerFrame)

	samples := make([]float64, numSamples)
	for i := 0; i < numSamples; i++ {
		offset := i * bytesPerSample * samplesPerFrame
		var sum float64
		for ch := 0; ch < samplesPerFrame; ch++ {
			chOffset := offset + ch*bytesPerSample
			sample := int16(data[chOffset]) | int16(data[chOffset+1])<<8
			sum += float64(sample) / 32768.0
		}
		samples[i] = sum / float64(samplesPerFrame)
	}
	return samples
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func computeDynamicRange(rmsValues []float64) float64 {
	if len(rmsValues) < 2 {
		return 0
	}
	// Find 10th and 90th percentile
	sorted := make([]float64, len(rmsValues))
	copy(sorted, rmsValues)
	// Simple bubble sort for small arrays
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	p10 := sorted[len(sorted)/10]
	p90 := sorted[len(sorted)*9/10]
	if p10 == 0 {
		return 0
	}
	return math.Min((p90-p10)/p10, 1.0)
}

func computeRhythmComplexity(onsets []float64) float64 {
	if len(onsets) < 4 {
		return 0
	}
	// Measure irregularity of onset intervals
	var intervals []float64
	for i := 1; i < len(onsets); i++ {
		if onsets[i] > 0 && onsets[i-1] > 0 {
			intervals = append(intervals, onsets[i]/onsets[i-1])
		}
	}
	if len(intervals) < 2 {
		return 0
	}
	// Higher variance = more complex rhythm
	avg := average(intervals)
	var variance float64
	for _, v := range intervals {
		variance += (v - avg) * (v - avg)
	}
	variance /= float64(len(intervals))
	return math.Min(variance, 1.0)
}
