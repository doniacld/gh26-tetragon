package main

import (
	"sync"
	"time"
)

// rateLimit allows at most n events per window, across the whole site.
type rateLimit struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	start  time.Time
	count  int
}

func newRateLimit(n int, window time.Duration) *rateLimit {
	return &rateLimit{n: n, window: window}
}

func (l *rateLimit) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := time.Now(); now.Sub(l.start) >= l.window {
		l.start, l.count = now, 0
	}
	if l.count >= l.n {
		return false
	}
	l.count++
	return true
}
