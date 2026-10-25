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

// PluginProvidedChannel is one parameterized channel a plugin provides.
// ParamsSchema is the JSON Schema of the subscription parameters.
type PluginProvidedChannel struct {
	Channel      string          `json:"channel"`
	ParamsSchema json.RawMessage `json:"params_schema,omitempty"`
	Help         string          `json:"help,omitempty"`
}

// PluginChannels is a plugin's declared bus surface.
type PluginChannels struct {
	Publish   []string                `json:"publish" jsonschema_extras:"nullable=true"`
	Subscribe []string                `json:"subscribe" jsonschema_extras:"nullable=true"`
	Provide   []PluginProvidedChannel `json:"provide,omitempty"`
}

// PluginView is one plugin's stored record, live state and socket
// (plugin.inspect).
type PluginView struct {
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	Types           []string       `json:"types" jsonschema_extras:"nullable=true"`
	ProtocolVersion int            `json:"protocol_version"`
	Exec            string         `json:"exec"`
	SourcePath      string         `json:"source_path"`
	Channels        PluginChannels `json:"channels"`
	Enabled         bool           `json:"enabled"`
	State           string         `json:"state"`
	InstalledAt     string         `json:"installed_at"`
	Socket          string         `json:"socket"`
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
