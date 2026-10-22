package aiproxy

// Provider names the upstream API family a proxied request belongs to.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderCursor    Provider = "cursor"
)

// providerChatGPT is only a router default key: OpenAI requests carrying a
// ChatGPT account header resolve to it. Exchanges never carry it.
const providerChatGPT = "chatgpt"

// providerSpec holds the per-provider behavior the proxy branches on.
type providerSpec struct {
	// baseURL and keyEnv are the router defaults.
	baseURL string
	keyEnv  string
	// passthrough streams the request and response bodies without buffering,
	// records no usage, and bypasses model-glob routing rules (cursor's
	// Connect/gRPC traffic).
	passthrough bool
	// connectErrors writes budget denials as Connect error JSON instead of an
	// Anthropic-shaped error.
	connectErrors bool
	// requireAuthorization rejects requests without an Authorization header.
	requireAuthorization bool
	// parseUsage parses a non-streaming response body; nil records no usage
	// from the JSON body.
	parseUsage func([]byte) (Usage, string, bool)
}

var providerSpecs = map[Provider]providerSpec{
	ProviderAnthropic: {baseURL: "https://api.anthropic.com", keyEnv: "ANTHROPIC_API_KEY", parseUsage: ParseAnthropicUsage},
	ProviderOpenAI: {baseURL: "https://api.openai.com", keyEnv: "OPENAI_API_KEY", requireAuthorization: true,
		parseUsage: ParseOpenAIUsage},
	ProviderCursor: {baseURL: "https://api2.cursor.sh", passthrough: true, connectErrors: true},
}

// spec returns the provider's spec; an unknown provider gets the Anthropic one.
func (p Provider) spec() providerSpec {
	if s, ok := providerSpecs[p]; ok {
		return s
	}
	return providerSpecs[ProviderAnthropic]
}
