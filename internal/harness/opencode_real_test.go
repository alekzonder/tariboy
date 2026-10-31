//go:build tmux

package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
)

func TestOpenCodeRealProxy(t *testing.T) {
	executable := os.Getenv("TARIBOY_TEST_OPENCODE_BIN")
	if executable == "" {
		t.Skip("set TARIBOY_TEST_OPENCODE_BIN to run installed CLI against fake local upstream")
	}
	shimBin := filepath.Join(t.TempDir(), "tariboy-shim")
	build := exec.Command("go", "build", "-o", shimBin, "../../cmd/tariboy-shim")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build isolated shim: %v: %s", err, out)
	}
	for _, interactive := range []bool{false, true} {
		t.Run(fmt.Sprint(interactive), func(t *testing.T) {
			root := t.TempDir()
			work := filepath.Join(root, "work")
			if err := os.MkdirAll(work, 0700); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var models []string
			var rows []aiproxy.AIRequest
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				model, _ := req["model"].(string)
				mu.Lock()
				models = append(models, model)
				mu.Unlock()
				if r.URL.Path != "/coding/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fake-local-key" {
					t.Errorf("upstream path/auth mismatch: %q", r.URL.Path)
				}
				if options, _ := req["stream_options"].(map[string]any); options["include_usage"] != true {
					t.Error("stream usage not requested")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Local answer\"},\"finish_reason\":null}]}\n\n", model)
				fmt.Fprintf(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n", model)
			}))
			defer up.Close()
			tokens := aiproxy.NewTokenRegistry(nil)
			p := aiproxy.New(aiproxy.Config{Tokens: tokens, Router: aiproxy.NewRouter(), Pricing: aiproxy.NewPricing(nil, nil, nil), AgentsDir: filepath.Join(root, "agents"), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Ingest: func(row aiproxy.AIRequest) { mu.Lock(); defer mu.Unlock(); rows = append(rows, row) }})
			server := httptest.NewServer(p)
			defer server.Close()
			token, err := p.Mint(aiproxy.Attribution{Agent: "test", Iteration: "test-1"})
			if err != nil {
				t.Fatal(err)
			}
			config := `{"model":"kimi-code-plan-global/k3","small_model":"kimi-code-plan-global/k3-256k","provider":{"kimi-code-plan-global":{"options":{"baseURL":"${TARIBOY_TEST_UPSTREAM}/coding/v1","apiKey":"fake-local-key"}}},"permission":"deny","autoupdate":false}`
			bridge := filepath.Join(root, "bridge")
			if err := os.MkdirAll(filepath.Join(bridge, "skills"), 0700); err != nil {
				t.Fatal(err)
			}
			skillConfig, _ := json.Marshal(map[string]any{"skills": map[string]any{"paths": []string{filepath.Join(bridge, "skills")}}})
			if err := os.WriteFile(filepath.Join(bridge, "opencode.json"), skillConfig, 0600); err != nil {
				t.Fatal(err)
			}
			env := []string{"PATH=" + filepath.Dir(executable) + ":" + os.Getenv("PATH"), "HOME=" + root, "XDG_DATA_HOME=" + filepath.Join(root, "data"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_STATE_HOME=" + filepath.Join(root, "state"), "TARIBOY_BASE_DIR=" + filepath.Join(root, "base"), "TARIBOY_RUNTIME_DIR=" + filepath.Join(root, "runtime"), "TARIBOY_TEST_UPSTREAM=" + up.URL, "OPENCODE_DISABLE_MODELS_FETCH=true", "OPENCODE_CONFIG_CONTENT=" + config, "OPENCODE_CONFIG_DIR=" + bridge}
			if interactive {
				configPath := filepath.Join(root, "user.jsonc")
				if err := os.WriteFile(configPath, []byte("// native JSONC credential source\n"+config), 0600); err != nil {
					t.Fatal(err)
				}
				env = replaceOpenCodeEnv(env, "OPENCODE_CONFIG_CONTENT", `{}`)
				env = append(env, "OPENCODE_CONFIG="+configPath)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			env, routes, err := PrepareOpenCodeProxy(ctx, executable, work, env, "kimi-code-plan-global/k3", server.URL+"/_tariboy/"+token)
			if err != nil {
				t.Fatal(err)
			}
			if err = p.RegisterProviderRoutes(token, routes); err != nil {
				t.Fatal(err)
			}
			cfg, err := openCodeConfig(ctx, executable, work, env)
			if err != nil {
				t.Fatal(err)
			}
			paths, _ := readObject(cfg, "skills")["paths"].([]any)
			if len(paths) != 1 || paths[0] != filepath.Join(bridge, "skills") {
				t.Fatal("native skills configuration lost")
			}
			prompt := filepath.Join(root, "prompt")
			if err := os.WriteFile(prompt, []byte("Say Local answer. Do not use tools."), 0600); err != nil {
				t.Fatal(err)
			}
			argv, _, err := opencode{}.Command(work, prompt, Config{Model: "kimi-code-plan-global/k3", Interactive: interactive})
			if err != nil {
				t.Fatal(err)
			}
			iterationDir := filepath.Join(root, "agents", "test", "iterations", "test-1")
			if err := os.MkdirAll(filepath.Join(iterationDir, "logs"), 0700); err != nil {
				t.Fatal(err)
			}
			tmuxDir := filepath.Join(root, "tmux")
			if err := os.MkdirAll(tmuxDir, 0700); err != nil {
				t.Fatal(err)
			}
			env = append(env, "TMUX_TMPDIR="+tmuxDir)
			session := ""
			if interactive {
				session = "opencode-local-test"
			}
			args := []string{"--iteration-dir", iterationDir, "--agent", "test", "--iteration-id", "test-1", "--hard-timeout-s", "30", "--tmux-session", session, "--shim-sock", filepath.Join(root, "shim.sock"), "--"}
			args = append(args, argv...)
			cmd := exec.CommandContext(ctx, shimBin, args...)
			cmd.Env, cmd.Dir = env, work
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("isolated shim failed: %v: %.3000s", err, out)
			}
			resultBody, err := os.ReadFile(filepath.Join(iterationDir, "result.json"))
			var result struct {
				ExitCode int `json:"exit_code"`
			}
			if err != nil || json.Unmarshal(resultBody, &result) != nil || result.ExitCode != 0 {
				log, _ := os.ReadFile(filepath.Join(iterationDir, "logs", "shim.log"))
				t.Fatalf("isolated OpenCode result: %s %v: %.3000s", resultBody, err, log)
			}
			out, err = os.ReadFile(filepath.Join(iterationDir, "logs", "harness.stdout.log"))
			if err != nil || !strings.Contains(string(out), "Local answer") {
				t.Fatalf("missing local response: %v: %.3000s", err, out)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(models) == 0 || len(rows) != len(models) {
				t.Fatalf("upstream/proxy count %d/%d", len(models), len(rows))
			}
			seen := map[string]bool{}
			for _, row := range rows {
				seen[row.Model] = true
				if row.Agent != "test" || row.Iteration != "test-1" || (row.Model != "k3" && row.Model != "k3-256k") || row.InputTokens != 10 || row.OutputTokens != 2 || row.Status != "ok" {
					t.Fatalf("bad recorded usage: %+v", row)
				}
			}
			if !seen["k3"] || !seen["k3-256k"] {
				t.Fatalf("main/small model traffic missing: %v", seen)
			}
			t.Logf("OpenCode interactive=%v: %d upstream calls, %d attributed proxy usage rows", interactive, len(models), len(rows))
		})
	}
}
