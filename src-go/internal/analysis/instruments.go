package analysis

import (
	"math"
)

// InstrumentDetector analyzes spectral patterns to identify instrument families
type InstrumentDetector struct {
	sampleRate int
	fftSize    int
}

// NewInstrumentDetector creates a new instrument detector
func NewInstrumentDetector(sampleRate, fftSize int) *InstrumentDetector {
	return &InstrumentDetector{
		sampleRate: sampleRate,
		fftSize:    fftSize,
	}
}

// DetectInstruments analyzes spectral data to identify instrument families
// spectrum: magnitude spectrum from FFT
// prevSpectrum: previous frame's spectrum (for transient detection)
// zcr: zero crossing rate
// rms: root mean square energy
func (d *InstrumentDetector) DetectInstruments(spectrum, prevSpectrum []float64, zcr, rms float64) InstrumentProfile {
	profile := InstrumentProfile{}

	freqPerBin := float64(d.sampleRate) / float64(d.fftSize)

	// 1. Brass Detection
	// Brass instruments have strong harmonics in 300-3000Hz with characteristic "buzzy" quality
	profile.BrassLike = d.detectBrass(spectrum, freqPerBin)

	// 2. String Detection
	// Strings have smooth spectral rolloff, presence of bow noise (2-4kHz) or pluck transients
	profile.StringLike = d.detectStrings(spectrum, prevSpectrum, freqPerBin)

	// 3. Woodwind Detection
	// Woodwinds have specific harmonic patterns, often with strong fundamental
	profile.WoodwindLike = d.detectWoodwind(spectrum, freqPerBin)

	// 4. Percussion Detection
	// High zero-crossing rate, sharp transients, broadband energy
	profile.Percussive = d.detectPercussion(spectrum, prevSpectrum, zcr)

	// 5. Synth/Pad Detection
	// Unusually regular harmonic spacing, sustained notes without natural decay
	profile.SynthPad = d.detectSynth(spectrum, freqPerBin)

	// 6. Vocal Detection
	// Formant patterns (F1: 300-800Hz, F2: 800-2500Hz, F3: 2500-3500Hz)
	profile.VocalPresence = d.detectVocals(spectrum, freqPerBin)

	// Contextual features
	profile.ArticulationStyle = d.computeArticulation(spectrum, prevSpectrum, zcr)
	profile.EnsembleSize = d.computeEnsembleSize(spectrum, freqPerBin)
	profile.PlayingIntensity = d.computeIntensity(rms, spectrum)

	return profile
}

// detectBrass analyzes for brass-like spectral characteristics
func (d *InstrumentDetector) detectBrass(spectrum []float64, freqPerBin float64) float32 {
	// Brass characteristics:
	// - Strong harmonics in 300-3000Hz range
	// - Even harmonics relatively prominent (compared to woodwinds)
	// - Spectral centroid typically in mid-range
	// - "Buzzy" quality from many harmonics

	var brassEnergy, totalEnergy float64
	var harmonicCount, evenHarmonicStrength float64

	for i := 0; i < len(spectrum); i++ {
		freq := float64(i) * freqPerBin
		energy := spectrum[i] * spectrum[i]
		totalEnergy += energy

		// Brass frequency range
		if freq >= 300 && freq <= 3000 {
			brassEnergy += energy
		}

		// Check for harmonic content
		if freq > 100 && freq < 4000 {
			harmonicCount += energy
		}
	}

	if totalEnergy == 0 {
		return 0
	}

	// Brass should have significant energy in 300-3000Hz range
	brassRatio := brassEnergy / totalEnergy

	// Look for even harmonics (characteristic of brass)
	evenHarmonicStrength = d.analyzeHarmonicEvenness(spectrum, freqPerBin)

	// Combine factors
	score := (brassRatio*0.6 + evenHarmonicStrength*0.4)
	return float32(math.Min(score*1.5, 1.0))
}

// detectStrings analyzes for string instrument characteristics
func (d *InstrumentDetector) detectStrings(spectrum, prevSpectrum []float64, freqPerBin float64) float32 {
	// String characteristics:
	// - Smooth spectral envelope
	// - Energy distributed across harmonics
	// - Bow noise in 2-4kHz range (bowed strings)
	// - Or pluck transients (guitar, etc.)

	var stringEnergy, totalEnergy float64
	var bowNoiseEnergy float64

	for i := 0; i < len(spectrum); i++ {
		freq := float64(i) * freqPerBin
		energy := spectrum[i] * spectrum[i]
		totalEnergy += energy

		// Typical string frequency range
		if freq >= 80 && freq <= 5000 {
			stringEnergy += energy
		}

		// Bow noise range
		if freq >= 2000 && freq <= 4000 {
			bowNoiseEnergy += energy
		}
	}

	if totalEnergy == 0 {
		return 0
	}

	// Spectral smoothness (strings have smoother envelopes)
	smoothness := d.computeSpectralSmoothness(spectrum)

	// Harmonic richness
	harmonicRichness := stringEnergy / totalEnergy

	// Bow noise presence
	bowNoise := bowNoiseEnergy / totalEnergy

	score := harmonicRichness*0.4 + smoothness*0.4 + bowNoise*0.2
	return float32(math.Min(score*1.5, 1.0))
}

// detectWoodwind analyzes for woodwind characteristics
func (d *InstrumentDetector) detectWoodwind(spectrum []float64, freqPerBin float64) float32 {
	// Woodwind characteristics:
	// - Strong fundamental frequency
	// - Odd harmonics typically stronger than even (especially clarinet)
	// - Cleaner tone than brass
	// - Air noise in upper frequencies

	var fundamentalEnergy, totalEnergy float64
	var airNoiseEnergy float64

	// Find dominant frequency (approximate fundamental)
	maxEnergy := 0.0
	fundamentalBin := 0
	for i := 0; i < len(spectrum); i++ {
		freq := float64(i) * freqPerBin
		if freq >= 200 && freq <= 2000 {
			if spectrum[i] > maxEnergy {
				maxEnergy = spectrum[i]
				fundamentalBin = i
			}
		}
	}

	for i := 0; i < len(spectrum); i++ {
		freq := float64(i) * freqPerBin
		energy := spectrum[i] * spectrum[i]
		totalEnergy += energy

		// Fundamental region
		if i >= fundamentalBin-2 && i <= fundamentalBin+2 {
			fundamentalEnergy += energy
		}

		// Air noise (breath sounds)
		if freq >= 6000 && freq <= 12000 {
			airNoiseEnergy += energy
		}
	}

	if totalEnergy == 0 {
		return 0
	}

	// Strong fundamental is characteristic
	fundamentalStrength := fundamentalEnergy / totalEnergy

	// Odd harmonic dominance (clarinet characteristic)
	oddDominance := d.analyzeOddHarmonics(spectrum, fundamentalBin, freqPerBin)

	// Air noise presence
	airNoise := airNoiseEnergy / totalEnergy

	score := fundamentalStrength*0.4 + oddDominance*0.3 + airNoise*0.3
	return float32(math.Min(score*2.0, 1.0))
}

// detectPercussion analyzes for percussive characteristics
func (d *InstrumentDetector) detectPercussion(spectrum, prevSpectrum []float64, zcr float64) float32 {
	// Percussion characteristics:
	// - High zero-crossing rate
	// - Sharp transients (large spectral flux)
	// - Broadband energy distribution
	// - Specific frequency bands for different drums

	// Zero-crossing rate is a strong indicator
	zcrScore := math.Min(zcr*5, 1.0)

	// Compute spectral flux (transient detection)
	var flux float64
	for i := 0; i < len(spectrum) && i < len(prevSpectrum); i++ {
		diff := spectrum[i] - prevSpectrum[i]
		if diff > 0 {
			flux += diff * diff
		}
	}
	flux = math.Sqrt(flux)
	fluxScore := math.Min(flux/100.0, 1.0)

	// Broadband energy (percussion has energy across frequencies)
	broadband := d.computeBroadbandScore(spectrum)

	score := zcrScore*0.3 + fluxScore*0.4 + broadband*0.3
	return float32(math.Min(score, 1.0))
}

// detectSynth analyzes for synthesizer/pad characteristics
func (d *InstrumentDetector) detectSynth(spectrum []float64, freqPerBin float64) float32 {
	// Synth characteristics:
	// - Very regular harmonic spacing (perfect ratios)
	// - Often pure waveforms (saw, square, sine)
	// - Can have unusual spectral shapes
	// - Sustained, consistent energy

	// Look for unusually regular harmonics
	harmonicRegularity := d.computeHarmonicRegularity(spectrum, freqPerBin)

	// Spectral flatness (synths can be very uniform)
	flatness := d.computeSpectralFlatness(spectrum)

	score := harmonicRegularity*0.6 + flatness*0.4
	return float32(math.Min(score, 1.0))
}

// detectVocals analyzes for human voice characteristics
func (d *InstrumentDetector) detectVocals(spectrum []float64, freqPerBin float64) float32 {
	// Vocal characteristics (formants):
	// F1: 300-800Hz (first formant)
	// F2: 800-2500Hz (second formant)
	// F3: 2500-3500Hz (third formant)

	var f1Energy, f2Energy, f3Energy, totalEnergy float64

	for i := 0; i < len(spectrum); i++ {
		freq := float64(i) * freqPerBin
		energy := spectrum[i] * spectrum[i]
		totalEnergy += energy

		if freq >= 300 && freq <= 800 {
			f1Energy += energy
		} else if freq >= 800 && freq <= 2500 {
			f2Energy += energy
		} else if freq >= 2500 && freq <= 3500 {
			f3Energy += energy
		}
	}

	if totalEnergy == 0 {
		return 0
	}

	// All three formants should have significant energy
	f1Ratio := f1Energy / totalEnergy
	f2Ratio := f2Energy / totalEnergy
	f3Ratio := f3Energy / totalEnergy

	// Vocals typically have balanced formant distribution
	// with F2 being strongest
	hasFormants := f1Ratio > 0.05 && f2Ratio > 0.1 && f3Ratio > 0.02
	f2Dominant := f2Ratio > f1Ratio*0.5 && f2Ratio > f3Ratio

	score := 0.0
	if hasFormants && f2Dominant {
		score = (f1Ratio + f2Ratio + f3Ratio) * 2
	}

	return float32(math.Min(score, 1.0))
}

// computeArticulation determines staccato vs legato playing style
func (d *InstrumentDetector) computeArticulation(spectrum, prevSpectrum []float64, zcr float64) float32 {
	// Staccato: sharp attacks, quick decay (high flux, high ZCR)
	// Legato: smooth transitions (low flux, low ZCR)

	var flux float64
	for i := 0; i < len(spectrum) && i < len(prevSpectrum); i++ {
		diff := math.Abs(spectrum[i] - prevSpectrum[i])
		flux += diff
	}

	// Normalize
	avgDiff := flux / float64(len(spectrum))

	// Combine with ZCR
	score := avgDiff*0.5 + zcr*0.5
	return float32(math.Min(score*2, 1.0)) // 1 = staccato, 0 = legato
}

// computeEnsembleSize estimates solo vs full ensemble
func (d *InstrumentDetector) computeEnsembleSize(spectrum []float64, freqPerBin float64) float32 {
	// Full ensemble: energy distributed across all frequencies
	// Solo: energy concentrated in specific instrument range

	// Compute spectral flatness (measure of energy distribution)
	flatness := d.computeSpectralFlatness(spectrum)

	// Count active frequency bands
	activeBands := 0
	threshold := 0.0
	for _, v := range spectrum {
		threshold += v
	}
	threshold = threshold / float64(len(spectrum)) * 0.5

	for _, v := range spectrum {
		if v > threshold {
			activeBands++
		}
	}
	activeRatio := float64(activeBands) / float64(len(spectrum))

	score := flatness*0.5 + activeRatio*0.5
	return float32(math.Min(score*2, 1.0))
}

// computeIntensity estimates soft vs aggressive playing
func (d *InstrumentDetector) computeIntensity(rms float64, spectrum []float64) float32 {
	// High RMS and high-frequency energy indicate aggressive playing

	var highFreqEnergy, totalEnergy float64
	for i := 0; i < len(spectrum); i++ {
		energy := spectrum[i] * spectrum[i]
		totalEnergy += energy
		// High frequency content (more harmonics = more intensity)
		if i > len(spectrum)/2 {
			highFreqEnergy += energy
		}
	}

	if totalEnergy == 0 {
		return 0
	}

	highFreqRatio := highFreqEnergy / totalEnergy
	rmsScore := math.Min(rms*10, 1.0) // Normalize RMS

	score := rmsScore*0.6 + highFreqRatio*0.4
	return float32(math.Min(score, 1.0))
}

// Helper methods

func (d *InstrumentDetector) analyzeHarmonicEvenness(spectrum []float64, freqPerBin float64) float64 {
	// Find fundamental and analyze even vs odd harmonics
	maxIdx := 0
	maxVal := 0.0
	for i := 1; i < len(spectrum)/4; i++ {
		if spectrum[i] > maxVal {
			maxVal = spectrum[i]
			maxIdx = i
		}
	}

	if maxIdx == 0 {
		return 0
	}

	var evenSum, oddSum float64
	fundamentalFreq := float64(maxIdx) * freqPerBin

	for h := 2; h <= 8; h++ {
		harmonicBin := int(fundamentalFreq * float64(h) / freqPerBin)
		if harmonicBin >= len(spectrum) {
			break
		}
		if h%2 == 0 {
			evenSum += spectrum[harmonicBin]
		} else {
			oddSum += spectrum[harmonicBin]
		}
	}

	if evenSum+oddSum == 0 {
		return 0
	}
	return evenSum / (evenSum + oddSum)
}

func (d *InstrumentDetector) analyzeOddHarmonics(spectrum []float64, fundamentalBin int, freqPerBin float64) float64 {
	if fundamentalBin == 0 {
		return 0
	}

	var evenSum, oddSum float64

	for h := 2; h <= 8; h++ {
		harmonicBin := fundamentalBin * h
		if harmonicBin >= len(spectrum) {
			break
		}
		if h%2 == 0 {
			evenSum += spectrum[harmonicBin]
		} else {
			oddSum += spectrum[harmonicBin]
		}
	}

	if evenSum+oddSum == 0 {
		return 0
	}
	return oddSum / (evenSum + oddSum)
}

func (d *InstrumentDetector) computeSpectralSmoothness(spectrum []float64) float64 {
	if len(spectrum) < 3 {
		return 0
	}

	var roughness float64
	for i := 1; i < len(spectrum)-1; i++ {
		// Second derivative approximation
		d2 := spectrum[i-1] - 2*spectrum[i] + spectrum[i+1]
		roughness += math.Abs(d2)
	}

	avgRoughness := roughness / float64(len(spectrum))
	// Invert and normalize (lower roughness = smoother)
	return math.Max(0, 1-avgRoughness*10)
}

func (d *InstrumentDetector) computeBroadbandScore(spectrum []float64) float64 {
	// Divide spectrum into octaves and check energy in each
	octaves := 8
	octaveEnergies := make([]float64, octaves)
	binPerOctave := len(spectrum) / octaves

	for i, v := range spectrum {
		octave := i / binPerOctave
		if octave >= octaves {
			octave = octaves - 1
		}
		octaveEnergies[octave] += v * v
	}

	// Count octaves with significant energy
	var totalEnergy float64
	for _, e := range octaveEnergies {
		totalEnergy += e
	}
	if totalEnergy == 0 {
		return 0
	}

	activeOctaves := 0
	for _, e := range octaveEnergies {
		if e/totalEnergy > 0.05 {
			activeOctaves++
		}
	}

	return float64(activeOctaves) / float64(octaves)
}

func (d *InstrumentDetector) computeHarmonicRegularity(spectrum []float64, freqPerBin float64) float64 {
	// Find peaks and check if they're evenly spaced
	var peaks []int
	for i := 1; i < len(spectrum)-1; i++ {
		if spectrum[i] > spectrum[i-1] && spectrum[i] > spectrum[i+1] {
			peaks = append(peaks, i)
		}
	}

	if len(peaks) < 3 {
		return 0
	}

	// Check spacing regularity
	var spacings []float64
	for i := 1; i < len(peaks) && i < 10; i++ {
		spacings = append(spacings, float64(peaks[i]-peaks[i-1]))
	}

	if len(spacings) < 2 {
		return 0
	}

	// Compute variance of spacings (low variance = regular)
	var sum float64
	for _, s := range spacings {
		sum += s
	}
	mean := sum / float64(len(spacings))

	var variance float64
	for _, s := range spacings {
		variance += (s - mean) * (s - mean)
	}
	variance /= float64(len(spacings))

	// Low variance means regular spacing (more synth-like)
	regularity := math.Max(0, 1-variance/mean)
	return regularity
}

func (d *InstrumentDetector) computeSpectralFlatness(spectrum []float64) float64 {
	if len(spectrum) == 0 {
		return 0
	}

	// Geometric mean / Arithmetic mean
	var logSum, sum float64
	for _, v := range spectrum {
		if v > 1e-10 {
			logSum += math.Log(v)
			sum += v
		}
	}

	n := float64(len(spectrum))
	geometricMean := math.Exp(logSum / n)
	arithmeticMean := sum / n

	if arithmeticMean == 0 {
		return 0
	}

	return geometricMean / arithmeticMean
}
