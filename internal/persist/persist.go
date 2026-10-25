package persist

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopherdis/internal/store"
)

// magic and version open every dump file, so a file written by something
// else (or a future format) is rejected with a clear message.
const (
	magic   = "GOPHERDIS-DUMP"
	version = 1
)

type header struct {
	Magic   string
	Version int
}

// Save writes items to path. It writes to path + ".tmp" first, syncs it to
// disk and then renames it over path, so a crash halfway through leaves the
// old file untouched.
func Save(path string, items []store.Item) (err error) {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()

	w := bufio.NewWriter(f)
	enc := gob.NewEncoder(w)
	if err := enc.Encode(header{Magic: magic, Version: version}); err != nil {
		return err
	}
	if err := enc.Encode(items); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	syncDir(filepath.Dir(path))
	return nil
}

// syncDir makes the rename itself durable on systems that need it. It is
// best effort: some systems (Windows) can't open a directory for syncing.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// Load reads the items saved at path. A missing file is not an error: it
// returns no items and found == false. Any other problem, including a
// damaged file, is returned as an error.
func Load(path string) (items []store.Item, found bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	dec := gob.NewDecoder(bufio.NewReader(f))
	var h header
	if err := dec.Decode(&h); err != nil {
		return nil, true, fmt.Errorf("%s is not a valid dump file: %w", path, err)
	}
	if h.Magic != magic {
		return nil, true, fmt.Errorf("%s is not a gopherdis dump file", path)
	}
	if h.Version != version {
		return nil, true, fmt.Errorf("%s has dump format version %d; this build reads version %d", path, h.Version, version)
	}
	if err := dec.Decode(&items); err != nil {
		return nil, true, fmt.Errorf("%s is damaged: %w", path, err)
	}
	return items, true, nil
}

// Saver saves one store to one file, one save at a time.
type Saver struct {
	path  string
	store *store.Store
	mu    sync.Mutex // held for the whole of a save
}

// NewSaver returns a Saver that writes st to path.
func NewSaver(path string, st *store.Store) *Saver {
	return &Saver{path: path, store: st}
}

// Path returns the file the saver writes.
func (s *Saver) Path() string {
	return s.path
}

// Save copies the data under the store's lock, releases it, then writes the
// file, so clients keep working during the slow disk write. It returns the
// number of keys saved.
func (s *Saver) Save() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.store.Snapshot()
	return len(items), Save(s.path, items)
}

// Load reads the file into the store, replacing what is there. found is
// false if there was no file.
func (s *Saver) Load() (keys int, found bool, err error) {
	items, found, err := Load(s.path)
	if err != nil || !found {
		return 0, found, err
	}
	return s.store.Restore(items), true, nil
}
