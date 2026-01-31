//go:build linux

package auth

import (
	"fmt"
	"log"
	"os/exec"
)

// ShowPairingNotification displays a system notification for pairing requests on Linux
func ShowPairingNotification(clientName string) error {
	// Use notify-send which is available on most Linux systems with a desktop environment
	cmd := exec.Command("notify-send",
		"Music Daemon Pairing Request",
		fmt.Sprintf("Client '%s' wants to connect to your music daemon", clientName),
		"--urgency=critical",
		"--icon=audio-x-generic",
	)

	if err := cmd.Run(); err != nil {
		log.Printf("[AUTH] Failed to show notification: %v", err)
		return err
	}

	log.Printf("[AUTH] Showed pairing notification for client: %s", clientName)
	return nil
}
