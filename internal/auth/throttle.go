// SPDX-License-Identifier: MIT

package auth

import (
	"sync"
	"time"
)

// Throttle is the limit against guessing: after a few failed sign-ins for the same key, such as tenant and login, each
// further try has to wait, twice as long each time, up to a quarter of an hour. A success clears it. It lives in memory,
// so it is per instance; the waiting time is short enough that a restart gains nothing worth having.
type Throttle struct {
	mu    sync.Mutex
	now   func() time.Time
	state map[string]*tries
}

type tries struct {
	failures int
	until    time.Time
	last     time.Time
}

// Free failures before the waiting starts, and its bounds.
const (
	freeFailures = 5
	firstWait    = 2 * time.Second
	longestWait  = 15 * time.Minute
)

// NewThrottle returns an empty limit.
func NewThrottle() *Throttle {
	return &Throttle{now: time.Now, state: map[string]*tries{}}
}

// Wait returns how long the key has to wait before its next try; zero means it may try now.
func (t *Throttle) Wait(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.state[key]
	if s == nil {
		return 0
	}
	if left := s.until.Sub(t.now()); left > 0 {
		return left
	}
	return 0
}

// Failed counts a failed try of the key.
func (t *Throttle) Failed(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	t.forget(now)
	s := t.state[key]
	if s == nil {
		s = &tries{}
		t.state[key] = s
	}
	s.failures++
	s.last = now
	if over := s.failures - freeFailures; over > 0 {
		wait := longestWait
		if over < 20 {
			wait = min(firstWait<<(over-1), longestWait)
		}
		s.until = now.Add(wait)
	}
}

// Succeeded clears the key.
func (t *Throttle) Succeeded(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.state, key)
}

// forget drops keys quiet for a day, so the map does not grow without end.
func (t *Throttle) forget(now time.Time) {
	if len(t.state) < 10000 {
		return
	}
	for key, s := range t.state {
		if now.Sub(s.last) > 24*time.Hour {
			delete(t.state, key)
		}
	}
}
