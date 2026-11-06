package compose

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/stores"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

var taskQueuePrefixRE = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)
var poolRouteSegmentRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type TaskQueueSpec struct {
	Name  string              `yaml:"name"`
	Pools map[string][]string `yaml:"pools"`
	// Workflow is "[STORE/]NAME[:TAG]", the workflow image the queue follows
	// (see ParseWorkflowRef). Empty leaves an existing binding untouched.
	Workflow string `yaml:"workflow,omitempty"`
}

// WorkflowRef is a parsed task_queues.<PREFIX>.workflow value. A Store prefix
// means "build STORE/NAME first"; an empty Tag means the build's own version
// with a Store prefix and "latest" without one.
type WorkflowRef struct {
	Store string
	Name  string
	Tag   string
}

// ParseWorkflowRef parses "[STORE/]NAME[:TAG]". Names follow the Store
// selector and workflow name rules and the tag follows the image tag rule.
func ParseWorkflowRef(s string) (WorkflowRef, error) {
	var ref WorkflowRef
	if s == "" || s != strings.TrimSpace(s) {
		return ref, fmt.Errorf("want [STORE/]NAME[:TAG] without surrounding whitespace")
	}
	rest := s
	if strings.Count(rest, "/") > 1 || strings.Count(rest, ":") > 1 {
		return ref, fmt.Errorf("want [STORE/]NAME[:TAG] with at most one '/' and one ':'")
	}
	if i := strings.Index(rest, "/"); i >= 0 {
		ref.Store, rest = rest[:i], rest[i+1:]
		if !stores.ValidName(ref.Store) {
			return WorkflowRef{}, fmt.Errorf("invalid Store name %q", ref.Store)
		}
	}
	ref.Name = rest
	if i := strings.Index(rest, ":"); i >= 0 {
		ref.Name, ref.Tag = rest[:i], rest[i+1:]
		// A tag names a directory of the image store, so "." and ".." are refused.
		if parsed, err := image.ParseRef("x:" + ref.Tag); err != nil || parsed.Tag != ref.Tag || ref.Tag == "." || ref.Tag == ".." {
			return WorkflowRef{}, fmt.Errorf("invalid tag %q", ref.Tag)
		}
	}
	if !workflowfile.ValidName(ref.Name) {
		return WorkflowRef{}, fmt.Errorf("invalid workflow name %q", ref.Name)
	}
	return ref, nil
}

// WorkflowRef returns the parsed workflow of the queue; declared is false
// when the file names none (an empty value and an absent key are the same).
func (q TaskQueueSpec) WorkflowRef() (ref WorkflowRef, declared bool, err error) {
	if q.Workflow == "" {
		return WorkflowRef{}, false, nil
	}
	ref, err = ParseWorkflowRef(q.Workflow)
	return ref, err == nil, err
}

// Selector is the Store selector "STORE/NAME" the build step sends.
func (r WorkflowRef) Selector() string { return r.Store + "/" + r.Name }

// Ref is "NAME:TAG", TAG defaulting to latest, the form the daemon resolves.
func (r WorkflowRef) Ref() string {
	tag := r.Tag
	if tag == "" {
		tag = "latest"
	}
	return r.Name + ":" + tag
}

func (f File) validateTaskQueue(prefix string, queue TaskQueueSpec) error {
	if !taskQueuePrefixRE.MatchString(prefix) {
		return fmt.Errorf("invalid task queue prefix %q", prefix)
	}
	if strings.TrimSpace(queue.Name) == "" {
		return fmt.Errorf("task queue %q name is required", prefix)
	}
	if _, _, err := queue.WorkflowRef(); err != nil {
		return fmt.Errorf("task queue %q workflow %q: %w", prefix, queue.Workflow, err)
	}
	for pool, members := range queue.Pools {
		if !poolRouteSegmentRE.MatchString(pool) {
			return fmt.Errorf("task queue %q has unsafe pool route name %q", prefix, pool)
		}
		for _, name := range normalizeAgentNames(members) {
			if _, exists := f.Agents[name]; !exists {
				return fmt.Errorf("task queue %q pool %q references undeclared agent %q", prefix, pool, name)
			}
		}
	}
	return nil
}
