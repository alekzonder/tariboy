package loop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/shim"
)

// stubIterationStore is the injectable slice of the agent store used by
// terminal-status finalization. It records the on-disk state of the shim socket
// at the moment the status is committed, which is what pins the removal order.
type stubIterationStore struct {
	it        agent.Iteration
	sockPath  string
	updateErr error

	updates       int
	sockAtCommit  bool
	attemptedStat string
	committedStat string
}

func (s *stubIterationStore) GetIteration(agentName, id string) (agent.Iteration, error) {
	if s.it.Agent != agentName || s.it.ID != id {
		return agent.Iteration{}, errors.New("no such iteration")
	}
	return s.it, nil
}

func (s *stubIterationStore) UpdateIteration(it agent.Iteration) error {
	s.updates++
	_, err := os.Stat(s.sockPath)
	s.sockAtCommit = err == nil
	s.attemptedStat = it.Status
	if s.updateErr != nil {
		return s.updateErr
	}
	s.it = it
	s.committedStat = it.Status
	return nil
}

// finalizeFixture wires a manager whose finalization store is the stub, plus a
// layout with a shim socket marker on disk for one running iteration.
func finalizeFixture(t *testing.T, updateErr error) (*Manager, *stubIterationStore, agentdir.Layout, agentdir.LiveIteration) {
	t.Helper()
	m, _, agentsDir, _ := newManager(t, &fakeRunner{})
	l := agentdir.New(agentsDir, "smoke")
	if err := os.MkdirAll(filepath.Dir(l.ShimSock()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.ShimSock(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	st := &stubIterationStore{
		it:        agent.Iteration{ID: "it-1", Agent: "smoke", Status: "running"},
		sockPath:  l.ShimSock(),
		updateErr: updateErr,
	}
	m.cfg.iterationStore = st
	li := agentdir.LiveIteration{Agent: "smoke", ID: "it-1", ShimSock: l.ShimSock()}
	return m, st, l, li
}

// finalizePaths are the three call sites that must behave identically: they
// close the same window (socket gone before a committed terminal status) and
// keep the same fallback (socket kept when the commit fails).
var finalizePaths = []struct {
	name string
	run  func(m *Manager, l agentdir.Layout, li agentdir.LiveIteration) error
}{
	{"recordAdopted", func(m *Manager, l agentdir.Layout, li agentdir.LiveIteration) error {
		m.recordAdopted(l, li, shim.IterationResult{ExitCode: 0})
		return nil
	}},
	{"recordStaleAdoption", func(m *Manager, l agentdir.Layout, li agentdir.LiveIteration) error {
		m.recordStaleAdoption(l, li)
		return nil
	}},
	{"recoverStaleKill", func(m *Manager, l agentdir.Layout, li agentdir.LiveIteration) error {
		// A non-nil runtime that is not the agent's registered one, so the
		// abort branch (which needs a live engine) is skipped.
		return m.recoverStaleKill(li.Agent, li.ID, &runtime{}, l, errors.New("shim unreachable"))
	}},
}

// A committed terminal status must never be observable while the shim socket is
// still on disk: the status is what every observer polls, so the socket has to
// go first. Moving os.Remove after the store write makes this fail.
func TestFinalizeIterationRemovesSocketBeforeCommittingTerminalStatus(t *testing.T) {
	for _, p := range finalizePaths {
		t.Run(p.name, func(t *testing.T) {
			m, st, l, li := finalizeFixture(t, nil)
			if err := p.run(m, l, li); err != nil {
				t.Fatalf("%s = %v, want nil", p.name, err)
			}
			if st.updates != 1 {
				t.Fatalf("UpdateIteration calls = %d, want 1", st.updates)
			}
			if st.committedStat == "running" || st.committedStat == "" {
				t.Fatalf("committed status = %q, want terminal", st.committedStat)
			}
			if st.sockAtCommit {
				t.Fatal("shim.sock was still on disk when the terminal status was committed")
			}
			if _, err := os.Stat(l.ShimSock()); !os.IsNotExist(err) {
				t.Fatalf("shim.sock after a committed status: %v, want removed", err)
			}
		})
	}
}

// The mirror image: when the store rejects the terminal status the iteration
// stays "running", so the shim.sock marker must survive. agentdir.ListLive
// finds an iteration to re-classify only by that marker, and without it the row
// is running forever and the orphaned session is never reaped. Dropping the
// rollback in finalizeIteration makes this fail.
//
// The assertions deliberately pin the whole sequence — a terminal status was
// attempted exactly once, the socket was already gone at that moment, nothing
// was committed, and the marker is back on disk afterwards — because the
// surviving marker alone is also what a finalizeIteration that does nothing at
// all would leave behind.
func TestFinalizeIterationKeepsSocketWhenTerminalStatusIsRejected(t *testing.T) {
	for _, p := range finalizePaths {
		t.Run(p.name, func(t *testing.T) {
			m, st, l, li := finalizeFixture(t, errors.New("store is down"))
			err := p.run(m, l, li)
			if p.name == "recoverStaleKill" {
				if err == nil || !errors.Is(err, st.updateErr) {
					t.Fatalf("%s = %v, want the store error", p.name, err)
				}
			}
			if st.updates != 1 {
				t.Fatalf("UpdateIteration calls = %d, want 1", st.updates)
			}
			if st.attemptedStat == "running" || st.attemptedStat == "" {
				t.Fatalf("attempted status = %q, want terminal", st.attemptedStat)
			}
			if st.sockAtCommit {
				t.Fatal("shim.sock was still on disk when the terminal status was attempted")
			}
			if st.committedStat != "" {
				t.Fatalf("committed status = %q, want nothing committed", st.committedStat)
			}
			if st.it.Status != "running" {
				t.Fatalf("iteration status = %q, want it left running", st.it.Status)
			}
			if _, err := os.Stat(l.ShimSock()); err != nil {
				t.Fatalf("shim.sock after a rejected status: %v, want kept for re-adoption", err)
			}
		})
	}
}
