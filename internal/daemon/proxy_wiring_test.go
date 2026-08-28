package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/alekzonder/tariboy/internal/store"
)

// captureSpawner records the environment an iteration's harness would be started
// with, then writes an exit-0 result.json so the iteration completes without a
// real process. It parses the iteration dir out of the shim argv so it can drop
// result.json where the runner polls for it.
type captureSpawner struct {
	mu         sync.Mutex
	argv       []string
	env        []string
	fired      chan struct{}
	once       sync.Once
	holdResult bool
	iterDir    string
}

type survivingShimHandler struct {
	running atomic.Bool
}

func (h *survivingShimHandler) Status() shim.StatusResult {
	return shim.StatusResult{Running: h.running.Load(), PID: os.Getpid()}
}
func (*survivingShimHandler) Kill() error                        { return nil }
func (*survivingShimHandler) Screen() (string, error)            { return "", nil }
func (*survivingShimHandler) SendKeys(shim.SendKeysParams) error { return nil }
func (*survivingShimHandler) Report() shim.ReportResult {
	return shim.ReportResult{Finished: false}
}

type survivingShimSpawner struct {
	mu         sync.Mutex
	env        []string
	resultPath string
	listener   net.Listener
	handler    survivingShimHandler
	fired      chan struct{}
	once       sync.Once
	starts     atomic.Int32
}

func (s *survivingShimSpawner) Start(argv, env []string, _ string) error {
	var sock, iterationDir string
	for i, arg := range argv {
		if i+1 >= len(argv) {
			continue
		}
		switch arg {
		case "--shim-sock":
			sock = argv[i+1]
		case "--iteration-dir":
			iterationDir = argv[i+1]
		}
	}
	if sock == "" || iterationDir == "" {
		return fmt.Errorf("missing shim paths in argv: %v", argv)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		return err
	}
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	s.handler.running.Store(true)
	s.mu.Lock()
	s.env = append([]string(nil), env...)
	s.resultPath = filepath.Join(iterationDir, "result.json")
	s.listener = ln
	s.mu.Unlock()
	s.starts.Add(1)
	go shim.Serve(ln, &s.handler)
	s.once.Do(func() { close(s.fired) })
	return nil
}

func (s *survivingShimSpawner) proxyURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.env {
		if strings.HasPrefix(item, "ANTHROPIC_BASE_URL=") {
			return strings.TrimPrefix(item, "ANTHROPIC_BASE_URL=")
		}
	}
	return ""
}

func (s *survivingShimSpawner) finish() error {
	s.mu.Lock()
	path := s.resultPath
	s.mu.Unlock()
	s.handler.running.Store(false)
	return os.WriteFile(path, []byte(`{"exit_code":0,"ended_at":"2026-07-29T11:00:00Z"}`), 0o600)
}

func (s *survivingShimSpawner) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func (s *captureSpawner) Start(argv, env []string, dir string) error {
	iterDir := ""
	for i, a := range argv {
		if a == "--iteration-dir" && i+1 < len(argv) {
			iterDir = argv[i+1]
		}
	}
	s.mu.Lock()
	s.argv = append([]string(nil), argv...)
	s.env = append([]string(nil), env...)
	s.iterDir = iterDir
	s.mu.Unlock()
	if iterDir != "" && !s.holdResult {
		_ = os.WriteFile(filepath.Join(iterDir, "result.json"), []byte(`{"exit_code":0}`), 0o600)
	}
	s.once.Do(func() { close(s.fired) })
	return nil
}

func (s *captureSpawner) finish() error {
	s.mu.Lock()
	iterDir := s.iterDir
	s.mu.Unlock()
	if iterDir == "" {
		return nil
	}
	return os.WriteFile(filepath.Join(iterDir, "result.json"), []byte(`{"exit_code":0}`), 0o600)
}

func (s *captureSpawner) argvSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.argv...)
}

func (s *captureSpawner) envMap(t *testing.T) map[string]string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]string{}
	for _, kv := range s.env {
		if i := strings.Index(kv, "="); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}

func buildBasicImage(t *testing.T, dir string) {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "task.md"), []byte("BODY"), 0o600); err != nil {
		t.Fatal(err)
	}
	im := &imagefile.Imagefile{SchemaVersion: 1,
		Plugins: []imagefile.Plugin{{Name: "context"}},
		Prompts: []imagefile.Prompt{{Filepath: filepath.Join(src, "task.md")}}, Dir: src}
	st := &image.Store{Dir: dir}
	if _, err := image.Build(im, image.Ref{Name: "basic", Tag: "latest"}, st, time.Now); err != nil {
		t.Fatal(err)
	}
}

func TestProxyHandoffReusesAddressAndPrunesTerminalLeases(t *testing.T) {
	base := t.TempDir()
	p := paths.New(base)
	st, err := store.Open(p.DB())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	agents := agent.NewStore(st)
	for _, it := range []agent.Iteration{
		{ID: "alice-1", Agent: "alice", Trigger: "manual", Status: "running", StartedAt: "2026-07-29T10:00:00Z"},
		{ID: "alice-2", Agent: "alice", Trigger: "manual", Status: "done", StartedAt: "2026-07-29T09:00:00Z"},
	} {
		if err := agents.CreateIteration(it); err != nil {
			t.Fatal(err)
		}
	}
	tokens, err := aiproxy.OpenTokenRegistry(p.ProxyHandoffFile(), nil)
	if err != nil {
		t.Fatal(err)
	}
	liveToken, _ := tokens.Mint(aiproxy.Attribution{Agent: "alice", Iteration: "alice-1"})
	doneToken, _ := tokens.Mint(aiproxy.Attribution{Agent: "alice", Iteration: "alice-2"})
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	savedAddr := reserved.Addr().String()
	reserved.Close()
	if err := tokens.SetListenAddr(savedAddr); err != nil {
		t.Fatal(err)
	}

	proxy := aiproxy.New(aiproxy.Config{Tokens: tokens})
	got, err := listenProxyWithHandoff(agents, proxy, tokens)
	if err != nil {
		t.Fatal(err)
	}
	if got != savedAddr {
		t.Fatalf("proxy address = %q, want carried %q", got, savedAddr)
	}
	if _, ok := tokens.Resolve(liveToken); !ok {
		t.Fatal("startup pruning removed live iteration lease")
	}
	if _, ok := tokens.Resolve(doneToken); ok {
		t.Fatal("startup pruning retained terminal iteration lease")
	}
	go proxy.Serve()
	if err := proxy.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestProxyHandoffFailsIfLiveAddressIsOccupied(t *testing.T) {
	base := t.TempDir()
	p := paths.New(base)
	st, err := store.Open(p.DB())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	agents := agent.NewStore(st)
	if err := agents.CreateIteration(agent.Iteration{
		ID: "alice-1", Agent: "alice", Trigger: "manual", Status: "running", StartedAt: "2026-07-29T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	tokens, err := aiproxy.OpenTokenRegistry(p.ProxyHandoffFile(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Mint(aiproxy.Attribution{Agent: "alice", Iteration: "alice-1"}); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	if err := tokens.SetListenAddr(occupied.Addr().String()); err != nil {
		t.Fatal(err)
	}

	proxy := aiproxy.New(aiproxy.Config{Tokens: tokens})
	if _, err := listenProxyWithHandoff(agents, proxy, tokens); err == nil {
		t.Fatal("occupied carried address with a live lease did not fail closed")
	}
}

func TestDaemonRestartPreservesShimIterationAndProxyLease(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"claude-test","usage":{"input_tokens":2,"output_tokens":1}}`))
	}))
	defer upstream.Close()
	t.Setenv("TARIBOY_UPSTREAM_ANTHROPIC_BASE_URL", upstream.URL)
	t.Setenv("TARIBOY_STUB_HARNESS", "/bin/true")

	base := t.TempDir()
	buildBasicImage(t, filepath.Join(base, "images"))
	spawner := &survivingShimSpawner{fired: make(chan struct{})}
	t.Cleanup(spawner.close)

	startDaemon := func() (*client.Client, func()) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, daemonTestOptions(Options{
				BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: "", Spawner: spawner,
			}))
		}()
		var once sync.Once
		stop := func() {
			once.Do(func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("daemon shutdown: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Error("daemon did not shut down")
				}
			})
		}
		t.Cleanup(stop)
		c := client.New(paths.New(base).Socket())
		deadline := time.Now().Add(5 * time.Second)
		var err error
		for time.Now().Before(deadline) {
			if _, err = c.Call("GET", "/api/daemon/status", nil); err == nil {
				return c, stop
			}
			time.Sleep(20 * time.Millisecond)
		}
		stop()
		t.Fatalf("daemon did not become ready: %v", err)
		return nil, nil
	}

	first, stopFirst := startDaemon()
	raw, err := first.Call("POST", "/api/agents", map[string]any{
		"name": "restart-agent", "image": "basic:latest", "harness": "stub",
		"plugins": "context", "loop": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.Name != "restart-agent" {
		t.Fatalf("created agent = %q, err=%v, body=%s", created.Name, err, raw)
	}
	if _, err := first.Call("POST", "/api/agents/restart-agent/start", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Call("POST", "/api/agents/restart-agent/exec", map[string]any{"prompt": "continue"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-spawner.fired:
	case <-time.After(5 * time.Second):
		t.Fatal("shim was not spawned")
	}
	oldProxyURL := spawner.proxyURL()
	if oldProxyURL == "" {
		t.Fatal("spawned harness did not receive ANTHROPIC_BASE_URL")
	}

	stopFirst()
	st, err := store.Open(paths.New(base).DB())
	if err != nil {
		t.Fatal(err)
	}
	agents := agent.NewStore(st)
	iterations, err := agents.ListIterations("restart-agent")
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	if len(iterations) != 1 || iterations[0].Status != "running" {
		st.Close()
		t.Fatalf("iteration after daemon stop = %+v, want one running row", iterations)
	}
	iterationID := iterations[0].ID
	st.Close()

	_, stopSecond := startDaemon()
	defer stopSecond()
	deadline := time.Now().Add(5 * time.Second)
	var responseStatus int
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequest(http.MethodPost, oldProxyURL+"/v1/messages", strings.NewReader(`{"model":"claude-test"}`))
		if reqErr != nil {
			t.Fatal(reqErr)
		}
		req.Header.Set("x-api-key", "provider-key")
		if resp, callErr := http.DefaultClient.Do(req); callErr == nil {
			responseStatus = resp.StatusCode
			_ = resp.Body.Close()
			if responseStatus == http.StatusOK {
				break
			}
			if responseStatus == http.StatusUnauthorized {
				t.Fatalf("carried proxy token was rejected at the old URL %s", oldProxyURL)
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if responseStatus != http.StatusOK {
		t.Fatalf("old proxy URL did not recover after restart, last status=%d", responseStatus)
	}
	if got := spawner.starts.Load(); got != 1 {
		t.Fatalf("daemon restart spawned %d shims, want the original one only", got)
	}

	if err := spawner.finish(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		check, openErr := store.Open(paths.New(base).DB())
		if openErr == nil {
			it, getErr := agent.NewStore(check).GetIteration("restart-agent", iterationID)
			check.Close()
			if getErr == nil && it.Status == "no_i_am_done" {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	check, err := store.Open(paths.New(base).DB())
	if err != nil {
		t.Fatal(err)
	}
	final, err := agent.NewStore(check).GetIteration("restart-agent", iterationID)
	check.Close()
	if err != nil || final.Status != "no_i_am_done" {
		t.Fatalf("adopted iteration = %+v, err=%v, want no_i_am_done", final, err)
	}
	deadline = time.Now().Add(5 * time.Second)
	leaseCount := -1
	for time.Now().Before(deadline) {
		registry, openErr := aiproxy.OpenTokenRegistry(paths.New(base).ProxyHandoffFile(), nil)
		if openErr == nil {
			leaseCount = registry.Count()
			if leaseCount == 0 {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if leaseCount != 0 {
		t.Fatalf("proxy leases after adopted completion = %d, want 0", leaseCount)
	}
}

// TestProxyWiredIntoIteration is the daemon-level assembly test: it boots a real
// daemon, runs an agent, wakes it with a message, and verifies the harness env
// the runner produced points at the AI proxy the daemon started — and that the
// URL in that env is a live proxy (a tokenless request gets a 401 from it).
func TestProxyWiredIntoIteration(t *testing.T) {
	t.Setenv("TARIBOY_STUB_HARNESS", "/bin/true")
	base := t.TempDir()
	buildBasicImage(t, filepath.Join(base, "images"))

	spawner := &captureSpawner{fired: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", Spawner: spawner}))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not shut down")
		}
	})

	sock := paths.New(base).Socket()
	c := client.New(sock)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := c.Call("GET", "/api/daemon/status", nil); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Create + start a message-triggered agent.
	raw, err := c.Call("POST", "/api/agents", map[string]any{
		"image": "basic:latest", "harness": "stub", "plugins": "context", "loop": true})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	var created struct {
		Name string `json:"name"`
	}
	json.Unmarshal(raw, &created)
	if created.Name == "" {
		t.Fatalf("no agent name in %s", raw)
	}
	if _, err := c.Call("POST", "/api/agents/"+created.Name+"/start", nil); err != nil {
		t.Fatalf("start agent: %v", err)
	}

	// Subscribe the agent to its own inbox (as its harness normally would) so the
	// publish below produces a pending delivery that wakes a message iteration.
	seed, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bus.New(seed, time.Now).Subscribe(created.Name, bus.InboxChannel(created.Name), bus.Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	seed.Close()

	// Publish through the daemon so its bus hook wakes the engine.
	if _, err := c.Call("POST", "/api/messages", map[string]any{
		"channel": bus.InboxChannel(created.Name), "type": "note", "text": "go"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-spawner.fired:
	case <-time.After(5 * time.Second):
		t.Fatal("iteration never spawned after wake")
	}

	env := spawner.envMap(t)
	baseURL := env["ANTHROPIC_BASE_URL"]
	if !strings.HasPrefix(baseURL, "http://127.0.0.1:") {
		t.Fatalf("ANTHROPIC_BASE_URL = %q, want a 127.0.0.1 proxy URL", baseURL)
	}
	if !strings.Contains(baseURL, "/_tariboy/sk-tariboy-") {
		t.Fatalf("ANTHROPIC_BASE_URL = %q, want tokenized attribution URL", baseURL)
	}
	if env["OPENAI_BASE_URL"] != baseURL+"/v1" {
		t.Fatalf("OPENAI_BASE_URL = %q, want %q", env["OPENAI_BASE_URL"], baseURL+"/v1")
	}

	// The base proxy without an attribution path must reject requests.
	proxyRoot := strings.Split(baseURL, "/_tariboy/")[0]
	resp, err := http.Get(proxyRoot + "/v1/messages")
	if err != nil {
		t.Fatalf("proxy not reachable at %s: %v", proxyRoot, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), "attribution token") {
		t.Fatalf("proxy check: status=%d body=%s", resp.StatusCode, body)
	}
}

// TestCodexChatGPTProxyWiring exercises the complete daemon assembly rather
// than constructing a proxy or harness adapter in isolation. The captured
// command is the exact argv that would launch Codex; its tokenized URL is then
// used as a client to prove the daemon-loaded ChatGPT route, persistence split,
// and credential redaction all hold together.
func TestCodexChatGPTProxyWiring(t *testing.T) {
	const (
		oauthCredential = "oauth-integration-secret"
		accountID       = "acct-integration-secret"
	)
	type upstreamCall struct {
		path          string
		authorization string
		account       string
	}
	var upstreamMu sync.Mutex
	var upstreamCalls []upstreamCall
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamMu.Lock()
		upstreamCalls = append(upstreamCalls, upstreamCall{
			path: r.URL.RequestURI(), authorization: r.Header.Get("Authorization"), account: r.Header.Get("chatgpt-account-id"),
		})
		upstreamMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/responses":
			_, _ = w.Write([]byte(`{"model":"gpt-5.6-terra","usage":{"input_tokens":13,"output_tokens":8}}`))
		default:
			http.Error(w, "unexpected upstream path", http.StatusNotFound)
		}
	}))
	defer fake.Close()

	base := t.TempDir()
	t.Setenv("TARIBOY_RUNTIME_DIR", filepath.Join(base, "runtime"))
	t.Setenv("TARIBOY_UPSTREAM_CHATGPT_BASE_URL", fake.URL)
	buildBasicImage(t, filepath.Join(base, "images"))

	spawner := &captureSpawner{fired: make(chan struct{}), holdResult: true}
	var ingester *aiproxy.Ingester
	var aiStore *aiproxy.Store
	ready := make(chan struct{})
	opts := daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", Spawner: spawner})
	opts.wireHook = func(ing *aiproxy.Ingester, st *aiproxy.Store) {
		ingester, aiStore = ing, st
		close(ready)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not shut down")
		}
	})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("daemon never wired the AI proxy")
	}

	c := client.New(paths.New(base).Socket())
	deadline := time.Now().Add(5 * time.Second)
	var statusErr error
	for time.Now().Before(deadline) {
		if _, statusErr = c.Call("GET", "/api/daemon/status", nil); statusErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if statusErr != nil {
		t.Fatalf("daemon never came up: %v", statusErr)
	}

	raw, err := c.Call("POST", "/api/agents", map[string]any{
		"image": "basic:latest", "harness": "codex", "plugins": "context", "loop": true,
		"model": "gpt-5.6-terra",
	})
	if err != nil {
		t.Fatalf("create Codex agent: %v", err)
	}
	var created struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.Name == "" {
		t.Fatalf("decode created agent: err=%v body=%s", err, raw)
	}
	if _, err := c.Call("POST", "/api/agents/"+created.Name+"/start", nil); err != nil {
		t.Fatalf("start agent: %v", err)
	}

	seed, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bus.New(seed, time.Now).Subscribe(created.Name, bus.InboxChannel(created.Name), bus.Matcher{}, nil); err != nil {
		seed.Close()
		t.Fatal(err)
	}
	seed.Close()
	if _, err := c.Call("POST", "/api/messages", map[string]any{
		"channel": bus.InboxChannel(created.Name), "type": "note", "text": "go",
	}); err != nil {
		t.Fatalf("publish wake message: %v", err)
	}
	select {
	case <-spawner.fired:
	case <-time.After(5 * time.Second):
		t.Fatal("Codex iteration never spawned")
	}

	argv := spawner.argvSnapshot()
	joinedArgv := strings.Join(argv, "\n")
	if strings.Count(joinedArgv, `model_providers.tariboy.requires_openai_auth=true`) != 1 {
		t.Fatalf("Codex argv requires_openai_auth count = %d, want 1", strings.Count(joinedArgv, `model_providers.tariboy.requires_openai_auth=true`))
	}
	if strings.Contains(joinedArgv, "env_key") {
		t.Fatal("Codex argv contains forbidden env_key")
	}
	const baseURLPrefix = "model_providers.tariboy.base_url="
	if strings.Count(joinedArgv, baseURLPrefix) != 1 {
		t.Fatalf("Codex argv base_url config count = %d, want 1", strings.Count(joinedArgv, baseURLPrefix))
	}
	proxyURL := ""
	iterationID := ""
	for i, arg := range argv {
		if strings.HasPrefix(arg, baseURLPrefix) {
			proxyURL, err = strconv.Unquote(strings.TrimPrefix(arg, baseURLPrefix))
			if err != nil {
				t.Fatalf("unquote Codex proxy URL config: %v", err)
			}
		}
		if arg == "--iteration-id" && i+1 < len(argv) {
			iterationID = argv[i+1]
		}
	}
	isLoopback := strings.HasPrefix(proxyURL, "http://127.0.0.1:")
	isTokenized := strings.Contains(proxyURL, "/_tariboy/sk-tariboy-")
	hasV1Suffix := strings.HasSuffix(proxyURL, "/v1")
	if !isLoopback || !isTokenized || hasV1Suffix {
		t.Fatalf("Codex proxy URL properties: loopback=%t tokenized=%t v1_suffix=%t", isLoopback, isTokenized, hasV1Suffix)
	}
	if iterationID == "" {
		t.Fatalf("iteration id missing from captured argv (argc=%d)", len(argv))
	}

	request := func(method, path, body string) {
		t.Helper()
		req, err := http.NewRequest(method, proxyURL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+oauthCredential)
		req.Header.Set("chatgpt-account-id", accountID)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s through captured proxy URL: %v", method, path, err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s: status=%d body=%s", method, path, resp.StatusCode, respBody)
		}
	}

	request(http.MethodGet, "/models?client_version=integration", "")
	if err := ingester.Flush(); err != nil {
		t.Fatalf("flush models request: %v", err)
	}
	usage, err := aiStore.Aggregate(aiproxy.UsageFilter{Agent: created.Name})
	if err != nil {
		t.Fatal(err)
	}
	if len(usage) != 0 {
		t.Fatalf("/models created usage rows: %+v", usage)
	}
	transcriptPath := filepath.Join(base, "agents", created.Name, "iterations", iterationID, "proxy-transcript.jsonl")
	if _, err := os.Stat(transcriptPath); !os.IsNotExist(err) {
		t.Fatalf("/models created transcript or stat failed: %v", err)
	}

	request(http.MethodPost, "/responses", `{"model":"gpt-5.6-terra","input":"hello"}`)
	usageDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(usageDeadline) {
		if err := ingester.Flush(); err != nil {
			t.Fatalf("flush responses request: %v", err)
		}
		usage, err = aiStore.Aggregate(aiproxy.UsageFilter{Agent: created.Name})
		if err != nil {
			t.Fatal(err)
		}
		if len(usage) == 1 && usage[0].Requests == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(usage) != 1 || usage[0].Requests != 1 || usage[0].InputTokens != 13 || usage[0].OutputTokens != 8 {
		t.Fatalf("responses usage = %+v, want one 13/8-token request", usage)
	}

	transcript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("read responses transcript: %v", err)
	}
	trimmedTranscript := strings.TrimSpace(string(transcript))
	if trimmedTranscript == "" {
		t.Fatal("responses transcript is empty")
	}
	transcriptLines := strings.Split(trimmedTranscript, "\n")
	if len(transcriptLines) != 1 {
		t.Fatalf("transcript lines = %d, want only the /responses exchange", len(transcriptLines))
	}
	var transcriptEntry aiproxy.TranscriptEntry
	if err := json.Unmarshal([]byte(transcriptLines[0]), &transcriptEntry); err != nil {
		t.Fatalf("decode responses transcript entry: %v", err)
	}
	if transcriptEntry.Meta.Iteration != iterationID || transcriptEntry.Meta.InputTokens != 13 || transcriptEntry.Meta.OutputTokens != 8 {
		t.Fatalf("responses transcript metadata: iteration_match=%t input_tokens=%d output_tokens=%d",
			transcriptEntry.Meta.Iteration == iterationID, transcriptEntry.Meta.InputTokens, transcriptEntry.Meta.OutputTokens)
	}
	for _, secret := range []string{oauthCredential, accountID} {
		if strings.Contains(string(transcript), secret) {
			t.Fatal("transcript contains a credential/account value")
		}
	}
	for _, path := range []string{
		filepath.Join(base, "agents", created.Name, "audit.jsonl"),
		filepath.Join(base, "agents", created.Name, "iterations", iterationID, "logs"),
	} {
		err := filepath.Walk(path, func(file string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), oauthCredential) || strings.Contains(string(data), accountID) {
				t.Errorf("credential/account value found in %s", filepath.Base(file))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan credential-sensitive path %s: %v", path, err)
		}
	}

	upstreamMu.Lock()
	calls := append([]upstreamCall(nil), upstreamCalls...)
	upstreamMu.Unlock()
	if len(calls) != 2 || calls[0].path != "/models?client_version=integration" || calls[1].path != "/responses" {
		paths := make([]string, len(calls))
		for i := range calls {
			paths[i] = calls[i].path
		}
		t.Fatalf("fake upstream call count=%d paths=%v, want exactly the two proxy-routed requests", len(calls), paths)
	}
	for _, call := range calls {
		if call.authorization != "Bearer "+oauthCredential || call.account != accountID {
			t.Fatalf("OAuth header presence at fake upstream for %s: authorization=%t account=%t",
				call.path, call.authorization != "", call.account != "")
		}
	}

	// Let the captured iteration finish normally so the runner revokes its token
	// and the daemon can shut down without a context-canceled runner error.
	if err := spawner.finish(); err != nil {
		t.Fatalf("finish captured iteration: %v", err)
	}
	iterationStore, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer iterationStore.Close()
	agentStore := agent.NewStore(iterationStore)
	finishDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(finishDeadline) {
		it, err := agentStore.GetIteration(created.Name, iterationID)
		if err == nil && it.Status != "running" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("captured Codex iteration did not finish normally")
}

func TestProviderKeysFromAgentEnvAndSecretsReachHarness(t *testing.T) {
	t.Setenv("TARIBOY_STUB_HARNESS", "/bin/true")
	for _, tc := range []struct {
		name string
		body map[string]any
		seed func(*testing.T, *client.Client, string)
		want string
	}{
		{
			name: "agent-env",
			body: map[string]any{
				"image": "basic:latest", "harness": "stub", "plugins": "context", "loop": true,
				"env": "ANTHROPIC_API_KEY=agent-env-key",
			},
			want: "agent-env-key",
		},
		{
			name: "secret-env",
			body: map[string]any{
				"image": "basic:latest", "harness": "stub", "plugins": "context", "loop": true,
			},
			seed: func(t *testing.T, c *client.Client, agent string) {
				t.Helper()
				if _, err := c.Call("POST", "/api/agents/"+agent+"/secrets", map[string]string{
					"key": "ANTHROPIC_API_KEY", "value": "secret-env-key",
				}); err != nil {
					t.Fatalf("secret set: %v", err)
				}
			},
			want: "secret-env-key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			buildBasicImage(t, filepath.Join(base, "images"))

			spawner := &captureSpawner{fired: make(chan struct{})}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				done <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", Spawner: spawner}))
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("daemon shutdown: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Error("daemon did not shut down")
				}
			})

			sock := paths.New(base).Socket()
			c := client.New(sock)
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := c.Call("GET", "/api/daemon/status", nil); err == nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}

			raw, err := c.Call("POST", "/api/agents", tc.body)
			if err != nil {
				t.Fatalf("run agent: %v", err)
			}
			var created struct {
				Name string `json:"name"`
			}
			json.Unmarshal(raw, &created)
			if created.Name == "" {
				t.Fatalf("no agent name in %s", raw)
			}
			if _, err := c.Call("POST", "/api/agents/"+created.Name+"/start", nil); err != nil {
				t.Fatalf("start agent: %v", err)
			}
			if tc.seed != nil {
				tc.seed(t, c, created.Name)
			}

			seed, err := store.Open(filepath.Join(base, "tariboyd.db"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := bus.New(seed, time.Now).Subscribe(created.Name, bus.InboxChannel(created.Name), bus.Matcher{}, nil); err != nil {
				t.Fatal(err)
			}
			seed.Close()

			if _, err := c.Call("POST", "/api/messages", map[string]any{
				"channel": bus.InboxChannel(created.Name), "type": "note", "text": "go"}); err != nil {
				t.Fatalf("publish: %v", err)
			}
			select {
			case <-spawner.fired:
			case <-time.After(5 * time.Second):
				t.Fatal("iteration never spawned after wake")
			}
			env := spawner.envMap(t)
			if got := env["ANTHROPIC_API_KEY"]; got != tc.want {
				t.Fatalf("ANTHROPIC_API_KEY = %q, want %q in harness env", got, tc.want)
			}
			if !strings.Contains(env["ANTHROPIC_BASE_URL"], "/_tariboy/sk-tariboy-") {
				t.Fatalf("ANTHROPIC_BASE_URL = %q, want tokenized proxy URL", env["ANTHROPIC_BASE_URL"])
			}
		})
	}
}
