// Package onboarding sets up a fresh daemon without user action: it registers
// the default Store, builds the starter image from it, and reports the state
// of both together with the harness CLIs found on the daemon PATH.
package onboarding

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	storedb "github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/stores"
	"github.com/alekzonder/tariboy/internal/toolcheck"
)

// The default Store and its starter image. These are the single source of
// truth for the name and the URL.
const (
	DefaultStoreName   = "tariboy"
	DefaultStoreSource = "https://github.com/alekzonder/tariboy-store.git"
	StarterImage       = "basic"
)

// ConfigKey records the outcome of the default Store registration in the
// daemon config: registered, failed or removed. Absent means never tried.
const ConfigKey = "onboarding.default_store"

// The default Store and starter image states.
const (
	StateRegistered = "registered"
	StateFailed     = "failed"
	StateRemoved    = "removed"
	StatePending    = "pending"
	StateReady      = "ready"
	StateBuilding   = "building"
	StateSkipped    = "skipped"
)

const harnessTTL = 5 * time.Minute

const offError = "automatic setup is turned off (TARIBOY_ONBOARDING=off)"

const waitsForStore = "the starter image waits for the Tariboy Store"

// DefaultStore is the state of the default Store registration.
type DefaultStore struct {
	State  string `json:"state"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Error  string `json:"error,omitempty"`
}

// Starter is the state of the starter image.
type Starter struct {
	State string `json:"state"`
	Ref   string `json:"ref,omitempty"`
	Error string `json:"error,omitempty"`
}

// Status is the onboarding state of one daemon. The agent count is added by
// the command that reports it.
type Status struct {
	DefaultStore DefaultStore
	StarterImage Starter
	Harnesses    []toolcheck.Harness
}

// Service runs the first-run routine and reports its state. Errors stay in
// memory; only the registration outcome is persisted under ConfigKey.
type Service struct {
	DB      *storedb.Store
	Catalog *stores.Catalog
	Images  *image.Store
	// Build builds a Store image selector through the operator build path.
	Build func(ctx context.Context, selector string) error
	// Source is the default Store source; tests point it at a directory.
	Source string
	// Off skips the routine at start. An explicit Retry still runs it.
	Off bool
	Log *slog.Logger

	mu        sync.Mutex
	base      context.Context
	running   bool
	closed    bool
	retries   sync.WaitGroup
	storeErr  string
	starter   Starter
	harnesses []toolcheck.Harness
	checkedAt time.Time
}

// Start runs the routine once unless Off, then serves retries until ctx ends
// and waits for them.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.base = ctx
	s.mu.Unlock()
	if !s.Off {
		s.Run(ctx)
	}
	<-ctx.Done()
	// Retry checks closed under mu, so no Add runs after this point.
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.retries.Wait()
}

func (s *Service) started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.base != nil
}

// Retry starts the routine in the background unless it already runs, and
// returns the current status.
func (s *Service) Retry() Status {
	s.mu.Lock()
	base := s.base
	if base != nil && !s.closed && !s.running {
		s.running = true
		s.retries.Go(func() { s.run(base) })
	}
	s.mu.Unlock()
	return s.Status(context.Background(), false)
}

// Run registers the default Store and builds the starter image when they are
// missing. A call while another run is active returns at once.
func (s *Service) Run(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	s.run(ctx)
}

func (s *Service) run(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	if store, ok := s.ensureStore(ctx); ok {
		s.ensureStarter(ctx, store.Name)
	}
}

func (s *Service) ensureStore(ctx context.Context) (stores.Store, bool) {
	found, ok, err := s.findDefault(ctx, s.Catalog.List)
	if err != nil {
		s.fail(err)
		return stores.Store{}, false
	}
	key, _, err := s.DB.ConfigGet(ConfigKey)
	if err != nil {
		s.fail(err)
		return stores.Store{}, false
	}
	if ok {
		if key == "" {
			s.setKey(StateRegistered)
		}
		return found, true
	}
	if key == StateRemoved {
		return stores.Store{}, false
	}
	added, err := s.Catalog.Add(ctx, DefaultStoreName, s.source())
	if err != nil {
		s.setKey(StateFailed)
		s.fail(withToolHint(err, "git"))
		return stores.Store{}, false
	}
	s.setKey(StateRegistered)
	s.mu.Lock()
	s.storeErr = ""
	s.mu.Unlock()
	return added, true
}

func (s *Service) ensureStarter(ctx context.Context, storeName string) {
	if _, ok := s.basicRef(); ok {
		return
	}
	s.setStarter(Starter{State: StateBuilding})
	if err := s.Build(ctx, storeName+"/"+StarterImage); err != nil {
		s.setStarter(Starter{State: StateFailed, Error: withToolHint(err, "npx").Error()})
		return
	}
	s.setStarter(Starter{State: StateReady, Ref: StarterImage + ":latest"})
}

// Status reads the current state. It is cheap: config, the Store list, the
// image list and the harness cache.
func (s *Service) Status(ctx context.Context, refresh bool) Status {
	store, found, err := s.findDefault(ctx, s.registeredStores)
	st := Status{DefaultStore: s.storeStatus(store, found, err), Harnesses: s.harnessList(refresh)}
	st.StarterImage = s.starterStatus(found)
	return st
}

func (s *Service) storeStatus(store stores.Store, found bool, listErr error) DefaultStore {
	if found {
		return DefaultStore{State: StateRegistered, Name: store.Name, Source: store.Source}
	}
	out := DefaultStore{State: StatePending, Name: DefaultStoreName, Source: s.source()}
	if listErr != nil {
		out.State, out.Error = StateFailed, listErr.Error()
		return out
	}
	if key, _, _ := s.DB.ConfigGet(ConfigKey); key == StateRemoved {
		out.State = StateRemoved
		return out
	}
	s.mu.Lock()
	running, storeErr := s.running, s.storeErr
	s.mu.Unlock()
	switch {
	case running:
	case storeErr != "":
		out.State, out.Error = StateFailed, storeErr
	case s.Off:
		// The routine never runs by itself here; failed tells the UI to stop
		// waiting and offer Retry.
		out.State, out.Error = StateFailed, offError
	}
	return out
}

func (s *Service) starterStatus(storeFound bool) Starter {
	s.mu.Lock()
	mem := s.starter
	s.mu.Unlock()
	// The build publishes basic before it returns.
	if mem.State == StateBuilding {
		return mem
	}
	if ref, ok := s.basicRef(); ok {
		if mem.State == StateReady {
			return Starter{State: StateReady, Ref: ref}
		}
		return Starter{State: StateSkipped, Ref: ref}
	}
	if mem.State == StateFailed {
		return mem
	}
	if !storeFound {
		return Starter{State: StatePending, Error: waitsForStore}
	}
	return Starter{State: StatePending}
}

// basicRef returns basic:latest when it exists, else the newest basic tag.
func (s *Service) basicRef() (string, bool) {
	list, err := s.Images.List()
	if err != nil {
		return "", false
	}
	var newest image.Manifest
	for _, m := range list {
		if m.Name != StarterImage {
			continue
		}
		if m.Tag == "latest" {
			return m.Name + ":" + m.Tag, true
		}
		if newest.Name == "" || m.BuiltAt > newest.BuiltAt {
			newest = m
		}
	}
	if newest.Name == "" {
		return "", false
	}
	return newest.Name + ":" + newest.Tag, true
}

func (s *Service) harnessList(refresh bool) []toolcheck.Harness {
	s.mu.Lock()
	defer s.mu.Unlock()
	if refresh || s.harnesses == nil || time.Since(s.checkedAt) > harnessTTL {
		s.harnesses, s.checkedAt = toolcheck.Harnesses(), time.Now()
	}
	return append([]toolcheck.Harness(nil), s.harnesses...)
}

func (s *Service) findDefault(ctx context.Context, list func(context.Context) ([]stores.Store, error)) (stores.Store, bool, error) {
	all, err := list(ctx)
	if err != nil {
		return stores.Store{}, false, err
	}
	for _, store := range all {
		if IsDefault(store, s.source()) {
			return store, true, nil
		}
	}
	return stores.Store{}, false, nil
}

// registeredStores reads the Store registrations without the catalog lock,
// which a clone or a build holds for minutes. Status needs only name and
// source.
func (s *Service) registeredStores(ctx context.Context) ([]stores.Store, error) {
	rows, err := s.DB.DB.QueryContext(ctx, `SELECT name, source FROM image_stores`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []stores.Store
	for rows.Next() {
		var st stores.Store
		if err := rows.Scan(&st.Name, &st.Source); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DefaultSource is the source the service registers the default Store from.
func (s *Service) DefaultSource() string { return s.source() }

func (s *Service) source() string {
	if s.Source == "" {
		return DefaultStoreSource
	}
	return s.Source
}

func (s *Service) fail(err error) {
	s.mu.Lock()
	s.storeErr = err.Error()
	s.mu.Unlock()
	s.warn("register default Store", err)
}

func (s *Service) setStarter(st Starter) {
	s.mu.Lock()
	s.starter = st
	s.mu.Unlock()
	if st.State == StateFailed && s.Log != nil {
		s.Log.Warn("build starter image", "err", st.Error)
	}
}

func (s *Service) setKey(value string) {
	if err := s.DB.ConfigSet(ConfigKey, value); err != nil {
		s.warn("record default Store state", err)
	}
}

func (s *Service) warn(msg string, err error) {
	if s.Log != nil {
		s.Log.Warn(msg, "err", err)
	}
}

// IsDefault reports the default Store: the name tariboy or the default source,
// the same rule the UI applies.
func IsDefault(store stores.Store, source string) bool {
	return store.Name == DefaultStoreName || store.Source == source || store.Source == DefaultStoreSource
}

// MarkRemoved records that the user removed the default Store, so the daemon
// never registers it again by itself. Other Stores leave the key unchanged.
func MarkRemoved(db *storedb.Store, removed stores.Store, source string) error {
	if !IsDefault(removed, source) {
		return nil
	}
	return db.ConfigSet(ConfigKey, StateRemoved)
}

// withToolHint prefixes err with the install hint of tool when tool is
// missing on the daemon PATH, the likely cause of the failure.
func withToolHint(err error, tool string) error {
	if t, missing := toolcheck.Missing(tool); missing {
		return &hintError{msg: t.Name + " is missing: " + t.Install, err: err}
	}
	return err
}

type hintError struct {
	msg string
	err error
}

func (e *hintError) Error() string { return e.msg + "; " + e.err.Error() }
func (e *hintError) Unwrap() error { return e.err }
