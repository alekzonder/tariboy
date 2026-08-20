package aiproxy

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentTokenRegistrySurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aiproxy-handoff.json")
	reg, err := OpenTokenRegistry(path, bytes.NewReader(bytes.Repeat([]byte{0xAB}, 1024)))
	if err != nil {
		t.Fatal(err)
	}
	attr := Attribution{
		Agent: "alice", Iteration: "alice-1", ImageName: "basic",
		ImageTag: "latest", ImageDigest: "sha256:abc",
	}
	tok, err := reg.Mint(attr)
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.UpdateTask(tok, "dev-t-1.2", "dev-t-1"); got != 1 {
		t.Fatalf("UpdateTask = %d, want 1", got)
	}
	if err := reg.SetListenAddr("127.0.0.1:43123"); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenTokenRegistry(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.Resolve(tok)
	if !ok {
		t.Fatal("persisted token did not resolve after reopen")
	}
	want := attr
	want.TaskID, want.EpicID = "dev-t-1.2", "dev-t-1"
	if got != want {
		t.Fatalf("attribution = %+v, want %+v", got, want)
	}
	if got := reopened.ListenAddr(); got != "127.0.0.1:43123" {
		t.Fatalf("listen address = %q, want 127.0.0.1:43123", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("handoff mode = %#o, want 0600", got)
	}
}

func TestPersistentTokenRegistryRevokesIterationAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aiproxy-handoff.json")
	random := append(bytes.Repeat([]byte{0xCD}, 24), bytes.Repeat([]byte{0xCE}, 24)...)
	reg, err := OpenTokenRegistry(path, bytes.NewReader(random))
	if err != nil {
		t.Fatal(err)
	}
	tok1, err := reg.Mint(Attribution{Agent: "alice", Iteration: "alice-1"})
	if err != nil {
		t.Fatal(err)
	}
	tok2, err := reg.Mint(Attribution{Agent: "alice", Iteration: "alice-2"})
	if err != nil {
		t.Fatal(err)
	}

	reg.RevokeIteration("alice-1")
	reopened, err := OpenTokenRegistry(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Resolve(tok1); ok {
		t.Fatal("iteration-wide revoke left the matching token active")
	}
	if _, ok := reopened.Resolve(tok2); !ok {
		t.Fatal("iteration-wide revoke removed an unrelated token")
	}
}

func TestPersistentTokenRegistryPrunesLeases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aiproxy-handoff.json")
	random := append(bytes.Repeat([]byte{0xEF}, 24), bytes.Repeat([]byte{0xF0}, 24)...)
	reg, err := OpenTokenRegistry(path, bytes.NewReader(random))
	if err != nil {
		t.Fatal(err)
	}
	keep, _ := reg.Mint(Attribution{Agent: "alice", Iteration: "alice-live"})
	drop, _ := reg.Mint(Attribution{Agent: "alice", Iteration: "alice-done"})

	if err := reg.Prune(func(a Attribution) bool { return a.Iteration == "alice-live" }); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenTokenRegistry(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Resolve(keep); !ok {
		t.Fatal("prune removed the live lease")
	}
	if _, ok := reopened.Resolve(drop); ok {
		t.Fatal("prune retained a terminal lease")
	}
}

func TestPersistentTokenRegistryRejectsMalformedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aiproxy-handoff.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenTokenRegistry(path, nil); err == nil {
		t.Fatal("malformed handoff state was accepted")
	}
}
