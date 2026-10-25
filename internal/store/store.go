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
// after the store has moved on.
type Entry struct {
	Kind Kind
	Str  []byte   // used when Kind is KindString
	List [][]byte // used when Kind is KindList
	// ExpireAt is when the key stops existing. Zero means never.
	ExpireAt time.Time
}

// Store is the in-memory database: a map from key to entry. It is safe for
// use by many goroutines. Reads take the read lock, writes the full lock, and
// no method does slow work (like network I/O) while holding either.
type Store struct {
	mu   sync.RWMutex
	data map[string]*Entry
}

// New returns an empty store.
func New() *Store {
	return &Store{data: make(map[string]*Entry)}
}

// Get returns the string stored at key. ok is false if the key doesn't exist.
func (s *Store) Get(key string) (val []byte, ok bool, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	if !ok {
		return nil, false, nil
	}
	if e.Kind != KindString {
		return nil, false, ErrWrongType
	}
	return e.Str, true, nil
}

// Set stores a copy of val at key, replacing whatever was there before,
// whatever its type.
func (s *Store) Set(key string, val []byte) {
	e := &Entry{Kind: KindString, Str: clone(val)} // copy before taking the lock
	s.mu.Lock()
	s.data[key] = e
	s.mu.Unlock()
}

// clone copies b, keeping an empty value non-nil so it stays distinct from "missing".
func clone(b []byte) []byte {
	return append(make([]byte, 0, len(b)), b...)
}
