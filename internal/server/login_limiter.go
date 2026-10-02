package server

import (
	"sync"
	"time"
)

type loginAttempt struct {
	Count int
	Reset time.Time
}
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
	limit    int
	window   time.Duration
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: map[string]loginAttempt{}, limit: 5, window: time.Minute}
}
func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	a := l.attempts[key]
	if now.After(a.Reset) {
		a = loginAttempt{Reset: now.Add(l.window)}
	}
	if a.Count >= l.limit {
		return false
	}
	return true
}
func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	a := l.attempts[key]
	if now.After(a.Reset) {
		a = loginAttempt{Reset: now.Add(l.window)}
	}
	a.Count++
	l.attempts[key] = a
}
func (l *loginLimiter) success(key string) { l.mu.Lock(); delete(l.attempts, key); l.mu.Unlock() }
