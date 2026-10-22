package image

import (
	"path/filepath"
	"sync"
)

// Image publication locks. A ref lock serializes everything that publishes or
// reads one image name, because the tags and refs of one name change together.
// The store lock is taken for writing by operations that touch several names
// or the whole store: migration, removal, and agent assignment and activation,
// which read an agent's current and pending images together. A ref lock holds
// the store lock for reading, so a store-wide operation excludes every ref
// operation while different names still proceed in parallel.
//
// Lock order: the store lock before a name lock. Neither lock is reentrant, so
// a holder never takes either again. The locks are in-process: tariboyd is the
// one process that owns the image store.
var storeLock sync.RWMutex

var refLocks = struct {
	mu    sync.Mutex
	names map[string]*refLock
}{names: map[string]*refLock{}}

type refLock struct {
	mu    sync.Mutex
	users int
}

// LockRef takes the lock for image name and returns its release.
func LockRef(name string) (unlock func()) {
	storeLock.RLock()
	refLocks.mu.Lock()
	l := refLocks.names[name]
	if l == nil {
		l = &refLock{}
		refLocks.names[name] = l
	}
	l.users++
	refLocks.mu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		refLocks.mu.Lock()
		if l.users--; l.users == 0 {
			delete(refLocks.names, name)
		}
		refLocks.mu.Unlock()
		storeLock.RUnlock()
	}
}

// LockStore takes the store lock for writing and returns its release.
func LockStore() (unlock func()) {
	storeLock.Lock()
	return storeLock.Unlock
}

// WithRefLock runs fn holding the lock for image name.
func WithRefLock(name string, fn func() error) error {
	defer LockRef(name)()
	return fn()
}

// WithStoreLock runs fn holding the store lock for writing.
func WithStoreLock(fn func() error) error {
	defer LockStore()()
	return fn()
}

var migrated = struct {
	mu   sync.Mutex
	dirs map[string]bool
}{dirs: map[string]bool{}}

// MigrateOnce runs Migrate under the store lock the first time this process
// asks for s.Dir and does nothing afterwards. Nothing writes the legacy layout
// once a store is migrated, and a publisher that took the store lock on every
// build would wait for the publications of every other name. Call it before
// taking a name lock.
func (s *Store) MigrateOnce() error {
	dir := filepath.Clean(s.Dir)
	migrated.mu.Lock()
	done := migrated.dirs[dir]
	migrated.mu.Unlock()
	if done {
		return nil
	}
	defer LockStore()()
	if err := s.Migrate(); err != nil {
		return err
	}
	migrated.mu.Lock()
	migrated.dirs[dir] = true
	migrated.mu.Unlock()
	return nil
}
