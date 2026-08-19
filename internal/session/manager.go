package session

import (
	"context"
	"errors"
	"sync"
)

// Manager stores sessions in memory for the lifetime of the Ivy process.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	options  normalizedOptions
	closed   bool
}

// NewManager creates an empty session manager.
func NewManager(options ManagerOptions) (*Manager, error) {
	normalized, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	return &Manager{
		sessions: make(map[string]*Session),
		options:  normalized,
	}, nil
}

// Start launches and registers a session.
func (m *Manager) Start(ctx context.Context, argv []string, options StartOptions) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}

	started, err := start(ctx, argv, options, m.options)
	if err != nil {
		return nil, err
	}
	if _, exists := m.sessions[started.id]; exists {
		_ = started.Close()
		return nil, errors.New("generated duplicate session ID")
	}
	m.sessions[started.id] = started
	return started, nil
}

// Get resolves a session by its non-authenticating routing ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resolved, ok := m.sessions[id]
	return resolved, ok
}

// Close gracefully shuts down all registered sessions and prevents new ones.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	sessions := make([]*Session, 0, len(m.sessions))
	for _, managed := range m.sessions {
		sessions = append(sessions, managed)
	}
	m.mu.Unlock()

	results := make(chan error, len(sessions))
	for _, managed := range sessions {
		go func(session *Session) {
			results <- session.Close()
		}(managed)
	}

	var closeErr error
	for range sessions {
		select {
		case err := <-results:
			closeErr = errors.Join(closeErr, err)
		case <-ctx.Done():
			return errors.Join(closeErr, ctx.Err())
		}
	}
	return closeErr
}
