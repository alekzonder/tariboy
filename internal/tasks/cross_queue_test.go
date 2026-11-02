package tasks

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// crossQueueTree is a TARI parent run by lead, with a same-queue child and a
// DESIGN child run by designer. writer is assigned the parent and authored the
// DESIGN child but runs neither queue.
type crossQueueTree struct {
	svc                    *Service
	parent, sameChild, kid Task
	lead, designer, writer Actor
	customer               Actor
}

func newCrossQueueTree(t *testing.T) crossQueueTree {
	t.Helper()
	svc := newTestService(t)
	ctx := context.Background()
	customer := CustomerActor("customer")
	for _, q := range []CreateQueueInput{
		{Prefix: "TARI", Name: "Tari", Owners: []string{"lead"}},
		{Prefix: "DESIGN", Name: "Design", Owners: []string{"designer"}},
	} {
		if _, err := svc.CreateQueue(ctx, customer, q); err != nil {
			t.Fatal(err)
		}
	}
	parent, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "TARI", Title: "parent", Assignee: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	sameChild, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: parent.Key, Title: "same queue child"})
	if err != nil {
		t.Fatal(err)
	}
	kid, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "DESIGN", Title: "design child"})
	if err != nil {
		t.Fatal(err)
	}
	// Built by hand so the access rule is tested apart from create and move.
	forceTask(t, svc, kid, `parent_id = ?, author = 'agent:writer'`, parent.ID)
	return crossQueueTree{
		svc: svc, parent: parent, sameChild: sameChild, kid: kid,
		lead: AgentActor("lead"), designer: AgentActor("designer"), writer: AgentActor("writer"),
		customer: customer,
	}
}

func TestCrossQueueChildHiddenFromParentQueueOwner(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	if _, err := tr.svc.GetTask(ctx, tr.lead, tr.kid.Key); ErrorCode(err) != "not_found" {
		t.Fatalf("lead show = %v; want not_found", err)
	}
	title := "renamed"
	if _, err := tr.svc.UpdateTask(ctx, tr.lead, tr.kid.Key, UpdateTaskInput{Title: &title, Revision: tr.kid.Revision}); ErrorCode(err) != "not_found" {
		t.Fatalf("lead update = %v; want not_found", err)
	}
	if _, err := tr.svc.AddComment(ctx, tr.lead, tr.kid.Key, AddCommentInput{Body: "hi"}); ErrorCode(err) != "not_found" {
		t.Fatalf("lead comment = %v; want not_found", err)
	}
	// Inheritance inside one queue is unchanged.
	detail, err := tr.svc.GetTask(ctx, tr.lead, tr.sameChild.Key)
	if err != nil || detail.Task.Access != "write" {
		t.Fatalf("lead same-queue child = %q, %v; want write", detail.Task.Access, err)
	}
}

func TestCrossQueueParentHiddenFromChildQueueOwner(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	detail, err := tr.svc.GetTask(ctx, tr.designer, tr.kid.Key)
	if err != nil || detail.Task.Access != "write" {
		t.Fatalf("designer child = %q, %v; want write", detail.Task.Access, err)
	}
	if detail.Task.ParentKey != tr.parent.Key {
		t.Fatalf("parent key = %q; want %q", detail.Task.ParentKey, tr.parent.Key)
	}
	if _, err := tr.svc.GetTask(ctx, tr.designer, tr.parent.Key); ErrorCode(err) != "not_found" {
		t.Fatalf("designer parent = %v; want not_found", err)
	}
}

func TestCrossQueueChildAuthorReadsOnly(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	detail, err := tr.svc.GetTask(ctx, tr.writer, tr.kid.Key)
	if err != nil || detail.Task.Access != "context" {
		t.Fatalf("author child = %q, %v; want context", detail.Task.Access, err)
	}
	title := "renamed"
	if _, err := tr.svc.UpdateTask(ctx, tr.writer, tr.kid.Key, UpdateTaskInput{Title: &title, Revision: tr.kid.Revision}); ErrorCode(err) != "not_found" {
		t.Fatalf("author update = %v; want not_found", err)
	}
	if _, err := tr.svc.AddComment(ctx, tr.writer, tr.kid.Key, AddCommentInput{Body: "hi"}); ErrorCode(err) != "not_found" {
		t.Fatalf("author comment = %v; want not_found", err)
	}
	// The author still writes the parent it is assigned.
	if parent, err := tr.svc.GetTask(ctx, tr.writer, tr.parent.Key); err != nil || parent.Task.Access != "write" {
		t.Fatalf("author parent = %q, %v; want write", parent.Task.Access, err)
	}
}

func TestCrossQueueChildAssigneeWritesAndSeesNoParent(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	forceTask(t, tr.svc, tr.kid, `assignee = 'agent:dev'`)
	dev := AgentActor("dev")
	detail, err := tr.svc.GetTask(ctx, dev, tr.kid.Key)
	if err != nil || detail.Task.Access != "write" {
		t.Fatalf("assignee child = %q, %v; want write", detail.Task.Access, err)
	}
	if _, err := tr.svc.GetTask(ctx, dev, tr.parent.Key); ErrorCode(err) != "not_found" {
		t.Fatalf("assignee parent = %v; want not_found", err)
	}
}

func TestCrossQueueShowCountsVisibleDescendantsCompletionCountsAll(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	detail, err := tr.svc.GetTask(ctx, tr.lead, tr.parent.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Descendants != 1 || detail.ActiveDescendants != 1 {
		t.Fatalf("lead counts = %d/%d; want 1/1 without the hidden child", detail.Descendants, detail.ActiveDescendants)
	}
	full, err := tr.svc.GetTask(ctx, tr.customer, tr.parent.Key)
	if err != nil || full.Descendants != 2 {
		t.Fatalf("customer descendants = %d, %v; want 2", full.Descendants, err)
	}
	done := StatusDone
	if _, err := tr.svc.UpdateTask(ctx, tr.lead, tr.sameChild.Key, UpdateTaskInput{Status: &done, Revision: tr.sameChild.Revision}); err != nil {
		t.Fatal(err)
	}
	_, err = tr.svc.UpdateTask(ctx, tr.lead, tr.parent.Key, UpdateTaskInput{Status: &done, Revision: tr.parent.Revision})
	if ErrorCode(err) != "active_descendants" {
		t.Fatalf("complete parent = %v; want active_descendants from the hidden child", err)
	}
}

func TestCreateChildInAnotherQueueByItsOwner(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	forceTask(t, tr.svc, tr.parent, `assignee = 'agent:designer', group_name = 'tari'`)
	child, err := tr.svc.CreateTask(ctx, tr.designer, CreateTaskInput{
		ParentKey: tr.parent.Key, Queue: "design", Title: "mockups", Assignee: "painter", Group: "art",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Queue != "DESIGN" || !strings.HasPrefix(child.Key, "DESIGN-") || child.ParentKey != tr.parent.Key {
		t.Fatalf("child = %s in %s under %s; want DESIGN under %s", child.Key, child.Queue, child.ParentKey, tr.parent.Key)
	}
	if child.Customer != tr.parent.Customer || child.Group != "art" || child.Assignee != "agent:painter" || child.Filed || child.Access != "write" {
		t.Fatalf("child scope = %#v", child)
	}
	// Without --group a child in another queue does not take the parent's group.
	plain, err := tr.svc.CreateTask(ctx, tr.designer, CreateTaskInput{ParentKey: tr.parent.Key, Queue: "DESIGN", Title: "plain"})
	if err != nil || plain.Group != "" {
		t.Fatalf("plain child group = %q, %v; want empty", plain.Group, err)
	}
	// The parent's queue keeps today's behavior: the child takes the parent's group.
	same, err := tr.svc.CreateTask(ctx, tr.designer, CreateTaskInput{ParentKey: tr.parent.Key, Queue: "TARI", Title: "same"})
	if err != nil || same.Queue != "TARI" || same.Group != "tari" {
		t.Fatalf("same-queue child = %s/%q, %v; want TARI/tari", same.Queue, same.Group, err)
	}
}

func TestCreateChildInAnotherQueueIsFiledByAnOutsider(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	child, err := tr.svc.CreateTask(ctx, tr.writer, CreateTaskInput{ParentKey: tr.parent.Key, Queue: "DESIGN", Title: "filed"})
	if err != nil {
		t.Fatal(err)
	}
	if !child.Filed || child.Access != "context" || child.Assignee != "" || child.Group != "" || child.Queue != "DESIGN" {
		t.Fatalf("filed child = %#v", child)
	}
	if detail, err := tr.svc.GetTask(ctx, tr.writer, child.Key); err != nil || detail.Task.Access != "context" {
		t.Fatalf("author reads filed child = %v; want context", err)
	}
	page, err := tr.svc.ListTasks(ctx, tr.writer, ListFilter{SubtreeKey: tr.parent.Key})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, task := range page.Tasks {
		if task.Key == child.Key {
			found = task.Queue == "DESIGN" && task.Status == StatusOpen
		}
	}
	if !found {
		t.Fatalf("author subtree = %#v; want the DESIGN child with queue and status", page.Tasks)
	}
	// The parent queue's owner does not see the filed child.
	page, err = tr.svc.ListTasks(ctx, tr.lead, ListFilter{SubtreeKey: tr.parent.Key})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range page.Tasks {
		if task.Queue != "TARI" {
			t.Fatalf("lead subtree has %s from %s", task.Key, task.Queue)
		}
	}
	events, err := tr.svc.ListEvents(ctx, tr.lead, "", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.TaskKey == child.Key {
			t.Fatalf("lead sees event %s of the hidden child", event.Kind)
		}
	}
}

func TestCreateChildInAnotherQueueWithAssigneeOrGroupByAnOutsider(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	child, err := tr.svc.CreateTask(ctx, tr.writer, CreateTaskInput{
		ParentKey: tr.parent.Key, Queue: "DESIGN", Title: "assigned", Assignee: "painter",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Filed || child.Assignee != "agent:painter" || child.Access != "context" {
		t.Fatalf("assigned child = %#v", child)
	}
	if _, err := tr.svc.CreateTask(ctx, tr.writer, CreateTaskInput{
		ParentKey: tr.parent.Key, Queue: "DESIGN", Title: "grouped", Group: "art",
	}); ErrorCode(err) != "report_cannot_assign" {
		t.Fatalf("grouped child = %v; want report_cannot_assign", err)
	}
}

func TestCreateChildInAnotherQueueNeedsWriteOnParent(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	// The designer runs DESIGN but cannot write the TARI parent.
	if _, err := tr.svc.CreateTask(ctx, tr.designer, CreateTaskInput{
		ParentKey: tr.parent.Key, Queue: "DESIGN", Title: "x",
	}); ErrorCode(err) != "not_found" {
		t.Fatalf("create = %v; want not_found", err)
	}
	if _, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{
		ParentKey: tr.parent.Key, Queue: "NOPE", Title: "x",
	}); ErrorCode(err) != "queue_not_found" {
		t.Fatalf("create in a missing queue = %v; want queue_not_found", err)
	}
}

func TestCrossQueueChildFollowsItsOwnQueueWorkflow(t *testing.T) {
	svc, customer, devTask := requestFixture(t)
	ctx := context.Background()
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "TARI", Name: "Tari"}); err != nil {
		t.Fatal(err)
	}
	lead, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "TARI", Title: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTask(ctx, customer, CreateTaskInput{
		ParentKey: lead.Key, Queue: "DEV", Title: "assigned", Assignee: "dev-2",
	}); ErrorCode(err) != "workflow_managed" {
		t.Fatalf("explicit assignee in a workflow queue = %v; want workflow_managed", err)
	}
	child, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: lead.Key, Queue: "DEV", Title: "dev child"})
	if err != nil {
		t.Fatal(err)
	}
	if child.Queue != "DEV" || child.ParentKey != lead.Key || child.WorkflowDigest == "" || child.WorkflowStatus != "develop" {
		t.Fatalf("workflow child = %#v; want DEV in its initial status under %s", child, lead.Key)
	}
	done := StatusDone
	if _, err := svc.UpdateTask(ctx, customer, lead.Key, UpdateTaskInput{Status: &done, Revision: lead.Revision}); ErrorCode(err) != "active_descendants" {
		t.Fatalf("complete flexible parent = %v; want active_descendants", err)
	}
	if _, err := svc.CompleteTask(ctx, customer, child.Key, CompleteInput{Revision: child.Revision}); ErrorCode(err) != "workflow_managed" {
		t.Fatalf("done on the workflow child = %v; want workflow_managed", err)
	}
	flexible, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: devTask.Key, Queue: "TARI", Title: "flexible child"})
	if err != nil {
		t.Fatal(err)
	}
	if flexible.Queue != "TARI" || flexible.WorkflowDigest != "" || flexible.Status != StatusOpen || flexible.ParentKey != devTask.Key {
		t.Fatalf("flexible child = %#v", flexible)
	}
	if got := taskDetail(t, svc, devTask).Task; got.WorkflowStatus != "develop" || got.Assignee != "agent:dev-1" {
		t.Fatalf("workflow parent = %s/%s; want unchanged", got.WorkflowStatus, got.Assignee)
	}
}

func TestMoveUnderParentInAnotherQueueKeepsQueueAndKey(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	loose, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{Queue: "DESIGN", Title: "loose", Group: "art"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{ParentKey: loose.Key, Queue: "TARI", Title: "grandchild"})
	if err != nil {
		t.Fatal(err)
	}
	// The group differs from the TARI parent's, which only a same-queue parent refuses.
	forceTask(t, tr.svc, tr.parent, `group_name = 'tari'`)
	moved, err := tr.svc.MoveTask(ctx, tr.customer, loose.Key, MoveInput{ParentKey: tr.parent.Key, Revision: loose.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if moved.Key != loose.Key || moved.Queue != "DESIGN" || moved.ParentKey != tr.parent.Key || moved.Group != "art" {
		t.Fatalf("moved = %s in %s under %s group %q", moved.Key, moved.Queue, moved.ParentKey, moved.Group)
	}
	if got := taskDetail(t, tr.svc, grandchild).Task; got.ParentKey != loose.Key {
		t.Fatalf("grandchild parent = %s; want %s", got.ParentKey, loose.Key)
	}
	sameQueue, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{Queue: "TARI", Title: "same", Group: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.svc.MoveTask(ctx, tr.customer, sameQueue.Key, MoveInput{ParentKey: tr.parent.Key, Revision: sameQueue.Revision}); ErrorCode(err) != "scope_change" {
		t.Fatalf("same-queue group change = %v; want scope_change", err)
	}
}

func TestMoveUnderParentInAnotherQueueChecksAccessAndCustomer(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	loose, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{Queue: "DESIGN", Title: "loose"})
	if err != nil {
		t.Fatal(err)
	}
	// The designer writes the DESIGN task but not the TARI parent.
	if _, err := tr.svc.MoveTask(ctx, tr.designer, loose.Key, MoveInput{ParentKey: tr.parent.Key, Revision: loose.Revision}); ErrorCode(err) != "not_found" {
		t.Fatalf("move without write on the parent = %v; want not_found", err)
	}
	forceTask(t, tr.svc, loose, `customer = 'user:someone'`)
	if _, err := tr.svc.MoveTask(ctx, tr.customer, loose.Key, MoveInput{ParentKey: tr.parent.Key, Revision: loose.Revision}); ErrorCode(err) != "scope_change" {
		t.Fatalf("move to another customer's parent = %v; want scope_change", err)
	}
}

func TestMoveBeforeTaskAcrossQueues(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	loose, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{Queue: "DESIGN", Title: "loose"})
	if err != nil {
		t.Fatal(err)
	}
	// A before task of the moved task's queue under a TARI parent moves it there.
	moved, err := tr.svc.MoveTask(ctx, tr.customer, loose.Key, MoveInput{BeforeKey: tr.kid.Key, Revision: loose.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if moved.ParentKey != tr.parent.Key || moved.Position != 0 {
		t.Fatalf("moved = under %s at %d; want under %s at 0", moved.ParentKey, moved.Position, tr.parent.Key)
	}
	_, err = tr.svc.MoveTask(ctx, tr.customer, moved.Key, MoveInput{BeforeKey: tr.sameChild.Key, Revision: moved.Revision})
	if ErrorCode(err) != "invalid_before" || !strings.Contains(err.Error(), "moved task's queue") {
		t.Fatalf("before a task of another queue = %v; want invalid_before", err)
	}
}

func TestDetachFromParentInAnotherQueueNeedsWriteOnParent(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	if _, err := tr.svc.MoveTask(ctx, tr.designer, tr.kid.Key, MoveInput{Revision: tr.kid.Revision}); ErrorCode(err) != "not_found" {
		t.Fatalf("designer detach = %v; want not_found without write on the parent", err)
	}
	root, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{Queue: "DESIGN", Title: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.svc.MoveTask(ctx, tr.designer, tr.kid.Key, MoveInput{BeforeKey: root.Key, Revision: tr.kid.Revision}); ErrorCode(err) != "not_found" {
		t.Fatalf("designer detach before a root = %v; want not_found", err)
	}
	// The designer also writes the TARI parent once assigned to it.
	forceTask(t, tr.svc, tr.parent, `assignee = 'agent:designer'`)
	moved, err := tr.svc.MoveTask(ctx, tr.designer, tr.kid.Key, MoveInput{Revision: tr.kid.Revision})
	if err != nil || moved.ParentKey != "" {
		t.Fatalf("detach with write on both = %v, parent %q", err, moved.ParentKey)
	}
	// A same-queue parent keeps today's rule: write on the task is enough.
	child, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{ParentKey: root.Key, Title: "same"})
	if err != nil {
		t.Fatal(err)
	}
	forceTask(t, tr.svc, child, `assignee = 'agent:painter'`)
	child = taskDetail(t, tr.svc, child).Task
	if _, err := tr.svc.MoveTask(ctx, AgentActor("painter"), child.Key, MoveInput{Revision: child.Revision}); err != nil {
		t.Fatalf("same-queue detach = %v", err)
	}
}

func TestExportRefusesSubtreeWithAnotherQueue(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	_, err := tr.svc.ExportTask(ctx, tr.customer, tr.parent.Key)
	if ErrorCode(err) != "cross_queue_subtree" {
		t.Fatalf("export = %v; want cross_queue_subtree", err)
	}
	var taskErr *Error
	if !errors.As(err, &taskErr) || taskErr.Status != http.StatusConflict {
		t.Fatalf("export error = %#v; want 409", err)
	}
	if _, err := tr.svc.ExportTask(ctx, tr.customer, tr.sameChild.Key); err != nil {
		t.Fatalf("export a one-queue subtree = %v", err)
	}
}
