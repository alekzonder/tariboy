package tasks

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

func seedWorkflowImage(t *testing.T, svc *Service, digest string) workflowimage.Manifest {
	t.Helper()
	m := workflowimage.Manifest{
		SchemaVersion: 1, Name: "dev-flow", Version: "1.2.0", Digest: digest, BuiltAt: "2026-10-02T00:00:00Z",
		Definition: workflowfile.File{
			Name: "dev-flow", WorkflowVersion: "1.2.0", InitialStatus: "build",
			Statuses: []workflowfile.Status{
				{ID: "build", Owner: workflowfile.Owner{Kind: workflowfile.OwnerPool, Pool: "devs"}},
				{ID: "approve", Owner: workflowfile.Owner{Kind: workflowfile.OwnerCustomer}},
				{ID: "verify", Owner: workflowfile.Owner{Kind: workflowfile.OwnerScript}},
				{ID: "shipped", Terminal: true},
			},
		},
	}
	storeManifest(t, svc, m)
	return m
}

// seedWorkflowTask creates a flexible task and rewrites its row into a workflow
// task, because nothing in the engine sets workflow_digest yet.
func seedWorkflowTask(t *testing.T, svc *Service, actor Actor, digest, category, status, paused string) Task {
	t.Helper()
	task, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "wf " + status})
	if err != nil {
		t.Fatal(err)
	}
	forceTask(t, svc, task, `status = ?, workflow_digest = ?, workflow_status = ?, workflow_paused_reason = ?`,
		category, digest, status, paused)
	got, err := taskByID(context.Background(), svc.db, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func jsonFields(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTaskMarshalFlexibleTaskReportsStatusAsCategory(t *testing.T) {
	svc, actor := workflowFixture(t)
	task, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "flex"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := taskByID(context.Background(), svc.db, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusOpen || got.Category != StatusOpen || got.WorkflowDigest != "" || got.WaitingOn != "" {
		t.Fatalf("scanned flexible task = %#v", got)
	}
	f := jsonFields(t, got)
	if f["status"] != "open" || f["category"] != "open" {
		t.Fatalf("status/category = %v/%v; want open/open", f["status"], f["category"])
	}
	for _, absent := range []string{"waiting_on", "workflow_digest", "workflow_name", "workflow_version", "workflow_paused_reason"} {
		if _, ok := f[absent]; ok {
			t.Fatalf("flexible task JSON has %s", absent)
		}
	}
}

func TestTaskScanFillsWorkflowFieldsAndWaitingOn(t *testing.T) {
	svc, actor := workflowFixture(t)
	const digest = "d1"
	seedWorkflowImage(t, svc, digest)
	cases := []struct {
		name, category, status, paused string
		wantWaiting                    string
	}{
		{"pool without holder", StatusOpen, "build", "", ""},
		{"pool with holder", StatusInProgress, "build", "", ""},
		{"pool holder question", StatusWaitCustomer, "build", "", WaitingOnCustomer},
		{"customer", StatusWaitCustomer, "approve", "", WaitingOnCustomer},
		{"script", StatusWaitCustomer, "verify", "", WaitingOnScript},
		{"paused", StatusWaitCustomer, "approve", "operator hold", WaitingOnPause},
		{"unknown status", StatusWaitCustomer, "gone", "", ""},
		{"terminal", StatusDone, "shipped", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := seedWorkflowTask(t, svc, actor, digest, tc.category, tc.status, tc.paused)
			if got.Status != tc.category || got.Category != tc.category || got.WorkflowStatus != tc.status {
				t.Fatalf("status/category/workflow = %q/%q/%q", got.Status, got.Category, got.WorkflowStatus)
			}
			if got.WorkflowDigest != digest || got.WorkflowName != "dev-flow" || got.WorkflowVersion != "1.2.0" {
				t.Fatalf("workflow identity = %q/%q/%q", got.WorkflowDigest, got.WorkflowName, got.WorkflowVersion)
			}
			if got.WaitingOn != tc.wantWaiting || got.WorkflowPausedReason != tc.paused {
				t.Fatalf("waiting_on/paused = %q/%q; want %q/%q", got.WaitingOn, got.WorkflowPausedReason, tc.wantWaiting, tc.paused)
			}
			f := jsonFields(t, got)
			if f["status"] != tc.status || f["category"] != tc.category {
				t.Fatalf("JSON status/category = %v/%v; want %s/%s", f["status"], f["category"], tc.status, tc.category)
			}
			if tc.wantWaiting == "" {
				if _, ok := f["waiting_on"]; ok {
					t.Fatal("JSON has waiting_on")
				}
			} else if f["waiting_on"] != tc.wantWaiting {
				t.Fatalf("JSON waiting_on = %v", f["waiting_on"])
			}
			if f["workflow_name"] != "dev-flow" || f["workflow_version"] != "1.2.0" || f["workflow_digest"] != digest {
				t.Fatalf("JSON workflow identity = %v", f)
			}
		})
	}
}

func TestTaskJSONRoundTrip(t *testing.T) {
	svc, actor := workflowFixture(t)
	seedWorkflowImage(t, svc, "d1")
	flexible, err := svc.CreateTask(context.Background(), actor, CreateTaskInput{Queue: "DEV", Title: "flex"})
	if err != nil {
		t.Fatal(err)
	}
	flexible, err = taskByID(context.Background(), svc.db, flexible.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []Task{
		flexible,
		seedWorkflowTask(t, svc, actor, "d1", StatusWaitCustomer, "approve", ""),
		seedWorkflowTask(t, svc, actor, "d1", StatusInProgress, "build", ""),
	} {
		raw, err := json.Marshal(task)
		if err != nil {
			t.Fatal(err)
		}
		var back Task
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatal(err)
		}
		task.ID = 0 // not part of the JSON form
		if !reflect.DeepEqual(back, task) {
			t.Fatalf("round trip\n got %#v\nwant %#v", back, task)
		}
	}
}

// A list row carries a summary of the task's workflow trouble and position,
// computed by the task select itself so a page of tasks costs no extra
// queries.
func TestTaskScanWorkflowAttentionAndStep(t *testing.T) {
	svc, actor, task, _ := runFixture(t)
	ctx := context.Background()
	stored := storedTask(t, svc, task.Key)
	if stored.WorkflowAttention != nil || !reflect.DeepEqual(stored.WorkflowStep, &WorkflowStep{Index: 2, Total: 6}) {
		t.Fatalf("fresh task = %#v %#v", stored.WorkflowAttention, stored.WorkflowStep)
	}
	fields := jsonFields(t, stored)
	if _, ok := fields["workflow_attention"]; ok || fields["workflow_step"] == nil {
		t.Fatalf("fresh JSON = %v", fields)
	}

	// An idle holder is the quietest signal.
	forceOpenVisit(t, svc, task, `idle_iterations = 2`)
	if got := storedTask(t, svc, task.Key).WorkflowAttention; got == nil || got.Kind != "idle" || got.Count != 2 || got.Message == "" {
		t.Fatalf("idle = %#v", got)
	}

	// A rejected request names the check's message and the request.
	request, first := advanceApprove(t, svc, actor, task)
	if got := storedTask(t, svc, task.Key).WorkflowAttention; got != nil {
		t.Fatalf("pending request = %#v; a pending request is not trouble", got)
	}
	complete(t, svc, first.ID, RunCompletion{Verdict: "reject", Message: "CI is red\nsee the log", ExitCode: exitCode(112)})
	got := storedTask(t, svc, task.Key).WorkflowAttention
	if got == nil || got.Kind != "rejected" || got.Message != "CI is red" || got.Count != 1 || got.RequestID != request.ID || got.RunID != first.ID {
		t.Fatalf("rejected = %#v", got)
	}

	// A failed request outranks the rejection before it and drops the log line
	// of its result message.
	request, first = advanceApprove(t, svc, actor, task)
	complete(t, svc, first.ID, RunCompletion{Verdict: "failure", Message: "the script failed with exit 3", ExitCode: exitCode(3), LogPath: "/base/runs/1/run.log"})
	got = storedTask(t, svc, task.Key).WorkflowAttention
	if got == nil || got.Kind != "failed" || got.Message != "the script failed with exit 3" || got.Count != 1 ||
		got.RequestID != request.ID || got.RunID != first.ID {
		t.Fatalf("failed = %#v", got)
	}

	// A pause outranks everything and says why.
	paused := pause(t, svc, task.Key, PauseScriptFailures, "boom")
	if got := paused.WorkflowAttention; got == nil || got.Kind != "paused" || got.Message != pauseReasonText(PauseScriptFailures) {
		t.Fatalf("paused = %#v", got)
	}
	if _, err := svc.ResumeWorkflow(ctx, actor, task.Key, "continue"); err != nil {
		t.Fatal(err)
	}
	// A resumed visit starts its counters again: the failed request is still
	// the latest one of the visit, but the pause is over.
	if got := storedTask(t, svc, task.Key).WorkflowAttention; got == nil || got.Kind != "failed" {
		t.Fatalf("resumed = %#v", got)
	}

	// A watch that fails in a script status is reported with its run.
	task = enter(t, svc, task.Key, "merge", "approved")
	run := scheduleWatch(t, svc, actor, task)
	if got := storedTask(t, svc, task.Key).WorkflowAttention; got != nil {
		t.Fatalf("new visit = %#v; the failed request belongs to the old visit", got)
	}
	complete(t, svc, run.ID, RunCompletion{Verdict: "failure", Message: "GitHub answered 502"})
	got = storedTask(t, svc, task.Key).WorkflowAttention
	if got == nil || got.Kind != "script_failed" || got.Message != "GitHub answered 502" || got.Count != 1 || got.RunID != run.ID || got.RequestID != 0 {
		t.Fatalf("script_failed = %#v", got)
	}
	if step := storedTask(t, svc, task.Key).WorkflowStep; !reflect.DeepEqual(step, &WorkflowStep{Index: 4, Total: 6}) {
		t.Fatalf("step = %#v", step)
	}

	// A closed task has neither.
	if _, err := svc.CancelWorkflowTask(ctx, actor, task.Key); err != nil {
		t.Fatal(err)
	}
	closed := storedTask(t, svc, task.Key)
	if closed.WorkflowAttention != nil || closed.WorkflowStep != nil {
		t.Fatalf("closed = %#v %#v", closed.WorkflowAttention, closed.WorkflowStep)
	}

	// A flexible task has neither field in its JSON.
	flexible, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "TASK", Title: "flex"})
	if err != nil {
		t.Fatal(err)
	}
	fields = jsonFields(t, storedTask(t, svc, flexible.Key))
	if _, ok := fields["workflow_attention"]; ok {
		t.Fatalf("flexible JSON = %v", fields)
	}
	if _, ok := fields["workflow_step"]; ok {
		t.Fatalf("flexible JSON = %v", fields)
	}
}

// The attention summary and the step reach a list page, not only a single
// task read.
func TestListTasksCarriesWorkflowAttention(t *testing.T) {
	svc, actor, task, _ := runFixture(t)
	_, first := advanceApprove(t, svc, actor, task)
	complete(t, svc, first.ID, RunCompletion{Verdict: "reject", Message: "CI is red", ExitCode: exitCode(112)})
	page, err := svc.ListTasks(context.Background(), actor, ListFilter{Queue: "DEV"})
	if err != nil {
		t.Fatal(err)
	}
	var found *Task
	for i := range page.Tasks {
		if page.Tasks[i].Key == task.Key {
			found = &page.Tasks[i]
		}
	}
	if found == nil || found.WorkflowAttention == nil || found.WorkflowAttention.Kind != "rejected" || found.WorkflowStep == nil {
		t.Fatalf("listed = %#v", found)
	}
}

// A list row marks an open workflow task that is still in its workflow's
// initial status, so a list can show it as not started.
func TestListTasksMarksWorkflowInitialStatus(t *testing.T) {
	svc, actor, later, _ := runFixture(t)
	fresh := mustCreateDev(t, svc, actor, "fresh")
	page, err := svc.ListTasks(context.Background(), actor, ListFilter{Queue: "DEV"})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]Task{}
	for _, task := range page.Tasks {
		listed[task.Key] = task
	}
	if got, ok := listed[fresh.Key]; !ok || !got.WorkflowInitial {
		t.Fatalf("initial task = %#v", got)
	}
	if fields := jsonFields(t, listed[fresh.Key]); fields["workflow_initial"] != true {
		t.Fatalf("initial JSON = %v", fields)
	}
	if got, ok := listed[later.Key]; !ok || got.WorkflowInitial {
		t.Fatalf("later task = %#v", got)
	}
	if _, ok := jsonFields(t, listed[later.Key])["workflow_initial"]; ok {
		t.Fatalf("later JSON carries workflow_initial")
	}
	if _, err := svc.CancelWorkflowTask(context.Background(), actor, fresh.Key); err != nil {
		t.Fatal(err)
	}
	if storedTask(t, svc, fresh.Key).WorkflowInitial {
		t.Fatalf("a closed task is not in its initial status")
	}
}

// The task select reaches the open visit, its latest request, and its latest
// run through indexes, so a page of tasks does not scan those tables.
func TestTaskSelectUsesIndexes(t *testing.T) {
	svc, _ := workflowFixture(t)
	rows, err := svc.db.Query(`EXPLAIN QUERY PLAN `+taskSelect+` WHERE t.id = ?`, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, detail := range plan {
		for _, table := range []string{"task_status_visits", "task_transition_requests", "task_script_runs"} {
			if strings.HasPrefix(detail, "SCAN "+table) || strings.Contains(detail, "SCAN "+table+" ") {
				t.Errorf("plan scans %s: %q\n%s", table, detail, strings.Join(plan, "\n"))
			}
		}
	}
}

func TestLoadManifestTx(t *testing.T) {
	svc, _ := workflowFixture(t)
	want := seedWorkflowImage(t, svc, "d1")
	got, err := loadManifestTx(context.Background(), svc.db, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.Digest != "d1" || len(got.Definition.Statuses) != 4 {
		t.Fatalf("manifest = %#v", got)
	}
	_, err = loadManifestTx(context.Background(), svc.db, "missing")
	e, ok := err.(*Error)
	if !ok || e.Code != "workflow_not_found" || e.Status != 409 {
		t.Fatalf("missing digest error = %#v; want workflow_not_found 409", err)
	}
}
