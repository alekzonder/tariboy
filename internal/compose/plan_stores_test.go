package compose

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const teamGit = "https://example.com/team.git"

func storesFile(stores map[string]StoreSpec) File {
	return File{Version: 2, Stores: stores}
}

// A Store the daemon lacks is created; a path Store gets the absolute path of
// its directory, resolved from the compose file's directory.
func TestUpCreatesMissingStoresWithAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "store")
	fc := newSourcesCaller()
	fc.remote[teamGit] = &sourceState{commit: "c1"}
	fc.remote[local] = &sourceState{}
	var out strings.Builder
	f := storesFile(map[string]StoreSpec{"team": {Git: teamGit, Ref: "main"}, "local": {Path: "./store"}})
	if err := NewRunner(fc, dir, &out).Up(f); err != nil {
		t.Fatalf("up: %v\n%s", err, out.String())
	}
	want := []string{"POST /api/stores", "POST /api/stores"}
	if got := mutating(fc.calls, 0); !slices.Equal(got, want) {
		t.Fatalf("mutating calls = %v, want %v", got, want)
	}
	if got := fc.registered["local"]; got == nil || got.source != local {
		t.Fatalf("local = %+v, want source %s", got, local)
	}
	if got := fc.registered["team"]; got == nil || got.source != teamGit || got.ref != "main" {
		t.Fatalf("team = %+v", got)
	}
	for _, res := range []string{"store:local", "store:team"} {
		if got := reportOutcome(out.String(), res); got != "changed" {
			t.Errorf("%s = %q:\n%s", res, got, out.String())
		}
	}
}

// A git Store at another ref is switched to the file's ref, then refreshed.
func TestUpChangesTheRefOfAGitStore(t *testing.T) {
	fc := newSourcesCaller()
	fc.remote[teamGit] = &sourceState{commit: "c2"}
	fc.registered["team"] = &fakeStore{source: teamGit, ref: "main", commit: "c1"}
	var out strings.Builder
	if err := NewRunner(fc, "", &out).Up(storesFile(map[string]StoreSpec{"team": {Git: teamGit, Ref: "v2"}})); err != nil {
		t.Fatalf("up: %v", err)
	}
	want := []string{"POST /api/stores/team/ref", "POST /api/stores/team/refresh"}
	if got := mutating(fc.calls, 0); !slices.Equal(got, want) {
		t.Fatalf("mutating calls = %v, want %v", got, want)
	}
	if fc.registered["team"].ref != "v2" || reportOutcome(out.String(), "store:team") != "changed" {
		t.Fatalf("team = %+v\n%s", fc.registered["team"], out.String())
	}
}

// Every up refreshes a git Store; the Store is ok when the commit stays.
func TestUpRefreshesAGitStoreAndReportsOKOnTheSameCommit(t *testing.T) {
	fc := newSourcesCaller()
	fc.remote[teamGit] = &sourceState{commit: "c1"}
	fc.registered["team"] = &fakeStore{source: teamGit, commit: "c1"}
	var out strings.Builder
	if err := NewRunner(fc, "", &out).Up(storesFile(map[string]StoreSpec{"team": {Git: teamGit}})); err != nil {
		t.Fatalf("up: %v", err)
	}
	if got := mutating(fc.calls, 0); !slices.Equal(got, []string{"POST /api/stores/team/refresh"}) {
		t.Fatalf("mutating calls = %v", got)
	}
	if got := reportOutcome(out.String(), "store:team"); got != "ok" {
		t.Fatalf("store:team = %q\n%s", got, out.String())
	}
	fc.remote[teamGit].commit = "c2"
	out.Reset()
	if err := NewRunner(fc, "", &out).Up(storesFile(map[string]StoreSpec{"team": {Git: teamGit}})); err != nil {
		t.Fatalf("up: %v", err)
	}
	if got := reportOutcome(out.String(), "store:team"); got != "changed" {
		t.Fatalf("store:team after a new commit = %q\n%s", got, out.String())
	}
}

// A path Store is never refreshed: compose reads it with GET only.
func TestUpChecksAPathStoreWithoutARefresh(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "store")
	fc := newSourcesCaller()
	fc.remote[local] = &sourceState{}
	fc.registered["local"] = &fakeStore{source: local}
	var out strings.Builder
	if err := NewRunner(fc, dir, &out).Up(storesFile(map[string]StoreSpec{"local": {Path: "store"}})); err != nil {
		t.Fatalf("up: %v", err)
	}
	if got := mutating(fc.calls, 0); len(got) != 0 {
		t.Fatalf("mutating calls = %v", got)
	}
	if callIndex(fc.calls, "GET /api/stores/local") < 0 || reportOutcome(out.String(), "store:local") != "ok" {
		t.Fatalf("calls %v\n%s", fc.calls, out.String())
	}
}

// A Store registered with another source fails, and compose does not change
// it.
func TestUpFailsAStoreWithAnotherSource(t *testing.T) {
	fc := newSourcesCaller()
	fc.remote["https://example.com/other.git"] = &sourceState{commit: "c1"}
	fc.registered["team"] = &fakeStore{source: "https://example.com/other.git", commit: "c1"}
	var out strings.Builder
	err := NewRunner(fc, "", &out).Up(storesFile(map[string]StoreSpec{"team": {Git: teamGit}}))
	if err == nil {
		t.Fatal("up succeeded")
	}
	if got := mutating(fc.calls, 0); len(got) != 0 {
		t.Fatalf("mutating calls = %v", got)
	}
	want := "failed   store:team: store team is registered with source https://example.com/other.git; the file declares " + teamGit +
		"; compose does not change a Store source (remove it with tariboy store rm team, then run up)"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("report lacks %q:\n%s", want, out.String())
	}
}
