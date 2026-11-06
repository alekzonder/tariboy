package compose

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/stores"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

var taskQueuePrefixRE = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)
var poolRouteSegmentRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// TaskQueueSpec is one task queue. An empty description, owners or
// responsible_agent leaves the daemon's value untouched, as an empty workflow
// does; up never removes a pool, env key, secret or trigger the file omits.
type TaskQueueSpec struct {
	Name             string              `yaml:"name"`
	Description      string              `yaml:"description,omitempty"`
	Owners           []string            `yaml:"owners,omitempty"`
	ResponsibleAgent string              `yaml:"responsible_agent,omitempty"`
	Pools            map[string][]string `yaml:"pools"`
	// Workflow is "[STORE/]NAME[:TAG]", the workflow image the queue follows
	// (see ParseWorkflowRef). Empty leaves an existing binding untouched.
	Workflow string `yaml:"workflow,omitempty"`
	// Triggers create tasks from letters to the queue; up creates the ones
	// the queue lacks.
	Triggers []TriggerSpec `yaml:"triggers,omitempty"`
	// Env holds plain variables for workflow scripts, CUSTOMER_AGENT_POOL
	// among them.
	Env map[string]string `yaml:"env,omitempty"`
	// Secrets names, for each secret, the variable of the compose process
	// that holds its value. The file never holds a secret value.
	Secrets map[string]SecretSpec `yaml:"secrets,omitempty"`
}

// TriggerSpec is one workflow trigger. An empty Action means create_task.
type TriggerSpec struct {
	Pattern        string `yaml:"pattern"`
	CorrelationKey string `yaml:"correlation_key,omitempty"`
	Action         string `yaml:"action,omitempty"`
}

// SecretSpec reads a secret value from the environment variable FromEnv.
type SecretSpec struct {
	FromEnv string `yaml:"from_env"`
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

// taskQueueProblems adds the version 2 workflow reference to
// validateTaskQueue: the workflow, and its Store prefix, must be declared.
func (f File) taskQueueProblems(prefix string, queue TaskQueueSpec) []error {
	var out []error
	if err := f.validateTaskQueue(prefix, queue); err != nil {
		out = append(out, err)
	}
	ref, declared, err := queue.WorkflowRef()
	if f.Version != 2 || !declared || err != nil || !taskQueuePrefixRE.MatchString(prefix) {
		return out
	}
	if ref.Store != "" {
		if _, ok := f.Stores[ref.Store]; !ok {
			out = append(out, fmt.Errorf("task queue %q references undeclared store %q", prefix, ref.Store))
		}
	}
	if _, ok := f.Workflows[ref.Name]; !ok {
		out = append(out, fmt.Errorf("task queue %q references undeclared workflow %q", prefix, ref.Name))
	}
	return out
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
	for _, pool := range sortedKeys(queue.Pools) {
		if !poolRouteSegmentRE.MatchString(pool) {
			return fmt.Errorf("task queue %q has unsafe pool route name %q", prefix, pool)
		}
		for _, name := range normalizeAgentNames(queue.Pools[pool]) {
			if _, exists := f.Agents[name]; !exists {
				return fmt.Errorf("task queue %q pool %q references undeclared agent %q", prefix, pool, name)
			}
		}
	}
	if queue.ResponsibleAgent != "" {
		if _, exists := f.Agents[queue.ResponsibleAgent]; !exists {
			return fmt.Errorf("task queue %q responsible_agent references undeclared agent %q", prefix, queue.ResponsibleAgent)
		}
	}
	if err := validateQueueValues(queue); err != nil {
		return fmt.Errorf("task queue %q %w", prefix, err)
	}
	if err := validateQueueTriggers(queue.Triggers); err != nil {
		return fmt.Errorf("task queue %q %w", prefix, err)
	}
	return nil
}

// queueValueKeyRE is the daemon's rule for an env or secret key, and the
// rule for the name of the variable a secret reads.
var queueValueKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validQueueValueKey(key string) bool {
	return queueValueKeyRE.MatchString(key) && !strings.HasPrefix(key, "TARIBOY_")
}

// validateQueueValues checks the env and secrets keys as the daemon does. A
// key is either env or a secret, and CUSTOMER_AGENT_POOL is env only.
func validateQueueValues(queue TaskQueueSpec) error {
	for _, key := range sortedKeys(queue.Env) {
		if !validQueueValueKey(key) {
			return fmt.Errorf("env key %q: want letters, digits and underscores, not starting with a digit or TARIBOY_", key)
		}
	}
	for _, key := range sortedKeys(queue.Secrets) {
		from := queue.Secrets[key].FromEnv
		switch {
		case !validQueueValueKey(key):
			return fmt.Errorf("secret key %q: want letters, digits and underscores, not starting with a digit or TARIBOY_", key)
		case key == tasks.CustomerAgentPoolEnv:
			return fmt.Errorf("secret %q: CUSTOMER_AGENT_POOL is a plain env var; declare it in env", key)
		case from == "":
			return fmt.Errorf("secret %q needs from_env, the variable that holds its value", key)
		case !queueValueKeyRE.MatchString(from):
			return fmt.Errorf("secret %q from_env %q is not a valid environment variable name", key, from)
		}
		if _, dup := queue.Env[key]; dup {
			return fmt.Errorf("key %q is declared in env and secrets", key)
		}
	}
	return nil
}

func validateQueueTriggers(triggers []TriggerSpec) error {
	seen := map[TriggerSpec]bool{}
	for _, t := range triggers {
		kind, name, ok := strings.Cut(t.Pattern, ":")
		if !ok || !slices.Contains([]string{"agent", "user", "plugin", "system"}, kind) || name == "" {
			return fmt.Errorf("trigger pattern %q: want KIND:NAME, KIND one of agent, user, plugin, system", t.Pattern)
		}
		if t.Action != "" && t.Action != "create_task" {
			return fmt.Errorf("trigger action %q: want create_task", t.Action)
		}
		if seen[t.normalized()] {
			return fmt.Errorf("trigger %s is declared twice", t)
		}
		seen[t.normalized()] = true
	}
	return nil
}

// normalized fills the default action, the form the daemon stores.
func (t TriggerSpec) normalized() TriggerSpec {
	if t.Action == "" {
		t.Action = "create_task"
	}
	return t
}

// String is the trigger as status prints it: pattern, action and the
// correlation key when set.
func (t TriggerSpec) String() string {
	t = t.normalized()
	s := t.Pattern + " " + t.Action
	if t.CorrelationKey != "" {
		s += " correlation_key=" + t.CorrelationKey
	}
	return s
}
