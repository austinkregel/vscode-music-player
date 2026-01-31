//go:build windows

package auth

import (
	"fmt"
	"log"
	"os/exec"
)

// ShowPairingNotification displays a system notification for pairing requests on Windows
func ShowPairingNotification(clientName string) error {
	// Use PowerShell to display a toast notification
	// This works on Windows 10 and later
	script := fmt.Sprintf(`
$ErrorActionPreference = "Stop"
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$template = @"
<toast>
    <visual>
        <binding template="ToastText02">
            <text id="1">Music Daemon Pairing Request</text>
            <text id="2">Client '%s' wants to connect to your music daemon</text>
        </binding>
    </visual>
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("MusicD").Show($toast)
`, clientName)

	cmd := exec.Command("powershell", "-Command", script)
	if err := cmd.Run(); err != nil {
		log.Printf("[AUTH] Failed to show notification (toast might not be supported): %v", err)
		// Fall back to a simple message box
		return showFallbackNotification(clientName)
	}

	log.Printf("[AUTH] Showed pairing notification for client: %s", clientName)
	return nil
}

// showFallbackNotification shows a simple message box as fallback
func showFallbackNotification(clientName string) error {
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.MessageBox]::Show("Client '%s' wants to connect to your music daemon", "Music Daemon Pairing Request", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Information)
`, clientName)

	cmd := exec.Command("powershell", "-Command", script)
	return cmd.Run()
}
