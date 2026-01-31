// Package auth handles client authentication and authorization.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	tokenBytes      = 32 // 256-bit tokens
	maxAuthFailures = 5
	lockoutDuration = 60 * time.Second
)

// Manager handles client authentication
type Manager struct {
	store    *Store
	testMode bool

	mu           sync.RWMutex
	authFailures map[string]int       // IP -> failure count
	lockouts     map[string]time.Time // IP -> lockout end time
}

// NewManager creates a new auth manager
func NewManager(store *Store, testMode bool) *Manager {
	return &Manager{
		store:        store,
		testMode:     testMode,
		authFailures: make(map[string]int),
		lockouts:     make(map[string]time.Time),
	}
}

// Pair initiates the pairing process for a client
// In test mode, pairing is auto-approved
// Returns: token, clientID, requiresApproval, error
func (m *Manager) Pair(clientName string) (string, string, bool, error) {
	// Generate client ID
	clientID := generateClientID()

	// Generate token
	token, err := generateToken()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to generate token: %w", err)
	}

	// In test mode, auto-approve
	if m.testMode {
		if err := m.store.AddClient(clientID, clientName, token); err != nil {
			return "", "", false, fmt.Errorf("failed to store client: %w", err)
		}
		return token, clientID, false, nil
	}

	// Show OS notification for pairing request
	if err := ShowPairingNotification(clientName); err != nil {
		// Log the error but continue - notification is not critical
		log.Printf("[AUTH] Failed to show pairing notification: %v", err)
	}

	// Store the client (currently auto-approves after notification)
	// Future: implement pending approval mechanism
	if err := m.store.AddClient(clientID, clientName, token); err != nil {
		return "", "", false, fmt.Errorf("failed to store client: %w", err)
	}

	return token, clientID, true, nil
}

// ValidateToken checks if a token is valid
func (m *Manager) ValidateToken(token string) bool {
	if token == "" {
		return false
	}

	return m.store.ValidateToken(token)
}

// RecordAuthFailure records an authentication failure
func (m *Manager) RecordAuthFailure(clientIP string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.authFailures[clientIP]++

	if m.authFailures[clientIP] >= maxAuthFailures {
		m.lockouts[clientIP] = time.Now().Add(lockoutDuration)
		m.authFailures[clientIP] = 0
	}
}

// IsLockedOut checks if a client IP is locked out
func (m *Manager) IsLockedOut(clientIP string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lockoutEnd, exists := m.lockouts[clientIP]
	if !exists {
		return false
	}

	if time.Now().After(lockoutEnd) {
		// Lockout expired, clean up
		go func() {
			m.mu.Lock()
			delete(m.lockouts, clientIP)
			m.mu.Unlock()
		}()
		return false
	}

	return true
}

// RevokeClient revokes a client's access
func (m *Manager) RevokeClient(clientID string) error {
	return m.store.RemoveClient(clientID)
}

// ListClients returns all registered clients
func (m *Manager) ListClients() ([]ClientInfo, error) {
	return m.store.ListClients()
}

func generateToken() (string, error) {
	bytes := make([]byte, tokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateClientID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// HashToken creates a SHA-256 hash of a token for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// ClientInfo contains information about a registered client
type ClientInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

var (
	ErrClientNotFound = errors.New("client not found")
	ErrUnauthorized   = errors.New("unauthorized")
)
