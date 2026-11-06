// Package compose is the client-side declarative reconciler for
// tariboy-compose.yaml (spec §5): parse+validate, diff desired-vs-actual,
// and converge by driving the existing daemon command surface. It adds no new
// daemon capability.
package compose

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/alekzonder/tariboy/internal/agent"
)

// File is a parsed tariboy-compose.yaml (spec §5.1).
type File struct {
	Version    int                      `yaml:"version"`
	Images     map[string]ImageSpec     `yaml:"images"`
	Groups     map[string]GroupSpec     `yaml:"groups"`
	Agents     map[string]AgentSpec     `yaml:"agents"`
	TaskQueues map[string]TaskQueueSpec `yaml:"task_queues"`
	// RemovedWorkflows captures the top-level workflows: map of the removed
	// task workflow engine so Parse can reject it by name. An empty map is
	// accepted: team exports before the removal always wrote "workflows: {}".
	RemovedWorkflows yaml.Node `yaml:"workflows,omitempty"`
}

// Parse decodes a tariboy-compose.yaml document. It does not validate the
// result; call Validate separately.
func Parse(b []byte) (File, error) {
	var f File
	decoder := yaml.NewDecoder(strings.NewReader(string(b)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&f); err != nil {
		return File{}, fmt.Errorf("parse compose file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return File{}, fmt.Errorf("parse compose file: multiple YAML documents are not supported")
		}
		return File{}, fmt.Errorf("parse compose file: %w", err)
	}
	if declaresValue(f.RemovedWorkflows) {
		return File{}, fmt.Errorf("parse compose file: the top-level workflows key is no longer supported: the task workflow engine was removed")
	}
	return f, nil
}

// declaresValue reports whether a captured YAML value says anything: an absent
// key, null, an empty string, and an empty map or list do not.
func declaresValue(n yaml.Node) bool {
	switch n.Kind {
	case 0:
		return false
	case yaml.ScalarNode:
		return n.Tag != "!!null" && n.Value != ""
	case yaml.MappingNode, yaml.SequenceNode:
		return len(n.Content) != 0
	default:
		return true
	}
}

// Validate enforces the schema rules (spec §5.1): version, name charset (the
// shared path-traversal guard), lead-is-a-member, group references, image
// presence, and budget mode.
func (f File) Validate() error {
	if f.Version != 1 {
		return fmt.Errorf("unsupported compose version %d (want 1)", f.Version)
	}
	for name := range f.Images {
		if !agent.ValidName(name) {
			return fmt.Errorf("invalid image name %q", name)
		}
	}
	for name, g := range f.Groups {
		if err := f.validateGroup(name, g); err != nil {
			return err
		}
	}
	for name, a := range f.Agents {
		if err := f.validateAgent(name, a); err != nil {
			return err
		}
	}
	for prefix, queue := range f.TaskQueues {
		if err := f.validateTaskQueue(prefix, queue); err != nil {
			return err
		}
	}
	return nil
}

func normalizeAgentNames(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, name := range in {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
