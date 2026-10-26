package commands

import (
	"bytes"
	"encoding/json"

	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/invopop/jsonschema"
)

// Response types of the plugin, harness and secret commands. Each command declares its type with registry.WithResult, which
// derives the OpenAPI result schema from it.

// PluginInstallResult reports the plugin version plugin.install activated.
type PluginInstallResult struct {
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Types     []string `json:"types" jsonschema_extras:"nullable=true"`
	Installed bool     `json:"installed"`
}

// PluginSummary is one installed plugin in the plugin.ls list. Health is the
// plugin's last health report, a JSON object.
type PluginSummary struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Types   []string        `json:"types" jsonschema_extras:"nullable=true"`
	Enabled bool            `json:"enabled"`
	State   string          `json:"state"`
	Health  json.RawMessage `json:"health"`
}

// PluginList is the plugin.ls result.
type PluginList struct {
	Plugins []PluginSummary `json:"plugins"`
	Count   int             `json:"count"`
}

// PluginContribution is the operator commands and settings of one enabled
// plugin. It encodes like plugins.Contribution under a plugin-prefixed
// component name.
type PluginContribution plugins.Contribution

// PluginContributionList is the plugin.contributions result: the operator
// commands and settings of every enabled plugin that declares any.
type PluginContributionList struct {
	Plugins []PluginContribution `json:"plugins"`
	Count   int                  `json:"count"`
}

func pluginContributionList(items []plugins.Contribution) PluginContributionList {
	out := PluginContributionList{Count: len(items)}
	if items != nil {
		out.Plugins = make([]PluginContribution, len(items))
		for i, item := range items {
			out.Plugins[i] = PluginContribution(item)
		}
	}
	return out
}

// PluginMail is a plugin's declared mail surface: whether it sends letters
// and whether it receives the letters of its inbox.
type PluginMail struct {
	Send    bool `json:"send"`
	Receive bool `json:"receive"`
}

// PluginView is one plugin's stored record, live state and socket
// (plugin.inspect).
type PluginView struct {
	Name            string     `json:"name"`
	Version         string     `json:"version"`
	Types           []string   `json:"types" jsonschema_extras:"nullable=true"`
	ProtocolVersion int        `json:"protocol_version"`
	Exec            string     `json:"exec"`
	SourcePath      string     `json:"source_path"`
	Mail            PluginMail `json:"mail"`
	Enabled         bool       `json:"enabled"`
	State           string     `json:"state"`
	InstalledAt     string     `json:"installed_at"`
	Socket          string     `json:"socket"`
}

// PluginRemoved names the plugin plugin.rm removed.
type PluginRemoved struct {
	Removed string `json:"removed"`
}

// PluginRestarted names the plugin plugin.restart restarted.
type PluginRestarted struct {
	Restarted string `json:"restarted"`
}

// PluginLogs is the tail of a plugin's captured output. Lines is null when
// the log file exists but is empty.
type PluginLogs struct {
	Lines []string `json:"lines" jsonschema_extras:"nullable=true"`
	Count int      `json:"count"`
}

// pluginResult converts a plugin host map into its response type. The host
// interface returns maps; decoding rejects a key the type lacks, so the typed
// response can never drop one silently.
func pluginResult[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return out, decoder.Decode(&out)
}

// HarnessModel is one model a harness offers, with the efforts it accepts
// when they differ from the harness-wide list.
type HarnessModel struct {
	ID      string   `json:"id"`
	Label   string   `json:"label,omitempty"`
	Efforts []string `json:"efforts,omitempty"`
}

// HarnessModelCatalog is the model and effort list of one harness on the
// daemon's host (harness.models). Error explains an empty list.
type HarnessModelCatalog struct {
	Harness string         `json:"harness"`
	Models  []HarnessModel `json:"models" jsonschema_extras:"nullable=true"`
	Efforts []string       `json:"efforts" jsonschema_extras:"nullable=true"`
	Error   string         `json:"error"`
}

func harnessModelCatalog(catalog harness.Catalog) HarnessModelCatalog {
	out := HarnessModelCatalog{Harness: catalog.Harness, Efforts: catalog.Efforts, Error: catalog.Error}
	if catalog.Models != nil {
		out.Models = make([]HarnessModel, len(catalog.Models))
		for i, model := range catalog.Models {
			out.Models[i] = HarnessModel(model)
		}
	}
	return out
}

// CodexApprovalMode is how the daemon starts codex.
type CodexApprovalMode string

// JSONSchemaExtend lists the codex approval modes.
func (CodexApprovalMode) JSONSchemaExtend(s *jsonschema.Schema) {
	s.Enum = make([]any, len(harness.CodexApprovalModes))
	for i, mode := range harness.CodexApprovalModes {
		s.Enum[i] = mode
	}
}

// HarnessCodexApproval is the codex approval mode of this daemon.
type HarnessCodexApproval struct {
	Mode CodexApprovalMode `json:"mode"`
}

// SecretStored confirms secret.store saved an agent secret.
type SecretStored struct {
	Name   string `json:"name"`
	Key    string `json:"key"`
	Stored bool   `json:"stored"`
}

// SecretKeyList lists an agent's secret keys; values are never returned.
type SecretKeyList struct {
	Keys  []string `json:"keys"`
	Count int      `json:"count"`
}

// SecretRemoved names the secret key secret.rm removed.
type SecretRemoved struct {
	Removed string `json:"removed"`
}

// PluginRoutes is the plugin.routes result: the body the running plugin
// answers on GET /routes, encoded unchanged. The host guarantees only a JSON
// object, or null when the plugin sends an empty or unparsable body.
type PluginRoutes struct {
	Body map[string]any `json:"-"`
}

// MarshalJSON encodes the plugin body as is.
func (r PluginRoutes) MarshalJSON() ([]byte, error) { return json.Marshal(r.Body) }

// JSONSchema describes the plugin-defined routes body as an open object.
func (PluginRoutes) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "object",
		Description: "Plugin-defined external channel routes; the plugin defines the shape.",
		Extras:      map[string]any{"nullable": true},
	}
}

// PluginActionResult is the plugin.action result: the result value of the
// action response when the plugin sends one, otherwise the response without
// its subscriptions. The plugin defines the value, which may be any JSON value.
type PluginActionResult struct {
	Value any `json:"-"`
}

// MarshalJSON encodes the plugin value as is.
func (r PluginActionResult) MarshalJSON() ([]byte, error) { return json.Marshal(r.Value) }

// JSONSchema accepts any JSON value.
func (PluginActionResult) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Description: "Plugin-defined action result; the plugin defines the shape, which may be any JSON value."}
}

// pluginActionResult returns the result value of the response when present,
// otherwise a copy of the response (an object, never null) without the
// subscriptions the host applied.
func pluginActionResult(response map[string]any) PluginActionResult {
	if result, ok := response["result"]; ok {
		return PluginActionResult{Value: result}
	}
	public := PluginActionResult{Value: map[string]any{}}
	for key, value := range response {
		if key != "subscriptions" {
			public.Value.(map[string]any)[key] = value
		}
	}
	return public
}
