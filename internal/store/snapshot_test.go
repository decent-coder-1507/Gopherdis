package store

import (
	"testing"
	"time"
)

func TestSnapshotIsACopy(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Set("a", []byte("1"), time.Time{})
	s.Set("gone", []byte("x"), clock.Now().Add(time.Second))
	s.Push("l", vals("x", "y"), false)
	clock.Advance(time.Second)

	snap, _ := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot has %d items, want 2 (expired key left out)", len(snap))
	}

	// Changing the store afterwards doesn't change the snapshot.
	s.Push("l", vals("z"), true)
	s.Set("a", []byte("2"), time.Time{})
	for _, it := range snap {
		switch it.Key {
		case "a":
			if string(it.Str) != "1" {
				t.Errorf("a = %q", it.Str)
			}
		case "l":
			if len(it.List) != 2 || string(it.List[0]) != "x" {
				t.Errorf("l = %q", it.List)
			}
		}
	}
}

func TestRestore(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Set("old", []byte("x"), time.Time{})

	n := s.Restore([]Item{
		{Key: "a", Str: nil}, // gob decodes an empty value as nil
		{Key: "l", Kind: KindList, List: vals("x")},
		{Key: "emptylist", Kind: KindList},
		{Key: "ttl", Str: []byte("1"), ExpireAt: clock.Now().Add(time.Minute)},
		{Key: "dead", Str: []byte("1"), ExpireAt: clock.Now()},
	})
	if n != 3 {
		t.Fatalf("Restore loaded %d keys, want 3", n)
	}
	if _, ok, _ := s.Get("old"); ok {
		t.Error("Restore kept an old key")
	}
	if v, ok, _ := s.Get("a"); !ok || v == nil {
		t.Errorf("empty string after restore: %q, %v", v, ok)
	}
	if _, exp := s.size(); exp != 1 {
		t.Errorf("expiry index has %d keys, want 1", exp)
	}
	clock.Advance(time.Minute)
	if s.Exists("ttl") != 0 {
		t.Error("restored expiry not applied")
	}
}
