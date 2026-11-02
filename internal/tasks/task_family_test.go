package tasks

import (
	"context"
	"testing"
)

func childKeys(children []Task) []string {
	keys := make([]string, 0, len(children))
	for _, child := range children {
		keys = append(keys, child.Key)
	}
	return keys
}

func TestGetTaskChildrenHideOtherQueueChildFromReader(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	detail, err := tr.svc.GetTask(ctx, tr.lead, tr.parent.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got := childKeys(detail.Children); len(got) != 1 || got[0] != tr.sameChild.Key {
		t.Fatalf("lead children = %v; want [%s]", got, tr.sameChild.Key)
	}
	if detail.HiddenChildren != 1 {
		t.Fatalf("lead hidden_children = %d; want 1", detail.HiddenChildren)
	}
	if detail.Children[0].Access != "write" {
		t.Fatalf("child access = %q; want write", detail.Children[0].Access)
	}
	if detail.Parent != nil {
		t.Fatalf("root parent = %+v; want nil", detail.Parent)
	}
}

func TestGetTaskChildrenIncludeClosedInTreeOrder(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	urgent, err := tr.svc.CreateTask(ctx, tr.customer, CreateTaskInput{ParentKey: tr.parent.Key, Title: "urgent", Priority: PriorityP0})
	if err != nil {
		t.Fatal(err)
	}
	forceTask(t, tr.svc, tr.sameChild, `status = 'done', position = 2`)
	forceTask(t, tr.svc, tr.kid, `position = 1`)
	detail, err := tr.svc.GetTask(ctx, tr.customer, tr.parent.Key)
	if err != nil {
		t.Fatal(err)
	}
	// P0 first, then the P2 children by position, whatever their queue.
	want := []string{urgent.Key, tr.kid.Key, tr.sameChild.Key}
	got := childKeys(detail.Children)
	if len(got) != len(want) {
		t.Fatalf("children = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children = %v; want %v", got, want)
		}
	}
	if detail.Children[2].Status != StatusDone {
		t.Fatalf("closed child status = %q; want done", detail.Children[1].Status)
	}
	if detail.HiddenChildren != 0 {
		t.Fatalf("customer hidden_children = %d; want 0", detail.HiddenChildren)
	}
}

func TestGetTaskParentHiddenFromChildQueueOwner(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	detail, err := tr.svc.GetTask(ctx, tr.designer, tr.kid.Key)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Parent != nil {
		t.Fatalf("designer parent = %+v; want nil", detail.Parent)
	}
	if detail.Task.ParentKey != tr.parent.Key {
		t.Fatalf("parent_key = %q; want %q", detail.Task.ParentKey, tr.parent.Key)
	}
	if detail.Children == nil || len(detail.Children) != 0 || detail.HiddenChildren != 0 {
		t.Fatalf("leaf children = %v, hidden %d; want empty", detail.Children, detail.HiddenChildren)
	}
}

func TestGetTaskParentVisibleToReader(t *testing.T) {
	tr := newCrossQueueTree(t)
	ctx := context.Background()
	detail, err := tr.svc.GetTask(ctx, tr.writer, tr.kid.Key)
	if err != nil {
		t.Fatal(err)
	}
	want := TaskParent{Key: tr.parent.Key, Title: "parent", Queue: "TARI", Access: "write"}
	if detail.Parent == nil || *detail.Parent != want {
		t.Fatalf("writer parent = %+v; want %+v", detail.Parent, want)
	}
}
