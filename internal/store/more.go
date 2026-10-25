package store

import "time"

// TTL returns how long key has left to live. exists is false if there is no
// such key; hasExpiry is false if the key never expires.
func (s *Store) TTL(key string) (left time.Duration, exists, hasExpiry bool) {
	s.mu.RLock()
	now := s.now()
	e, expired := s.lookup(key, now)
	s.mu.RUnlock()

	if expired {
		s.deleteIfExpired(key)
		return 0, false, false
	}
	if e == nil {
		return 0, false, false
	}
	if e.ExpireAt.IsZero() {
		return 0, true, false
	}
	return e.ExpireAt.Sub(now), true, true
}

// Expire sets key to expire at the given time. A time that has already
// passed deletes the key. It returns false if the key doesn't exist.
func (s *Store) Expire(key string, at time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	e, expired := s.lookup(key, now)
	if expired {
		s.remove(key)
		return false
	}
	if e == nil {
		return false
	}
	if !now.Before(at) {
		s.remove(key)
		s.changes++
		return true
	}
	// Entries are replaced, not edited, so a reader holding the old entry
	// isn't affected. Copying an Entry shares its value, which is fine.
	updated := *e
	updated.ExpireAt = at
	s.put(key, &updated)
	s.changes++
	return true
}

// Persist removes the expiry from key. It returns true only if the key
// exists and had an expiry.
func (s *Store) Persist(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, expired := s.lookup(key, s.now())
	if expired {
		s.remove(key)
		return false
	}
	if e == nil || e.ExpireAt.IsZero() {
		return false
	}
	updated := *e
	updated.ExpireAt = time.Time{}
	s.put(key, &updated)
	s.changes++
	return true
}

// Keys returns every live key for which match returns true, in no
// particular order.
func (s *Store) Keys(match func(key string) bool) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	keys := []string{}
	for k, e := range s.data {
		if !e.expired(now) && match(k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// LRange returns the list items from start to stop, both included. Negative
// indexes count from the end (-1 is the last item). Out-of-range indexes
// are clamped; a range with nothing in it, or a missing key, gives an empty
// slice.
func (s *Store) LRange(key string, start, stop int64) ([][]byte, error) {
	s.mu.RLock()
	e, expired := s.lookup(key, s.now())
	if e == nil {
		s.mu.RUnlock()
		if expired {
			s.deleteIfExpired(key)
		}
		return [][]byte{}, nil
	}
	defer s.mu.RUnlock()
	if e.Kind != KindList {
		return nil, ErrWrongType
	}

	n := int64(e.List.Len())
	if start < 0 {
		start += n
	}
	if stop < 0 {
		stop += n
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		return [][]byte{}, nil
	}
	// Slice copies the item headers while we still hold the read lock.
	return e.List.Slice(int(start), int(stop)), nil
}

// LLen returns the length of the list at key, 0 if there is no such key.
func (s *Store) LLen(key string) (int, error) {
	s.mu.RLock()
	e, expired := s.lookup(key, s.now())
	if e == nil {
		s.mu.RUnlock()
		if expired {
			s.deleteIfExpired(key)
		}
		return 0, nil
	}
	defer s.mu.RUnlock()
	if e.Kind != KindList {
		return 0, ErrWrongType
	}
	return e.List.Len(), nil
}

// MGet returns the string value of each key, in order. The result has nil
// for a key that is missing or holds something other than a string.
func (s *Store) MGet(keys []string) [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	out := make([][]byte, len(keys))
	for i, k := range keys {
		if e, _ := s.lookup(k, now); e != nil && e.Kind == KindString {
			out[i] = e.Str
		}
	}
	return out
}

// DBSize returns the number of live keys. Keys that have expired but
// haven't been cleaned up yet are not counted.
func (s *Store) DBSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	n := len(s.data)
	for k := range s.expires {
		if s.data[k].expired(now) {
			n--
		}
	}
	return n
}

// Flush deletes every key.
func (s *Store) Flush() {
	s.mu.Lock()
	s.data = make(map[string]*Entry)
	s.expires = make(map[string]struct{})
	s.changes++
	s.mu.Unlock()
}
