package store

import "time"

// Item is one key with its value, as copied out by Snapshot and put back by
// Restore. It has only exported fields so it can be written with encoding/gob.
type Item struct {
	Key      string
	Kind     Kind
	Str      []byte
	List     [][]byte
	ExpireAt time.Time
}

// Snapshot returns a copy of every live key, and the change count it
// matches (see Changes). It holds the read lock only while copying; the
// byte slices are shared, which is safe because stored bytes are never
// changed in place.
func (s *Store) Snapshot() (items []Item, changes uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	items = make([]Item, 0, len(s.data))
	for k, e := range s.data {
		if e.expired(now) {
			continue
		}
		it := Item{Key: k, Kind: e.Kind, ExpireAt: e.ExpireAt}
		if e.Kind == KindList {
			it.List = e.List.Items()
		} else {
			it.Str = e.Str
		}
		items = append(items, it)
	}
	return items, s.changes
}

// Restore replaces the whole database with items, skipping any whose
// expiry time has already passed. It returns how many keys were loaded.
func (s *Store) Restore(items []Item) int {
	data := make(map[string]*Entry, len(items))
	expires := make(map[string]struct{})
	now := s.now()
	for _, it := range items {
		e := &Entry{Kind: it.Kind, ExpireAt: it.ExpireAt}
		if e.expired(now) {
			continue
		}
		switch it.Kind {
		case KindList:
			if len(it.List) == 0 {
				continue // an empty list is no key at all
			}
			e.List = NewList(it.List)
		default:
			e.Kind = KindString
			e.Str = it.Str
			if e.Str == nil {
				e.Str = []byte{} // gob turns an empty slice into nil
			}
		}
		data[it.Key] = e
		if !e.ExpireAt.IsZero() {
			expires[it.Key] = struct{}{}
		}
	}

	s.mu.Lock()
	s.data, s.expires = data, expires
	s.mu.Unlock()
	return len(data)
}

// Len returns the number of keys, counting expired keys that haven't been
// removed yet.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}
