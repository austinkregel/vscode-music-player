//go:build !linux && !darwin && !windows
// +build !linux,!darwin,!windows

package media

import "fmt"

// NewSession creates a new platform-specific media session
// This is the fallback for unsupported platforms
func NewSession() (Session, error) {
	return nil, fmt.Errorf("media session not supported on this platform")
}
