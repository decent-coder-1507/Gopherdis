package store

import (
	"sync"
	"time"
)

// Active expiry settings, the same numbers real Redis uses.
const (
	// cleanupInterval is how often the background cleanup runs (10 times a second).
	cleanupInterval = 100 * time.Millisecond
	// sampleSize is how many keys with an expiry each round looks at.
	sampleSize = 20
	// repeatAbove: if more than this many of the sample were expired, many
	// more probably are too, so the cleanup goes again straight away.
	repeatAbove = sampleSize / 4
	// cycleBudget caps one cleanup run, so a huge batch of keys expiring at
	// once can't keep clients waiting for the lock for long.
	cycleBudget = 25 * time.Millisecond
)

// ExpireCycle runs one cleanup: it samples up to 20 keys that have an
// expiry and deletes the expired ones, and repeats while more than 5 of a
// sample were expired. It returns how many keys it deleted.
//
// Go's map iteration starts at a random position, which gives us a cheap
// random sample. Each round takes the write lock separately, so clients can
// run commands between rounds.
func (s *Store) ExpireCycle() int {
	start := time.Now()
	total := 0
	for {
		s.mu.Lock()
		now := s.now()
		sampled, expired := 0, 0
		for key := range s.expires {
			if sampled == sampleSize {
				break
			}
			sampled++
			if s.data[key].expired(now) {
				s.remove(key)
				expired++
			}
		}
		s.mu.Unlock()

		total += expired
		if expired <= repeatAbove || time.Since(start) > cycleBudget {
			return total
		}
	}
}

// StartCleanup runs ExpireCycle in the background about 10 times a second
// until the returned stop function is called. stop waits for the
// background goroutine to finish and is safe to call more than once.
func (s *Store) StartCleanup() (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(cleanupInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.ExpireCycle()
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		<-finished
	}
}
