package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
)

const openCodeOutputLimit = 4 << 20

// PrepareOpenCodeProxy keeps OpenCode's own credential sources and overlays only
// iteration routing. Resolved debug output is never copied: it masks API keys.
func PrepareOpenCodeProxy(ctx context.Context, executable, cwd string, env []string, model, proxyURL string) ([]string, map[string]aiproxy.ProviderRoute, error) {
	if proxyURL == "" {
		return nil, nil, errors.New("OpenCode requires an iteration proxy")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cfg, err := openCodeConfig(ctx, executable, cwd, env)
	if err != nil {
		return nil, nil, err
	}
	selected, err := openCodeModels(cfg, model)
	if err != nil {
		return nil, nil, err
	}
	metadata, err := openCodeMetadata(ctx, executable, cwd, env)
	if err != nil {
		return nil, nil, err
	}
	routes, err := openCodeRoutes(cfg, selected, metadata, env)
	if err != nil {
		return nil, nil, err
	}
	inline := map[string]any{}
	if raw := openCodeEnv(env, "OPENCODE_CONFIG_CONTENT"); raw != "" {
		if json.Unmarshal([]byte(raw), &inline) != nil || inline == nil {
			return nil, nil, errors.New("OpenCode inline configuration must be a JSON object")
		}
	}
	providers := object(inline, "provider")
	ids := make([]string, 0, len(routes))
	for id := range routes {
		ids = append(ids, id)
		p := object(providers, id)
		object(p, "options")["baseURL"] = proxyURL + "/_opencode/" + id
		p["whitelist"] = selected[id]
	}
	sort.Strings(ids)
	inline["enabled_providers"] = ids
	// A deterministic small model avoids OpenCode choosing an unprepared provider.
	if model == "" {
		model, _ = cfg["model"].(string)
	}
	inline["model"] = model
	if small, _ := cfg["small_model"].(string); small == "" {
		inline["small_model"] = model
	}
	body, err := json.Marshal(inline)
	if err != nil || len(body) >= maxArgStrLen-64 {
		return nil, nil, errors.New("OpenCode inline configuration is too large")
	}
	env = replaceOpenCodeEnv(env, "OPENCODE_CONFIG_CONTENT", string(body))
	final, err := openCodeConfig(ctx, executable, cwd, env)
	if err != nil {
		return nil, nil, err
	}
	finalModels, selectionErr := openCodeModels(final, model)
	if selectionErr != nil || !maps.EqualFunc(selected, finalModels, slices.Equal[[]string]) || !openCodeRoutesApplied(final, ids, selected, proxyURL) {
		return nil, nil, errors.New("OpenCode did not apply iteration proxy configuration")
	}
	return env, routes, nil
}

func openCodeEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], key+"="); ok {
			return v
		}
	}
	return ""
}
func replaceOpenCodeEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, v := range env {
		if !strings.HasPrefix(v, key+"=") {
			out = append(out, v)
		}
	}
	return append(out, key+"="+value)
}
func object(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	if v == nil {
		v = map[string]any{}
		m[key] = v
	}
	return v
}
func readObject(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

type openCodeOutput struct {
	buf      bytes.Buffer
	overflow bool
}

func (b *openCodeOutput) Write(p []byte) (int, error) {
	if len(p) > openCodeOutputLimit-b.buf.Len() {
		b.overflow = true
		return 0, errors.New("output limit")
	}
	return b.buf.Write(p)
}
func openCodeProbe(ctx context.Context, executable, cwd string, env []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env, cmd.Dir = env, cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	out := &openCodeOutput{}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil || out.overflow {
		return nil, errors.New("OpenCode configuration probe failed or exceeded its limit")
	}
	return out.buf.Bytes(), nil
}
func openCodeConfig(ctx context.Context, executable, cwd string, env []string) (map[string]any, error) {
	out, err := openCodeProbe(ctx, executable, cwd, env, "debug", "config")
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if json.Unmarshal(out, &cfg) != nil || cfg == nil {
		return nil, errors.New("OpenCode configuration probe returned invalid JSON")
	}
	return cfg, nil
}

func openCodeModels(cfg map[string]any, model string) (map[string][]string, error) {
	if plugins, _ := cfg["plugin"].([]any); len(plugins) > 0 {
		return nil, errors.New("OpenCode external plugins are not supported by iteration proxy routing")
	}
	if model == "" {
		model, _ = cfg["model"].(string)
	}
	models := []string{model}
	if small, _ := cfg["small_model"].(string); small != "" {
		models = append(models, small)
	}
	for _, field := range []string{"agent", "mode"} {
		for _, v := range readObject(cfg, field) {
			agent, _ := v.(map[string]any)
			if m, _ := agent["model"].(string); m != "" {
				models = append(models, m)
			}
		}
	}
	selected := map[string][]string{}
	for _, m := range models {
		id, name, ok := strings.Cut(m, "/")
		if !ok || name == "" || id == "" || strings.Trim(id, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
			return nil, errors.New("OpenCode requires explicit provider/model selections")
		}
		if !slices.Contains(selected[id], name) {
			selected[id] = append(selected[id], name)
		}
	}
	for _, models := range selected {
		sort.Strings(models)
	}
	return selected, nil
}

type openCodeModelMetadata struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	API        struct {
		ID  string `json:"id"`
		URL string `json:"url"`
		NPM string `json:"npm"`
	} `json:"api"`
	Options map[string]any `json:"options"`
}

func openCodeMetadata(ctx context.Context, executable, cwd string, env []string) (map[string]openCodeModelMetadata, error) {
	out, err := openCodeProbe(ctx, executable, cwd, env, "models", "--verbose")
	if err != nil {
		return nil, err
	}
	result := map[string]openCodeModelMetadata{}
	for len(bytes.TrimSpace(out)) > 0 {
		line, rest, ok := bytes.Cut(bytes.TrimSpace(out), []byte("\n"))
		if !ok {
			return nil, errors.New("OpenCode model metadata is unsupported")
		}
		dec := json.NewDecoder(bytes.NewReader(rest))
		var m openCodeModelMetadata
		if dec.Decode(&m) != nil || string(line) != m.ProviderID+"/"+m.ID {
			return nil, errors.New("OpenCode model metadata is unsupported")
		}
		result[string(line)] = m
		out = rest[dec.InputOffset():]
	}
	return result, nil
}
func openCodeRoutes(cfg map[string]any, selected map[string][]string, metadata map[string]openCodeModelMetadata, env []string) (map[string]aiproxy.ProviderRoute, error) {
	routes := map[string]aiproxy.ProviderRoute{}
	for id, models := range selected {
		if err := openCodeCheckAuth(id, env); err != nil {
			return nil, err
		}
		provider := readObject(readObject(cfg, "provider"), id)
		options := readObject(provider, "options")
		for _, name := range models {
			m, ok := metadata[id+"/"+name]
			if !ok {
				return nil, errors.New("OpenCode selected model is unavailable")
			}
			configured := readObject(readObject(provider, "models"), name)
			if hasTransportOverride(m.Options) || hasTransportOverride(readObject(configured, "options")) || hasTransportOverride(readObject(configured, "variants")) {
				return nil, errors.New("OpenCode model transport overrides are unsupported")
			}
			family := ""
			switch m.API.NPM {
			case "@ai-sdk/openai", "@ai-sdk/openai-compatible":
				family = "openai"
			case "@ai-sdk/anthropic":
				family = "anthropic"
			default:
				return nil, errors.New("OpenCode selected provider transport is unsupported")
			}
			base, _ := options["baseURL"].(string)
			if base == "" {
				base = m.API.URL
			}
			if base == "" && id == "openai" && m.API.NPM == "@ai-sdk/openai" {
				base = openCodeEnv(env, "OPENAI_BASE_URL")
				if base == "" {
					base = "https://api.openai.com/v1"
				}
			}
			if base == "" && id == "anthropic" && m.API.NPM == "@ai-sdk/anthropic" {
				base = openCodeEnv(env, "ANTHROPIC_BASE_URL")
				if base == "" {
					base = "https://api.anthropic.com/v1"
				}
			}
			if base == "" {
				return nil, errors.New("OpenCode selected provider has no resolved upstream URL")
			}
			route := aiproxy.ProviderRoute{BaseURL: base, Provider: family}
			if prior, ok := routes[id]; ok && prior != route {
				return nil, errors.New("OpenCode provider uses incompatible model transports")
			}
			routes[id] = route
		}
	}
	return routes, nil
}
func hasTransportOverride(m map[string]any) bool {
	for key, value := range m {
		switch strings.ToLower(key) {
		case "baseurl", "url", "fetch":
			return true
		}
		if nested, ok := value.(map[string]any); ok && hasTransportOverride(nested) {
			return true
		}
	}
	return false
}
func openCodeCheckAuth(id string, env []string) error {
	var body []byte
	if inline := openCodeEnv(env, "OPENCODE_AUTH_CONTENT"); inline != "" {
		body = []byte(inline)
	} else {
		data := openCodeEnv(env, "XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(openCodeEnv(env, "HOME"), ".local", "share")
		}
		if !filepath.IsAbs(data) {
			return errors.New("OpenCode credential directory is unresolved")
		}
		path := filepath.Join(data, "opencode", "auth.json")
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > openCodeOutputLimit {
			return errors.New("OpenCode credential type could not be checked")
		}
		f, err := os.Open(path)
		if err != nil {
			return errors.New("OpenCode credential type could not be checked")
		}
		defer f.Close()
		body, err = io.ReadAll(io.LimitReader(f, openCodeOutputLimit+1))
		if err != nil || len(body) > openCodeOutputLimit {
			return errors.New("OpenCode credential type could not be checked")
		}
	}
	var auth map[string]struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(body, &auth) != nil {
		return errors.New("OpenCode credential type could not be checked")
	}
	if a, ok := auth[id]; ok && a.Type != "api" {
		return errors.New("OpenCode selected provider requires unsupported authentication transport")
	}
	return nil
}

func openCodeRoutesApplied(cfg map[string]any, ids []string, selected map[string][]string, proxyURL string) bool {
	raw, _ := cfg["enabled_providers"].([]any)
	got := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		got = append(got, s)
	}
	sort.Strings(got)
	if !slices.Equal(got, ids) {
		return false
	}
	for _, id := range ids {
		p := readObject(readObject(cfg, "provider"), id)
		if readObject(p, "options")["baseURL"] != proxyURL+"/_opencode/"+id {
			return false
		}
		raw, _ := p["whitelist"].([]any)
		names := make([]string, 0, len(raw))
		for _, v := range raw {
			s, _ := v.(string)
			names = append(names, s)
		}
		sort.Strings(names)
		if !slices.Equal(names, selected[id]) {
			return false
		}
	}
	return true
}
