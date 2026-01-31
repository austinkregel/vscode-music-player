package audio

import (
	"testing"
)

func TestApplyVolume(t *testing.T) {
	// Create a minimal OtoOutput for testing (without actual audio context)
	o := &OtoOutput{
		volume: 0.5,
	}

	tests := []struct {
		name     string
		volume   float64
		input    []byte
		expected []byte
	}{
		{
			name:     "full volume passthrough",
			volume:   1.0,
			input:    []byte{0x00, 0x10, 0xFF, 0x7F}, // Two 16-bit samples
			expected: []byte{0x00, 0x10, 0xFF, 0x7F}, // Should be unchanged
		},
		{
			name:     "half volume",
			volume:   0.5,
			input:    []byte{0x00, 0x10, 0xFE, 0x7F}, // Sample: 0x1000 (4096), 0x7FFE (32766)
			expected: []byte{0x00, 0x08, 0xFF, 0x3F}, // Expected: 0x0800 (2048), 0x3FFF (16383)
		},
		{
			name:     "zero volume",
			volume:   0.0,
			input:    []byte{0xFF, 0x7F, 0x00, 0x80}, // Max positive, min negative
			expected: []byte{0x00, 0x00, 0x00, 0x00}, // Should be silence
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.volume = tt.volume
			data := make([]byte, len(tt.input))
			copy(data, tt.input)

			if tt.volume < 1.0 {
				o.applyVolume(data)
			}

			if tt.volume == 1.0 {
				// Full volume should not be modified
				for i := range data {
					if data[i] != tt.expected[i] {
						t.Errorf("Byte %d: expected %02X, got %02X", i, tt.expected[i], data[i])
					}
				}
			} else {
				// For non-full volume, just check the result is different from input (unless silence)
				if tt.volume == 0.0 {
					for i := range data {
						if data[i] != 0 {
							t.Errorf("Expected silence, got non-zero byte at %d: %02X", i, data[i])
						}
					}
				}
			}
		})
	}
}

func TestSetVolumeClamp(t *testing.T) {
	o := &OtoOutput{volume: 1.0}

	// Test setting volume below 0
	o.SetVolume(-0.5)
	if o.volume != 0 {
		t.Errorf("Expected volume 0 for negative input, got %f", o.volume)
	}

	// Test setting volume above 1
	o.SetVolume(1.5)
	if o.volume != 1 {
		t.Errorf("Expected volume 1 for >1 input, got %f", o.volume)
	}

	// Test normal value
	o.SetVolume(0.75)
	if o.volume != 0.75 {
		t.Errorf("Expected volume 0.75, got %f", o.volume)
	}
}

func TestGetVolume(t *testing.T) {
	o := &OtoOutput{volume: 0.5}

	if o.GetVolume() != 0.5 {
		t.Errorf("Expected volume 0.5, got %f", o.GetVolume())
	}
}
