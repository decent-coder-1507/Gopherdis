package store

import (
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"
)

func vals(vs ...string) [][]byte {
	out := make([][]byte, len(vs))
	for i, v := range vs {
		out[i] = []byte(v)
	}
	return out
}

func TestExistsDel(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Set("a", []byte("1"), time.Time{})
	s.Set("b", []byte("2"), time.Time{})
	s.Set("old", []byte("3"), clock.Now().Add(time.Second))
	s.Push("l", vals("x"), false)

	if n := s.Exists("a", "b", "l", "nope"); n != 3 {
		t.Fatalf("Exists = %d, want 3", n)
	}
	if n := s.Exists("a", "a", "a"); n != 3 {
		t.Fatalf("a key named three times: %d, want 3", n)
	}

	clock.Advance(2 * time.Second)
	if n := s.Exists("old"); n != 0 {
		t.Fatalf("expired key counted by Exists")
	}

	s.Set("old2", []byte("x"), clock.Now().Add(time.Second))
	clock.Advance(2 * time.Second)
	if n := s.Del("a", "l", "nope", "old2", "a"); n != 2 {
		t.Fatalf("Del = %d, want 2 (a and l; old2 had expired, a named twice)", n)
	}
	if keys, exp := s.size(); keys != 1 || exp != 0 {
		t.Fatalf("after Del: %d keys, %d with expiry; want 1 and 0", keys, exp)
	}
}

func TestIncrBy(t *testing.T) {
	s := New()
	if n, err := s.IncrBy("new", 1); err != nil || n != 1 {
		t.Fatalf("INCR new = %d, %v", n, err)
	}
	if n, err := s.IncrBy("neg", -1); err != nil || n != -1 {
		t.Fatalf("DECR neg = %d, %v", n, err)
	}
	s.Set("n", []byte("10"), time.Time{})
	if n, _ := s.IncrBy("n", 1); n != 11 {
		t.Fatalf("INCR n = %d", n)
	}
	if v, _, _ := s.Get("n"); string(v) != "11" {
		t.Fatalf("stored value %q", v)
	}
}

func TestIncrByBadValues(t *testing.T) {
	s := New()
	for _, v := range []string{"hello", "1.5", "", " 1", "1 ", "+1", "01", "-0", "--1", "0x10",
		"99999999999999999999", "-9223372036854775809"} {
		s.Set("k", []byte(v), time.Time{})
		if _, err := s.IncrBy("k", 1); err != ErrNotInteger {
			t.Errorf("INCR on %q: %v, want ErrNotInteger", v, err)
		}
		if got, _, _ := s.Get("k"); string(got) != v {
			t.Errorf("failed INCR changed %q into %q", v, got)
		}
	}
	for _, v := range []string{"0", "-1", "9223372036854775806", "-9223372036854775808"} {
		s.Set("k", []byte(v), time.Time{})
		if _, err := s.IncrBy("k", 1); err != nil {
			t.Errorf("INCR on %q: %v", v, err)
		}
	}
}

func TestIncrByOverflow(t *testing.T) {
	s := New()
	s.Set("max", []byte(strconv.FormatInt(math.MaxInt64, 10)), time.Time{})
	if _, err := s.IncrBy("max", 1); err != ErrNotInteger {
		t.Errorf("INCR past MaxInt64: %v", err)
	}
	s.Set("min", []byte(strconv.FormatInt(math.MinInt64, 10)), time.Time{})
	if _, err := s.IncrBy("min", -1); err != ErrNotInteger {
		t.Errorf("DECR past MinInt64: %v", err)
	}
	if n, err := s.IncrBy("max", -1); err != nil || n != math.MaxInt64-1 {
		t.Errorf("DECR from MaxInt64: %d, %v", n, err)
	}
}

func TestIncrKeepsExpiry(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Set("n", []byte("5"), clock.Now().Add(10*time.Second))
	s.IncrBy("n", 1)
	s.IncrBy("n", -3)
	if _, exp := s.size(); exp != 1 {
		t.Fatal("INCR dropped the key from the expiry index")
	}
	clock.Advance(9 * time.Second)
	if v, ok, _ := s.Get("n"); !ok || string(v) != "3" {
		t.Fatalf("before expiry: %q, %v", v, ok)
	}
	clock.Advance(time.Second)
	if _, ok, _ := s.Get("n"); ok {
		t.Fatal("INCR removed the expiry")
	}

	// INCR on an expired key starts again from 0, with no expiry.
	s.Set("e", []byte("100"), clock.Now().Add(time.Second))
	clock.Advance(time.Second)
	if n, _ := s.IncrBy("e", 1); n != 1 {
		t.Fatalf("INCR on expired key = %d, want 1", n)
	}
	clock.Advance(time.Hour)
	if _, ok, _ := s.Get("e"); !ok {
		t.Fatal("new key got the old key's expiry")
	}
}

func TestPushOrder(t *testing.T) {
	s := New()
	items := func(key string) string {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return fmt.Sprintf("%q", s.data[key].List.Items())
	}

	if n, _ := s.Push("l", vals("a", "b", "c"), true); n != 3 {
		t.Fatalf("LPUSH len %d", n)
	}
	if got := items("l"); got != `["c" "b" "a"]` {
		t.Fatalf("LPUSH a b c: %s", got)
	}
	if n, _ := s.Push("r", vals("a", "b"), false); n != 2 {
		t.Fatalf("RPUSH len %d", n)
	}
	if n, _ := s.Push("r", vals("z"), true); n != 3 {
		t.Fatalf("LPUSH onto RPUSHed list: %d", n)
	}
	s.Push("r", vals("end"), false)
	if got := items("r"); got != `["z" "a" "b" "end"]` {
		t.Fatalf("mixed pushes: %s", got)
	}
}

func TestWrongType(t *testing.T) {
	s := New()
	s.Set("str", []byte("hi"), time.Time{})
	s.Push("list", vals("a"), false)

	if _, err := s.Push("str", vals("x"), true); err != ErrWrongType {
		t.Errorf("LPUSH on string: %v", err)
	}
	if _, _, err := s.Get("list"); err != ErrWrongType {
		t.Errorf("GET on list: %v", err)
	}
	if _, err := s.IncrBy("list", 1); err != ErrWrongType {
		t.Errorf("INCR on list: %v", err)
	}
	// SET may replace a key of any type.
	s.Set("list", []byte("now a string"), time.Time{})
	if v, _, err := s.Get("list"); err != nil || string(v) != "now a string" {
		t.Errorf("SET over list: %q, %v", v, err)
	}
}

func TestPushCopiesValues(t *testing.T) {
	s := New()
	buf := []byte("abc")
	s.Push("l", [][]byte{buf}, false)
	buf[0] = 'X'
	s.mu.RLock()
	got := string(s.data["l"].List.Items()[0])
	s.mu.RUnlock()
	if got != "abc" {
		t.Fatalf("list kept the caller's buffer: %q", got)
	}
}

func TestPushOntoExpiredList(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Push("l", vals("old"), false)
	s.mu.Lock()
	s.data["l"].ExpireAt = clock.Now().Add(time.Second)
	s.expires["l"] = struct{}{}
	s.mu.Unlock()
	clock.Advance(time.Second)
	if n, _ := s.Push("l", vals("new"), false); n != 1 {
		t.Fatalf("push onto expired list: len %d, want 1", n)
	}
}

func TestExpireReplacesEntry(t *testing.T) {
	clock := newFakeClock()
	s := NewWithClock(clock.Now)
	s.Set("k", []byte("v"), time.Time{})
	s.mu.RLock()
	before := s.data["k"]
	s.mu.RUnlock()

	if !s.Expire("k", clock.Now().Add(time.Minute)) {
		t.Fatal("Expire returned false")
	}
	if !before.ExpireAt.IsZero() {
		t.Fatal("Expire changed the old entry in place")
	}
	if _, exp := s.size(); exp != 1 {
		t.Fatal("Expire did not add the key to the expiry index")
	}
	if !s.Persist("k") {
		t.Fatal("Persist returned false")
	}
	if _, exp := s.size(); exp != 0 {
		t.Fatal("Persist left the key in the expiry index")
	}
	// A time in the past deletes the key.
	if !s.Expire("k", clock.Now()) || s.Exists("k") != 0 {
		t.Fatal("Expire with a past time did not delete the key")
	}
}
