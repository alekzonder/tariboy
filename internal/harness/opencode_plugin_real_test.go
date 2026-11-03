package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
)

// openCodeToolUpstream is a fake OpenAI-compatible model. A request with
// tools and no tool result asks for one bash call; every other request gets
// text. It records each tool result the model receives and every path.
type openCodeToolUpstream struct {
	mu          sync.Mutex
	paths       []string
	toolResults []string
}

func (u *openCodeToolUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string `json:"model"`
		Tools    []any  `json:"tools"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		return
	}
	toolResult := ""
	for _, m := range req.Messages {
		if m.Role == "tool" {
			var s string
			if json.Unmarshal(m.Content, &s) != nil {
				s = string(m.Content)
			}
			toolResult = s
		}
	}
	u.mu.Lock()
	u.paths = append(u.paths, r.URL.Path)
	if toolResult != "" {
		u.toolResults = append(u.toolResults, toolResult)
	}
	u.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	chunk := func(delta any, finish any) {
		body, _ := json.Marshal(map[string]any{"id": "t", "object": "chat.completion.chunk", "created": 1, "model": req.Model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 2}})
		fmt.Fprintf(w, "data: %s\n\n", body)
	}
	if len(req.Tools) > 0 && toolResult == "" {
		args, _ := json.Marshal(map[string]string{"command": "echo tool-ran", "description": "Print a marker"})
		chunk(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call_1", "type": "function",
			"function": map[string]string{"name": "bash", "arguments": string(args)}}}}, nil)
		chunk(map[string]any{}, "tool_calls")
	} else {
		chunk(map[string]any{"role": "assistant", "content": "Finished"}, nil)
		chunk(map[string]any{}, "stop")
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

// TestOpenCodeInstantPluginReal runs the generated plugin in the installed
// OpenCode against a fake model and a fake tmail. It runs only with
// TARIBOY_TEST_OPENCODE_BIN; HOME and every XDG directory are temporary.
// The first run may wait for OpenCode to install @opencode-ai/plugin into the
// temporary configuration directory.
func TestOpenCodeInstantPluginReal(t *testing.T) {
	executable := os.Getenv("TARIBOY_TEST_OPENCODE_BIN")
	if executable == "" {
		t.Skip("set TARIBOY_TEST_OPENCODE_BIN to run the instant mail plugin in the installed OpenCode")
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	pluginDir := filepath.Join(root, "agent", "harness-hooks", "opencode")
	tmail := filepath.Join(root, "agent", "bin", "tmail")
	flag := filepath.Join(root, "agent", "instant-mail")
	calls := filepath.Join(root, "tmail-calls")
	for _, dir := range []string{work, filepath.Dir(tmail)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	hooks, err := opencode{}.InstantMailHooks(tmail, pluginDir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentdir.WritePluginFiles(pluginDir, hooks.Files); err != nil {
		t.Fatal(err)
	}
	up := &openCodeToolUpstream{}
	server := httptest.NewServer(up)
	defer server.Close()
	config := `{"model":"kimi-code-plan-global/k3","small_model":"kimi-code-plan-global/k3-256k","provider":{"kimi-code-plan-global":{"options":{"baseURL":"${TARIBOY_TEST_UPSTREAM}/coding/v1","apiKey":"fake-local-key"}}},"permission":"allow","autoupdate":false}`
	env := []string{"PATH=" + filepath.Dir(executable) + ":/usr/bin:/bin", "HOME=" + root,
		"XDG_DATA_HOME=" + filepath.Join(root, "data"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"TARIBOY_TEST_UPSTREAM=" + server.URL, "OPENCODE_DISABLE_MODELS_FETCH=true", "OPENCODE_CONFIG_CONTENT=" + config,
		"TARIBOY_INSTANT_MAIL_FLAG=" + flag, "TARIBOY_TOOLS_SOCKET=" + filepath.Join(root, "agent.sock")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	env, _, err = PrepareOpenCodeProxy(ctx, executable, work, env, "kimi-code-plan-global/k3", server.URL+"/proxy", hooks.ConfigPlugin)
	if err != nil {
		t.Fatal(err)
	}
	// The final environment loads only the Tariboy plugin and routes the
	// provider through the proxy overlay. Not openCodeConfig: with a plugin,
	// OpenCode first installs @opencode-ai/plugin, which can outlast its 20 s.
	debug := exec.CommandContext(ctx, executable, "debug", "config")
	debug.Env, debug.Dir = env, work
	start := time.Now()
	out, err := debug.Output()
	var cfg map[string]any
	if err != nil || json.Unmarshal(out, &cfg) != nil {
		t.Fatalf("debug config: %v: %.2000s", err, out)
	}
	t.Logf("debug config with the plugin took %s", time.Since(start).Round(time.Millisecond))
	origins, _ := json.Marshal(cfg["plugin_origins"])
	baseURL := readObject(readObject(readObject(cfg, "provider"), "kimi-code-plan-global"), "options")["baseURL"]
	t.Logf("debug config: plugin=%v plugin_origins=%s baseURL=%v", cfg["plugin"], origins, baseURL)
	var gotOrigins []struct{ Spec, Source string }
	if json.Unmarshal(origins, &gotOrigins) != nil || len(gotOrigins) != 1 || gotOrigins[0].Spec != hooks.ConfigPlugin ||
		gotOrigins[0].Source != "OPENCODE_CONFIG_CONTENT" || baseURL != server.URL+"/proxy/_opencode/kimi-code-plan-global" {
		t.Fatalf("final config: plugin_origins %s, baseURL %v", origins, baseURL)
	}
	prompt := filepath.Join(root, "prompt")
	if err := os.WriteFile(prompt, []byte("Run echo tool-ran with bash."), 0o600); err != nil {
		t.Fatal(err)
	}
	argv, _, err := opencode{}.Command(work, prompt, Config{Model: "kimi-code-plan-global/k3"})
	if err != nil {
		t.Fatal(err)
	}
	const letter = "\n\n# Instant messages\n- id m-1 Hello from instant mail\n"
	record := `echo "$@" >> '` + calls + `'` + "\n"
	for _, tc := range []struct {
		name, script string
		flag, spawn  bool
		want         string
	}{
		{name: "letter", script: record + "printf '" + strings.ReplaceAll(letter, "\n", `\n`) + "'\n", flag: true, spawn: true, want: "tool-ran\n" + letter},
		{name: "no flag", script: record + "printf 'never'\n", flag: false, spawn: false, want: "tool-ran\n"},
		{name: "empty", script: record, flag: true, spawn: true, want: "tool-ran\n"},
		{name: "failure", script: record + "printf 'partial'; echo private-noise >&2; exit 3\n", flag: true, spawn: true, want: "tool-ran\n"},
		{name: "stderr", script: record + "echo private-noise >&2\n", flag: true, spawn: true, want: "tool-ran\n"},
		{name: "hang", script: record + "printf 'late'; exec sleep 30\n", flag: true, spawn: true, want: "tool-ran\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(tmail, []byte("#!/bin/sh\n"+tc.script), 0o700); err != nil {
				t.Fatal(err)
			}
			_ = os.Remove(calls)
			_ = os.Remove(flag)
			if tc.flag {
				if err := os.WriteFile(flag, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			up.mu.Lock()
			up.toolResults = nil
			up.mu.Unlock()
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			cmd.Env, cmd.Dir = env, work
			start := time.Now()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("opencode run: %v: %.3000s", err, out)
			}
			elapsed := time.Since(start)
			up.mu.Lock()
			results := up.toolResults
			up.mu.Unlock()
			if len(results) == 0 || results[0] != tc.want {
				t.Fatalf("tool results = %q; want first %q; output: %.3000s", results, tc.want, out)
			}
			got, _ := os.ReadFile(calls)
			if spawned := len(got) > 0; spawned != tc.spawn || (tc.spawn && string(got) != "instant --format opencode-tool-after\n") {
				t.Fatalf("tmail calls = %q; want spawned %v", got, tc.spawn)
			}
			if strings.Contains(string(out), "private-noise") || strings.Contains(string(out), "partial") || strings.Contains(string(out), "late") {
				t.Fatalf("plugin output leaked into opencode output: %.3000s", out)
			}
			t.Logf("%s: opencode run took %s", tc.name, elapsed.Round(time.Millisecond))
		})
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	for _, p := range up.paths {
		if !strings.HasPrefix(p, "/proxy/_opencode/kimi-code-plan-global/") {
			t.Fatalf("model request bypassed the proxy overlay: %s", p)
		}
	}
	t.Logf("%d model requests, all through the proxy overlay", len(up.paths))
}
