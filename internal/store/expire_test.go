package store

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock tests move by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// size returns the number of stored keys and keys with an expiry, without
// touching (and so lazily expiring) any of them.
func (s *Store) size() (keys, withExpiry int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data), len(s.expires)
}

func TestLazyExpiry(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Set("temp", []byte("hello"), clock.Now().Add(2*time.Second))

	clock.Advance(1999 * time.Millisecond)
	if v, ok, _ := s.Get("temp"); !ok || string(v) != "hello" {
		t.Fatalf("before expiry: %q, %v", v, ok)
	}
	clock.Advance(time.Millisecond)
	if _, ok, _ := s.Get("temp"); ok {
		t.Fatal("key still readable at its expiry time")
	}
	// The read deleted it, index included.
	if keys, exp := s.size(); keys != 0 || exp != 0 {
		t.Fatalf("after lazy delete: %d keys, %d with expiry", keys, exp)
	}
}

func TestSetWithoutExpiryClearsOldExpiry(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Set("k", []byte("v1"), clock.Now().Add(time.Second))
	s.Set("k", []byte("v2"), time.Time{})
	if _, exp := s.size(); exp != 0 {
		t.Fatalf("expires index still has %d keys", exp)
	}
	clock.Advance(time.Hour)
	if v, ok, _ := s.Get("k"); !ok || string(v) != "v2" {
		t.Fatalf("plain SET did not clear the expiry: %q, %v", v, ok)
	}
}

func TestExpireCycleRemovesUnreadKeys(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	for i := 0; i < 1000; i++ {
		s.Set(fmt.Sprintf("short%d", i), []byte("x"), clock.Now().Add(time.Second))
	}
	for i := 0; i < 10; i++ {
		s.Set(fmt.Sprintf("long%d", i), []byte("x"), clock.Now().Add(time.Hour))
		s.Set(fmt.Sprintf("forever%d", i), []byte("x"), time.Time{})
	}

	// Nothing has expired yet: one round looks at 20 keys and stops.
	if n := s.ExpireCycle(); n != 0 {
		t.Fatalf("deleted %d keys before anything expired", n)
	}

	clock.Advance(2 * time.Second)
	// With most sampled keys expired the cycle keeps going, so a few cycles
	// clear almost everything without any key being read.
	for i := 0; i < 50; i++ {
		s.ExpireCycle()
	}
	keys, exp := s.size()
	if keys != 20 || exp != 10 {
		t.Fatalf("after cleanup: %d keys (want 20), %d with expiry (want 10)", keys, exp)
	}
}

func TestExpireCycleRepeatsWhenManyExpired(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	for i := 0; i < 500; i++ {
		s.Set(fmt.Sprintf("k%d", i), []byte("x"), clock.Now().Add(time.Second))
	}
	clock.Advance(time.Minute)
	// All sampled keys are expired, so one call must do many rounds of 20.
	if n := s.ExpireCycle(); n <= sampleSize {
		t.Fatalf("one cycle deleted %d keys; it should repeat while most samples are expired", n)
	}
}

func TestStartCleanupWithRealClock(t *testing.T) {
	s := New()
	for i := 0; i < 100; i++ {
		s.Set(fmt.Sprintf("k%d", i), []byte("x"), time.Now().Add(50*time.Millisecond))
	}
	stop := s.StartCleanup()
	defer stop()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if keys, _ := s.size(); keys == 0 {
			stop()
			stop() // safe to call twice
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	keys, _ := s.size()
	t.Fatalf("background cleanup left %d of 100 expired keys", keys)
}
