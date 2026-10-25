package persist

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gopherdis/internal/store"
)

func sorted(items []store.Item) []store.Item {
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items
}

// fill puts one of everything into st.
func fill(st *store.Store) {
	st.Set("a", []byte("1"), time.Time{})
	st.Set("empty", []byte{}, time.Time{})
	st.Set("bin", []byte("\x00\xff\r\n\x01"), time.Time{})
	st.Set("t", []byte("ttl"), time.Now().Add(time.Hour))
	st.Push("l", [][]byte{[]byte("x"), []byte("y"), []byte("")}, false)
	st.Push("l", [][]byte{[]byte("front")}, true)
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gob")
	src := store.New()
	fill(src)
	if n, err := NewSaver(path, src).Save(); err != nil || n != 5 {
		t.Fatalf("Save: %d keys, %v", n, err)
	}

	dst := store.New()
	dst.Set("stale", []byte("must be replaced"), time.Time{})
	n, found, err := NewSaver(path, dst).Load()
	if err != nil || !found || n != 5 {
		t.Fatalf("Load: %d keys, found=%v, %v", n, found, err)
	}

	want, got := sorted(src.Snapshot()), sorted(dst.Snapshot())
	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d", len(got), len(want))
	}
	for i := range want {
		w, g := want[i], got[i]
		if g.Key != w.Key || g.Kind != w.Kind || !bytes.Equal(g.Str, w.Str) || !g.ExpireAt.Equal(w.ExpireAt) {
			t.Errorf("key %s: got %+v, want %+v", w.Key, g, w)
		}
		if len(g.List) != len(w.List) {
			t.Errorf("key %s: list %q, want %q", w.Key, g.List, w.List)
			continue
		}
		for j := range w.List {
			if !bytes.Equal(g.List[j], w.List[j]) {
				t.Errorf("key %s item %d: %q, want %q", w.Key, j, g.List[j], w.List[j])
			}
		}
	}

	// Values come back usable, not just equal: the empty string is still a
	// string, the list keeps its order and the expiry still counts.
	if v, ok, _ := dst.Get("empty"); !ok || len(v) != 0 {
		t.Errorf("empty value after load: %q, %v", v, ok)
	}
	if n, _ := dst.Push("l", [][]byte{[]byte("z")}, false); n != 5 {
		t.Errorf("list length after load + push: %d", n)
	}
	if _, ok, _ := dst.Get("stale"); ok {
		t.Error("Load kept a key that was not in the file")
	}
}

func TestLoadSkipsExpiredKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gob")
	items := []store.Item{
		{Key: "live", Str: []byte("1"), ExpireAt: time.Now().Add(time.Hour)},
		{Key: "dead", Str: []byte("2"), ExpireAt: time.Now().Add(-time.Second)},
		{Key: "forever", Str: []byte("3")},
		{Key: "deadlist", Kind: store.KindList, List: [][]byte{[]byte("x")}, ExpireAt: time.Now().Add(-time.Hour)},
	}
	if err := Save(path, items); err != nil {
		t.Fatal(err)
	}
	st := store.New()
	n, _, err := NewSaver(path, st).Load()
	if err != nil || n != 2 {
		t.Fatalf("loaded %d keys, %v; want 2", n, err)
	}
	if st.Exists("live", "forever", "dead", "deadlist") != 2 {
		t.Fatal("wrong keys loaded")
	}
}

func TestLoadMissingFile(t *testing.T) {
	st := store.New()
	n, found, err := NewSaver(filepath.Join(t.TempDir(), "nope.gob"), st).Load()
	if err != nil || found || n != 0 {
		t.Fatalf("missing file: %d, found=%v, %v", n, found, err)
	}
}

func TestLoadBrokenFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.gob")
	if err := Save(good, []store.Item{{Key: "a", Str: []byte("1")}, {Key: "b", Str: []byte("2")}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(good)

	otherMagic := filepath.Join(dir, "other.gob")
	Save(otherMagic, nil)
	otherData, _ := os.ReadFile(otherMagic)
	otherData = bytes.Replace(otherData, []byte(magic), []byte("SOMETHING-ELSE"), 1)

	cases := map[string][]byte{
		"garbage":      []byte("this is not a dump file at all"),
		"empty":        {},
		"truncated":    data[:len(data)-5],
		"header only":  data[:len(data)/3],
		"wrong magic":  otherData,
		"random bytes": bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 100),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".gob")
			os.WriteFile(path, content, 0o644)
			st := store.New()
			_, found, err := NewSaver(path, st).Load()
			if err == nil {
				t.Fatal("damaged file loaded without an error")
			}
			if !found || !strings.Contains(err.Error(), path) {
				t.Errorf("error should name the file: %v", err)
			}
			if st.Len() != 0 {
				t.Errorf("store has %d keys after a failed load", st.Len())
			}
		})
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.gob")
	st := store.New()
	fill(st)
	for i := 0; i < 3; i++ {
		if _, err := NewSaver(path, st).Save(); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !reflect.DeepEqual(names, []string{"dump.gob"}) {
		t.Fatalf("directory holds %v, want only dump.gob", names)
	}
}

func TestFailedSaveKeepsOldFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.gob")
	if err := Save(path, []store.Item{{Key: "old", Str: []byte("1")}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	// Make the temp file impossible to create: a directory is in its way.
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, []store.Item{{Key: "new", Str: []byte("2")}}); err == nil {
		t.Fatal("Save succeeded although the temp file could not be written")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("a failed save changed the existing dump file")
	}
}

func TestSaveWhileWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gob")
	st := store.New()
	saver := NewSaver(path, st)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			st.Set("k", []byte(strings.Repeat("x", i%50)), time.Time{})
			st.Push("l", [][]byte{[]byte("v")}, i%2 == 0)
		}
	}()
	for i := 0; i < 20; i++ {
		if _, err := saver.Save(); err != nil {
			t.Fatal(err)
		}
	}
	<-done
	if _, _, err := NewSaver(path, store.New()).Load(); err != nil {
		t.Fatal(err)
	}
}
