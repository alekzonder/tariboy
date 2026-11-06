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
)

// File is a parsed Tariboycompose.yaml (spec §5.1). Version 1 has images,
// groups, agents and task_queues; version 2 adds tariboy, stores and
// workflows, and the source fields of images.
type File struct {
	Version int                  `yaml:"version"`
	Tariboy *TariboySpec         `yaml:"tariboy,omitempty"`
	Stores  map[string]StoreSpec `yaml:"stores,omitempty"`
	// Workflows is a version 2 section. In version 1 the key belonged to the
	// removed task workflow engine: Parse rejects a non-empty one there, and
	// accepts the empty "workflows: {}" that older team exports always wrote.
	Workflows  map[string]WorkflowSpec  `yaml:"workflows,omitempty"`
	Images     map[string]ImageSpec     `yaml:"images"`
	Groups     map[string]GroupSpec     `yaml:"groups"`
	Agents     map[string]AgentSpec     `yaml:"agents"`
	TaskQueues map[string]TaskQueueSpec `yaml:"task_queues"`
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
	if f.Version != 2 && len(f.Workflows) != 0 {
		return File{}, fmt.Errorf("parse compose file: the top-level workflows key is no longer supported in version 1; declare version: 2 to use the workflows section")
	}
	return f, nil
}

// Validate enforces the schema rules (spec §5.1) and returns the first of
// problems: version, name charset (the shared path-traversal guard),
// lead-is-a-member, references between entries, sources, and budget mode.
func (f File) Validate() error {
	if p := f.problems(); len(p) != 0 {
		return p[0]
	}
	return nil
}

// problems returns every schema and reference error of the file, section by
// section (version, tariboy, stores, workflows, images, groups, agents,
// task_queues) and by entry name within a section. An unsupported version is
// the only problem reported, since the other rules depend on it.
func (f File) problems() []error {
	if f.Version != 1 && f.Version != 2 {
		return []error{fmt.Errorf("unsupported compose version %d (want 1 or 2)", f.Version)}
	}
	var out []error
	out = append(out, f.tariboyProblems()...)
	out = append(out, f.storeProblems()...)
	out = append(out, f.workflowProblems()...)
	out = append(out, f.imageProblems()...)
	for _, name := range sortedKeys(f.Groups) {
		if err := f.validateGroup(name, f.Groups[name]); err != nil {
			out = append(out, err)
		}
	}
	for _, name := range sortedKeys(f.Agents) {
		out = append(out, f.agentProblems(name, f.Agents[name])...)
	}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		out = append(out, f.taskQueueProblems(prefix, f.TaskQueues[prefix])...)
	}
	return out
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
