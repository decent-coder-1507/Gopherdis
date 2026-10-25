package store

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestSetGet(t *testing.T) {
	s := New()
	if _, ok, err := s.Get("missing"); ok || err != nil {
		t.Fatalf("missing key: ok=%v err=%v", ok, err)
	}

	s.Set("name", []byte("Srijoy"))
	if v, ok, _ := s.Get("name"); !ok || string(v) != "Srijoy" {
		t.Fatalf("got %q, %v", v, ok)
	}

	s.Set("name", []byte("Paul"))
	if v, _, _ := s.Get("name"); string(v) != "Paul" {
		t.Fatalf("overwrite: got %q", v)
	}
}

func TestEmptyAndBinaryValues(t *testing.T) {
	s := New()
	s.Set("empty", []byte{})
	v, ok, _ := s.Get("empty")
	if !ok || v == nil || len(v) != 0 {
		t.Fatalf("empty value: %q, ok=%v, nil=%v", v, ok, v == nil)
	}

	bin := []byte{0, 1, '\r', '\n', 0xff, 0}
	s.Set("bin", bin)
	if v, _, _ := s.Get("bin"); !bytes.Equal(v, bin) {
		t.Fatalf("binary value: got %v", v)
	}
}

func TestSetCopiesValue(t *testing.T) {
	s := New()
	buf := []byte("abc")
	s.Set("k", buf)
	buf[0] = 'X'
	if v, _, _ := s.Get("k"); string(v) != "abc" {
		t.Fatalf("store kept a reference to the caller's buffer: %q", v)
	}
}

func TestGetWrongType(t *testing.T) {
	s := New()
	s.data["l"] = &Entry{Kind: KindList, List: [][]byte{[]byte("a")}}
	if _, _, err := s.Get("l"); err != ErrWrongType {
		t.Fatalf("got %v, want ErrWrongType", err)
	}
	s.Set("l", []byte("now a string"))
	if v, _, err := s.Get("l"); err != nil || string(v) != "now a string" {
		t.Fatalf("SET over a list: %q, %v", v, err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				key := fmt.Sprintf("k%d-%d", g, i)
				s.Set(key, []byte(key))
				if v, ok, _ := s.Get(key); !ok || string(v) != key {
					t.Errorf("GET %s right after SET: %q", key, v)
				}
				// Everyone also hits one shared key.
				s.Set("shared", []byte(key))
				s.Get("shared")
			}
		}(g)
	}
	wg.Wait()
	for g := 0; g < 50; g++ {
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("k%d-%d", g, i)
			if v, ok, _ := s.Get(key); !ok || string(v) != key {
				t.Fatalf("%s: got %q", key, v)
			}
		}
	}
}
