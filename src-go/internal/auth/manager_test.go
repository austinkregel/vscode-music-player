package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	store := createTestStore(t)
	manager := NewManager(store, false)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.testMode {
		t.Error("Expected testMode to be false")
	}
}

func TestNewManagerTestMode(t *testing.T) {
	store := createTestStore(t)
	manager := NewManager(store, true)

	if !manager.testMode {
		t.Error("Expected testMode to be true")
	}
}

func TestPair(t *testing.T) {
	store := createTestStore(t)
	manager := NewManager(store, true) // Use test mode for auto-approval

	token, clientID, requiresApproval, err := manager.Pair("Test Client")
	if err != nil {
		t.Fatalf("Pair failed: %v", err)
	}

	if token == "" {
		t.Error("Expected non-empty token")
	}

	if clientID == "" {
		t.Error("Expected non-empty clientID")
	}

	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("Expected token length 64, got %d", len(token))
	}

	// In test mode, should not require approval
	if requiresApproval {
		t.Error("Expected requiresApproval to be false in test mode")
	}
}

func TestPairNonTestMode(t *testing.T) {
	store := createTestStore(t)
	manager := NewManager(store, false) // Normal mode

	_, _, requiresApproval, err := manager.Pair("Test Client")
	if err != nil {
		t.Fatalf("Pair failed: %v", err)
	}

	// In normal mode, should require approval
	if !requiresApproval {
		t.Error("Expected requiresApproval to be true in normal mode")
	}
}

func TestValidateToken(t *testing.T) {
	store := createTestStore(t)
	manager := NewManager(store, true)

	// Get a valid token
	token, _, _, err := manager.Pair("Test Client")
	if err != nil {
		t.Fatalf("Pair failed: %v", err)
	}

	// Should validate the token
	if !manager.ValidateToken(token) {
		t.Error("Expected token to be valid")
	}

	// Invalid token should fail
	if manager.ValidateToken("invalid-token") {
		t.Error("Expected invalid token to fail validation")
	}

	// Empty token should fail
	if manager.ValidateToken("") {
		t.Error("Expected empty token to fail validation")
	}
}

func TestRecordAuthFailure(t *testing.T) {
	store := createTestStore(t)
	manager := NewManager(store, false)

	clientIP := "192.168.1.1"

	// Record failures up to the limit
	for i := 0; i < maxAuthFailures-1; i++ {
		manager.RecordAuthFailure(clientIP)
		if manager.IsLockedOut(clientIP) {
			t.Errorf("Should not be locked out after %d failures", i+1)
		}
	}

	// One more failure should trigger lockout
	manager.RecordAuthFailure(clientIP)
	if !manager.IsLockedOut(clientIP) {
		t.Error("Should be locked out after max failures")
	}
}

func TestLockoutExpires(t *testing.T) {
	store := createTestStore(t)
	manager := NewManager(store, false)

	clientIP := "192.168.1.2"

	// Manually set a lockout that's already expired
	manager.mu.Lock()
	manager.lockouts[clientIP] = time.Now().Add(-1 * time.Second)
	manager.mu.Unlock()

	// Should not be locked out (lockout expired)
	if manager.IsLockedOut(clientIP) {
		t.Error("Should not be locked out after lockout expires")
	}
}

func TestHashToken(t *testing.T) {
	token := "test-token-123"
	hash1 := HashToken(token)
	hash2 := HashToken(token)

	// Same input should produce same hash
	if hash1 != hash2 {
		t.Error("Same token should produce same hash")
	}

	// Different input should produce different hash
	hash3 := HashToken("different-token")
	if hash1 == hash3 {
		t.Error("Different tokens should produce different hashes")
	}

	// Hash should be 64 characters (SHA-256 = 32 bytes = 64 hex chars)
	if len(hash1) != 64 {
		t.Errorf("Expected hash length 64, got %d", len(hash1))
	}
}

func createTestStore(t *testing.T) *Store {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	store, err := NewStore(filepath.Join(tmpDir, "clients.json"))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	return store
}
