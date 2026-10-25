package persist

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

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

// ErrSaveInProgress is returned when a save is asked for while another is
// still running.
var ErrSaveInProgress = errors.New("ERR Background save already in progress")

// Saver saves one store to one file. Only one save runs at a time.
type Saver struct {
	path  string
	store *store.Store
	// busy has room for one token; whoever holds it is saving.
	busy chan struct{}
	// lastSave is the Unix time of the last good save. Like Redis, it
	// starts as the time the Saver was created.
	lastSave atomic.Int64
	// savedChanges is the store's change count at the last good save
	// (or load), so Dirty can tell whether anything changed since.
	savedChanges atomic.Uint64
	// saves counts good saves; tests use it.
	saves atomic.Int64
	// bg tracks the background save goroutine so Wait can wait for it.
	bg sync.WaitGroup
	// beforeWrite, if set, runs after the snapshot and before the file is
	// written. Tests use it to hold a save in progress.
	beforeWrite func()
}

// NewSaver returns a Saver that writes st to path.
func NewSaver(path string, st *store.Store) *Saver {
	s := &Saver{path: path, store: st, busy: make(chan struct{}, 1)}
	s.lastSave.Store(time.Now().Unix())
	return s
}

// LastSave returns the time of the last good save.
func (s *Saver) LastSave() time.Time {
	return time.Unix(s.lastSave.Load(), 0)
}

// Path returns the file the saver writes.
func (s *Saver) Path() string {
	return s.path
}

// Dirty reports whether the store changed since the last good save.
func (s *Saver) Dirty() bool {
	return s.store.Changes() != s.savedChanges.Load()
}

// Save saves now and returns the number of keys saved. If another save is
// running it returns ErrSaveInProgress instead of waiting.
func (s *Saver) Save() (int, error) {
	select {
	case s.busy <- struct{}{}:
	default:
		return 0, ErrSaveInProgress
	}
	defer func() { <-s.busy }()
	return s.write()
}

// SaveWait saves now, first waiting for any save that is already running.
// Shutdown uses it: the data must be saved no matter what.
func (s *Saver) SaveWait() (int, error) {
	s.busy <- struct{}{}
	defer func() { <-s.busy }()
	return s.write()
}

// BackgroundSave starts a save in its own goroutine and returns at once,
// or returns ErrSaveInProgress if a save is already running.
func (s *Saver) BackgroundSave() error {
	select {
	case s.busy <- struct{}{}:
	default:
		return ErrSaveInProgress
	}
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		defer func() { <-s.busy }()
		s.write() // write logs any error
	}()
	return nil
}

// Wait waits for a background save to finish.
func (s *Saver) Wait() {
	s.bg.Wait()
}

// write copies the data under the store's lock, releases it, then writes
// the file, so clients keep working during the slow disk write. The caller
// holds the busy token.
func (s *Saver) write() (int, error) {
	start := time.Now()
	items, changes := s.store.Snapshot()
	if s.beforeWrite != nil {
		s.beforeWrite()
	}
	if err := Save(s.path, items); err != nil {
		slog.Error("save failed", "file", s.path, "err", err)
		return 0, err
	}
	s.lastSave.Store(time.Now().Unix())
	s.savedChanges.Store(changes)
	s.saves.Add(1)
	slog.Info("save finished", "file", s.path, "keys", len(items), "took", time.Since(start).Round(time.Microsecond))
	return len(items), nil
}

// StartAutoSave saves in the background every interval, but only if the
// data changed since the last save. It runs until stop is called; stop
// also waits for a background save it started.
func (s *Saver) StartAutoSave(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if s.Dirty() {
					// A save already running (BGSAVE, SAVE) is fine: try again next tick.
					s.BackgroundSave()
				}
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		<-finished
		s.Wait()
	}
}

// Load reads the file into the store, replacing what is there. found is
// false if there was no file. Freshly loaded data counts as saved.
func (s *Saver) Load() (keys int, found bool, err error) {
	items, found, err := Load(s.path)
	if err != nil || !found {
		return 0, found, err
	}
	n := s.store.Restore(items)
	s.savedChanges.Store(s.store.Changes())
	return n, true, nil
}
