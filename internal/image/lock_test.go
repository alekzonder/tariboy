package image

import (
	"testing"
	"time"
)

// acquired runs lock in a goroutine and reports when it returns. The caller
// releases the lock it took through the returned unlock channel.
func acquired(lock func() func()) (got <-chan func()) {
	ch := make(chan func(), 1)
	go func() { ch <- lock() }()
	return ch
}

func mustAcquire(t *testing.T, got <-chan func(), what string) func() {
	t.Helper()
	select {
	case unlock := <-got:
		return unlock
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not proceed", what)
		return nil
	}
}

func mustWait(t *testing.T, got <-chan func(), what string) {
	t.Helper()
	select {
	case unlock := <-got:
		unlock()
		t.Fatalf("%s did not wait", what)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestLockRefDifferentNamesProceedInParallel(t *testing.T) {
	unlockA := LockRef("lock-test-a")
	defer unlockA()
	mustAcquire(t, acquired(func() func() { return LockRef("lock-test-b") }), "a second name")()
}

func TestLockRefSameNameSerializes(t *testing.T) {
	unlock := LockRef("lock-test-same")
	second := acquired(func() func() { return LockRef("lock-test-same") })
	mustWait(t, second, "the same name")
	unlock()
	mustAcquire(t, second, "the same name after release")()
}

func TestLockStoreExcludesRefLocks(t *testing.T) {
	unlockStore := LockStore()
	ref := acquired(func() func() { return LockRef("lock-test-store") })
	mustWait(t, ref, "a ref lock under the store lock")
	unlockStore()
	unlockRef := mustAcquire(t, ref, "a ref lock after the store lock")

	store := acquired(LockStore)
	mustWait(t, store, "the store lock under a ref lock")
	unlockRef()
	mustAcquire(t, store, "the store lock after the ref lock")()
}

func TestLockRefForgetsReleasedNames(t *testing.T) {
	LockRef("lock-test-forget")()
	refLocks.mu.Lock()
	defer refLocks.mu.Unlock()
	if _, ok := refLocks.names["lock-test-forget"]; ok {
		t.Fatal("a released name kept its lock entry")
	}
}

// A build migrates through MigrateOnce before it takes its name lock. Only the
// first call for a store takes the store lock, so later builds do not wait for
// other names.
func TestMigrateOnceTakesTheStoreLockOnlyFirst(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	unlockRef := LockRef("lock-test-migrate")
	first := make(chan error, 1)
	go func() { first <- store.MigrateOnce() }()
	select {
	case err := <-first:
		unlockRef()
		t.Fatalf("first migration did not wait for the store lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unlockRef()
	if err := <-first; err != nil {
		t.Fatal(err)
	}

	defer LockRef("lock-test-migrate")()
	again := make(chan error, 1)
	go func() { again <- store.MigrateOnce() }()
	select {
	case err := <-again:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a migrated store took the store lock again")
	}
}
