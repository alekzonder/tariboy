package commands

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
)

var updateWorkflowsGolden = flag.Bool("update-workflows-golden", false, "rewrite testdata/workflows_responses.golden.json")

// workflowsGoldenVolatile names keys whose values depend on the wall clock.
var workflowsGoldenVolatile = map[string]bool{"built_at": true}

// workflowRichManifest sets every optional key of a Workflowfile.yaml, so an
// inspected manifest carries each optional field of the definition.
const workflowRichManifest = `schema_version: 1
name: rich
workflow_version: 0.1.0
initial_status: plan
requires_secrets: [GH_TOKEN]
env:
  PR_POLL_SECONDS: "60"
limits:
  idle_iterations: 3
  rejected_requests: 5
  script_failures: 3
  unavailable_grace: 5m
watch_spacing: 30s
artifacts:
  - name: plan
    description: The implementation plan, as Markdown.
  - name: pull_request
sources:
  - name: incoming
    script: ./scripts/incoming.sh
    every: 2m
    timeout: 30s
    description: Lists the open pull requests.
statuses:
  - id: plan
    owner: { pool: developers }
    instructions: ./statuses/plan.md
    limits: { idle_iterations: 6 }
    transitions:
      - on: planned
        to: approval
        requires: [plan]
        checks:
          - script: ./scripts/check.sh
            run_as: agent
            timeout: 60s
            description: The plan exists.
  - id: approval
    owner: customer
    transitions:
      - { on: approved, to: review }
      - { on: rejected, to: cancelled }
  - id: review
    owner: script
    watch:
      script: ./scripts/monitor.sh
      every: 60s
      timeout: 60s
      description: Watches the pull request.
    transitions:
      - { on: merged, to: done }
  - id: done
    terminal: true
  - id: cancelled
    terminal: true
    cancelled: true
`

func writeRichWorkflowSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"Workflowfile.yaml":   workflowRichManifest,
		"statuses/plan.md":    "plan the work\n",
		"scripts/incoming.sh": "#!/bin/sh\nexit 0\n",
		"scripts/check.sh":    "#!/bin/sh\nexit 0\n",
		"scripts/monitor.sh":  "#!/bin/sh\nexit 0\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if filepath.Ext(rel) == ".sh" {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// scriptsFixture serves the fake script control for the agent alice with no
// scripts yet.
func scriptsFixture(t *testing.T) (*registry.Ctx, *fakeScriptControl) {
	t.Helper()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "alice", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeScriptControl{definitions: map[string]script.Definition{}, runs: map[string][]script.Run{}}
	c.Scripts = fake
	return c, fake
}

// addFinishedRun records a finished run with a pid and an exit code for the
// script id and makes it the script's latest run.
func addFinishedRun(fake *fakeScriptControl, id string) {
	pid, code := 4242, 3
	run := script.Run{ID: "srun-done", ScriptID: id, Agent: "alice", Status: script.RunFailed, PID: &pid, ExitCode: &code,
		CreatedAt: "2026-07-14T10:00:00Z", StartedAt: "2026-07-14T10:00:01Z", FinishedAt: "2026-07-14T10:00:02Z", LogPath: "/logs/srun-done.log"}
	fake.runs[id] = append([]script.Run{run}, fake.runs[id]...)
	definition := fake.definitions[id]
	definition.LatestRun = &run
	fake.definitions[id] = definition
}

// addGoldenSchedule stores one cron schedule for alice at a fixed clock.
func addGoldenSchedule(t *testing.T, c *registry.Ctx) {
	t.Helper()
	fixed := func() time.Time { return time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC) }
	if _, err := schedule.NewStore(c.Store, fixed).Add(schedule.Schedule{Agent: "alice", Kind: "cron", Spec: "0 9 * * *", Channel: "agent.alice.inbox"}); err != nil {
		t.Fatal(err)
	}
}

// Catches a typed workflow, script or schedule response changing the JSON a
// client sees: the set of keys, their JSON types, and their values must match
// the recorded responses.
func TestWorkflowsResponsesMatchGolden(t *testing.T) {
	c := workflowCtx(t)
	recorder := newGoldenRecorder(t, workflowsGoldenVolatile, c.BaseDir, "$BASE")
	call := func(c *registry.Ctx, label, path string, params registry.Params) {
		t.Helper()
		value, err := h(t, path)(c, params)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		recorder.record(label, value)
	}
	demo := writeWorkflowSource(t, t.TempDir(), "1.0.0")
	rich := writeRichWorkflowSource(t)
	call(c, "workflow.validate valid", "workflow.validate", registry.Params{"path": demo})
	call(c, "workflow.validate rich", "workflow.validate", registry.Params{"path": rich})
	call(c, "workflow.ls empty", "workflow.ls", registry.Params{})
	call(c, "workflow.build created", "workflow.build", registry.Params{"path": demo})
	call(c, "workflow.build again", "workflow.build", registry.Params{"path": demo})
	call(c, "workflow.build rich", "workflow.build", registry.Params{"path": rich})
	call(c, "workflow.ls", "workflow.ls", registry.Params{})
	call(c, "workflow.inspect demo", "workflow.inspect", registry.Params{"name": "demo", "tag": "latest"})
	call(c, "workflow.inspect rich", "workflow.inspect", registry.Params{"name": "rich", "tag": "0.1.0"})
	call(c, "workflow.rm latest", "workflow.rm", registry.Params{"name": "demo", "tag": "latest"})
	call(c, "workflow.rm last tag", "workflow.rm", registry.Params{"name": "demo", "tag": "1.0.0"})
	if err := os.Remove(filepath.Join(demo, "statuses", "work.md")); err != nil {
		t.Fatal(err)
	}
	call(c, "workflow.validate invalid", "workflow.validate", registry.Params{"path": demo})

	sc, fake := scriptsFixture(t)
	call(sc, "script.ls empty", "script.ls", registry.Params{"name": "alice"})
	call(sc, "script.run", "script.run", registry.Params{"name": "alice", "script_name": "check", "description": "check repo", "command": "make check"})
	addFinishedRun(fake, "scr-1")
	call(sc, "script.ls latest run", "script.ls", registry.Params{"name": "alice"})
	call(sc, "script.runs", "script.runs", registry.Params{"name": "alice", "id": "scr-1"})
	call(sc, "script.run-get finished", "script.run-get", registry.Params{"name": "alice", "id": "srun-done"})
	call(sc, "script.run-get pending", "script.run-get", registry.Params{"name": "alice", "id": "srun-1"})
	call(sc, "script.logs", "script.logs", registry.Params{"name": "alice", "id": "srun-done"})
	call(sc, "script.rerun", "script.rerun", registry.Params{"name": "alice", "id": "scr-1"})
	call(sc, "script.cancel", "script.cancel", registry.Params{"name": "alice", "id": "scr-1"})
	definition := fake.definitions["scr-1"]
	definition.State = script.StateCancelled
	fake.definitions["scr-1"] = definition
	call(sc, "script.rm", "script.rm", registry.Params{"name": "alice", "id": "scr-1"})
	call(sc, "script.schedule", "script.schedule", registry.Params{"name": "alice", "script_name": "poll", "description": "poll repo", "command": "make poll", "interval_seconds": 60, "quiet_exit": 111})

	call(sc, "schedule.ls empty", "schedule.ls", registry.Params{"name": "alice"})
	addGoldenSchedule(t, sc)
	call(sc, "schedule.ls", "schedule.ls", registry.Params{"name": "alice"})
	recorder.check("workflows_responses.golden.json", *updateWorkflowsGolden)
}
