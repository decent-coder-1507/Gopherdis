package persist

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopherdis/internal/store"
)

// holdSaves makes every save stop after taking its snapshot until release
// is called. started receives a value when a save reaches that point.
func holdSaves(s *Saver) (started <-chan struct{}, release func()) {
	gate := make(chan struct{})
	reached := make(chan struct{}, 10)
	s.beforeWrite = func() {
		reached <- struct{}{}
		<-gate
	}
	return reached, func() { close(gate) }
}

func TestOnlyOneSaveAtATime(t *testing.T) {
	st := store.New()
	st.Set("a", []byte("1"), time.Time{})
	s := NewSaver(filepath.Join(t.TempDir(), "dump.gob"), st)
	started, release := holdSaves(s)

	if err := s.BackgroundSave(); err != nil {
		t.Fatalf("first BGSAVE: %v", err)
	}
	<-started

	if err := s.BackgroundSave(); !errors.Is(err, ErrSaveInProgress) {
		t.Errorf("second BGSAVE: %v, want ErrSaveInProgress", err)
	}
	if _, err := s.Save(); !errors.Is(err, ErrSaveInProgress) {
		t.Errorf("SAVE during BGSAVE: %v, want ErrSaveInProgress", err)
	}

	// The store keeps working while the save is in progress.
	done := make(chan struct{})
	go func() {
		st.Set("b", []byte("2"), time.Time{})
		st.Get("a")
		st.Push("l", [][]byte{[]byte("x")}, false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("store commands blocked during a background save")
	}

	release()
	s.Wait()
	if s.saves.Load() != 1 {
		t.Fatalf("saves = %d, want 1", s.saves.Load())
	}
	// Now another save may run.
	s.beforeWrite = nil
	if _, err := s.Save(); err != nil {
		t.Fatalf("SAVE after BGSAVE finished: %v", err)
	}
}

func TestSaveWaitWaitsForRunningSave(t *testing.T) {
	st := store.New()
	s := NewSaver(filepath.Join(t.TempDir(), "dump.gob"), st)
	started, release := holdSaves(s)
	s.BackgroundSave()
	<-started

	finished := make(chan error, 1)
	go func() {
		_, err := s.SaveWait()
		finished <- err
	}()
	select {
	case <-finished:
		t.Fatal("SaveWait didn't wait for the running save")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if s.saves.Load() != 2 {
		t.Fatalf("saves = %d, want 2", s.saves.Load())
	}
}

func TestDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gob")
	st := store.New()
	s := NewSaver(path, st)
	if s.Dirty() {
		t.Fatal("new empty store is dirty")
	}
	st.Set("a", []byte("1"), time.Time{})
	if !s.Dirty() {
		t.Fatal("SET didn't make the store dirty")
	}
	s.Save()
	if s.Dirty() {
		t.Fatal("still dirty after a save")
	}
	// Reads and failed writes are not changes.
	st.Get("a")
	st.Exists("a")
	st.Del("nope")
	st.IncrBy("a", 0) // a real write, even if the value is the same
	if !s.Dirty() {
		t.Fatal("INCR didn't count as a change")
	}
	s.Save()
	st.Push("a", [][]byte{[]byte("x")}, true) // WRONGTYPE, no change
	st.Persist("a")                           // no expiry to remove, no change
	if s.Dirty() {
		t.Fatal("failed writes counted as changes")
	}

	// Loading counts as saved.
	s2 := NewSaver(path, store.New())
	if _, _, err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if s2.Dirty() {
		t.Fatal("store is dirty right after loading")
	}
}

func TestAutoSaveOnlyWhenChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gob")
	st := store.New()
	s := NewSaver(path, st)
	stop := s.StartAutoSave(20 * time.Millisecond)
	defer stop()

	time.Sleep(150 * time.Millisecond)
	if s.saves.Load() != 0 {
		t.Fatalf("auto-save ran %d times with nothing changed", s.saves.Load())
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("dump file written with nothing changed")
	}

	st.Set("a", []byte("1"), time.Time{})
	deadline := time.Now().Add(2 * time.Second)
	for s.saves.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.saves.Load() == 0 {
		t.Fatal("auto-save didn't save a change")
	}

	// Nothing changed since: no more saves.
	time.Sleep(50 * time.Millisecond)
	n := s.saves.Load()
	time.Sleep(150 * time.Millisecond)
	if s.saves.Load() != n {
		t.Fatalf("auto-save kept saving unchanged data: %d -> %d", n, s.saves.Load())
	}
	stop()
	stop() // safe to call twice
}

func TestLastSave(t *testing.T) {
	s := NewSaver(filepath.Join(t.TempDir(), "dump.gob"), store.New())
	created := s.LastSave()
	if time.Since(created) > time.Minute {
		t.Fatalf("LastSave starts at %v, want about now", created)
	}
	s.lastSave.Store(1000) // pretend the last save was long ago
	s.Save()
	if s.LastSave().Unix() <= 1000 {
		t.Fatal("LastSave not updated by a save")
	}

	// A failed save doesn't move it.
	bad := NewSaver(filepath.Join(t.TempDir(), "no", "such", "dir", "dump.gob"), store.New())
	bad.lastSave.Store(1000)
	if _, err := bad.Save(); err == nil {
		t.Fatal("save into a missing folder succeeded")
	}
	if bad.LastSave().Unix() != 1000 {
		t.Fatal("a failed save changed LastSave")
	}
}
