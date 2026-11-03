package commands

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/assessment"
	"github.com/alekzonder/tariboy/internal/loop"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// rmFixture is a context with a real agent Manager and two built images,
// "solo:latest" and "shared:latest".
func rmFixture(t *testing.T) (*registry.Ctx, *agent.Store) {
	t.Helper()
	c := localCtx(t)
	src := writeExample(t)
	for _, ref := range []string{"solo:latest", "shared:latest"} {
		if _, err := cmdHandler(t, "image.build")(c, registry.Params{"tag": ref, "path": src}); err != nil {
			t.Fatal(err)
		}
	}
	as := agentStore(c)
	m := loop.NewManager(loop.ManagerConfig{
		AgentsDir: agentsDir(c), RuntimeDir: testutil.ShortDir(t),
		Store: as, ImgStore: imageStore(c), Log: c.Log, Clock: time.Now,
		Spawner: heldImageSpawner{started: make(chan string, 1)}, ShimBin: "/unused/fake-shim",
	})
	t.Cleanup(m.Shutdown)
	c.Control = m
	c.Assessments = assessment.New(c.Store.DB, steppingClock())
	return c, as
}

// TestAgentRmModesAndReport proves agent rm routes its three modes and its
// answer names the mode and what it deleted and kept.
func TestAgentRmModesAndReport(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		params registry.Params
		mode   store.RemoveMode
	}{
		{registry.Params{"name": "smoke"}, store.RemoveModeRemove},
		{registry.Params{"name": "smoke", "keep": true}, store.RemoveModeKeep},
		{registry.Params{"name": "smoke", "purge": true}, store.RemoveModePurge},
	} {
		c, _, fc := ctxWithStore(t)
		out, err := h(t, "agent.rm")(c, tc.params)
		if err != nil {
			t.Fatalf("%v: %v", tc.params, err)
		}
		if fc.removeMode != tc.mode {
			t.Fatalf("%v: mode = %q, want %q", tc.params, fc.removeMode, tc.mode)
		}
		res := out.(AgentRemoveResult)
		deleted, kept := store.RemovalReport(tc.mode)
		if res.Removed != "smoke" || res.Mode != AgentRemoveMode(tc.mode) || res.Purged != (tc.mode == store.RemoveModePurge) ||
			!slices.Equal(res.Deleted, deleted) || !slices.Equal(res.Kept, kept) {
			t.Fatalf("%v: result = %+v", tc.params, res)
		}
	}
}

func TestAgentRmRejectsKeepWithPurge(t *testing.T) {
	t.Parallel()
	c, _, fc := ctxWithStore(t)
	_, err := h(t, "agent.rm")(c, registry.Params{"name": "smoke", "keep": true, "purge": true})
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "bad_args" {
		t.Fatalf("keep with purge = %#v, want bad_args", err)
	}
	if fc.removed != "" {
		t.Fatalf("keep with purge reached Remove for %q", fc.removed)
	}
}

func runAgent(t *testing.T, c *registry.Ctx, name, ref string) {
	t.Helper()
	if _, err := c.Control.Run(registry.RunSpec{Name: name, ImageRef: ref, Harness: "stub"}); err != nil {
		t.Fatal(err)
	}
}

// TestAgentRmDefaultFreesTheImage reproduces the defect: a plain agent rm
// answered "removed" but kept the agents row, so image rm of the agent's image
// answered image_in_use.
func TestAgentRmDefaultFreesTheImage(t *testing.T) {
	c, _ := rmFixture(t)
	runAgent(t, c, "solo-agent", "solo:latest")
	runAgent(t, c, "first", "shared:latest")
	runAgent(t, c, "second", "shared:latest")

	for _, name := range []string{"solo-agent", "first"} {
		if _, err := cmdHandler(t, "agent.rm")(c, registry.Params{"name": name, "force": true}); err != nil {
			t.Fatalf("agent rm %s: %v", name, err)
		}
	}
	if _, err := cmdHandler(t, "image.rm")(c, registry.Params{"ref": "solo:latest"}); err != nil {
		t.Fatalf("image rm of the removed agent's image: %v", err)
	}
	_, err := cmdHandler(t, "image.rm")(c, registry.Params{"ref": "shared:latest"})
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "image_in_use" {
		t.Fatalf("image rm of an image another agent uses: %#v, want image_in_use", err)
	}
}

// TestAgentRmDefaultKeepsHistoryReadable proves the history a plain agent rm
// keeps stays readable through the API after the agent is gone.
func TestAgentRmDefaultKeepsHistoryReadable(t *testing.T) {
	c, as := rmFixture(t)
	runAgent(t, c, "dev", "solo:latest")
	for _, it := range []agent.Iteration{
		{ID: "it-dev", Agent: "dev", Trigger: "manual", Status: "done", StartedAt: "2026-11-02T07:00:00Z"},
		{ID: "it-judge", Agent: "judge", Trigger: "manual", Status: "running", StartedAt: "2026-11-02T07:00:00Z"},
	} {
		if err := as.CreateIteration(it); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := as.MutateIterationTags("dev", agent.TagOpAdd, []string{"it-dev"}, []string{"good"}); err != nil {
		t.Fatal(err)
	}
	five := 5
	if _, err := c.Assessments.Add("judge", "it-judge", assessment.AssessmentInput{
		RubricVersion: "rubric-1", IterationID: "it-dev", Summary: "ok",
		Axes: []assessment.AssessmentAxis{
			{Axis: assessment.AxisImageProcess, Verdict: assessment.VerdictScored, Score: &five},
			{Axis: assessment.AxisRole, Verdict: assessment.VerdictNotApplicable},
			{Axis: assessment.AxisTaskFit, Verdict: assessment.VerdictNotApplicable},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := cmdHandler(t, "agent.rm")(c, registry.Params{"name": "dev", "force": true}); err != nil {
		t.Fatalf("agent rm: %v", err)
	}
	_, err := cmdHandler(t, "agent.inspect")(c, registry.Params{"name": "dev"})
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "not_found" {
		t.Fatalf("agent inspect after rm: %#v, want not_found", err)
	}
	out, err := cmdHandler(t, "iteration.ls")(c, registry.Params{"name": "dev"})
	if err != nil {
		t.Fatalf("iteration ls after rm: %v", err)
	}
	its := out.(IterationList).Iterations
	if len(its) != 1 || its[0].ID != "it-dev" || len(its[0].Tags) != 1 || its[0].Tags[0] != "good" {
		t.Fatalf("iteration ls after rm = %+v, want it-dev tagged good", its)
	}
	listed, err := cmdHandler(t, "assessment.ls")(c, registry.Params{"agent": "dev"})
	if err != nil {
		t.Fatalf("assessment ls after rm: %v", err)
	}
	if got := listed.(assessment.AssessmentList).Assessments; len(got) != 1 {
		t.Fatalf("assessment ls after rm = %+v, want one", got)
	}
	l := agentdir.New(agentsDir(c), "dev")
	if _, err := os.Stat(filepath.Dir(l.ContextPath())); err != nil {
		t.Fatalf("agent rm dropped the agent directory: %v", err)
	}
}
