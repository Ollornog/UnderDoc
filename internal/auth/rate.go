// SPDX-License-Identifier: MIT

package auth

import (
	"sync"
	"time"
)

// Rate allows a key at most so many uses in a sliding window, such as revealing passwords (ADR-7): enough for someone
// working through a page, too few to read out a whole workspace at once. It lives in memory, per instance.
type Rate struct {
	mu     sync.Mutex
	now    func() time.Time
	most   int
	window time.Duration
	used   map[string][]time.Time
}

// NewRate returns a limit of most uses per window for each key.
func NewRate(most int, window time.Duration) *Rate {
	return &Rate{now: time.Now, most: most, window: window, used: map[string][]time.Time{}}
}

// Take counts a use of the key if the limit allows it, and otherwise returns how long until it does.
func (r *Rate) Take(key string) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	from := now.Add(-r.window)
	kept := r.used[key][:0]
	for _, at := range r.used[key] {
		if at.After(from) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= r.most {
		r.used[key] = kept
		return false, kept[0].Sub(from)
	}
	r.used[key] = append(kept, now)
	if len(r.used) > 10000 {
		for k, times := range r.used {
			if len(times) == 0 || !times[len(times)-1].After(from) {
				delete(r.used, k)
			}
		}
	}
	return true, 0
}
