package tasks

import (
	"context"
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// stickyDefinition is engineDefinition with sticky_holder set on the workflow
// and, when status is not nil, on the develop status.
func stickyDefinition(workflow bool, status *bool) workflowfile.File {
	def := engineDefinition()
	def.StickyHolder = workflow
	def.Statuses[0].StickyHolder = status
	return def
}

// wantDispatched checks the last workflow.dispatched event of task.
func wantDispatched(t *testing.T, svc *Service, task Task, want map[string]any) {
	t.Helper()
	got := eventPayload(t, svc, task, "workflow.dispatched")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workflow.dispatched = %v\nwant %v", got, want)
	}
}

func dispatched(status, pool, agent, reason string, sticky bool, previous string, holderFree, freeMember bool) map[string]any {
	return map[string]any{
		"status": status, "pool": pool, "agent": agent, "reason": reason, "sticky_holder": sticky,
		"previous_holder": previous, "holder_free": holderFree, "free_member_available": freeMember,
	}
}

// makeBusy gives agent another assigned task in develop: dev-2 is switched off
// while the task is created so that agent gets it.
func makeBusy(t *testing.T, svc *Service, actor Actor, agent, other string) Task {
	t.Helper()
	setAgent(t, svc, other, `enabled = 0`)
	busy := mustCreateDev(t, svc, actor, "other work")
	setAgent(t, svc, other, `enabled = 1`)
	if busy.Assignee != agentPrincipal(agent) {
		t.Fatalf("other work went to %q, want %s", busy.Assignee, agent)
	}
	return busy
}

// Test 1: the incident of 2026-11-03. Without the setting a task coming back
// to a pool goes to the free member, not to the busy previous holder.
func TestDispatchSkipsBusyPreviousHolderByDefault(t *testing.T) {
	t.Parallel()
	svc, actor, _ := engineFixture(t)
	task := mustCreateDev(t, svc, actor, "t")
	if task.Assignee != "agent:dev-1" {
		t.Fatalf("initial assignee = %q", task.Assignee)
	}
	wantDispatched(t, svc, task, dispatched("develop", "developers", "dev-1", "free_member", false, "", true, true))
	enter(t, svc, task.Key, "approval", "ask")
	makeBusy(t, svc, actor, "dev-1", "dev-2")
	back := enter(t, svc, task.Key, "develop", "rejected")
	if back.Assignee != "agent:dev-2" || back.Status != StatusInProgress || holderOf(t, svc, back, "developers") != "dev-2" {
		t.Fatalf("back to develop = %#v; want the free member dev-2", back)
	}
	wantDispatched(t, svc, back, dispatched("develop", "developers", "dev-2", "free_member", false, "dev-1", true, false))
}

// Test 2: a free previous holder comes first, before a free member that was
// dispatched less recently.
func TestDispatchPrefersFreePreviousHolder(t *testing.T) {
	t.Parallel()
	svc, actor, _ := engineFixture(t)
	task := mustCreateDev(t, svc, actor, "t")
	enter(t, svc, task.Key, "review", "ready")
	back := enter(t, svc, task.Key, "develop", "changes")
	if back.Assignee != "agent:dev-1" {
		t.Fatalf("back to develop = %q; want the free previous holder", back.Assignee)
	}
	wantDispatched(t, svc, back, dispatched("develop", "developers", "dev-1", "previous_holder", false, "dev-1", true, true))
}

// Test 3: with the previous holder busy, the free members are ordered by the
// least recent dispatch in the queue, then by pool order.
func TestDispatchBusyPreviousHolderPicksLeastRecentFreeMember(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, actor, _ := engineFixture(t)
	seedAgent(t, svc, "dev-3", "")
	enableAgents(t, svc)
	pool, err := svc.GetAgentPool(ctx, actor, "DEV", "developers")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebindAgentPool(ctx, actor, "DEV", "developers", []string{"dev-1", "dev-2", "dev-3"}, pool.Revision, "add-dev-3"); err != nil {
		t.Fatal(err)
	}
	task := mustCreateDev(t, svc, actor, "t")
	enter(t, svc, task.Key, "review", "ready")
	other := mustCreateDev(t, svc, actor, "other")
	if other.Assignee != "agent:dev-2" {
		t.Fatalf("other = %q; want dev-2 by pool order", other.Assignee)
	}
	enter(t, svc, other.Key, "done", "merged")
	setAgent(t, svc, "dev-1", `current_goal_task_key = 'DEV-other'`)
	back := enter(t, svc, task.Key, "develop", "changes")
	if back.Assignee != "agent:dev-3" {
		t.Fatalf("back to develop = %q; want dev-3, never dispatched", back.Assignee)
	}
	wantDispatched(t, svc, back, dispatched("develop", "developers", "dev-3", "free_member", false, "dev-1", true, true))
}

// Test 4: sticky_holder on the workflow keeps the old rule.
func TestDispatchStickyWorkflowKeepsBusyPreviousHolder(t *testing.T) {
	t.Parallel()
	svc, actor, _ := engineFixtureWith(t, stickyDefinition(true, nil))
	task := mustCreateDev(t, svc, actor, "t")
	enter(t, svc, task.Key, "approval", "ask")
	makeBusy(t, svc, actor, "dev-1", "dev-2")
	back := enter(t, svc, task.Key, "develop", "rejected")
	if back.Assignee != "agent:dev-1" {
		t.Fatalf("back to develop = %q; want the sticky previous holder", back.Assignee)
	}
	wantDispatched(t, svc, back, dispatched("develop", "developers", "dev-1", "sticky", true, "dev-1", false, true))
}

// Test 5: the status value replaces the workflow value both ways.
func TestDispatchStatusStickyHolderOverridesWorkflow(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	for name, tc := range map[string]struct {
		def         workflowfile.File
		want        string
		reason      string
		sticky      bool
		holderFree  bool
		otherIsFree bool
	}{
		"workflow true, status false": {stickyDefinition(true, &no), "agent:dev-2", "free_member", false, true, false},
		"workflow unset, status true": {stickyDefinition(false, &yes), "agent:dev-1", "sticky", true, false, true},
	} {
		t.Run(name, func(t *testing.T) {
			svc, actor, _ := engineFixtureWith(t, tc.def)
			task := mustCreateDev(t, svc, actor, "t")
			enter(t, svc, task.Key, "approval", "ask")
			makeBusy(t, svc, actor, "dev-1", "dev-2")
			back := enter(t, svc, task.Key, "develop", "rejected")
			if back.Assignee != tc.want {
				t.Fatalf("back to develop = %q; want %s", back.Assignee, tc.want)
			}
			agent := tc.want[len("agent:"):]
			wantDispatched(t, svc, back, dispatched("develop", "developers", agent, tc.reason, tc.sticky, "dev-1", tc.holderFree, tc.otherIsFree))
		})
	}
}

// Test 6: nobody is free, so the task waits unassigned; once the previous
// holder is free again, DispatchPending gives the task to it first.
func TestDispatchWaitsForFreeMemberThenPreviousHolder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, actor, _ := engineFixture(t)
	task := mustCreateDev(t, svc, actor, "t")
	enter(t, svc, task.Key, "review", "ready")
	setAgent(t, svc, "dev-1", `current_goal_task_key = 'DEV-other'`)
	setAgent(t, svc, "dev-2", `current_goal_task_key = 'DEV-another'`)
	before := countEvents(t, svc, task, "workflow.dispatched")
	back := enter(t, svc, task.Key, "develop", "changes")
	if back.Status != StatusOpen || back.Assignee != "" {
		t.Fatalf("back to develop = %#v; want open without an assignee", back)
	}
	if n := countEvents(t, svc, back, "workflow.dispatched"); n != before {
		t.Fatalf("dispatched events = %d, want %d: nobody was assigned", n, before)
	}
	setAgent(t, svc, "dev-2", `current_goal_task_key = ''`)
	setAgent(t, svc, "dev-1", `current_goal_task_key = ''`)
	if n, err := svc.DispatchPending(ctx); err != nil || n != 1 {
		t.Fatalf("dispatch = %d, %v", n, err)
	}
	stored := storedTask(t, svc, task.Key)
	if stored.Assignee != "agent:dev-1" {
		t.Fatalf("dispatched to %q; want the previous holder dev-1", stored.Assignee)
	}
	wantDispatched(t, svc, stored, dispatched("develop", "developers", "dev-1", "previous_holder", false, "dev-1", true, true))
}

// Test 7: a move between two statuses of one pool, asked by the holder whose
// Goal is this task, keeps the holder: its own Goal does not make it busy.
func TestDispatchSamePoolMoveKeepsHolderWithItsGoal(t *testing.T) {
	t.Parallel()
	svc, actor, _ := engineFixture(t)
	task := mustCreateDev(t, svc, actor, "t")
	setAgent(t, svc, "dev-1", `current_goal_task_key = '`+task.Key+`'`)
	again := enter(t, svc, task.Key, "develop", "again")
	if again.Assignee != "agent:dev-1" {
		t.Fatalf("same-pool move = %q; want the holder dev-1", again.Assignee)
	}
	wantDispatched(t, svc, again, dispatched("develop", "developers", "dev-1", "previous_holder", false, "dev-1", true, true))
}

// Test 8: a released holder is never picked, also with sticky_holder on.
func TestDispatchStickyNeverPicksReleasedHolder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, actor, _ := engineFixtureWith(t, stickyDefinition(true, nil))
	task := mustCreateDev(t, svc, actor, "t")
	pause(t, svc, task.Key, PauseIdleIterations, "")
	resumed, err := svc.ResumeWorkflow(ctx, actor, task.Key, ResumeRelease)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Assignee != "agent:dev-2" {
		t.Fatalf("released = %q; want dev-2", resumed.Assignee)
	}
	wantDispatched(t, svc, resumed, dispatched("develop", "developers", "dev-2", "free_member", true, "", true, false))
}

// An upgrade enters the new image's status with the new image's sticky_holder.
func TestUpgradeAppliesTheNewImageStickyHolder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, actor, task := requestFixture(t)
	def := requestDefinition()
	def.WorkflowVersion, def.StickyHolder = "0.2.0", true
	seedEngineImage(t, svc, def)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "development:0.2.0", 1); err != nil {
		t.Fatal(err)
	}
	setAgent(t, svc, "dev-1", `current_goal_task_key = 'DEV-other'`)
	upgraded, err := svc.UpgradeWorkflow(ctx, actor, task.Key, "")
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.Assignee != "agent:dev-1" {
		t.Fatalf("upgraded = %q; want the sticky previous holder", upgraded.Assignee)
	}
	wantDispatched(t, svc, upgraded, dispatched("develop", "developers", "dev-1", "sticky", true, "dev-1", false, true))
}

// The activity feed reads a task with workflow.dispatched events: the event
// counts in the window and adds no agent, since system:workflow wrote it.
func TestTaskActivityReadsDispatchedEvents(t *testing.T) {
	t.Parallel()
	svc, customer, _ := engineFixture(t)
	task := mustCreateDev(t, svc, customer, "t")
	if countEvents(t, svc, task, "workflow.dispatched") != 1 {
		t.Fatal("no workflow.dispatched event")
	}
	got, err := svc.TaskActivity(context.Background(), customer, ActivityFilter{Keys: []string{task.Key}})
	if err != nil || len(got) != 1 || len(got[0].Agents) != 0 {
		t.Fatalf("activity = %#v, %v", got, err)
	}
}
