package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storePath := filepath.Join(tmpDir, "clients.json")
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	if store == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestNewStoreWithExistingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storePath := filepath.Join(tmpDir, "clients.json")

	// Create store and add a client
	store1, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	err = store1.AddClient("client1", "Test Client", "test-token")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	// Create new store instance and verify client is loaded
	store2, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore (reload) failed: %v", err)
	}

	if !store2.ValidateToken("test-token") {
		t.Error("Token should be valid after reload")
	}
}

func TestAddClient(t *testing.T) {
	store := createTestStoreForStore(t)

	err := store.AddClient("client1", "Test Client", "test-token-123")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	// Verify client exists
	clients, err := store.ListClients()
	if err != nil {
		t.Fatalf("ListClients failed: %v", err)
	}

	if len(clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(clients))
	}

	if clients[0].ID != "client1" {
		t.Errorf("Expected client ID 'client1', got '%s'", clients[0].ID)
	}

	if clients[0].Name != "Test Client" {
		t.Errorf("Expected client name 'Test Client', got '%s'", clients[0].Name)
	}
}

func TestRemoveClient(t *testing.T) {
	store := createTestStoreForStore(t)

	// Add a client
	err := store.AddClient("client1", "Test Client", "test-token")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	// Remove the client
	err = store.RemoveClient("client1")
	if err != nil {
		t.Fatalf("RemoveClient failed: %v", err)
	}

	// Verify client is gone
	clients, err := store.ListClients()
	if err != nil {
		t.Fatalf("ListClients failed: %v", err)
	}

	if len(clients) != 0 {
		t.Errorf("Expected 0 clients, got %d", len(clients))
	}
}

func TestRemoveNonExistentClient(t *testing.T) {
	store := createTestStoreForStore(t)

	err := store.RemoveClient("nonexistent")
	if err != ErrClientNotFound {
		t.Errorf("Expected ErrClientNotFound, got %v", err)
	}
}

func TestStoreValidateToken(t *testing.T) {
	store := createTestStoreForStore(t)

	token := "valid-token-123"
	err := store.AddClient("client1", "Test Client", token)
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	// Valid token should pass
	if !store.ValidateToken(token) {
		t.Error("Expected valid token to pass validation")
	}

	// Invalid token should fail
	if store.ValidateToken("invalid-token") {
		t.Error("Expected invalid token to fail validation")
	}
}

func TestGetClientByToken(t *testing.T) {
	store := createTestStoreForStore(t)

	token := "test-token-456"
	err := store.AddClient("client1", "Test Client", token)
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	client, err := store.GetClientByToken(token)
	if err != nil {
		t.Fatalf("GetClientByToken failed: %v", err)
	}

	if client.ID != "client1" {
		t.Errorf("Expected client ID 'client1', got '%s'", client.ID)
	}
}

func TestGetClientByTokenNotFound(t *testing.T) {
	store := createTestStoreForStore(t)

	_, err := store.GetClientByToken("nonexistent-token")
	if err != ErrClientNotFound {
		t.Errorf("Expected ErrClientNotFound, got %v", err)
	}
}

func TestListClients(t *testing.T) {
	store := createTestStoreForStore(t)

	// Add multiple clients
	store.AddClient("client1", "Client 1", "token1")
	store.AddClient("client2", "Client 2", "token2")
	store.AddClient("client3", "Client 3", "token3")

	clients, err := store.ListClients()
	if err != nil {
		t.Fatalf("ListClients failed: %v", err)
	}

	if len(clients) != 3 {
		t.Errorf("Expected 3 clients, got %d", len(clients))
	}
}

func createTestStoreForStore(t *testing.T) *Store {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "store-test-*")
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
