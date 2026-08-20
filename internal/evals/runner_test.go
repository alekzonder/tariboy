package evals

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/store"
)

type fakeEvaluator struct {
	got []plugins.EvalRequestDTO
	err error
}

func (f *fakeEvaluator) Evaluate(_ context.Context, req plugins.EvalRequestDTO) (plugins.EvalVerdictDTO, error) {
	f.got = append(f.got, req)
	if f.err != nil {
		return plugins.EvalVerdictDTO{}, f.err
	}
	return plugins.EvalVerdictDTO{Verdict: "pass", Score: 1, Detail: "ok"}, nil
}

func buildEvalImage(t *testing.T) (*image.Store, string) {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "task.md"), []byte("task"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "judge.md"), []byte("PASS or FAIL?"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Tariboyfile.yaml"), []byte(`schema_version: 1
prompts:
  - task.md
evals:
  - { name: followed-task, type: llm-judge, prompt: judge.md }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	imgFile, err := imagefile.Parse(filepath.Join(src, "Tariboyfile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	st := &image.Store{Dir: t.TempDir()}
	ref, _ := image.ParseRef("evaldemo:latest")
	if _, err := image.Build(imgFile, ref, st, func() time.Time { return time.Unix(0, 0).UTC() }); err != nil {
		t.Fatal(err)
	}
	return st, "evaldemo:latest"
}

func TestRunnerRunsAndStores(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	imgStore, ref := buildEvalImage(t)
	fe := &fakeEvaluator{}
	es := NewStore(s, func() time.Time { return time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC) })
	r := NewRunner(RunnerConfig{
		Store: es, ImgStore: imgStore, Plugins: fe,
		AgentsDir: t.TempDir(), Clock: func() time.Time { return time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC) },
	})
	ag := agent.Agent{Name: "scout", ImageRef: ref}
	r.process(evalJob{ag: ag, iterationID: "scout-1", status: "done"})

	list, err := es.ListByIteration("scout-1")
	if err != nil || len(list) != 1 || list[0].Verdict != "pass" || list[0].EvalName != "followed-task" {
		t.Fatalf("results = %+v err=%v", list, err)
	}
	if len(fe.got) != 1 || fe.got[0].Status != "done" || fe.got[0].Prompt != "PASS or FAIL?" {
		t.Fatalf("evaluator got = %+v", fe.got)
	}

	// A failing/absent plugin records an "error" verdict, never panics.
	fe.err = plugins.ErrNoEvalPlugin
	r.process(evalJob{ag: ag, iterationID: "scout-2", status: "done"})
	l2, _ := es.ListByIteration("scout-2")
	if len(l2) != 1 || l2[0].Verdict != "error" {
		t.Fatalf("error verdict not recorded: %+v", l2)
	}
}

// fakeMinter records mint/revoke calls; MintToken returns a proxy token that is
// clearly NOT a raw upstream key, so the test can assert the plugin only ever
// sees a scoped proxy token.
type fakeMinter struct {
	baseURL   string
	minted    []string
	revoked   []string
	mintAttrs [][5]string
}

func (m *fakeMinter) ProxyBaseURL() string { return m.baseURL }

func (m *fakeMinter) MintToken(agent, iteration, imageName, imageTag, imageDigest string) (string, error) {
	m.mintAttrs = append(m.mintAttrs, [5]string{agent, iteration, imageName, imageTag, imageDigest})
	tok := "sk-tariboy-proxytoken"
	m.minted = append(m.minted, tok)
	return tok, nil
}

func (m *fakeMinter) RevokeToken(token string) { m.revoked = append(m.revoked, token) }

// TestRunnerLLMJudgeRoutesThroughProxy asserts an llm-judge request carries the
// proxy base URL, a scoped proxy token and the judge model (so the plugin's AI
// call routes THROUGH the daemon proxy) and never a raw upstream key. The token
// is minted with the iteration's attribution and revoked after the eval.
func TestRunnerLLMJudgeRoutesThroughProxy(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	imgStore, ref := buildEvalImage(t)
	fe := &fakeEvaluator{}
	fm := &fakeMinter{baseURL: "http://127.0.0.1:12345"}
	es := NewStore(s, nil)
	r := NewRunner(RunnerConfig{
		Store: es, ImgStore: imgStore, Plugins: fe, Minter: fm,
		AgentsDir: t.TempDir(), JudgeModel: "claude-opus-4-8",
	})
	r.process(evalJob{ag: agent.Agent{Name: "scout", ImageRef: ref}, iterationID: "scout-1", status: "done"})

	if len(fe.got) != 1 {
		t.Fatalf("expected 1 eval request, got %d", len(fe.got))
	}
	req := fe.got[0]
	if req.ProxyBaseURL != "http://127.0.0.1:12345" || req.ProxyToken != "sk-tariboy-proxytoken" || req.Model != "claude-opus-4-8" {
		t.Fatalf("llm-judge did not route through proxy: base=%q token=%q model=%q", req.ProxyBaseURL, req.ProxyToken, req.Model)
	}
	// The plugin must NEVER receive a raw upstream key: the token is a scoped
	// proxy token, not an sk-ant-*/sk-* upstream secret.
	if req.ProxyToken == "" || !strings.HasPrefix(req.ProxyToken, "sk-tariboy-") {
		t.Fatalf("proxy token is not a scoped tariboy token: %q", req.ProxyToken)
	}
	// The token was minted with the iteration's attribution and revoked after.
	if len(fm.minted) != 1 || len(fm.revoked) != 1 || fm.revoked[0] != fm.minted[0] {
		t.Fatalf("token lifecycle wrong: minted=%v revoked=%v", fm.minted, fm.revoked)
	}
	if fm.mintAttrs[0][0] != "scout" || fm.mintAttrs[0][1] != "scout-1" {
		t.Fatalf("mint attribution wrong: %v", fm.mintAttrs[0])
	}
}

// fakeTokenMinter is a fakeMinter that also implements ProxyBaseURLForToken, so
// the runner must hand the plugin the TOKENIZED proxy URL (path attribution)
// rather than the bare base URL.
type fakeTokenMinter struct{ *fakeMinter }

func (m *fakeTokenMinter) ProxyBaseURLForToken(token string) string {
	return m.baseURL + "/_tariboy/" + token
}

// TestRunnerLLMJudgeUsesTokenizedProxyURL asserts that when the minter supports
// path-token attribution, the eval request's ProxyBaseURL carries the token in
// the path (.../_tariboy/<token>), matching the proxy's resolvePathToken
// model. A bare base URL would reach the proxy without attribution and be
// rejected with 401 (the regression this guards against).
func TestRunnerLLMJudgeUsesTokenizedProxyURL(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	imgStore, ref := buildEvalImage(t)
	fe := &fakeEvaluator{}
	fm := &fakeTokenMinter{fakeMinter: &fakeMinter{baseURL: "http://127.0.0.1:12345"}}
	es := NewStore(s, nil)
	r := NewRunner(RunnerConfig{
		Store: es, ImgStore: imgStore, Plugins: fe, Minter: fm,
		AgentsDir: t.TempDir(), JudgeModel: "claude-opus-4-8",
	})
	r.process(evalJob{ag: agent.Agent{Name: "scout", ImageRef: ref}, iterationID: "scout-1", status: "done"})

	if len(fe.got) != 1 {
		t.Fatalf("expected 1 eval request, got %d", len(fe.got))
	}
	req := fe.got[0]
	want := "http://127.0.0.1:12345/_tariboy/" + req.ProxyToken
	if req.ProxyBaseURL != want {
		t.Fatalf("llm-judge proxy base URL = %q, want tokenized %q", req.ProxyBaseURL, want)
	}
}

// TestRunEvalsNonBlocking proves the loop-facing enqueue never blocks: with a
// tiny buffer and no worker draining, RunEvals returns immediately and sheds
// overflow (best-effort drop) rather than blocking the caller (the loop tick).
func TestRunEvalsNonBlocking(t *testing.T) {
	imgStore, ref := buildEvalImage(t)
	r := NewRunner(RunnerConfig{
		Store: NewStore(mustOpen(t), nil), ImgStore: imgStore,
		Plugins: &fakeEvaluator{}, AgentsDir: t.TempDir(), Buffer: 1,
	})
	ag := agent.Agent{Name: "scout", ImageRef: ref}
	done := make(chan struct{})
	go func() {
		// No worker is running, so only Buffer(1) job fits; the rest are dropped.
		for i := 0; i < 100; i++ {
			r.RunEvals(ag, "scout-x", "done")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunEvals blocked the caller")
	}
	if r.Dropped() == 0 {
		t.Fatal("expected overflow jobs to be dropped, got 0")
	}
}

func mustOpen(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRunnerDrainsOnCancel(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "x.db"))
	defer s.Close()
	imgStore, ref := buildEvalImage(t)
	es := NewStore(s, nil)
	r := NewRunner(RunnerConfig{Store: es, ImgStore: imgStore, Plugins: &fakeEvaluator{}, AgentsDir: t.TempDir()})
	r.RunEvals(agent.Agent{Name: "scout", ImageRef: ref}, "scout-3", "done")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled: Run must drain the queued job, then return.
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after drain")
	}
	if l, _ := es.ListByIteration("scout-3"); len(l) != 1 {
		t.Fatalf("queued job not drained: %+v", l)
	}
}
