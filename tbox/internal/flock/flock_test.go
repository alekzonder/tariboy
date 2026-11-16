package flock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "x.lock")
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		u, err := Lock(path)
		if err == nil {
			u()
		}
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("second lock acquired while held")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("second lock never acquired")
	}
}

// TestLockFollowsARemovedFile checks that a waiter does not keep a lock on a
// file the holder removed: it locks the file now at path.
func TestLockFollowsARemovedFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a")
	path := filepath.Join(dir, "x.lock")
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan func(), 1)
	go func() {
		u, err := Lock(path)
		if err != nil {
			t.Error(err)
			u = func() {}
		}
		got <- u
	}()
	time.Sleep(100 * time.Millisecond)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	unlock()
	var second func()
	select {
	case second = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("second lock never acquired")
	}
	// The second holder must hold the file at path, so a third waits.
	third := make(chan struct{})
	go func() {
		u, err := Lock(path)
		if err == nil {
			u()
		}
		close(third)
	}()
	select {
	case <-third:
		second()
		t.Fatal("third lock acquired while the second is held")
	case <-time.After(100 * time.Millisecond):
	}
	second()
	select {
	case <-third:
	case <-time.After(5 * time.Second):
		t.Fatal("third lock never acquired")
	}
}
