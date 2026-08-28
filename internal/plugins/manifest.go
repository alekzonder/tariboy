package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alekzonder/tariboy/internal/plugincaps"
)

// ProtocolVersion is the plugin protocol this daemon speaks (spec §7). A
// manifest/handshake with a different value is rejected.
const ProtocolVersion = 1

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidName reports whether name is a legal plugin name (the same pattern the
// manifest enforces). It is the single source of truth reused by the host
// primitives and the CLI handlers to refuse path-traversal names such as
// "../victim" — never duplicate the regex, call this instead.
func ValidName(name string) bool { return nameRE.MatchString(name) }

var knownTypes = map[string]bool{
	"channel-source": true,
	"channel-sink":   true,
	"harness":        true, // recognised; deep wiring deferred to Phase 2
	"tool":           true, // recognised; deep wiring deferred to Phase 2
	"eval":           true, // recognised; deep wiring deferred to Phase 2
}

// Provided is one channel a plugin fulfils for parameterized (query-like)
// subscriptions (spec §6.1). The daemon validates a subscriber's params against
// ParamsSchema at subscribe time; Help is surfaced by `tools sources` / the UI
// so an agent can discover how to subscribe. A provided channel must lie inside
// the plugin's Publish scope (enforced by Manifest.Validate).
type Provided struct {
	Channel      string          `json:"channel"`
	ParamsSchema json.RawMessage `json:"params_schema,omitempty"`
	Help         string          `json:"help,omitempty"`
}

// Channels is a plugin's declared bus surface (canonical home; also referenced
// by store.Record).
type Channels struct {
	Publish   []string   `json:"publish"`
	Subscribe []string   `json:"subscribe"`
	Provide   []Provided `json:"provide,omitempty"`
}

type OperatorCommand struct {
	Path    string        `json:"path"`
	Summary string        `json:"summary"`
	Action  string        `json:"action"`
	Args    []OperatorArg `json:"args,omitempty"`
}

type OperatorArg struct {
	Name     string `json:"name"`
	Flag     string `json:"flag,omitempty"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Help     string `json:"help,omitempty"`
}

type SettingsContribution struct {
	Title    string           `json:"title"`
	Status   []SettingStatus  `json:"status,omitempty"`
	Sections []SettingSection `json:"sections,omitempty"`
}

type SettingStatus struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type SettingSection struct {
	Title   string          `json:"title"`
	Fields  []SettingField  `json:"fields,omitempty"`
	Actions []SettingAction `json:"actions,omitempty"`
}

type SettingField struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Help     string `json:"help,omitempty"`
}

type SettingAction struct {
	Label  string   `json:"label"`
	Action string   `json:"action"`
	Fields []string `json:"fields,omitempty"`
}

// matchesAnyGlob reports whether channel matches at least one of the filepath
// glob patterns. It is the single glob test shared by publish-scope checks
// (Identity.CanPublish, provided-channel scope validation) — never inline
// filepath.Match, call this so the semantics stay identical everywhere.
func matchesAnyGlob(patterns []string, channel string) bool {
	for _, pat := range patterns {
		if ok, err := filepath.Match(pat, channel); err == nil && ok {
			return true
		}
	}
	return false
}

// Manifest is <plugin-dir>/plugin.json (spec §7.2).
type Manifest struct {
	Name             string                `json:"name"`
	Version          string                `json:"version"`
	ProtocolVersion  int                   `json:"protocol_version"`
	Types            []string              `json:"types"`
	Exec             string                `json:"exec"`
	Description      string                `json:"description"`
	Commands         []string              `json:"commands,omitempty"`
	OperatorCommands []OperatorCommand     `json:"operator_commands,omitempty"`
	Settings         *SettingsContribution `json:"settings,omitempty"`
	Channels         Channels              `json:"channels"`
	// Prompt names a Markdown file, relative to the plugin dir, holding the
	// system-prompt fragment this plugin contributes to an image that includes
	// it. Optional. Kept with the plugin so plugin-tied guidance is owned by the
	// plugin, not hardcoded in the daemon's builtin capability table.
	Prompt string `json:"prompt,omitempty"`
}

func ParseManifest(b []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse plugin.json: %w", err)
	}
	return m, nil
}

// Validate is the security gate for what the daemon will spawn (spec §13).
func (m Manifest) Validate() error {
	if !ValidName(m.Name) {
		return fmt.Errorf("invalid plugin name %q (want ^[a-z0-9][a-z0-9_-]*$)", m.Name)
	}
	if m.Version == "" || strings.ContainsAny(m.Version, `/\\`) || m.Version == "." || m.Version == ".." || m.Version == "active-version" || m.Version == "logs" || m.Version == "workdir" || m.Version == "plugin.sock" {
		return fmt.Errorf("plugin %s has invalid version %q", m.Name, m.Version)
	}
	if m.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("unsupported protocol_version %d (daemon speaks %d)", m.ProtocolVersion, ProtocolVersion)
	}
	if len(m.Types) == 0 {
		return fmt.Errorf("plugin %s declares no types", m.Name)
	}
	for _, t := range m.Types {
		if !knownTypes[t] {
			return fmt.Errorf("plugin %s declares unknown type %q", m.Name, t)
		}
	}
	if m.Exec == "" {
		return fmt.Errorf("plugin %s has no exec", m.Name)
	}
	if err := m.validateContributions(); err != nil {
		return err
	}
	if filepath.IsAbs(m.Exec) {
		return fmt.Errorf("plugin %s exec must be relative to the plugin dir, got absolute %q", m.Name, m.Exec)
	}
	clean := filepath.Clean(m.Exec)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("plugin %s exec escapes the plugin dir: %q", m.Name, m.Exec)
	}
	if m.Prompt != "" {
		if filepath.IsAbs(m.Prompt) {
			return fmt.Errorf("plugin %s prompt must be relative to the plugin dir, got absolute %q", m.Name, m.Prompt)
		}
		pc := filepath.Clean(m.Prompt)
		if pc == ".." || strings.HasPrefix(pc, ".."+string(filepath.Separator)) {
			return fmt.Errorf("plugin %s prompt escapes the plugin dir: %q", m.Name, m.Prompt)
		}
	}
	// Provided channels (spec §6.1): each must be a concrete channel name inside
	// the plugin's own publish scope — a plugin may only offer to fulfil demand
	// on channels it is already allowed to publish to — and any declared
	// params_schema must be a well-formed JSON Schema document.
	seen := map[string]bool{}
	for _, p := range m.Channels.Provide {
		if p.Channel == "" {
			return fmt.Errorf("plugin %s declares a provided channel with no name", m.Name)
		}
		if seen[p.Channel] {
			return fmt.Errorf("plugin %s declares provided channel %q more than once", m.Name, p.Channel)
		}
		seen[p.Channel] = true
		if !matchesAnyGlob(m.Channels.Publish, p.Channel) {
			return fmt.Errorf("plugin %s provides channel %q outside its publish scope %v", m.Name, p.Channel, m.Channels.Publish)
		}
		if len(p.ParamsSchema) > 0 {
			if err := validateSchemaDocument(p.ParamsSchema); err != nil {
				return fmt.Errorf("plugin %s provided channel %q has invalid params_schema: %w", m.Name, p.Channel, err)
			}
		}
	}
	return nil
}

func (m Manifest) validateContributions() error {
	commandTypes := map[string]bool{"string": true, "integer": true, "integer-list": true, "boolean": true, "secret-file": true}
	seenCommands := map[string]bool{}
	for _, command := range m.OperatorCommands {
		if command.Path == "" || strings.HasPrefix(command.Path, m.Name+".") {
			return fmt.Errorf("plugin %s has invalid relative operator command path %q", m.Name, command.Path)
		}
		for _, part := range strings.Split(command.Path, ".") {
			if !ValidName(part) {
				return fmt.Errorf("plugin %s has invalid operator command path %q", m.Name, command.Path)
			}
		}
		if seenCommands[command.Path] {
			return fmt.Errorf("plugin %s declares operator command %q more than once", m.Name, command.Path)
		}
		seenCommands[command.Path] = true
		if command.Summary == "" {
			return fmt.Errorf("plugin %s operator command %q has no summary", m.Name, command.Path)
		}
		if !ValidName(command.Action) {
			return fmt.Errorf("plugin %s operator command %q has invalid action %q", m.Name, command.Path, command.Action)
		}
		seenArgs, seenFlags := map[string]bool{}, map[string]bool{}
		for _, arg := range command.Args {
			if !ValidName(arg.Name) || !commandTypes[arg.Type] {
				return fmt.Errorf("plugin %s operator command %q has invalid argument %q", m.Name, command.Path, arg.Name)
			}
			if seenArgs[arg.Name] {
				return fmt.Errorf("plugin %s operator command %q repeats argument %q", m.Name, command.Path, arg.Name)
			}
			seenArgs[arg.Name] = true
			if arg.Flag != "" {
				if !ValidName(arg.Flag) || seenFlags[arg.Flag] {
					return fmt.Errorf("plugin %s operator command %q has invalid or duplicate flag %q", m.Name, command.Path, arg.Flag)
				}
				seenFlags[arg.Flag] = true
			}
			if arg.Type == "secret-file" && arg.Flag == "" {
				return fmt.Errorf("plugin %s operator command %q secret argument %q must be a flag", m.Name, command.Path, arg.Name)
			}
		}
	}
	if m.Settings == nil {
		return nil
	}
	if m.Settings.Title == "" {
		return fmt.Errorf("plugin %s settings title is required", m.Name)
	}
	seenStatus := map[string]bool{}
	for _, status := range m.Settings.Status {
		if !ValidName(status.Name) || status.Label == "" || seenStatus[status.Name] {
			return fmt.Errorf("plugin %s has invalid or duplicate settings status %q", m.Name, status.Name)
		}
		seenStatus[status.Name] = true
	}
	settingTypes := map[string]bool{"string": true, "password": true, "integer-list": true}
	seenFields := map[string]bool{}
	for _, section := range m.Settings.Sections {
		if section.Title == "" {
			return fmt.Errorf("plugin %s settings section title is required", m.Name)
		}
		fields := map[string]bool{}
		for _, field := range section.Fields {
			if !ValidName(field.Name) || field.Label == "" || !settingTypes[field.Type] || seenFields[field.Name] {
				return fmt.Errorf("plugin %s has invalid or duplicate settings field %q", m.Name, field.Name)
			}
			seenFields[field.Name] = true
			fields[field.Name] = true
		}
		for _, action := range section.Actions {
			if action.Label == "" || !ValidName(action.Action) {
				return fmt.Errorf("plugin %s has invalid settings action %q", m.Name, action.Action)
			}
			for _, field := range action.Fields {
				if !fields[field] {
					return fmt.Errorf("plugin %s settings action %q references unknown field %q", m.Name, action.Action, field)
				}
			}
		}
	}
	return nil
}

func (m Manifest) ValidateOperatorAction(action string, data map[string]any) error {
	types, required := map[string]string{}, map[string]bool{}
	declared := false
	for _, command := range m.OperatorCommands {
		if command.Action != action {
			continue
		}
		declared = true
		for _, arg := range command.Args {
			types[arg.Name] = arg.Type
			required[arg.Name] = required[arg.Name] || arg.Required
		}
	}
	for _, section := range settingsSections(m.Settings) {
		fieldTypes := map[string]string{}
		fieldRequired := map[string]bool{}
		for _, field := range section.Fields {
			fieldTypes[field.Name] = field.Type
			fieldRequired[field.Name] = field.Required
		}
		for _, button := range section.Actions {
			if button.Action != action {
				continue
			}
			declared = true
			for _, name := range button.Fields {
				types[name] = fieldTypes[name]
				required[name] = required[name] || fieldRequired[name]
			}
		}
	}
	if !declared {
		return nil
	}
	for name := range required {
		if required[name] {
			if _, ok := data[name]; !ok {
				return fmt.Errorf("plugin %s action %q requires field %q", m.Name, action, name)
			}
		}
	}
	for name, value := range data {
		if name == "action" {
			continue
		}
		typ, ok := types[name]
		if !ok {
			return fmt.Errorf("plugin %s action %q does not accept field %q", m.Name, action, name)
		}
		if !operatorValueMatches(typ, value) {
			return fmt.Errorf("plugin %s action %q field %q must be %s", m.Name, action, name, typ)
		}
	}
	return nil
}

func settingsSections(settings *SettingsContribution) []SettingSection {
	if settings == nil {
		return nil
	}
	return settings.Sections
}

func operatorValueMatches(typ string, value any) bool {
	switch typ {
	case "string", "secret-file", "password":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		return integerValue(value)
	case "integer-list":
		switch values := value.(type) {
		case []int64:
			return true
		case []int:
			return true
		case []any:
			for _, item := range values {
				if !integerValue(item) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func integerValue(value any) bool {
	switch value := value.(type) {
	case int, int32, int64:
		return true
	case float64:
		return value == float64(int64(value))
	case json.Number:
		_, err := value.Int64()
		return err == nil
	default:
		return false
	}
}

func (m Manifest) HasType(t string) bool {
	for _, x := range m.Types {
		if x == t {
			return true
		}
	}
	return false
}

// LoadPrompt returns the system-prompt fragment a plugin contributes, read from
// the file named by its manifest's `prompt` field. ok is false when the plugin
// declares no prompt. The manifest is validated first, so the path is guaranteed
// confined to the plugin dir.
func LoadPrompt(dir string) (body string, ok bool, err error) {
	m, err := LoadManifest(dir)
	if err != nil {
		return "", false, err
	}
	if m.Prompt == "" {
		return "", false, nil
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.Clean(m.Prompt)))
	if err != nil {
		return "", false, fmt.Errorf("plugin %s prompt: %w", m.Name, err)
	}
	return string(b), true, nil
}

type ResolvedPlugin = plugincaps.ResolvedPlugin

// ResolveInstalled returns a resolver for validated installed plugin
// manifests. A promptless plugin is installed; a missing directory is not.
func ResolveInstalled(pluginsDir string) plugincaps.ExternalResolver {
	return resolveInstalled(pluginsDir, true)
}

// ResolveInstalledMetadata validates only active plugin capability metadata.
// Schema-v2 images name plugins explicitly and never consume plugin.json's
// legacy prompt field, so a missing legacy prompt file must not affect them.
func ResolveInstalledMetadata(pluginsDir string) plugincaps.ExternalResolver {
	return resolveInstalled(pluginsDir, false)
}

func ResolveEnabledInstalledMetadata(pluginsDir string, store *Store) plugincaps.ExternalResolver {
	metadata := ResolveInstalledMetadata(pluginsDir)
	return func(name string) (ResolvedPlugin, error) {
		resolved, err := metadata(name)
		if err != nil || !resolved.Installed || store == nil {
			return resolved, err
		}
		record, ok, err := store.Get(name)
		if err != nil {
			return ResolvedPlugin{}, err
		}
		active, activeOK, err := store.ActiveVersion(name)
		if err != nil {
			return ResolvedPlugin{}, err
		}
		if !ok || !record.Enabled || !activeOK || active != record.Version {
			return ResolvedPlugin{}, nil
		}
		return resolved, nil
	}
}

func resolveInstalled(pluginsDir string, loadPrompt bool) plugincaps.ExternalResolver {
	return func(name string) (ResolvedPlugin, error) {
		if !ValidName(name) {
			return ResolvedPlugin{}, fmt.Errorf("invalid plugin name %q", name)
		}
		nameDir := filepath.Join(pluginsDir, name)
		dir := nameDir
		if active, readErr := os.ReadFile(filepath.Join(nameDir, "active-version")); readErr == nil {
			version := strings.TrimSpace(string(active))
			if version == "" || strings.ContainsAny(version, `/\\`) {
				return ResolvedPlugin{}, fmt.Errorf("plugin %s has invalid active version", name)
			}
			dir = filepath.Join(nameDir, version)
		}
		fi, statErr := os.Stat(dir)
		if os.IsNotExist(statErr) {
			return ResolvedPlugin{}, nil
		}
		if statErr != nil {
			return ResolvedPlugin{}, fmt.Errorf("plugin %s: %w", name, statErr)
		}
		if !fi.IsDir() {
			return ResolvedPlugin{}, fmt.Errorf("plugin %s path is not a directory", name)
		}
		manifest, err := LoadManifest(dir)
		if err != nil {
			return ResolvedPlugin{}, fmt.Errorf("plugin %s: %w", name, err)
		}
		if manifest.Name != name {
			return ResolvedPlugin{}, fmt.Errorf("plugin directory %q contains manifest for %q", name, manifest.Name)
		}
		resolved := ResolvedPlugin{Installed: true}
		if !loadPrompt || manifest.Prompt == "" {
			return resolved, nil
		}
		body, err := readConfinedPrompt(dir, manifest)
		if err != nil {
			return ResolvedPlugin{}, err
		}
		resolved.Prompt, resolved.HasPrompt = body, true
		return resolved, nil
	}
}

func readConfinedPrompt(dir string, manifest Manifest) (string, error) {
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("plugin %s directory: %w", manifest.Name, err)
	}
	promptPath, err := filepath.EvalSymlinks(filepath.Join(dir, filepath.Clean(manifest.Prompt)))
	if err != nil {
		return "", fmt.Errorf("plugin %s prompt: %w", manifest.Name, err)
	}
	rel, err := filepath.Rel(realDir, promptPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin %s prompt escapes the plugin dir", manifest.Name)
	}
	b, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("plugin %s prompt: %w", manifest.Name, err)
	}
	return string(b), nil
}

func LoadManifest(dir string) (Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("read plugin.json: %w", err)
	}
	m, err := ParseManifest(b)
	if err != nil {
		return Manifest{}, err
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
