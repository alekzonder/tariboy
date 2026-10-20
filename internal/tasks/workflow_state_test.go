package tasks

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// Each row is a row of the category table in
// docs/docs/task-workflows.mdx#status-and-category.
func TestDeriveCategory(t *testing.T) {
	const holder, other = "agent:h", "agent:o"
	pool := workflowfile.OwnerPool
	for _, tc := range []struct {
		name                         string
		in                           taskState
		category, assignee, waitedOn string
	}{
		{"pool with a holder", taskState{Owner: pool, Holder: holder, Assignee: other}, StatusInProgress, holder, ""},
		{"pool without an eligible agent", taskState{Owner: pool, Assignee: other}, StatusOpen, "", ""},
		{"pool, the holder asked the customer", taskState{Owner: pool, Holder: holder, HolderAsks: true}, StatusWaitCustomer, holder, WaitingOnCustomer},
		{"customer status", taskState{Owner: workflowfile.OwnerCustomer, Assignee: other}, StatusWaitCustomer, other, WaitingOnCustomer},
		{"script status", taskState{Owner: workflowfile.OwnerScript, Assignee: other}, StatusWaitCustomer, "", WaitingOnScript},
		{"paused pool status", taskState{Owner: pool, Paused: true, Holder: holder, Assignee: other}, StatusWaitCustomer, other, WaitingOnPause},
		{"paused script status", taskState{Owner: workflowfile.OwnerScript, Paused: true, Assignee: other}, StatusWaitCustomer, other, WaitingOnPause},
		{"paused status the manifest no longer declares", taskState{Paused: true, Assignee: other}, StatusWaitCustomer, other, WaitingOnPause},
		{"terminal status", taskState{Terminal: true, Assignee: other}, StatusDone, other, ""},
		{"cancelled terminal status", taskState{Terminal: true, Cancelled: true, Assignee: other}, StatusCancelled, other, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			category, assignee, waited, err := deriveCategory(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if category != tc.category || assignee != tc.assignee || waited != tc.waitedOn {
				t.Fatalf("got %s/%q/%q; want %s/%q/%q", category, assignee, waited, tc.category, tc.assignee, tc.waitedOn)
			}
			if got := waitingOn(category, tc.in.Owner, tc.in.Paused); got != waited {
				t.Fatalf("stored waiting_on %q differs from derived %q", got, waited)
			}
		})
	}
	if _, _, _, err := deriveCategory(taskState{Status: "x"}); err == nil {
		t.Fatal("a status without an owner derived a category")
	}
}

// Entering a status and resuming a pause share one holder rule: a paused or
// blocked pool status picks nobody, which once differed between the two.
func TestPicksHolderOnlyForAnUnpausedUnblockedPoolStatus(t *testing.T) {
	status := workflowfile.Status{ID: "build", Owner: workflowfile.Owner{Kind: workflowfile.OwnerPool, Pool: "dev"}}
	for _, tc := range []struct {
		name  string
		task  Task
		picks bool
	}{
		{"free", Task{}, true},
		{"paused", Task{WorkflowPausedReason: PauseIdleIterations}, false},
		{"blocked", Task{Blocked: true}, false},
		{"paused and blocked", Task{WorkflowPausedReason: PauseIdleIterations, Blocked: true}, false},
	} {
		if got := stateIn(tc.task, status).picksHolder(); got != tc.picks {
			t.Fatalf("%s: picksHolder = %v; want %v", tc.name, got, tc.picks)
		}
	}
	customer := workflowfile.Status{ID: "approve", Owner: workflowfile.Owner{Kind: workflowfile.OwnerCustomer}}
	if stateIn(Task{}, customer).picksHolder() {
		t.Fatal("a customer status picked a holder")
	}
	terminal := workflowfile.Status{ID: "done", Terminal: true, Owner: workflowfile.Owner{Kind: workflowfile.OwnerPool, Pool: "dev"}}
	if stateIn(Task{}, terminal).picksHolder() {
		t.Fatal("a terminal status picked a holder")
	}
}
