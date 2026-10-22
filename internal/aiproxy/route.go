package aiproxy

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/alekzonder/tariboy/internal/store"
)

type Upstream struct {
	BaseURL string
	KeyEnv  string
}

type Rule struct {
	ModelGlob string
	Upstream  Upstream
}

type Router struct {
	mu       sync.RWMutex
	defaults map[string]Upstream // provider -> upstream
	rules    []Rule
}

func NewRouter() *Router {
	defaults := map[string]Upstream{
		providerChatGPT: {BaseURL: "https://chatgpt.com/backend-api/codex"},
	}
	for p, s := range providerSpecs {
		defaults[string(p)] = Upstream{BaseURL: s.baseURL, KeyEnv: s.keyEnv}
	}
	return &Router{defaults: defaults}
}

func (r *Router) SetDefault(provider string, u Upstream) {
	r.mu.Lock()
	r.defaults[provider] = u
	r.mu.Unlock()
}

func (r *Router) SetRules(rules []Rule) {
	r.mu.Lock()
	r.rules = rules
	r.mu.Unlock()
}

// Resolve returns the upstream for a (provider, model). Model-glob rules win;
// otherwise the provider default.
func (r *Router) Resolve(provider, model string) Upstream {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rule := range r.rules {
		if globMatch(rule.ModelGlob, model) {
			return rule.Upstream
		}
	}
	return r.defaults[provider]
}

// ResolveDefault returns the configured provider default without applying
// model rules. It is used for credential-bound providers whose authentication
// headers must never be routed to an arbitrary model-rule upstream.
func (r *Router) ResolveDefault(provider string) Upstream {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaults[provider]
}

// globMatch supports only '*' (zero-or-more). Deterministic and dependency-free.
func globMatch(pattern, value string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = value[len(parts[0]):]
	last := parts[len(parts)-1]
	if !strings.HasSuffix(value, last) {
		return false
	}
	value = value[:len(value)-len(last)]
	for _, p := range parts[1 : len(parts)-1] {
		if p == "" {
			continue
		}
		i := strings.Index(value, p)
		if i < 0 {
			return false
		}
		value = value[i+len(p):]
	}
	return true
}

// proxyConfig is the daemon-config JSON shape under key "proxy".
type proxyConfig struct {
	Upstreams map[string]Upstream `json:"upstreams"` // provider -> upstream
	Rules     []Rule              `json:"rules"`
}

// LoadRouter builds the router from defaults, then a daemon_config "proxy"
// override, then env bootstrap overrides (used by the e2e fake upstream).
func LoadRouter(s *store.Store, getenv func(string) string) (*Router, error) {
	r := NewRouter()
	if v, ok, err := s.ConfigGet("proxy"); err != nil {
		return nil, err
	} else if ok && v != "" {
		var pc proxyConfig
		if err := json.Unmarshal([]byte(v), &pc); err == nil {
			for provider, u := range pc.Upstreams {
				r.SetDefault(provider, u)
			}
			if len(pc.Rules) > 0 {
				r.SetRules(pc.Rules)
			}
		}
	}
	if getenv != nil {
		if v := getenv("TARIBOY_UPSTREAM_ANTHROPIC_BASE_URL"); v != "" {
			d := r.ResolveDefault(string(ProviderAnthropic))
			d.BaseURL = v
			r.SetDefault(string(ProviderAnthropic), d)
		}
		if v := getenv("TARIBOY_UPSTREAM_CHATGPT_BASE_URL"); v != "" {
			d := r.ResolveDefault(providerChatGPT)
			d.BaseURL = v
			r.SetDefault(providerChatGPT, d)
		}
		if v := getenv("TARIBOY_UPSTREAM_OPENAI_BASE_URL"); v != "" {
			d := r.ResolveDefault(string(ProviderOpenAI))
			d.BaseURL = v
			r.SetDefault(string(ProviderOpenAI), d)
		}
		if v := getenv("TARIBOY_UPSTREAM_CURSOR_BASE_URL"); v != "" {
			d := r.ResolveDefault(string(ProviderCursor))
			d.BaseURL = v
			r.SetDefault(string(ProviderCursor), d)
		}
	}
	return r, nil
}
