package auth

import (
	"context"
	"sync"
	"time"
)

// PairingState represents the state of a pairing request
type PairingState string

const (
	PairingPending  PairingState = "pending"
	PairingApproved PairingState = "approved"
	PairingDenied   PairingState = "denied"
	PairingExpired  PairingState = "expired"
)

const pairingTimeout = 60 * time.Second

// PairingRequest represents a pending pairing request
type PairingRequest struct {
	ID         string
	ClientName string
	State      PairingState
	Token      string // Only set if approved
	CreatedAt  time.Time
}

// PairingManager handles the pairing approval flow
type PairingManager struct {
	mu       sync.RWMutex
	requests map[string]*PairingRequest

	// Approval callback - called when a pairing request is created
	// Should show UI to user and call Approve/Deny
	OnPairingRequest func(req *PairingRequest)
}

// NewPairingManager creates a new pairing manager
func NewPairingManager() *PairingManager {
	pm := &PairingManager{
		requests: make(map[string]*PairingRequest),
	}

	// Start cleanup goroutine
	go pm.cleanupLoop()

	return pm
}

// CreateRequest creates a new pairing request
func (pm *PairingManager) CreateRequest(clientName string) *PairingRequest {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	id := generateClientID()
	req := &PairingRequest{
		ID:         id,
		ClientName: clientName,
		State:      PairingPending,
		CreatedAt:  time.Now(),
	}

	pm.requests[id] = req

	if pm.OnPairingRequest != nil {
		go pm.OnPairingRequest(req)
	}

	return req
}

// Approve approves a pairing request and returns the token
func (pm *PairingManager) Approve(requestID string) (string, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	req, exists := pm.requests[requestID]
	if !exists {
		return "", ErrClientNotFound
	}

	if req.State != PairingPending {
		return "", ErrUnauthorized
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}

	req.State = PairingApproved
	req.Token = token

	return token, nil
}

// Deny denies a pairing request
func (pm *PairingManager) Deny(requestID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	req, exists := pm.requests[requestID]
	if !exists {
		return ErrClientNotFound
	}

	if req.State != PairingPending {
		return ErrUnauthorized
	}

	req.State = PairingDenied

	return nil
}

// GetRequest returns a pairing request by ID
func (pm *PairingManager) GetRequest(requestID string) (*PairingRequest, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	req, exists := pm.requests[requestID]
	if !exists {
		return nil, ErrClientNotFound
	}

	return req, nil
}

// WaitForApproval waits for a pairing request to be approved or denied
func (pm *PairingManager) WaitForApproval(ctx context.Context, requestID string) (*PairingRequest, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			req, err := pm.GetRequest(requestID)
			if err != nil {
				return nil, err
			}

			if req.State != PairingPending {
				return req, nil
			}
		}
	}
}

func (pm *PairingManager) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		pm.cleanup()
	}
}

func (pm *PairingManager) cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	for id, req := range pm.requests {
		if now.Sub(req.CreatedAt) > pairingTimeout {
			if req.State == PairingPending {
				req.State = PairingExpired
			}
			// Remove old requests
			if now.Sub(req.CreatedAt) > 5*time.Minute {
				delete(pm.requests, id)
			}
		}
	}
}
