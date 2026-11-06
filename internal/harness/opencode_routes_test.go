package harness

import (
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/aiproxy"
)

func openCodeTestModel(provider, name, sdk, url string) openCodeModelMetadata {
	m := openCodeModelMetadata{ID: name, ProviderID: provider}
	m.API.NPM = sdk
	m.API.URL = url
	return m
}

// TestOpenCodeRoutesBaseURLResolution pins the precedence of the upstream URL
// sources, variable expansion and the normalization of the Anthropic root.
func TestOpenCodeRoutesBaseURLResolution(t *testing.T) {
	t.Parallel()
	options := func(base string) map[string]any {
		return map[string]any{"provider": map[string]any{"p": map[string]any{"options": map[string]any{"baseURL": base}}}}
	}
	for _, tc := range []struct {
		name         string
		id, sdk, url string
		cfg          map[string]any
		env          []string
		want         string
		wantProvider string
		wantErr      string
	}{
		{name: "OpenAI default", id: "openai", sdk: "@ai-sdk/openai", want: "https://api.openai.com/v1", wantProvider: "openai"},
		{name: "OpenAI environment", id: "openai", sdk: "@ai-sdk/openai", env: []string{"OPENAI_BASE_URL=https://o.example/v1"}, want: "https://o.example/v1", wantProvider: "openai"},
		{name: "metadata beats OpenAI environment", id: "openai", sdk: "@ai-sdk/openai", url: "https://m.example", env: []string{"OPENAI_BASE_URL=https://o.example/v1"}, want: "https://m.example", wantProvider: "openai"},
		{name: "Anthropic environment", id: "anthropic", sdk: "@ai-sdk/anthropic", env: []string{"ANTHROPIC_BASE_URL=https://a.example/v1"}, want: "https://a.example/v1", wantProvider: "anthropic"},
		{name: "Anthropic root gets version", id: "anthropic", sdk: "@ai-sdk/anthropic", url: "https://api.anthropic.com/", want: "https://api.anthropic.com/v1", wantProvider: "anthropic"},
		{name: "Anthropic root normalized after expansion", id: "p", sdk: "@ai-sdk/anthropic", url: "${ROOT}", env: []string{"ROOT=https://api.anthropic.com"}, want: "https://api.anthropic.com/v1", wantProvider: "anthropic"},
		{name: "OpenAI family keeps the Anthropic root", id: "p", sdk: "@ai-sdk/openai", url: "https://api.anthropic.com", want: "https://api.anthropic.com", wantProvider: "openai"},
		{name: "configured URL beats metadata", id: "p", sdk: "@ai-sdk/openai-compatible", url: "https://m.example", cfg: options("https://c.example"), want: "https://c.example", wantProvider: "openai"},
		{name: "variable expands", id: "p", sdk: "@ai-sdk/openai", url: "https://${HOSTNAME_X}/v1", env: []string{"HOSTNAME_X=h.example"}, want: "https://h.example/v1", wantProvider: "openai"},
		{name: "empty variable expands", id: "p", sdk: "@ai-sdk/openai", url: "https://h.example${SUFFIX}", env: []string{"SUFFIX="}, want: "https://h.example", wantProvider: "openai"},
		{name: "unset variable fails", id: "p", sdk: "@ai-sdk/openai", url: "https://${MISSING}/v1", wantErr: "OpenCode upstream URL contains unresolved environment variables"},
		{name: "OpenAI id with a compatible SDK has no default", id: "openai", sdk: "@ai-sdk/openai-compatible", wantErr: "OpenCode selected provider has no resolved upstream URL"},
		{name: "Anthropic id with an OpenAI SDK has no default", id: "anthropic", sdk: "@ai-sdk/openai", env: []string{"ANTHROPIC_BASE_URL=https://a.example"}, wantErr: "OpenCode selected provider has no resolved upstream URL"},
		{name: "unsupported SDK is reported before the URL", id: "p", sdk: "@ai-sdk/other", wantErr: "OpenCode selected provider transport is unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := append([]string{"HOME=" + t.TempDir()}, tc.env...)
			metadata := map[string]openCodeModelMetadata{tc.id + "/model": openCodeTestModel(tc.id, "model", tc.sdk, tc.url)}
			routes, err := openCodeRoutes(tc.cfg, map[string][]string{tc.id: {"model"}}, metadata, env)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				if routes != nil {
					t.Fatalf("routes = %+v alongside error", routes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := aiproxy.ProviderRoute{BaseURL: tc.want, Provider: tc.wantProvider}
			if routes[tc.id] != want {
				t.Fatalf("route = %+v, want %+v", routes[tc.id], want)
			}
		})
	}
}

func TestOpenCodeRoutesCheckModelsInOrder(t *testing.T) {
	t.Parallel()
	const sdk = "@ai-sdk/openai-compatible"
	metadata := func(url string, models ...string) map[string]openCodeModelMetadata {
		out := map[string]openCodeModelMetadata{}
		for _, name := range models {
			out["p/"+name] = openCodeTestModel("p", name, sdk, url)
		}
		return out
	}
	configured := func(model map[string]any) map[string]any {
		return map[string]any{"provider": map[string]any{"p": map[string]any{"models": map[string]any{"a": model}}}}
	}
	override := map[string]any{"baseURL": "https://override.example"}
	metadataOverride := metadata("", "a")
	m := metadataOverride["p/a"]
	m.Options = override
	metadataOverride["p/a"] = m
	unsupportedOverride := metadataOverride["p/a"]
	unsupportedOverride.API.NPM = "@ai-sdk/other"
	for _, tc := range []struct {
		name     string
		cfg      map[string]any
		metadata map[string]openCodeModelMetadata
		models   []string
		env      []string
		wantErr  string
	}{
		{name: "unavailable model", metadata: metadata("https://u.example"), models: []string{"a"}, wantErr: "OpenCode selected model is unavailable"},
		{name: "unavailable model before its overrides", cfg: configured(map[string]any{"options": override}), metadata: metadata("https://u.example"), models: []string{"a"}, wantErr: "OpenCode selected model is unavailable"},
		{name: "metadata override", metadata: metadataOverride, models: []string{"a"}, wantErr: "OpenCode model transport overrides are unsupported"},
		{name: "override before SDK", metadata: map[string]openCodeModelMetadata{"p/a": unsupportedOverride}, models: []string{"a"}, wantErr: "OpenCode model transport overrides are unsupported"},
		{name: "configured option override", cfg: configured(map[string]any{"options": override}), metadata: metadata("https://u.example", "a"), models: []string{"a"}, wantErr: "OpenCode model transport overrides are unsupported"},
		{name: "configured variant override", cfg: configured(map[string]any{"variants": map[string]any{"fast": override}}), metadata: metadata("https://u.example", "a"), models: []string{"a"}, wantErr: "OpenCode model transport overrides are unsupported"},
		{name: "credential check runs first", metadata: metadata("https://u.example"), models: []string{"a"}, env: []string{"OPENCODE_AUTH_CONTENT=not json"}, wantErr: "OpenCode"},
		{name: "same transport twice", metadata: metadata("https://u.example", "a", "b"), models: []string{"a", "b"}},
		{
			name: "incompatible transports",
			metadata: map[string]openCodeModelMetadata{
				"p/a": openCodeTestModel("p", "a", sdk, "https://a.example"),
				"p/b": openCodeTestModel("p", "b", sdk, "https://b.example"),
			},
			models: []string{"a", "b"}, wantErr: "OpenCode provider uses incompatible model transports",
		},
		{
			name: "incompatible families",
			metadata: map[string]openCodeModelMetadata{
				"p/a": openCodeTestModel("p", "a", sdk, "https://a.example"),
				"p/b": openCodeTestModel("p", "b", "@ai-sdk/anthropic", "https://a.example"),
			},
			models: []string{"a", "b"}, wantErr: "OpenCode provider uses incompatible model transports",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := append([]string{"HOME=" + t.TempDir()}, tc.env...)
			routes, err := openCodeRoutes(tc.cfg, map[string][]string{"p": tc.models}, tc.metadata, env)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if want := (aiproxy.ProviderRoute{BaseURL: "https://u.example", Provider: "openai"}); routes["p"] != want {
					t.Fatalf("route = %+v, want %+v", routes["p"], want)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if routes != nil {
				t.Fatalf("routes = %+v alongside error", routes)
			}
		})
	}
}
