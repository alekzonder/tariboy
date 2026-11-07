package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/schedule"
)

// agentSettingRoutes maps the route suffix of an agent setting to the field
// agent.inspect returns and the body key the command reads.
var agentSettingRoutes = map[string][2]string{
	"/model":                    {"model", "value"},
	"/effort":                   {"effort", "value"},
	"/plugins":                  {"plugins", "value"},
	"/loop/hard-timeout":        {"hard_timeout_s", "value"},
	"/loop/ai-stall-timeout":    {"ai_stall_timeout_s", "value"},
	"/user-prompt":              {"user_prompt", "text"},
	"/messages/batch":           {"messages_batch", "value"},
	"/messages/max-queue":       {"messages_max_queue", "value"},
	"/messages/instant-senders": {"mail_instant_senders", "senders"},
	"/alias":                    {"alias", "value"},
	"/notes":                    {"notes", "value"},
	"/color":                    {"color", "value"},
}

// agentSettingCall serves agent.inspect, the agent secrets and retention, and
// the setting commands compose converges. ok is false for any other route.
func (f *fakeCaller) agentSettingCall(method, route string, body any) (raw json.RawMessage, ok bool, err error) {
	rest, found := strings.CutPrefix(route, "/api/agents/")
	if !found {
		return nil, false, nil
	}
	name, suffix, _ := strings.Cut(rest, "/")
	if suffix != "" {
		suffix = "/" + suffix
	}
	handled := method == "GET" && slices.Contains([]string{"", "/secrets", "/retention"}, suffix) ||
		method == "POST" && (slices.Contains([]string{"/secrets", "/retention", "/loop/schedule"}, suffix) || agentSettingRoutes[suffix][0] != "")
	if !handled {
		return nil, false, nil
	}
	a := f.agents[name]
	if a == nil {
		return nil, true, &client.APIError{Code: "not_found", Msg: "agent " + name + " not found"}
	}
	m, _ := body.(map[string]any)
	switch {
	case method == "GET" && suffix == "":
		return mustJSON(a), true, nil
	case method == "GET" && suffix == "/secrets":
		keys := []string{}
		for key := range agentSecrets(a) {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		return mustJSON(map[string]any{"keys": keys, "count": len(keys)}), true, nil
	case method == "GET" && suffix == "/retention":
		override, _ := a["retention"].(map[string]any)
		if override == nil {
			override = map[string]any{"keep_iterations": 0, "keep_days": 0, "max_bytes": 0}
		}
		return mustJSON(map[string]any{"override": override}), true, nil
	case method == "POST" && suffix == "/secrets":
		secrets := agentSecrets(a)
		secrets[m["key"].(string)] = m["value"].(string)
		a["secrets"] = secrets
		return mustJSON(map[string]any{"name": name, "key": m["key"], "stored": true}), true, nil
	case method == "POST" && suffix == "/retention":
		override, _ := a["retention"].(map[string]any)
		if override == nil {
			override = map[string]any{"keep_iterations": 0, "keep_days": 0, "max_bytes": 0}
		}
		for key, value := range m {
			override[strings.ReplaceAll(key, "-", "_")] = value
		}
		a["retention"] = override
		return mustJSON(map[string]any{"agent": name}), true, nil
	case method == "POST" && suffix == "/loop/schedule":
		rule := ""
		if m["clear"] != true {
			if rule, err = schedule.NormalizeRRule(m["value"].(string), time.Now(), time.Local); err != nil {
				return nil, true, err
			}
		}
		a["schedule"] = rule
		return mustJSON(map[string]any{"name": name, "schedule": rule}), true, nil
	default:
		key := agentSettingRoutes[suffix]
		value := m[key[1]]
		if key[0] == "mail_instant_senders" {
			// The daemon drops empty values: [""] sets an empty list.
			senders := []string{}
			for _, s := range value.([]string) {
				if s != "" {
					senders = append(senders, s)
				}
			}
			value = senders
		}
		a[key[0]] = value
		return mustJSON(map[string]any{"name": name}), true, nil
	}
}

func agentSecrets(a map[string]any) map[string]string {
	secrets, _ := a["secrets"].(map[string]string)
	if secrets == nil {
		secrets = map[string]string{}
	}
	return secrets
}

const agentSecretVar = "COMPOSE_TEST_AGENT_GH_TOKEN"

// allSettingsYAML declares an agent with every setting compose converges.
// The fake stores plugins as given, as the daemon does for a schema-v2 image.
const allSettingsYAML = `
version: 1
groups:
  team: {lead: lead}
agents:
  lead:
    image: basic:latest
    group: team
  dev:
    image: basic:latest
    group: team
    harness: {type: claude, model: opus, effort: high, interactive: true}
    plugins: [loop, mail]
    env_mode: replace
    env: {LOG_LEVEL: info}
    secrets:
      GH_TOKEN: {from_env: ` + agentSecretVar + `}
    cwd: /work/project
    create_cwd: true
    user_prompt: |
      Work only in your branch.
    alias: Dev
    notes: Project developer
    color: "#3366FF"
    messages_batch: 10
    messages_max_queue: 100
    mail_instant_senders: [lead]
    retention: {keep_iterations: 200, keep_days: 30, max_bytes: 1073741824}
    loop:
      timeout: 60m
      hard_timeout: 90m
      ai_stall_timeout: 30m
      schedule: "RRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=0"
`

func allSettingsFile(t *testing.T) File {
	t.Helper()
	f, err := Parse([]byte(allSettingsYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	return f
}

func mutatingCallsSince(calls []string, start int) []string {
	var out []string
	for _, c := range calls[start:] {
		if !strings.HasPrefix(c, "GET ") {
			out = append(out, c)
		}
	}
	return out
}

func statusOutput(t *testing.T, fc Caller, f File) string {
	t.Helper()
	var out strings.Builder
	if err := NewRunner(fc, "", &out).Status(f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// A new agent gets every setting in one up: the create request carries the
// scalar settings and later actions set the schedule, the secret, the
// retention and, in the groups phase, the instant senders.
func TestUpCreatesAgentWithEverySetting(t *testing.T) {
	t.Setenv(agentSecretVar, "secret-create-value")
	fc := newFake()
	upNoBuild(t, NewRunner(fc, "", io.Discard), allSettingsFile(t))
	var body map[string]any
	for i, c := range fc.calls {
		if c == "POST /api/agents" && fc.bodies[i].(map[string]any)["name"] == "dev" {
			body = fc.bodies[i].(map[string]any)
		}
	}
	want := map[string]any{"model": "opus", "effort": "high", "plugins": "loop,mail", "hard_timeout_s": 5400, "ai_stall_timeout_s": 1800,
		"user_prompt": "Work only in your branch.\n", "messages_batch": 10, "messages_max_queue": 100, "alias": "Dev",
		"notes": "Project developer", "color": "#3366FF", "create_cwd": true, "cwd": "/work/project"}
	for key, value := range want {
		if fmt.Sprint(body[key]) != fmt.Sprint(value) {
			t.Errorf("create body %s = %v, want %v", key, body[key], value)
		}
	}
	dev := fc.agents["dev"]
	if got := fmt.Sprint(dev["plugins"]); got != "[loop mail]" {
		t.Errorf("plugins = %s, want the file's list set after the create", got)
	}
	if s, _ := dev["schedule"].(string); !strings.Contains(s, "RRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=0") {
		t.Errorf("schedule = %q", s)
	}
	if agentSecrets(dev)["GH_TOKEN"] != "secret-create-value" {
		t.Errorf("secret not set: %v", dev["secrets"])
	}
	if got := fmt.Sprint(dev["retention"]); got != "map[keep_days:30 keep_iterations:200 max_bytes:1073741824]" {
		t.Errorf("retention = %s", got)
	}
	if got := fmt.Sprint(dev["mail_instant_senders"]); got != "[agent:lead]" {
		t.Errorf("mail_instant_senders = %s", got)
	}
	senders, created := callIndex(fc.calls, "POST /api/agents/dev/messages/instant-senders"), callIndex(fc.calls, "POST /api/groups/team/assign")
	if senders < created {
		t.Errorf("instant senders set at %d, before the groups phase (%d): %v", senders, created, fc.calls)
	}
}

// Criterion 5: a second up on an agent with every setting changes nothing; it
// only writes the secret again, which it cannot compare.
func TestUpAgentWithEverySettingIsIdempotent(t *testing.T) {
	t.Setenv(agentSecretVar, "secret-idempotent-value")
	fc := newFake()
	f := allSettingsFile(t)
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	before := len(fc.calls)
	var out strings.Builder
	upNoBuild(t, NewRunner(fc, "", &out), f)
	for _, c := range mutatingCallsSince(fc.calls, before) {
		if c != "POST /api/agents/dev/secrets" {
			t.Errorf("second up made a mutating call %s", c)
		}
	}
	if !strings.Contains(out.String(), "0 changed") {
		t.Errorf("second report:\n%s", out.String())
	}
	status := statusOutput(t, fc, f)
	if !strings.Contains(status, "drift: 0") || strings.Contains(status, "not checked") {
		t.Errorf("status:\n%s", status)
	}
}

type settingCase struct {
	name   string
	mutate func(a map[string]any)
	route  string // suffix after /api/agents/dev
	key    string // body key
	want   string // fmt.Sprint of the body value
	issue  string // what status prints
}

// Criterion 1: each setting of an existing agent that differs converges with
// exactly one call, status names it, and a later status reports no drift.
func TestUpConvergesEachSettingOfExistingAgent(t *testing.T) {
	for _, tc := range []settingCase{
		{"model", func(a map[string]any) { a["model"] = "sonnet" }, "/model", "value", "opus", "model drift: have=sonnet want=opus"},
		{"effort", func(a map[string]any) { a["effort"] = "low" }, "/effort", "value", "high", "effort drift: have=low want=high"},
		{"plugins", func(a map[string]any) { a["plugins"] = []string{"mail"} }, "/plugins", "value", "[loop mail]", "plugins drift: have=[mail] want=[loop,mail]"},
		{"hard_timeout", func(a map[string]any) { a["hard_timeout_s"] = 60 }, "/loop/hard-timeout", "value", "5400", "hard_timeout drift: have=60s want=5400s"},
		{"ai_stall_timeout", func(a map[string]any) { a["ai_stall_timeout_s"] = 60 }, "/loop/ai-stall-timeout", "value", "1800", "ai_stall_timeout drift: have=60s want=1800s"},
		{"user_prompt", func(a map[string]any) { a["user_prompt"] = "old prompt" }, "/user-prompt", "text", "Work only in your branch.\n", "user_prompt drift"},
		{"messages_batch", func(a map[string]any) { a["messages_batch"] = 1 }, "/messages/batch", "value", "10", "messages_batch drift: have=1 want=10"},
		{"messages_max_queue", func(a map[string]any) { a["messages_max_queue"] = 5 }, "/messages/max-queue", "value", "100", "messages_max_queue drift: have=5 want=100"},
		{"alias", func(a map[string]any) { a["alias"] = "Old" }, "/alias", "value", "Dev", "alias drift: have=Old want=Dev"},
		{"notes", func(a map[string]any) { a["notes"] = "old notes" }, "/notes", "value", "Project developer", "notes drift"},
		{"color", func(a map[string]any) { a["color"] = "#000000" }, "/color", "value", "#3366FF", "color drift: have=#000000 want=#3366FF"},
		{"schedule", func(a map[string]any) { a["schedule"] = "" }, "/loop/schedule", "value", "RRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=0", "schedule drift"},
		{"retention", func(a map[string]any) { a["retention"].(map[string]any)["keep_days"] = 1 }, "/retention", "keep-days", "30", "retention drift: keep_days have=1 want=30"},
		{"mail_instant_senders", func(a map[string]any) { a["mail_instant_senders"] = nil }, "/messages/instant-senders", "senders", "[agent:lead]", "mail_instant_senders drift: have=(not set) want=[lead]"},
		{"env replace", func(a map[string]any) { a["env"] = map[string]any{"LOG_LEVEL": "info", "EXTRA": "x"} }, "/env", "value", "map[LOG_LEVEL:info]", "env extra: EXTRA"},
		{"secret", func(a map[string]any) { delete(a, "secrets") }, "/secrets", "key", "GH_TOKEN", "secret GH_TOKEN MISSING"},
		{"cwd with create_cwd", func(a map[string]any) { a["cwd"] = "/old" }, "/cwd", "create", "true", "cwd drift: have=/old want=/work/project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agentSecretVar, "secret-converge-value")
			fc := newFake()
			f := allSettingsFile(t)
			upNoBuild(t, NewRunner(fc, "", io.Discard), f)
			tc.mutate(fc.agents["dev"])
			status := statusOutput(t, fc, f)
			if !strings.Contains(status, tc.issue) || !strings.Contains(status, "drift: 1") {
				t.Fatalf("status lacks %q:\n%s", tc.issue, status)
			}
			if strings.Contains(status, "old prompt") || strings.Contains(status, "old notes") {
				t.Fatalf("status printed a text value:\n%s", status)
			}
			before := len(fc.calls)
			upNoBuild(t, NewRunner(fc, "", io.Discard), f)
			route := "POST /api/agents/dev" + tc.route
			if n := countCallsSince(fc.calls, before, route); n != 1 {
				t.Fatalf("%s calls = %d: %v", route, n, fc.calls[before:])
			}
			for _, c := range mutatingCallsSince(fc.calls, before) {
				if c != route && c != "POST /api/agents/dev/secrets" {
					t.Fatalf("unexpected mutating call %s", c)
				}
			}
			if got := fmt.Sprint(bodyFor(fc, route).(map[string]any)[tc.key]); got != tc.want {
				t.Fatalf("%s body %s = %s, want %s", route, tc.key, got, tc.want)
			}
			if status := statusOutput(t, fc, f); !strings.Contains(status, "drift: 0") {
				t.Fatalf("status after up:\n%s", status)
			}
		})
	}
}

// An omitted setting keeps the live value, and model and effort are never
// cleared: the daemon reads an empty value as a read.
func TestUpOmittedSettingsKeepLiveValues(t *testing.T) {
	fc := newFake()
	fc.agents["dev"] = map[string]any{"name": "dev", "state": "running", "enabled": true, "image": "basic:latest", "harness": "claude",
		"model": "opus", "effort": "high", "plugins": []string{"loop"}, "alias": "Dev", "notes": "n", "color": "#000000",
		"user_prompt": "p", "messages_batch": 3, "hard_timeout_s": 10, "schedule": "DTSTART:20260101T090000\nRRULE:FREQ=DAILY;BYSECOND=0",
		"mail_instant_senders": []string{"agent:x"}, "env": map[string]any{"EXTRA": "1"}}
	f, err := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    harness: {type: claude}\n    env: {A: b}\n"))
	if err != nil {
		t.Fatal(err)
	}
	before := len(fc.calls)
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	for _, c := range mutatingCallsSince(fc.calls, before) {
		if c != "POST /api/agents/dev/env" && c != "POST /api/agents/dev/interactive" {
			t.Fatalf("omitted setting changed: %s", c)
		}
	}
	if got := fmt.Sprint(bodyFor(fc, "POST /api/agents/dev/env").(map[string]any)["value"]); got != "map[A:b EXTRA:1]" {
		t.Fatalf("env_mode merge dropped a key: %s", got)
	}
}

// Criterion 2: env_mode replace removes a key the file lacks; merge keeps it.
func TestEnvModeReplaceRemovesExtraKeyAndMergeKeepsIt(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{{"replace", "map[A:b]"}, {"merge", "map[A:b EXTRA:1]"}} {
		t.Run(tc.mode, func(t *testing.T) {
			fc := newFake()
			fc.agents["dev"] = map[string]any{"name": "dev", "state": "running", "enabled": true, "image": "basic:latest",
				"harness": "claude", "env": map[string]any{"A": "old", "EXTRA": "1"}}
			f, err := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    env_mode: " + tc.mode + "\n    env: {A: b}\n"))
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Validate(); err != nil {
				t.Fatal(err)
			}
			status := statusOutput(t, fc, f)
			if got := strings.Contains(status, "env extra: EXTRA"); got != (tc.mode == "replace") {
				t.Fatalf("status:\n%s", status)
			}
			upNoBuild(t, NewRunner(fc, "", io.Discard), f)
			if got := fmt.Sprint(bodyFor(fc, "POST /api/agents/dev/env").(map[string]any)["value"]); got != tc.want {
				t.Fatalf("env body = %s, want %s", got, tc.want)
			}
		})
	}
}

// Criterion 3: the value of an agent secret never appears in the output of
// up, status or --dry-run, or in a daemon error that echoes it.
func TestAgentSecretValueNeverPrinted(t *testing.T) {
	const value = "agent-secret-value-7f3a"
	t.Setenv(agentSecretVar, value)
	f := allSettingsFile(t)
	fc := newFake()
	var out strings.Builder
	r := NewRunner(fc, "", &out)
	if err := r.DryRun(f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "secret GH_TOKEN=from "+agentSecretVar) {
		t.Fatalf("dry-run of a create lacks the secret source:\n%s", out.String())
	}
	upNoBuild(t, r, f)
	if err := r.DryRun(f); err != nil {
		t.Fatal(err)
	}
	if err := r.Status(f); err != nil {
		t.Fatal(err)
	}
	upNoBuild(t, r, f)
	fc.fail = map[string]error{"POST /api/agents/dev/secrets": errors.New("bad request: value " + value + " refused")}
	r.skipBuild = true
	err := r.Up(f)
	if err == nil {
		t.Fatal("up with a failing secret write succeeded")
	}
	if got := out.String() + fmt.Sprint(errors.Unwrap(err)); strings.Contains(got, value) {
		t.Fatalf("output holds the secret value:\n%s", got)
	}
	if !strings.Contains(out.String(), "[redacted]") {
		t.Fatalf("daemon error was not reported redacted:\n%s", out.String())
	}
}

// The four rows of the agent secret table, and a live secret the file does
// not declare.
func TestAgentSecretOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name           string
		varSet, exists bool
		status         string
		outcome        Outcome
		writes         int
	}{
		{"var and secret", true, true, "secret GH_TOKEN ok (value not compared)", OutcomeOK, 1},
		{"var without secret", true, false, "secret GH_TOKEN MISSING", OutcomeChanged, 1},
		{"secret without var", false, true, "secret GH_TOKEN ok (" + agentSecretVar + " not set, kept)", OutcomeOK, 0},
		{"neither", false, false, "secret GH_TOKEN MISSING (" + agentSecretVar + " not set)", OutcomeFailed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.varSet {
				t.Setenv(agentSecretVar, "row-value")
			} else {
				t.Setenv(agentSecretVar, "")
			}
			fc := newFake()
			secrets := map[string]string{"OLD": "x"}
			if tc.exists {
				secrets["GH_TOKEN"] = "old"
			}
			fc.agents["dev"] = map[string]any{"name": "dev", "state": "running", "enabled": true, "image": "basic:latest", "harness": "claude", "secrets": secrets}
			f, _ := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    secrets:\n      GH_TOKEN: {from_env: " + agentSecretVar + "}\n"))
			status := statusOutput(t, fc, f)
			if !strings.Contains(status, tc.status) || !strings.Contains(status, "secret OLD not declared") {
				t.Fatalf("status:\n%s", status)
			}
			r := NewRunner(fc, "", io.Discard)
			plan, err := r.Plan(f)
			if err != nil {
				t.Fatal(err)
			}
			rep := r.Apply(plan)
			if got := entryOutcome(rep, "agent:dev"); got != tc.outcome {
				t.Fatalf("outcome = %s, want %s (%v)", got, tc.outcome, rep.Entries)
			}
			if n := countCalls(fc, "POST /api/agents/dev/secrets"); n != tc.writes {
				t.Fatalf("secret writes = %d, want %d", n, tc.writes)
			}
			if tc.outcome == OutcomeFailed && !strings.Contains(rep.Err().Error(), "tariboy secret set dev GH_TOKEN") {
				t.Fatalf("error = %v", rep.Err())
			}
		})
	}
}

// The daemon replaces the plugins of an agent with those of a new image when
// the agent adopts it, so up waits for the adoption: status reports the
// plugins as pending, and the up after the adoption sets them.
func TestPluginsWaitForImageAdoption(t *testing.T) {
	fc := newFake()
	fc.agents["dev"] = map[string]any{"name": "dev", "state": "running", "enabled": true, "image": "old:latest", "harness": "claude", "plugins": []string{"x"}}
	f, _ := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    plugins: [loop, mail]\n"))
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	if countCalls(fc, "POST /api/agents/dev/image") != 1 || countCalls(fc, "POST /api/agents/dev/plugins") != 0 {
		t.Fatalf("calls: %v", fc.calls)
	}
	status := statusOutput(t, fc, f)
	if !strings.Contains(status, "plugins pending: applied after the agent adopts image basic:latest; run up again") {
		t.Fatalf("status:\n%s", status)
	}
	// The agent adopts the image on its next iteration and takes its plugins.
	dev := fc.agents["dev"]
	dev["image"], dev["plugins"] = "basic:latest", []string{"loop", "mail", "extra"}
	delete(dev, "pending_image")
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	if got := fmt.Sprint(bodyFor(fc, "POST /api/agents/dev/plugins").(map[string]any)["value"]); got != "[loop mail]" {
		t.Fatalf("plugins body = %s: %v", got, fc.calls)
	}
	if status := statusOutput(t, fc, f); !strings.Contains(status, "drift: 0") {
		t.Fatalf("status after adoption:\n%s", status)
	}
}

// A re-provision may change the plugins, so up always sets the file's list
// after it.
func TestPluginsSetAfterReprovision(t *testing.T) {
	fc := newFake()
	fc.agents["dev"] = map[string]any{"name": "dev", "state": "stopped", "image_removed": true, "image": "basic:latest", "harness": "claude", "plugins": []string{"loop"}}
	f, _ := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    plugins: [loop]\n"))
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	re, plugins := callIndex(fc.calls, "POST /api/agents/dev/reprovision"), callIndex(fc.calls, "POST /api/agents/dev/plugins")
	if re < 0 || plugins < re {
		t.Fatalf("plugins %d not after reprovision %d: %v", plugins, re, fc.calls)
	}
}

// The instant senders are set in the groups phase, after the create of each
// sender the file declares, so a failed create of a sender skips them.
func TestInstantSendersDependOnSenderCreate(t *testing.T) {
	f, _ := Parse([]byte("version: 1\nagents:\n  a:\n    image: basic:latest\n    mail_instant_senders: [b]\n  b:\n    image: basic:latest\n"))
	plan, err := NewRunner(newFake(), "", io.Discard).Plan(f)
	if err != nil {
		t.Fatal(err)
	}
	createB, senders := -1, -1
	for i, a := range plan.Actions {
		switch {
		case a.Kind == "create" && a.Resource == "agent:b":
			createB = i
		case a.Call.Route == "/api/agents/a/messages/instant-senders":
			senders = i
			if a.Phase != "groups" {
				t.Fatalf("instant senders in phase %s", a.Phase)
			}
		}
	}
	if createB < 0 || senders < 0 || !slices.Contains(plan.Actions[senders].DependsOn, createB) {
		t.Fatalf("instant senders %d do not depend on create of b %d: %+v", senders, createB, plan.Actions)
	}
	if got := fmt.Sprint(plan.Actions[senders].Call.Body); got != "map[senders:[agent:b]]" {
		t.Fatalf("body = %s", got)
	}
}

// An empty mail_instant_senders refuses every agent; the daemon reads [""]
// as an empty list.
func TestEmptyInstantSendersSetsEmptyList(t *testing.T) {
	fc := newFake()
	fc.agents["a"] = map[string]any{"name": "a", "state": "running", "enabled": true, "image": "basic:latest", "harness": "claude"}
	f, _ := Parse([]byte("version: 1\nagents:\n  a:\n    image: basic:latest\n    mail_instant_senders: []\n"))
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	if got := fmt.Sprint(fc.agents["a"]["mail_instant_senders"]); got != "[]" {
		t.Fatalf("senders = %s: %v", got, fc.calls)
	}
	if status := statusOutput(t, fc, f); !strings.Contains(status, "drift: 0") {
		t.Fatalf("status:\n%s", status)
	}
}

// Criterion 4: a lead change to an agent that joins the group in the same up
// sets the lead after the assign.
func TestUpChangesGroupLeadToNewMember(t *testing.T) {
	fc := newFake()
	fc.groups["team"] = map[string]any{"name": "team", "lead": "old"}
	fc.agents["old"] = map[string]any{"name": "old", "state": "running", "enabled": true, "image": "basic:latest", "harness": "claude", "group": "team"}
	fc.agents["lead"] = map[string]any{"name": "lead", "state": "running", "enabled": true, "image": "basic:latest", "harness": "claude", "group": ""}
	f, _ := Parse([]byte("version: 1\ngroups:\n  team: {lead: lead}\nagents:\n  old: {image: basic:latest, group: team}\n  lead: {image: basic:latest, group: team}\n"))
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	assign, lead := callIndex(fc.calls, "POST /api/groups/team/assign"), callIndex(fc.calls, "PATCH /api/groups/team/lead")
	if assign < 0 || lead < assign || fc.groups["team"]["lead"] != "lead" {
		t.Fatalf("lead %d not after assign %d (lead=%v): %v", lead, assign, fc.groups["team"]["lead"], fc.calls)
	}
}

func TestValidateAgentSettings(t *testing.T) {
	for _, tc := range []struct{ field, want string }{
		{"env_mode: bogus", "env_mode"},
		{"color: blue", "color"},
		{"loop: {hard_timeout: -1m}", "hard_timeout"},
		{"loop: {ai_stall_timeout: 0s}", "ai_stall_timeout"},
		{"loop: {ai_stall_timeout: 1500ms}", "ai_stall_timeout"},
		{"loop: {schedule: \"FREQ=SECONDLY\"}", "schedule"},
		{"messages_batch: 0", "messages_batch"},
		{"messages_max_queue: 0", "messages_max_queue"},
		{"retention: {keep_days: -1}", "retention"},
		{"secrets: {1X: {from_env: V}}", "secret key"},
		{"secrets: {TOKEN: {}}", "from_env"},
		{"env: {TOKEN: x}\n    secrets: {TOKEN: {from_env: V}}", "env and secrets"},
		{"mail_instant_senders: [\"Bad Name\"]", "mail_instant_senders"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			f, err := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    " + tc.field + "\n"))
			if err == nil {
				err = f.Validate()
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to name %q", err, tc.want)
			}
		})
	}
	if _, err := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    bogus: 1\n")); err == nil {
		t.Fatal("unknown agent field accepted")
	}
}

// A schedule with DTSTART compares the whole rule; one without compares the
// RRULE only, since the daemon fills DTSTART with the time of the write.
func TestScheduleWithDTSTARTComparesStart(t *testing.T) {
	fc := newFake()
	fc.agents["dev"] = map[string]any{"name": "dev", "state": "running", "enabled": true, "image": "basic:latest", "harness": "claude",
		"schedule": "DTSTART:20260101T090000\nRRULE:FREQ=DAILY;BYSECOND=0"}
	for yaml, drift := range map[string]bool{
		"RRULE:FREQ=DAILY":                         false,
		"DTSTART:20260101T090000 RRULE:FREQ=DAILY": false,
		"DTSTART:20260202T090000 RRULE:FREQ=DAILY": true,
		"RRULE:FREQ=WEEKLY":                        true,
	} {
		f, _ := Parse([]byte("version: 1\nagents:\n  dev:\n    image: basic:latest\n    loop: {schedule: \"" + yaml + "\"}\n"))
		if got := strings.Contains(statusOutput(t, fc, f), "schedule drift"); got != drift {
			t.Errorf("%s: drift = %t, want %t", yaml, got, drift)
		}
	}
}
