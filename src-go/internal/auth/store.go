package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// StoredClient represents a client stored on disk
type StoredClient struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"tokenHash"` // SHA-256 hash of token
	CreatedAt time.Time `json:"createdAt"`
}

// Store persists client information to disk
type Store struct {
	path    string
	mu      sync.RWMutex
	clients map[string]*StoredClient // clientID -> client
}

// NewStore creates a new auth store
func NewStore(path string) (*Store, error) {
	store := &Store{
		path:    path,
		clients: make(map[string]*StoredClient),
	}

	// Load existing data
	if err := store.load(); err != nil {
		// If file doesn't exist, that's ok
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load store: %w", err)
		}
	}

	return store, nil
}

// AddClient adds a new client to the store
func (s *Store) AddClient(clientID, name, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &StoredClient{
		ID:        clientID,
		Name:      name,
		TokenHash: HashToken(token),
		CreatedAt: time.Now(),
	}

	s.clients[clientID] = client

	return s.saveLocked()
}

// RemoveClient removes a client from the store
func (s *Store) RemoveClient(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.clients[clientID]; !exists {
		return ErrClientNotFound
	}

	delete(s.clients, clientID)

	return s.saveLocked()
}

// ValidateToken checks if a token is valid
func (s *Store) ValidateToken(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokenHash := HashToken(token)

	for _, client := range s.clients {
		if client.TokenHash == tokenHash {
			return true
		}
	}

	return false
}

// GetClientByToken returns the client associated with a token
func (s *Store) GetClientByToken(token string) (*StoredClient, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokenHash := HashToken(token)

	for _, client := range s.clients {
		if client.TokenHash == tokenHash {
			return client, nil
		}
	}

	return nil, ErrClientNotFound
}

// ListClients returns all registered clients
func (s *Store) ListClients() ([]ClientInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clients := make([]ClientInfo, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, ClientInfo{
			ID:        client.ID,
			Name:      client.Name,
			CreatedAt: client.CreatedAt,
		})
	}

	return clients, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var stored struct {
		Clients []*StoredClient `json:"clients"`
	}

	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to parse store: %w", err)
	}

	s.clients = make(map[string]*StoredClient)
	for _, client := range stored.Clients {
		s.clients[client.ID] = client
	}

	return nil
}

func (s *Store) saveLocked() error {
	clients := make([]*StoredClient, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}

	stored := struct {
		Clients []*StoredClient `json:"clients"`
	}{
		Clients: clients,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("failed to write store: %w", err)
	}

	return nil
}
