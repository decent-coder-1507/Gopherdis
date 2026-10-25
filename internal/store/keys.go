package store

import (
	"errors"
	"math"
	"strconv"
	"time"
)

// ErrNotInteger is returned by IncrBy when the value is not a 64-bit
// integer or the result would not fit in one.
var ErrNotInteger = errors.New("ERR value is not an integer or out of range")

// Exists returns how many of keys exist. A key named twice counts twice.
func (s *Store) Exists(keys ...string) int {
	var expired []string
	n := 0
	s.mu.RLock()
	now := s.now()
	for _, k := range keys {
		e, exp := s.lookup(k, now)
		if e != nil {
			n++
		}
		if exp {
			expired = append(expired, k)
		}
	}
	s.mu.RUnlock()

	for _, k := range expired {
		s.deleteIfExpired(k)
	}
	return n
}

// Del deletes keys and returns how many existed. Expired keys are removed
// too but don't count, since to clients they were already gone.
func (s *Store) Del(keys ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	n := 0
	for _, k := range keys {
		e, ok := s.data[k]
		if !ok {
			continue
		}
		if !e.expired(now) {
			n++
		}
		s.remove(k)
	}
	return n
}

// IncrBy adds delta to the integer stored at key and returns the new value.
// A missing key counts as 0. The key keeps any expiry it had.
func (s *Store) IncrBy(key string, delta int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, expired := s.lookup(key, s.now())
	if expired {
		s.remove(key)
	}
	var cur int64
	var expireAt time.Time
	if e != nil {
		if e.Kind != KindString {
			return 0, ErrWrongType
		}
		n, ok := parseInt(e.Str)
		if !ok {
			return 0, ErrNotInteger
		}
		cur, expireAt = n, e.ExpireAt
	}

	if (delta > 0 && cur > math.MaxInt64-delta) || (delta < 0 && cur < math.MinInt64-delta) {
		return 0, ErrNotInteger
	}
	cur += delta
	s.put(key, &Entry{Kind: KindString, Str: strconv.AppendInt(nil, cur, 10), ExpireAt: expireAt})
	return cur, nil
}

// parseInt reads b as a base-10 int64 the way Redis does: an optional minus
// sign, no plus sign, no spaces and no leading zeros ("0" itself is fine).
func parseInt(b []byte) (int64, bool) {
	if len(b) == 0 || len(b) > 20 {
		return 0, false
	}
	digits := b
	if digits[0] == '-' {
		digits = digits[1:]
	}
	if len(digits) == 0 || (digits[0] == '0' && len(b) > 1) {
		return 0, false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(string(b), 10, 64)
	return n, err == nil
}

// Push adds vals to the list at key, one at a time, at the front (LPUSH)
// or the back (RPUSH), creating the list if needed. It returns the new
// length. LPUSH of a, b, c gives c, b, a.
func (s *Store) Push(key string, vals [][]byte, front bool) (int, error) {
	copies := make([][]byte, len(vals))
	for i, v := range vals {
		copies[i] = clone(v)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e, expired := s.lookup(key, s.now())
	if expired {
		s.remove(key)
	}
	if e == nil {
		e = &Entry{Kind: KindList, List: NewList(nil)}
		s.put(key, e)
	} else if e.Kind != KindList {
		return 0, ErrWrongType
	}
	for _, v := range copies {
		if front {
			e.List.PushFront(v)
		} else {
			e.List.PushBack(v)
		}
	}
	return e.List.Len(), nil
}
