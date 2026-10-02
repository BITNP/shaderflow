package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type session struct {
	Sub       string    `json:"sub"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expiresAt"`
	LastSeen  time.Time `json:"lastSeen"`
}
type sessionStore struct {
	mu        sync.Mutex
	items     map[string]session
	ttl, idle time.Duration
	path      string
}

func newSessionStore() *sessionStore {
	return &sessionStore{items: map[string]session{}, ttl: 12 * time.Hour, idle: 2 * time.Hour}
}
func newFileSessionStore(path string) (*sessionStore, error) {
	s := newSessionStore()
	s.path = path
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.items); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *sessionStore) create(sub, name, role string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = session{Sub: sub, Name: name, Role: role, ExpiresAt: now.Add(s.ttl), LastSeen: now}
	if err := s.persistLocked(); err != nil {
		delete(s.items, id)
		return "", err
	}
	return id, nil
}
func (s *sessionStore) createExclusive(sub, name, role string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := map[string]session{}
	for key, value := range s.items {
		if value.Sub == sub && value.Role == role {
			previous[key] = value
			delete(s.items, key)
		}
	}
	s.items[id] = session{Sub: sub, Name: name, Role: role, ExpiresAt: now.Add(s.ttl), LastSeen: now}
	if err := s.persistLocked(); err != nil {
		delete(s.items, id)
		for key, value := range previous {
			s.items[key] = value
		}
		return "", err
	}
	return id, nil
}
func (s *sessionStore) get(id string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.items[id]
	if !ok {
		return session{}, false
	}
	now := time.Now()
	if now.After(value.ExpiresAt) || now.Sub(value.LastSeen) > s.idle {
		delete(s.items, id)
		_ = s.persistLocked()
		return session{}, false
	}
	value.LastSeen = now
	s.items[id] = value
	_ = s.persistLocked()
	return value, true
}
func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.items, id)
	_ = s.persistLocked()
	s.mu.Unlock()
}
func (s *sessionStore) persistLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(s.items)
	if err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, s.path)
}
