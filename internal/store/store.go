package store

import (
	"errors"
	"sync"
	"time"
)

// Kind says what type of value an entry holds.
type Kind uint8

// The value types.
const (
	KindString Kind = iota
	KindList
)

// ErrWrongType is returned when a command meets a key of the wrong kind,
// for example GET on a list.
var ErrWrongType = errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")

// Entry is one stored value.
//
// Stored byte slices are never changed in place: a new value always gets a
// new slice. That lets callers keep using a slice they got from the store
// after the store has moved on. A List does change in place, under the write
// lock, so readers copy what they need out of it while holding the read lock.
type Entry struct {
	Kind Kind
	Str  []byte // used when Kind is KindString
	List *List  // used when Kind is KindList
	// ExpireAt is when the key stops existing. Zero means never.
	ExpireAt time.Time
}

// expired reports whether the entry's time is up at now.
func (e *Entry) expired(now time.Time) bool {
	return !e.ExpireAt.IsZero() && !now.Before(e.ExpireAt)
}

// Store is the in-memory database: a map from key to entry. It is safe for
// use by many goroutines. Reads take the read lock, writes the full lock, and
// no method does slow work (like network I/O) while holding either.
//
// Expired keys are removed in two ways: lazily, when a command touches them,
// and actively, by a background cleanup that samples keys with an expiry
// (see StartCleanup), so keys nobody reads again don't stay in memory.
type Store struct {
	mu   sync.RWMutex
	data map[string]*Entry
	// expires holds every key that has an expiry, so the cleanup can
	// sample them without walking the whole database.
	expires map[string]struct{}
	now     func() time.Time
}

// New returns an empty store that uses the real clock.
func New() *Store {
	return NewWithClock(time.Now)
}

// NewWithClock returns an empty store that reads the time from now.
// Tests use it to move time forward without sleeping.
func NewWithClock(now func() time.Time) *Store {
	return &Store{
		data:    make(map[string]*Entry),
		expires: make(map[string]struct{}),
		now:     now,
	}
}

// Now returns the store's current time. Commands use it to turn relative
// expiry times into absolute ones.
func (s *Store) Now() time.Time {
	return s.now()
}

// lookup returns the live entry for key. It must be called with at least
// the read lock held. An expired entry is reported as missing; the caller
// decides whether to delete it.
func (s *Store) lookup(key string, now time.Time) (e *Entry, expired bool) {
	e, ok := s.data[key]
	if !ok {
		return nil, false
	}
	if e.expired(now) {
		return nil, true
	}
	return e, false
}

// deleteIfExpired removes key if it is still expired. It takes the write
// lock itself, so it is called after a read-locked lookup found an expired
// key. The second check is needed because another client may have replaced
// the key in between.
func (s *Store) deleteIfExpired(key string) {
	s.mu.Lock()
	if e, ok := s.data[key]; ok && e.expired(s.now()) {
		s.remove(key)
	}
	s.mu.Unlock()
}

// remove deletes key and its expiry. The write lock must be held.
func (s *Store) remove(key string) {
	delete(s.data, key)
	delete(s.expires, key)
}

// put stores e at key and keeps the expires index in step. The write lock
// must be held.
func (s *Store) put(key string, e *Entry) {
	s.data[key] = e
	if e.ExpireAt.IsZero() {
		delete(s.expires, key)
	} else {
		s.expires[key] = struct{}{}
	}
}

// Get returns the string stored at key. ok is false if the key doesn't exist.
func (s *Store) Get(key string) (val []byte, ok bool, err error) {
	s.mu.RLock()
	e, expired := s.lookup(key, s.now())
	s.mu.RUnlock()

	if expired {
		s.deleteIfExpired(key)
		return nil, false, nil
	}
	if e == nil {
		return nil, false, nil
	}
	if e.Kind != KindString {
		return nil, false, ErrWrongType
	}
	return e.Str, true, nil
}

// Set stores a copy of val at key, replacing whatever was there before,
// whatever its type. expireAt is when the key should disappear; the zero
// time means never, which also clears any expiry the old value had.
func (s *Store) Set(key string, val []byte, expireAt time.Time) {
	e := &Entry{Kind: KindString, Str: clone(val), ExpireAt: expireAt} // copy before taking the lock
	s.mu.Lock()
	s.put(key, e)
	s.mu.Unlock()
}

// clone copies b, keeping an empty value non-nil so it stays distinct from "missing".
func clone(b []byte) []byte {
	return append(make([]byte, 0, len(b)), b...)
}
