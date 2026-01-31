package audio

import (
	"math"
	"sync"

	"gonum.org/v1/gonum/dsp/fourier"
)

const (
	// FFT size - must be power of 2
	// 2048 is the Web Audio API default for AnalyserNode
	// At 44100Hz stereo, this gives ~21 FFT frames/sec
	fftSize = 2048
	// Number of frequency bands to output
	// Original: NUM_BANDS = 128
	numBands = 128
	// Smoothing factor for temporal smoothing
	// Original: SMOOTHING = 0.5
	smoothingFactor = 0.5
)

// AudioDataCallback is called when new audio analysis data is ready
type AudioDataCallback func(bands []uint8)

// AudioAnalyzer performs real-time FFT analysis on audio samples
type AudioAnalyzer struct {
	mu sync.RWMutex

	// FFT
	fft *fourier.FFT

	// Sample buffer for collecting enough samples for FFT
	sampleBuffer []float64
	bufferIndex  int

	// Window function (Hanning)
	window []float64

	// Output: frequency bands (0-255 like Web Audio API getByteFrequencyData)
	bands         []float64
	smoothedBands []float64

	// Sample rate for frequency calculations
	sampleRate int
	channels   int

	// Whether we have enough data for valid output
	ready bool

	// Callback for real-time push (called immediately when new data is ready)
	callback AudioDataCallback
}

// NewAudioAnalyzer creates a new audio analyzer
func NewAudioAnalyzer(sampleRate, channels int) *AudioAnalyzer {
	// Create Hanning window
	window := make([]float64, fftSize)
	for i := range window {
		window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)))
	}

	return &AudioAnalyzer{
		fft:           fourier.NewFFT(fftSize),
		sampleBuffer:  make([]float64, fftSize),
		window:        window,
		bands:         make([]float64, numBands),
		smoothedBands: make([]float64, numBands),
		sampleRate:    sampleRate,
		channels:      channels,
	}
}

// ProcessSamples processes 16-bit PCM samples and updates frequency bands
func (a *AudioAnalyzer) ProcessSamples(data []byte) {
	var shouldNotify bool
	var bands []uint8

	a.mu.Lock()

	// Convert 16-bit PCM to float64 samples
	// Mix stereo to mono by averaging channels
	bytesPerSample := 2 // 16-bit
	samplesPerFrame := a.channels

	for i := 0; i+bytesPerSample*samplesPerFrame <= len(data); i += bytesPerSample * samplesPerFrame {
		var sum float64
		for ch := 0; ch < samplesPerFrame; ch++ {
			offset := i + ch*bytesPerSample
			// Read 16-bit little-endian sample
			sample := int16(data[offset]) | int16(data[offset+1])<<8
			// Normalize to -1.0 to 1.0
			sum += float64(sample) / 32768.0
		}
		// Average channels
		monoSample := sum / float64(samplesPerFrame)

		// Add to circular buffer
		a.sampleBuffer[a.bufferIndex] = monoSample
		a.bufferIndex = (a.bufferIndex + 1) % fftSize

		// When buffer wraps, we have a full window - compute FFT
		if a.bufferIndex == 0 {
			a.computeFFT()
			a.ready = true
			shouldNotify = a.callback != nil
			if shouldNotify {
				// Copy bands while holding lock
				bands = make([]uint8, numBands)
				for i, v := range a.smoothedBands {
					if v > 255 {
						bands[i] = 255
					} else if v < 0 {
						bands[i] = 0
					} else {
						bands[i] = uint8(v)
					}
				}
			}
		}
	}

	callback := a.callback
	a.mu.Unlock()

	// Call callback OUTSIDE of lock for true real-time push
	if shouldNotify && callback != nil {
		callback(bands)
	}
}

// computeFFT performs FFT on the current sample buffer
func (a *AudioAnalyzer) computeFFT() {
	// Apply window function to samples
	windowed := make([]float64, fftSize)
	for i := 0; i < fftSize; i++ {
		// Read from circular buffer in correct order
		idx := (a.bufferIndex + i) % fftSize
		windowed[i] = a.sampleBuffer[idx] * a.window[i]
	}

	// Compute FFT
	coeffs := a.fft.Coefficients(nil, windowed)

	// Compute magnitude spectrum (only use first half - Nyquist)
	// Group into logarithmically-spaced frequency bands
	nyquist := fftSize / 2
	freqPerBin := float64(a.sampleRate) / float64(fftSize)

	// Clear bands
	for i := range a.bands {
		a.bands[i] = 0
	}

	// Map FFT bins to frequency bands using logarithmic scale
	// This gives better resolution for lower frequencies (bass/mids)
	// which is more perceptually relevant
	minFreq := 20.0   // 20 Hz
	maxFreq := 20000.0 // 20 kHz (or Nyquist, whichever is lower)
	if float64(a.sampleRate)/2 < maxFreq {
		maxFreq = float64(a.sampleRate) / 2
	}

	logMin := math.Log10(minFreq)
	logMax := math.Log10(maxFreq)
	logRange := logMax - logMin

	// For each band, find the frequency range and sum magnitudes
	bandCounts := make([]int, numBands)

	for bin := 1; bin < nyquist; bin++ {
		freq := float64(bin) * freqPerBin
		if freq < minFreq || freq > maxFreq {
			continue
		}

		// Map frequency to band index (logarithmic)
		logFreq := math.Log10(freq)
		bandFloat := (logFreq - logMin) / logRange * float64(numBands)
		band := int(bandFloat)
		if band >= numBands {
			band = numBands - 1
		}
		if band < 0 {
			band = 0
		}

		// Compute magnitude
		real := real(coeffs[bin])
		imag := imag(coeffs[bin])
		magnitude := math.Sqrt(real*real + imag*imag)

		// Convert to dB scale with better dynamic range for music
		// Use -60dB to 0dB range (more sensitive than -100dB)
		db := 20 * math.Log10(magnitude/float64(fftSize)+1e-10)
		// Normalize to 0-255 (using -60dB to 0dB range for better sensitivity)
		normalized := (db + 60) / 60 * 255
		if normalized < 0 {
			normalized = 0
		}
		if normalized > 255 {
			normalized = 255
		}

		a.bands[band] += normalized
		bandCounts[band]++
	}

	// Average each band
	for i := range a.bands {
		if bandCounts[i] > 0 {
			a.bands[i] /= float64(bandCounts[i])
		}
	}

	// Spread energy to adjacent bands for smoother visualization
	// This helps fill in gaps where no FFT bins map directly
	spreadBands := make([]float64, numBands)
	for i := range a.bands {
		spreadBands[i] = a.bands[i]
		// Add 30% of adjacent bands
		if i > 0 {
			spreadBands[i] += a.bands[i-1] * 0.3
		}
		if i < numBands-1 {
			spreadBands[i] += a.bands[i+1] * 0.3
		}
		// Normalize back
		if spreadBands[i] > 255 {
			spreadBands[i] = 255
		}
	}

	// Apply temporal smoothing
	for i := range a.smoothedBands {
		a.smoothedBands[i] = smoothingFactor*a.smoothedBands[i] + (1-smoothingFactor)*spreadBands[i]
	}
}

// GetBands returns the current frequency bands (0-255 values, similar to Web Audio API)
func (a *AudioAnalyzer) GetBands() []uint8 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]uint8, numBands)
	for i, v := range a.smoothedBands {
		if v > 255 {
			result[i] = 255
		} else if v < 0 {
			result[i] = 0
		} else {
			result[i] = uint8(v)
		}
	}
	return result
}

// SetCallback registers a callback that is called immediately when new audio data is ready
// This enables true real-time push without polling
func (a *AudioAnalyzer) SetCallback(cb AudioDataCallback) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.callback = cb
}

// IsReady returns true if the analyzer has collected enough data
func (a *AudioAnalyzer) IsReady() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ready
}

// Reset clears the analyzer state
func (a *AudioAnalyzer) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.bufferIndex = 0
	a.ready = false
	for i := range a.sampleBuffer {
		a.sampleBuffer[i] = 0
	}
	for i := range a.bands {
		a.bands[i] = 0
	}
	for i := range a.smoothedBands {
		a.smoothedBands[i] = 0
	}
}
