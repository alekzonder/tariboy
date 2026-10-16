package tasks

import (
	"context"
	"slices"
	"testing"
)

func listKeys(t *testing.T, svc *Service, actor Actor, filter ListFilter) []string {
	t.Helper()
	page, err := svc.ListTasks(context.Background(), actor, filter)
	if err != nil {
		t.Fatalf("ListTasks(%+v) = %v", filter, err)
	}
	out := make([]string, len(page.Tasks))
	for i, task := range page.Tasks {
		out[i] = task.Key
	}
	return out
}

func assertListKeys(t *testing.T, svc *Service, actor Actor, filter ListFilter, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := listKeys(t, svc, actor, filter); !slices.Equal(got, want) {
		t.Fatalf("ListTasks(%+v) keys = %v, want %v", filter, got, want)
	}
}

func TestListTasksStatusMatchesWorkflowStatusAndCategory(t *testing.T) {
	ctx := context.Background()
	svc, actor, _ := engineFixture(t)
	develop := mustCreateDev(t, svc, actor, "develop")
	review := mustCreateDev(t, svc, actor, "review")
	enter(t, svc, review.Key, "review", "ready")
	plain, err := svc.CreateTask(ctx, actor, CreateTaskInput{Queue: "TASK", Title: "plain", Assignee: "dev-1"})
	if err != nil {
		t.Fatal(err)
	}
	inProgress := StatusInProgress
	if _, err := svc.UpdateTask(ctx, actor, plain.Key, UpdateTaskInput{Status: &inProgress, Revision: plain.Revision}); err != nil {
		t.Fatal(err)
	}

	assertListKeys(t, svc, actor, ListFilter{Statuses: []string{"review"}}, review.Key)
	assertListKeys(t, svc, actor, ListFilter{Statuses: []string{"review", "develop"}, Queue: "DEV"}, develop.Key, review.Key)
	// A category value still finds workflow tasks in that category.
	got := listKeys(t, svc, actor, ListFilter{Statuses: []string{StatusInProgress}})
	if !slices.Contains(got, plain.Key) || !slices.Contains(got, develop.Key) {
		t.Fatalf("status in_progress = %v, want %s and %s", got, plain.Key, develop.Key)
	}
	assertListKeys(t, svc, actor, ListFilter{Categories: []string{StatusInProgress}, Queue: "TASK"}, plain.Key)
	// The legacy single Status field keeps working.
	assertListKeys(t, svc, actor, ListFilter{Status: "review"}, review.Key)
	if _, err := svc.ListTasks(ctx, actor, ListFilter{Categories: []string{"review"}}); ErrorCode(err) != "invalid_category" {
		t.Fatalf("category review error = %v, want invalid_category", err)
	}
}

func TestListTasksFiltersByPriorityCustomerParentAndSubtree(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "TREE", Name: "Tree"}); err != nil {
		t.Fatal(err)
	}
	root, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "TREE", Title: "root", Priority: PriorityP0})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: root.Key, Title: "child", Priority: PriorityP1})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: child.Key, Title: "grandchild", Priority: PriorityP3})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "TREE", Title: "other"})
	if err != nil {
		t.Fatal(err)
	}

	assertListKeys(t, svc, customer, ListFilter{Priorities: []Priority{PriorityP0, PriorityP3}}, root.Key, grandchild.Key)
	if _, err := svc.ListTasks(ctx, customer, ListFilter{Priorities: []Priority{"urgent"}}); ErrorCode(err) != "invalid_priority" {
		t.Fatalf("priority urgent error = %v, want invalid_priority", err)
	}
	assertListKeys(t, svc, customer, ListFilter{ParentKey: root.Key}, child.Key)
	assertListKeys(t, svc, customer, ListFilter{SubtreeKey: root.Key}, child.Key, grandchild.Key)
	assertListKeys(t, svc, customer, ListFilter{SubtreeKey: other.Key})
	assertListKeys(t, svc, customer, ListFilter{SubtreeKey: "TREE-none"})
	if _, err := svc.ListTasks(ctx, customer, ListFilter{ParentKey: root.Key, SubtreeKey: root.Key}); ErrorCode(err) != "invalid_filter" {
		t.Fatalf("parent with subtree error = %v, want invalid_filter", err)
	}
	got := listKeys(t, svc, customer, ListFilter{Customer: root.Customer})
	if len(got) != 4 {
		t.Fatalf("customer %q = %v, want all four", root.Customer, got)
	}
	assertListKeys(t, svc, customer, ListFilter{Customer: "user:nobody"})
}

func TestListTasksSubtreeStaysInsideAgentVisibility(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "HIDE", Name: "Hidden"}); err != nil {
		t.Fatal(err)
	}
	root, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "HIDE", Title: "root", Assignee: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: root.Key, Title: "hidden", Assignee: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: hidden.Key, Title: "mine", Assignee: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTask(ctx, customer, CreateTaskInput{ParentKey: hidden.Key, Title: "sibling", Assignee: "bob"}); err != nil {
		t.Fatal(err)
	}
	// hidden is a read-only ancestor of alice's task; bob's sibling stays out.
	assertListKeys(t, svc, AgentActor("alice"), ListFilter{SubtreeKey: root.Key, StatusView: "all"}, hidden.Key, mine.Key)
	assertListKeys(t, svc, AgentActor("alice"), ListFilter{ParentKey: hidden.Key, StatusView: "all"}, mine.Key)
}

func TestListTasksClosedViewIncludesCancelled(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "SHUT", Name: "Shut"}); err != nil {
		t.Fatal(err)
	}
	done, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "SHUT", Title: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteTask(ctx, customer, done.Key, CompleteInput{Revision: done.Revision}); err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "SHUT", Title: "cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	status := StatusCancelled
	if _, err := svc.UpdateTask(ctx, customer, cancelled.Key, UpdateTaskInput{Status: &status, Revision: cancelled.Revision}); err != nil {
		t.Fatal(err)
	}
	assertListKeys(t, svc, customer, ListFilter{StatusView: "closed"}, done.Key, cancelled.Key)
}

func TestListTasksSortsAndPagesEveryOrder(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "SORT", Name: "Sort"}); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for i, priority := range []Priority{PriorityP2, PriorityP0, PriorityP3, PriorityP0} {
		task, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "SORT", Title: "t", Priority: priority})
		if err != nil {
			t.Fatal(err)
		}
		stamp := "2026-01-0" + string(rune('1'+i)) + "T00:00:00Z"
		if _, err := svc.db.Exec(`UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`, stamp, stamp, task.ID); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, task.Key)
	}
	want := map[string][]string{
		"created":  {keys[3], keys[2], keys[1], keys[0]},
		"updated":  {keys[3], keys[2], keys[1], keys[0]},
		"priority": {keys[3], keys[1], keys[0], keys[2]},
	}
	for sort, order := range want {
		assertListKeys(t, svc, customer, ListFilter{Sort: sort}, order...)
		var paged []string
		after := ""
		for {
			page, err := svc.ListTasks(ctx, customer, ListFilter{Sort: sort, Limit: 1, AfterKey: after})
			if err != nil {
				t.Fatal(err)
			}
			for _, task := range page.Tasks {
				paged = append(paged, task.Key)
			}
			if page.NextCursor == "" {
				break
			}
			after = page.NextCursor
		}
		if !slices.Equal(paged, order) {
			t.Fatalf("sort %s paged = %v, want %v", sort, paged, order)
		}
	}
	assertListKeys(t, svc, customer, ListFilter{Sort: "tree"}, keys[1], keys[3], keys[0], keys[2])
	if _, err := svc.ListTasks(ctx, customer, ListFilter{Sort: "title"}); ErrorCode(err) != "invalid_sort" {
		t.Fatalf("sort title error = %v, want invalid_sort", err)
	}
}

func TestAgentListActionPassesFiltersAndPages(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	customer := CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "AGNT", Name: "Agent"}); err != nil {
		t.Fatal(err)
	}
	alice := AgentActor("alice")
	var keys []string
	for _, priority := range []Priority{PriorityP0, PriorityP1, PriorityP1} {
		task, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "AGNT", Title: "work", Assignee: "alice", Priority: priority})
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, task.Key)
	}
	if _, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "AGNT", Title: "bob", Assignee: "bob"}); err != nil {
		t.Fatal(err)
	}
	pageOf := func(action string, body map[string]any) TaskPage {
		t.Helper()
		result, err := svc.AgentAction(ctx, alice, action, body)
		if err != nil {
			t.Fatalf("%s %v = %v", action, body, err)
		}
		return result.(TaskPage)
	}
	first := pageOf("list", map[string]any{"priority": "P1", "limit": "1", "by_customer": "user:customer", "scope_agent": "bob"})
	if len(first.Tasks) != 1 || first.Tasks[0].Key != keys[1] || first.NextCursor != keys[1] {
		t.Fatalf("first page = %+v", first)
	}
	second := pageOf("mine", map[string]any{"priority": "P1", "limit": "1", "after": first.NextCursor})
	if len(second.Tasks) != 1 || second.Tasks[0].Key != keys[2] || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
	if page := pageOf("list", map[string]any{"by_author": "user:nobody"}); len(page.Tasks) != 0 {
		t.Fatalf("by_author user:nobody = %+v, want none", page)
	}
	if page := pageOf("list", map[string]any{"by_author": "user:customer", "sort": "priority"}); len(page.Tasks) != 3 || page.Tasks[0].Key != keys[0] {
		t.Fatalf("by_author customer sorted by priority = %+v", page)
	}
}
