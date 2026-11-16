package flock

import (
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
