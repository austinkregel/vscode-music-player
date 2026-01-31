//go:build !linux && !darwin && !windows

package auth

import (
	"log"
)

// ShowPairingNotification is a stub for unsupported platforms
func ShowPairingNotification(clientName string) error {
	log.Printf("[AUTH] Pairing notification not supported on this platform")
	log.Printf("[AUTH] Client '%s' wants to connect - auto-approving", clientName)
	return nil
}
