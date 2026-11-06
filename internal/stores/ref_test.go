package stores

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// refRepo is a bare Git remote reachable as url without network: commit a on
// tag v1 and branch release, commit b on main.
type refRepo struct {
	seed, url string
	a, b      string
}

func gitOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, repo, file, body string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, file), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "-C", repo, "add", file)
	gitRun(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", file)
	return gitOut(t, "-C", repo, "rev-parse", "HEAD")
}

func newRefRepo(t *testing.T) refRepo {
	t.Helper()
	seed, remote := t.TempDir(), filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, "init", "-b", "main", seed)
	a := commitFile(t, seed, "a.txt", "a\n")
	gitRun(t, "-C", seed, "tag", "v1")
	gitRun(t, "-C", seed, "branch", "release")
	b := commitFile(t, seed, "b.txt", "b\n")
	gitRun(t, "clone", "--bare", seed, remote)
	gitRun(t, "-C", seed, "remote", "add", "origin", remote)

	url := "https://store.example/team.git"
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(config, []byte("[url \"file://"+remote+"\"]\n\tinsteadOf = "+url+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return refRepo{seed: seed, url: url, a: a, b: b}
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

func TestCatalogReportsRefAndCommit(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	catalog, db := openCatalog(t, t.TempDir())
	defer db.Close()
	local := t.TempDir()
	if got, err := catalog.Add(ctx, "local", local, ""); err != nil || got.Ref != "" || got.Commit != "" {
		t.Fatalf("local Add() = %#v, %v", got, err)
	}
	got, err := catalog.Add(ctx, "team", repo.url, "")
	if err != nil || got.Ref != "" || got.Commit != repo.b {
		t.Fatalf("git Add() = %#v, %v; want commit %s", got, err, repo.b)
	}
	list, err := catalog.List(ctx)
	if err != nil || len(list) != 2 || list[1].Commit != repo.b {
		t.Fatalf("List() = %#v, %v", list, err)
	}
	detail, err := catalog.Detail(ctx, "team")
	if err != nil || detail.Ref != "" || detail.Commit != repo.b {
		t.Fatalf("Detail() = %#v, %v", detail, err)
	}
}

func TestCatalogAddGitStoreAtTag(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	base := t.TempDir()
	catalog, db := openCatalog(t, base)
	defer db.Close()
	got, err := catalog.Add(ctx, "team", repo.url, "v1")
	if err != nil || got.Ref != "v1" || got.Commit != repo.a {
		t.Fatalf("Add(v1) = %#v, %v; want commit %s", got, err, repo.a)
	}
	if exists(t, filepath.Join(base, "stores", "team", "b.txt")) {
		t.Fatal("tree at v1 holds b.txt from main")
	}
	if detail, err := catalog.Detail(ctx, "team"); err != nil || detail.Ref != "v1" || detail.Commit != repo.a {
		t.Fatalf("Detail() = %#v, %v", detail, err)
	}
}

func TestCatalogAddGitStoreAtCommit(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	catalog, db := openCatalog(t, t.TempDir())
	defer db.Close()
	for name, ref := range map[string]string{"full": repo.a, "short": repo.a[:7]} {
		got, err := catalog.Add(ctx, name, repo.url, ref)
		if err != nil || got.Ref != ref || got.Commit != repo.a {
			t.Fatalf("Add(%s) = %#v, %v; want commit %s", ref, got, err, repo.a)
		}
	}
}

func TestCatalogAddUnknownRefRegistersNothing(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	base := t.TempDir()
	catalog, db := openCatalog(t, base)
	defer db.Close()
	if _, err := catalog.Add(ctx, "team", repo.url, "nope"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("Add(nope) = %v, want ErrRefNotFound", err)
	}
	if list, err := catalog.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List() = %#v, %v", list, err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "stores"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("stores dir = %v, %v; want empty", entries, err)
	}
}

func TestCatalogAddRefRejectsLocalStoreAndBadRef(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	catalog, db := openCatalog(t, t.TempDir())
	defer db.Close()
	if _, err := catalog.Add(ctx, "local", t.TempDir(), "main"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("local Store with ref = %v, want ErrInvalid", err)
	}
	for _, ref := range []string{"-x", "a..b", "a b", " ", strings.Repeat("r", 256)} {
		_, err := catalog.Add(ctx, "team", repo.url, ref)
		if !errors.Is(err, ErrInvalid) || !errors.Is(err, ErrBadRef) {
			t.Fatalf("Add(ref %q) = %v, want ErrBadRef", ref, err)
		}
	}
}

func TestCatalogRefreshFollowsRefBranch(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	base := t.TempDir()
	catalog, db := openCatalog(t, base)
	defer db.Close()
	if _, err := catalog.Add(ctx, "team", repo.url, "release"); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "-C", repo.seed, "checkout", "-q", "release")
	c := commitFile(t, repo.seed, "c.txt", "c\n")
	gitRun(t, "-C", repo.seed, "checkout", "-q", "main")
	commitFile(t, repo.seed, "d.txt", "d\n")
	gitRun(t, "-C", repo.seed, "push", "-q", "origin", "main", "release")

	detail, err := catalog.Refresh(ctx, "team")
	if err != nil || detail.Ref != "release" || detail.Commit != c {
		t.Fatalf("Refresh() = %#v, %v; want commit %s", detail, err, c)
	}
	managed := filepath.Join(base, "stores", "team")
	if !exists(t, filepath.Join(managed, "c.txt")) || exists(t, filepath.Join(managed, "d.txt")) {
		t.Fatal("refresh did not follow branch release")
	}
}

func TestCatalogSetRefSwitchesTree(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	catalog, db := openCatalog(t, t.TempDir())
	defer db.Close()
	if _, err := catalog.Add(ctx, "team", repo.url, "main"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ ref, commit string }{{"v1", repo.a}, {"main", repo.b}, {"release", repo.a}, {"", repo.b}} {
		detail, err := catalog.SetRef(ctx, "team", step.ref)
		if err != nil || detail.Ref != step.ref || detail.Commit != step.commit {
			t.Fatalf("SetRef(%q) = %#v, %v; want commit %s", step.ref, detail, err, step.commit)
		}
		if list, err := catalog.List(ctx); err != nil || list[0].Ref != step.ref {
			t.Fatalf("List after SetRef(%q) = %#v, %v", step.ref, list, err)
		}
	}
}

func TestCatalogSetRefUnknownKeepsTree(t *testing.T) {
	ctx := context.Background()
	repo := newRefRepo(t)
	base := t.TempDir()
	catalog, db := openCatalog(t, base)
	defer db.Close()
	if _, err := catalog.Add(ctx, "team", repo.url, "v1"); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(base, "stores", "team", "a.txt")
	if err := os.WriteFile(edited, []byte("local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.SetRef(ctx, "team", "nope"); !errors.Is(err, ErrRefNotFound) {
		t.Fatalf("SetRef(nope) = %v, want ErrRefNotFound", err)
	}
	if _, err := catalog.SetRef(ctx, "team", "-bad"); !errors.Is(err, ErrBadRef) {
		t.Fatalf("SetRef(-bad) = %v, want ErrBadRef", err)
	}
	detail, err := catalog.Detail(ctx, "team")
	if err != nil || detail.Ref != "v1" || detail.Commit != repo.a {
		t.Fatalf("Detail after failed SetRef = %#v, %v", detail, err)
	}
	if body, err := os.ReadFile(edited); err != nil || string(body) != "local\n" {
		t.Fatalf("failed SetRef changed the tree: %q, %v", body, err)
	}
}

func TestCatalogSetRefRejectsLocalStore(t *testing.T) {
	catalog, db := openCatalog(t, t.TempDir())
	defer db.Close()
	if _, err := catalog.Add(context.Background(), "local", t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.SetRef(context.Background(), "local", "main"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("SetRef on a local Store = %v, want ErrInvalid", err)
	}
}
