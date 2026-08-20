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
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	ProtocolVersion int      `json:"protocol_version"`
	Types           []string `json:"types"`
	Exec            string   `json:"exec"`
	Description     string   `json:"description"`
	Commands        []string `json:"commands,omitempty"`
	Channels        Channels `json:"channels"`
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
