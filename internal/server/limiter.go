package server

import (
	"net"
	"sync"
	"time"
)

const (
	maxAuthenticationFailures = 10
	authenticationWindow      = time.Minute
	maxLimiterEntries         = 1024
)

type failureRecord struct {
	windowStart time.Time
	updated     time.Time
	count       int
}

type failureLimiter struct {
	mu      sync.Mutex
	entries map[string]failureRecord
	now     func() time.Time
}

func newFailureLimiter() *failureLimiter {
	return &failureLimiter{
		entries: make(map[string]failureRecord),
		now:     time.Now,
	}
}

func (l *failureLimiter) allowed(remoteAddress string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	key := remoteIP(remoteAddress)
	record, ok := l.entries[key]
	if !ok || now.Sub(record.windowStart) >= authenticationWindow {
		if ok {
			delete(l.entries, key)
		}
		return true, 0
	}
	if record.count < maxAuthenticationFailures {
		return true, 0
	}
	retryAfter := authenticationWindow - now.Sub(record.windowStart)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter
}

func (l *failureLimiter) failed(remoteAddress string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	key := remoteIP(remoteAddress)
	record, ok := l.entries[key]
	if !ok || now.Sub(record.windowStart) >= authenticationWindow {
		if !ok && len(l.entries) >= maxLimiterEntries {
			l.evictOldest(now)
		}
		record = failureRecord{windowStart: now}
	}
	record.count++
	record.updated = now
	l.entries[key] = record
}

func (l *failureLimiter) succeeded(remoteAddress string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, remoteIP(remoteAddress))
}

func (l *failureLimiter) evictOldest(now time.Time) {
	var oldestKey string
	var oldestTime time.Time
	for key, record := range l.entries {
		if now.Sub(record.windowStart) >= authenticationWindow {
			delete(l.entries, key)
			return
		}
		if oldestKey == "" || record.updated.Before(oldestTime) {
			oldestKey = key
			oldestTime = record.updated
		}
	}
	if oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

func remoteIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		return remoteAddress
	}
	return host
}
