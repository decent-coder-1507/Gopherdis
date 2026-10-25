package store

import (
	"fmt"
	"testing"
)

func TestListBothEnds(t *testing.T) {
	l := NewList(nil)
	var want []string
	for i := 0; i < 100; i++ {
		v := fmt.Sprint(i)
		if i%3 == 0 {
			l.PushFront([]byte(v))
			want = append([]string{v}, want...)
		} else {
			l.PushBack([]byte(v))
			want = append(want, v)
		}
	}
	if l.Len() != len(want) {
		t.Fatalf("Len = %d, want %d", l.Len(), len(want))
	}
	got := l.Items()
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
	part := l.Slice(10, 12)
	if len(part) != 3 || string(part[0]) != want[10] || string(part[2]) != want[12] {
		t.Fatalf("Slice(10, 12) = %q", part)
	}
	// The returned slice is the caller's: pushing more doesn't change it.
	l.PushFront([]byte("new"))
	if string(part[0]) != want[10] {
		t.Fatal("Slice result changed after a push")
	}
}

func TestEmptyList(t *testing.T) {
	l := NewList(nil)
	if l.Len() != 0 || len(l.Items()) != 0 || l.Items() == nil {
		t.Fatalf("empty list: len %d, items %v", l.Len(), l.Items())
	}
}
