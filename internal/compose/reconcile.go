package compose

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Caller is the daemon HTTP client surface the reconciler drives (satisfied by
// *client.Client).
type Caller interface {
	Call(method, route string, body any) (json.RawMessage, error)
}

// Runner converges a compose File against a live daemon.
type Runner struct {
	call      Caller
	workdir   string // compose-file dir, to resolve relative image contexts
	out       io.Writer
	skipBuild bool // test seam: skip the image build
}

func NewRunner(call Caller, workdir string, out io.Writer) *Runner {
	return &Runner{call: call, workdir: workdir, out: out}
}

func (r *Runner) logf(format string, a ...any) {
	if r.out != nil {
		fmt.Fprintf(r.out, format+"\n", a...)
	}
}

// Up reconciles desired (the file) toward actual (the daemon): it builds the
// declared images, plans the actions from one read of the live state, applies
// them, and prints the per-resource report (spec §5.2). A failed action does
// not stop the others, so a later Up converges the rest. The report already
// shows each failure, so Up then returns only a count of the failed resources
// (which unwraps to the action errors). Idempotent: a converged file plans no
// mutating action.
func (r *Runner) Up(f File) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := r.Build(f); err != nil {
		return err
	}
	plan, err := r.Plan(f)
	if err != nil {
		return err
	}
	rep := r.Apply(plan)
	r.printReport(rep)
	return rep.summaryErr()
}

// desiredPeriodSeconds mirrors the daemon's budget.set default: an empty period
// means 24h (86400s); anything else is a Go duration.
func desiredPeriodSeconds(period string) int {
	if period == "" {
		return 86400
	}
	d, err := time.ParseDuration(period)
	if err != nil {
		return -1 // never matches a live row; forces a converge POST
	}
	return int(d.Seconds())
}

// Down stops+removes the file's agents and removes its groups (spec §5.3). The
// shared dir is kept unless volumes is set. Scoped strictly to the names in the
// file; unrelated agents/groups are untouched. A member the daemon does not
// know counts as already removed and is skipped; any other error still fails.
func (r *Runner) Down(f File, volumes bool) error {
	agents, err := r.liveAgents()
	if err != nil {
		return err
	}
	groups, err := r.liveGroups()
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(f.Agents) {
		if _, ok := agents[name]; !ok {
			r.logf("agent %s absent, skipping", name)
			continue
		}
		r.logf("removing agent %s (purge=%v)", name, volumes)
		q := map[string]string{"force": "true"}
		// Mirror the group --volumes semantics onto agents: a plain down preserves
		// each agent's data (row + CONTEXT.md + iterations + audit), while
		// down --volumes purges it. Without volumes, down keeps everything so a
		// later up re-provisions in place over the retained history.
		if volumes {
			q["purge"] = "true"
		}
		if _, err := r.call.Call("DELETE", "/api/agents/"+name, q); err != nil {
			return fmt.Errorf("remove agent %s: %w", name, err)
		}
	}
	for _, name := range sortedKeys(f.Groups) {
		if _, ok := groups[name]; !ok {
			r.logf("group %s absent, skipping", name)
			continue
		}
		r.logf("removing group %s (volumes=%v)", name, volumes)
		q := map[string]string{}
		if volumes {
			q["volumes"] = "true"
		}
		if _, err := r.call.Call("DELETE", "/api/groups/"+name, q); err != nil {
			return fmt.Errorf("remove group %s: %w", name, err)
		}
	}
	return nil
}

// Build publishes every declared image through the daemon. No-op when the file
// declares no images or the test seam is set.
func (r *Runner) Build(f File) error {
	if r.skipBuild {
		return nil
	}
	for _, name := range sortedKeys(f.Images) {
		spec := f.Images[name]
		ctx := spec.Context
		if !filepath.IsAbs(ctx) {
			ctx = filepath.Join(r.workdir, ctx)
		}
		r.logf("building image %s:latest", name)
		// The build is a daemon operation; wait answers when it has ended.
		if _, err := r.call.Call("POST", "/api/images/build", map[string]any{"name": name, "tag": "latest", "path": ctx, "wait": true, "initiator": "compose"}); err != nil {
			return fmt.Errorf("build image %s: %w", name, err)
		}
	}
	return nil
}
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func kvList(m map[string]string) string {
	var parts []string
	for _, k := range sortedKeys(m) {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}

// memberNames returns the file's agent names, or — when args is non-empty — the
// named subset. A name that is neither a file agent nor a file group is an
// error. A group name expands to that group's file members.
func (r *Runner) memberNames(f File, args []string) ([]string, error) {
	if len(args) == 0 {
		return sortedKeys(f.Agents), nil
	}
	var out []string
	for _, name := range args {
		switch {
		case fileHasAgent(f, name):
			out = append(out, name)
		case fileHasGroup(f, name):
			for _, an := range sortedKeys(f.Agents) {
				if f.Agents[an].Group == name {
					out = append(out, an)
				}
			}
		default:
			return nil, fmt.Errorf("%q is not an agent or group in this compose file", name)
		}
	}
	return out, nil
}

func fileHasAgent(f File, name string) bool { _, ok := f.Agents[name]; return ok }
func fileHasGroup(f File, name string) bool { _, ok := f.Groups[name]; return ok }

// Lifecycle applies an agent lifecycle verb to the selected members. For stop
// and kill, a member the daemon does not know is already stopped and is
// skipped; start and restart of an absent member still fail.
func (r *Runner) Lifecycle(f File, verb string, args []string) error {
	names, err := r.memberNames(f, args)
	if err != nil {
		return err
	}
	var live map[string]liveAgent
	if verb == "stop" || verb == "kill" {
		if live, err = r.liveAgents(); err != nil {
			return err
		}
	}
	for _, name := range names {
		if _, ok := live[name]; live != nil && !ok {
			r.logf("%s %s: absent, skipping", verb, name)
			continue
		}
		r.logf("%s %s", verb, name)
		if _, err := r.call.Call("POST", "/api/agents/"+name+"/"+verb, map[string]any{}); err != nil {
			return fmt.Errorf("%s %s: %w", verb, name, err)
		}
	}
	return nil
}

// Ps prints the file's members and their live state (scoped to the file).
func (r *Runner) Ps(f File) error {
	live, err := r.liveAgents()
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(f.Agents) {
		state := "absent"
		group := f.Agents[name].Group
		if a, ok := live[name]; ok {
			state = a.State
		}
		r.logf("%-20s %-12s group=%s", name, state, group)
	}
	return nil
}

// Exec runs a manual iteration in a named member, with an optional one-shot
// prompt (the remaining args joined by spaces). The target must be one of the
// file's own agents — exec never reaches an agent outside the compose file.
func (r *Runner) Exec(f File, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("exec needs a member name")
	}
	name := args[0]
	if !fileHasAgent(f, name) {
		return fmt.Errorf("%q is not an agent in this compose file", name)
	}
	body := map[string]any{}
	if len(args) > 1 {
		prompt := args[1]
		for _, w := range args[2:] {
			prompt += " " + w
		}
		body["prompt"] = prompt
	}
	r.logf("exec %s", name)
	_, err := r.call.Call("POST", "/api/agents/"+name+"/exec", body)
	return err
}

// Logs prints each selected member's recent events (scoped to the file, or the
// named member/group subset), one line per event. This maps onto the same
// GET /api/agents/{name}/logs route the top-level `logs` command uses (see
// internal/commands/logs.go): its handler returns {"events": [...],"count":N}
// with each event shaped {kind,data,at} — NOT a "lines" envelope. `tail` is
// sent as a query hint for forward-compat; the current handler ignores it and
// always returns its own fixed-size window, so this cannot be used to page
// through history. Follow (-f) and --source filtering are CLI-local features
// of the top-level `logs` command and are out of scope for `compose logs`,
// which only ever does one read pass.
func (r *Runner) Logs(f File, args []string, tail int) error {
	names, err := r.memberNames(f, args)
	if err != nil {
		return err
	}
	for _, name := range names {
		raw, err := r.call.Call("GET", "/api/agents/"+name+"/logs",
			map[string]string{"tail": fmt.Sprintf("%d", tail)})
		if err != nil {
			return fmt.Errorf("logs %s: %w", name, err)
		}
		var env struct {
			Events []struct {
				Kind string `json:"kind"`
				Data string `json:"data"`
				At   string `json:"at"`
			} `json:"events"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return err
		}
		for _, ev := range env.Events {
			r.logf("%s | %s [%s] %s", name, ev.At, ev.Kind, ev.Data)
		}
	}
	return nil
}

// Rm removes stopped members (spec §5.3). Running members are skipped (use
// down, or stop first) — this mirrors docker-compose rm, and is strictly
// scoped to the file's own agents (never touches an unrelated daemon agent).
func (r *Runner) Rm(f File, args []string) error {
	live, err := r.liveAgents()
	if err != nil {
		return err
	}
	names, err := r.memberNames(f, args)
	if err != nil {
		return err
	}
	for _, name := range names {
		a, ok := live[name]
		if !ok {
			continue
		}
		if a.State == "running" {
			r.logf("skip %s (running; stop it first)", name)
			continue
		}
		r.logf("removing %s", name)
		if _, err := r.call.Call("DELETE", "/api/agents/"+name, map[string]string{}); err != nil {
			return fmt.Errorf("rm %s: %w", name, err)
		}
	}
	return nil
}
