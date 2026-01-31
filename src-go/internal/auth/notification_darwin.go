//go:build darwin

package auth

import (
	"fmt"
	"log"
	"os/exec"
)

// ShowPairingNotification displays a system notification for pairing requests on macOS
func ShowPairingNotification(clientName string) error {
	// Use osascript to display a notification via AppleScript
	script := fmt.Sprintf(`display notification "Client '%s' wants to connect to your music daemon" with title "Music Daemon Pairing Request" sound name "default"`, clientName)

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		log.Printf("[AUTH] Failed to show notification: %v", err)
		return err
	}

	log.Printf("[AUTH] Showed pairing notification for client: %s", clientName)
	return nil
}
