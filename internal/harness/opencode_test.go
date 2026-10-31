package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openCodeFixture(t *testing.T, config string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	body, err := os.ReadFile("testdata/opencode-probe.py")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "opencode")
	if err = os.WriteFile(path, body, 0700); err != nil {
		t.Fatal(err)
	}
	return path, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TEST_OPENCODE_CONFIG=" + config}
}

func TestOpenCodeProxyPreservesConfigAndRoutesSelectedModels(t *testing.T) {
	t.Parallel()
	executable, env := openCodeFixture(t, `{"model":"kimi-code-plan-global/k3","small_model":"other/small","agent":{"review":{"model":"other/small"}},"skills":{"paths":["/image/skills"]}}`)
	env = append(env, `OPENCODE_CONFIG_CONTENT={"theme":"test","provider":{"kimi-code-plan-global":{"options":{"apiKey":"keep-private","baseURL":"https://original.example/v1"}}}}`, "OPENCODE_CONFIG_DIR=/image")
	got, routes, err := PrepareOpenCodeProxy(context.Background(), executable, t.TempDir(), env, "kimi-code-plan-global/k3", "http://127.0.0.1:1234/_tariboy/token")
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || routes["kimi-code-plan-global"].BaseURL != "https://original.example/v1" || routes["other"].Provider != "anthropic" {
		t.Fatalf("wrong upstream routes: %+v", routes)
	}
	var cfg map[string]any
	for _, kv := range got {
		if strings.HasPrefix(kv, "OPENCODE_CONFIG_CONTENT=") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(kv, "OPENCODE_CONFIG_CONTENT=")), &cfg); err != nil {
				t.Fatal(err)
			}
		}
	}
	provider := cfg["provider"].(map[string]any)["kimi-code-plan-global"].(map[string]any)
	opts := provider["options"].(map[string]any)
	if opts["apiKey"] != "keep-private" || opts["baseURL"] != "http://127.0.0.1:1234/_tariboy/token/_opencode/kimi-code-plan-global" || cfg["theme"] != "test" {
		t.Fatal("provider credentials, routing or user settings lost")
	}
	if !strings.Contains(strings.Join(got, "\n"), "OPENCODE_CONFIG_DIR=/image") {
		t.Fatal("image skills lost")
	}
}

func TestOpenCodeProxyFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, config, inline, model string
		ignore                      bool
	}{
		{name: "missing model", config: `{}`},
		{name: "unresolved model", config: `{}`, model: "missing/model"},
		{name: "external plugin", config: `{"model":"kimi-code-plan-global/k3","plugin":["unsafe-routing"]}`},
		{name: "inline JSONC", config: `{"model":"kimi-code-plan-global/k3"}`, inline: "{//comment\n}"},
		{name: "model transport override", config: `{"model":"kimi-code-plan-global/k3","provider":{"kimi-code-plan-global":{"models":{"k3":{"options":{"baseURL":"https://escape.example"}}}}}}`},
		{name: "overlay ignored", config: `{"model":"kimi-code-plan-global/k3"}`, ignore: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executable, env := openCodeFixture(t, tc.config)
			if tc.inline != "" {
				env = append(env, "OPENCODE_CONFIG_CONTENT="+tc.inline)
			}
			if tc.ignore {
				env = append(env, "TEST_IGNORE_OVERLAY=1")
			}
			_, _, err := PrepareOpenCodeProxy(context.Background(), executable, t.TempDir(), env, tc.model, "http://127.0.0.1:1234/_tariboy/token")
			if err == nil {
				t.Fatal("unsafe configuration accepted")
			}
			if strings.Contains(err.Error(), "escape.example") || strings.Contains(err.Error(), "unsafe-routing") {
				t.Fatal("configuration leaked in error")
			}
		})
	}
}

func TestOpenCodeProxyRejectsInlineOAuth(t *testing.T) {
	executable, env := openCodeFixture(t, `{"model":"kimi-code-plan-global/k3"}`)
	env = append(env, `OPENCODE_AUTH_CONTENT={"kimi-code-plan-global":{"type":"oauth","access":"private-access","refresh":"private-refresh"}}`)
	_, _, err := PrepareOpenCodeProxy(context.Background(), executable, t.TempDir(), env, "", "http://127.0.0.1:1234/_tariboy/token")
	if err == nil {
		t.Fatal("inline OAuth can bypass SDK baseURL")
	}
	if strings.Contains(err.Error(), "private-") {
		t.Fatal("OAuth data leaked")
	}
}

func TestOpenCodeProbeBoundsOutput(t *testing.T) {
	t.Parallel()
	_, err := openCodeProbe(context.Background(), "/bin/sh", t.TempDir(), []string{"PATH=/usr/bin:/bin"}, "-c", "head -c 5000000 /dev/zero")
	if err == nil {
		t.Fatal("unbounded discovery output accepted")
	}
}
func TestOpenCodeProbeCancellationAndFailureDoNotExposeOutput(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, runCtx := range []context.Context{ctx, context.Background()} {
		_, err := openCodeProbe(runCtx, "/bin/sh", t.TempDir(), []string{"PATH=/usr/bin:/bin"}, "-c", "echo private-provider-key >&2; exit 7")
		if err == nil || strings.Contains(err.Error(), "private-provider-key") {
			t.Fatalf("unsafe probe error: %v", err)
		}
	}
}

func TestOpenCodeRoutesResolveSupportedTransports(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id, sdk, url, env, want, family string
		fail                            bool
	}{
		{id: "openai", sdk: "@ai-sdk/openai", env: "OPENAI_BASE_URL=https://custom.example/v1", want: "https://custom.example/v1", family: "openai"},
		{id: "anthropic", sdk: "@ai-sdk/anthropic", want: "https://api.anthropic.com/v1", family: "anthropic"},
		{id: "custom", sdk: "@ai-sdk/openai-compatible", url: "https://kimi.example/coding/v1", want: "https://kimi.example/coding/v1", family: "openai"},
		{id: "unknown", sdk: "@ai-sdk/amazon-bedrock", fail: true},
		{id: "custom", sdk: "@ai-sdk/openai-compatible", fail: true},
	} {
		t.Run(tc.id+tc.sdk+tc.url, func(t *testing.T) {
			m := openCodeModelMetadata{ID: "model", ProviderID: tc.id}
			m.API.NPM = tc.sdk
			m.API.URL = tc.url
			routes, err := openCodeRoutes(nil, map[string][]string{tc.id: {"model"}}, map[string]openCodeModelMetadata{tc.id + "/model": m}, []string{"HOME=" + t.TempDir(), tc.env})
			if tc.fail {
				if err == nil {
					t.Fatal("unsupported transport accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if routes[tc.id].BaseURL != tc.want || routes[tc.id].Provider != tc.family {
				t.Fatalf("wrong route: %+v", routes)
			}
		})
	}
}
func TestOpenCodeAuthFilesStayUnchanged(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "opencode")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"selected":{"type":"api","key":"private-key"},"other":{"type":"oauth","access":"private-access"}}`)
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	env := []string{"XDG_DATA_HOME=" + root}
	if err := openCodeCheckAuth("selected", env); err != nil {
		t.Fatal(err)
	}
	if err := openCodeCheckAuth("other", env); err == nil {
		t.Fatal("OAuth file source accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(body) {
		t.Fatal("authentication source was changed")
	}
}

func TestOpenCodeProxyVerifiesFinalModelSelections(t *testing.T) {
	executable, env := openCodeFixture(t, `{"model":"kimi-code-plan-global/k3"}`)
	env = append(env, "TEST_CHANGE_FINAL_MODEL=1")
	_, _, err := PrepareOpenCodeProxy(context.Background(), executable, t.TempDir(), env, "", "http://127.0.0.1:1234/_tariboy/token")
	if err == nil {
		t.Fatal("changed final auxiliary model accepted without its route")
	}
}

func TestOpenCodeAuthRequiresUnambiguousDataDirectory(t *testing.T) {
	t.Parallel()
	if err := openCodeCheckAuth("openai", []string{"HOME="}); err == nil {
		t.Fatal("missing HOME could select unchecked account credentials")
	}
}

func TestOpenCodeAnthropicOfficialRootNormalization(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, id, sdk, source, base, want string }{
		{"config root", "anthropic", "@ai-sdk/anthropic", "config", "https://api.anthropic.com", "https://api.anthropic.com/v1"},
		{"config trailing slash", "anthropic", "@ai-sdk/anthropic", "config", "https://api.anthropic.com/", "https://api.anthropic.com/v1"},
		{"environment root", "anthropic", "@ai-sdk/anthropic", "env", "https://api.anthropic.com", "https://api.anthropic.com/v1"},
		{"environment trailing slash", "anthropic", "@ai-sdk/anthropic", "env", "https://api.anthropic.com/", "https://api.anthropic.com/v1"},
		{"already versioned", "anthropic", "@ai-sdk/anthropic", "config", "https://api.anthropic.com/v1", "https://api.anthropic.com/v1"},
		{"custom SDK provider", "custom", "@ai-sdk/anthropic", "config", "https://api.anthropic.com", "https://api.anthropic.com/v1"},
		{"custom root", "anthropic", "@ai-sdk/anthropic", "config", "https://custom.example", "https://custom.example"},
		{"custom path", "anthropic", "@ai-sdk/anthropic", "env", "https://custom.example/coding/v2", "https://custom.example/coding/v2"},
		{"official custom path", "anthropic", "@ai-sdk/anthropic", "config", "https://api.anthropic.com/custom", "https://api.anthropic.com/custom"},
		{"other SDK unchanged", "custom", "@ai-sdk/openai-compatible", "config", "https://api.anthropic.com", "https://api.anthropic.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := map[string]any{}
			env := []string{"HOME=" + t.TempDir()}
			if tc.source == "config" {
				cfg["provider"] = map[string]any{tc.id: map[string]any{"options": map[string]any{"baseURL": tc.base}}}
			} else {
				env = append(env, "ANTHROPIC_BASE_URL="+tc.base)
			}
			model := openCodeModelMetadata{ID: "model", ProviderID: tc.id}
			model.API.NPM = tc.sdk
			routes, err := openCodeRoutes(cfg, map[string][]string{tc.id: {"model"}}, map[string]openCodeModelMetadata{tc.id + "/model": model}, env)
			if err != nil {
				t.Fatal(err)
			}
			if got := routes[tc.id].BaseURL; got != tc.want {
				t.Fatalf("baseURL=%q, want %q", got, tc.want)
			}
		})
	}
}
