package loop

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

type attributionTaskReader struct {
	tasks map[string]tasks.Task
	err   map[string]error
	seen  []tasks.Actor
}

type attributionProxy struct {
	calls int
	key   string
	task  string
	epic  string
}

func (p *attributionProxy) UpdateTask(key, task, epic string) int {
	p.calls++
	p.key, p.task, p.epic = key, task, epic
	return 1
}

func (r *attributionTaskReader) GetTask(_ context.Context, actor tasks.Actor, key string) (tasks.TaskDetail, error) {
	r.seen = append(r.seen, actor)
	if err := r.err[key]; err != nil {
		return tasks.TaskDetail{}, err
	}
	task, ok := r.tasks[key]
	if !ok {
		return tasks.TaskDetail{}, errors.New("task not found")
	}
	return tasks.TaskDetail{Task: task}, nil
}

func TestResolveNativeTaskAttributionWalksToRootAsAgent(t *testing.T) {
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{
		"SUPER-1": {Key: "SUPER-1"},
		"SUPER-2": {Key: "SUPER-2", ParentKey: "SUPER-1"},
		"SUPER-3": {Key: "SUPER-3", ParentKey: "SUPER-2"},
	}}

	tests := []struct {
		name     string
		key      string
		wantTask string
		wantEpic string
	}{
		{name: "root", key: "SUPER-1", wantTask: "SUPER-1", wantEpic: "SUPER-1"},
		{name: "child", key: "SUPER-2", wantTask: "SUPER-2", wantEpic: "SUPER-1"},
		{name: "grandchild", key: "SUPER-3", wantTask: "SUPER-3", wantEpic: "SUPER-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader.seen = nil
			taskID, epicID, err := resolveNativeTaskAttribution(context.Background(), reader, "lively-quokka", tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if taskID != tt.wantTask || epicID != tt.wantEpic {
				t.Fatalf("attribution = %q/%q, want %q/%q", taskID, epicID, tt.wantTask, tt.wantEpic)
			}
			for _, actor := range reader.seen {
				if actor.Principal != "agent:lively-quokka" || actor.IsCustomer {
					t.Fatalf("lookup actor = %#v, want agent:lively-quokka", actor)
				}
			}
		})
	}
}

func TestResolveNativeTaskAttributionPropagatesLookupError(t *testing.T) {
	want := errors.New("native task unavailable")
	reader := &attributionTaskReader{err: map[string]error{"SUPER-9": want}}
	_, _, err := resolveNativeTaskAttribution(context.Background(), reader, "worker", "SUPER-9")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestResolveNativeTaskAttributionRejectsParentCycle(t *testing.T) {
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{
		"SUPER-1": {Key: "SUPER-1", ParentKey: "SUPER-2"},
		"SUPER-2": {Key: "SUPER-2", ParentKey: "SUPER-1"},
	}}
	_, _, err := resolveNativeTaskAttribution(context.Background(), reader, "worker", "SUPER-1")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error = %v, want parent cycle error", err)
	}
}

func TestResolveNativeTaskAttributionUsesRealServiceAuthorization(t *testing.T) {
	state, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	service := tasks.NewService(state.DB, "customer", func() time.Time {
		return time.Date(2026, 8, 3, 6, 0, 0, 0, time.UTC)
	})
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	if _, err := service.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: "SUPER", Name: "Super"}); err != nil {
		t.Fatal(err)
	}
	root, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "SUPER", Title: "root", Assignee: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{ParentKey: root.Key, Title: "child"})
	if err != nil {
		t.Fatal(err)
	}

	taskID, epicID, err := resolveNativeTaskAttribution(ctx, service, "worker", child.Key)
	if err != nil {
		t.Fatal(err)
	}
	if taskID != child.Key || epicID != root.Key {
		t.Fatalf("attribution=%q/%q, want %q/%q", taskID, epicID, child.Key, root.Key)
	}
	if _, _, err := resolveNativeTaskAttribution(ctx, service, "outsider", child.Key); tasks.ErrorCode(err) != "not_found" {
		t.Fatalf("outsider error=%v code=%q, want not_found", err, tasks.ErrorCode(err))
	}
}

func TestSetCurrentTaskAttributionRejectsUnavailableProxy(t *testing.T) {
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{"SUPER-1": {Key: "SUPER-1"}}}
	for _, clear := range []bool{false, true} {
		_, err := setCurrentTaskAttribution(context.Background(), reader, nil, "iter-1", "worker", "SUPER-1", clear)
		if err == nil || !strings.Contains(err.Error(), "proxy unavailable") {
			t.Fatalf("clear=%v error=%v, want proxy unavailable", clear, err)
		}
	}
	if len(reader.seen) != 0 {
		t.Fatalf("proxy-unavailable path performed %d task lookups, want 0", len(reader.seen))
	}
}

func TestSetCurrentTaskAttributionClearSkipsTaskLookup(t *testing.T) {
	reader := &attributionTaskReader{}
	proxy := &attributionProxy{}
	result, err := setCurrentTaskAttribution(context.Background(), reader, proxy, "iter-1", "worker", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.seen) != 0 || proxy.calls != 1 || proxy.task != "" || proxy.epic != "" {
		t.Fatalf("clear lookups=%d proxy=%+v", len(reader.seen), proxy)
	}
	if result["cleared"] != true || result["updated"] != 1 {
		t.Fatalf("clear result=%v", result)
	}
}

func TestSetCurrentTaskAttributionUpdatesResolvedNativePair(t *testing.T) {
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{
		"SUPER-1": {Key: "SUPER-1"},
		"SUPER-3": {Key: "SUPER-3", ParentKey: "SUPER-1"},
	}}
	proxy := &attributionProxy{}
	result, err := setCurrentTaskAttribution(context.Background(), reader, proxy, "iter-1", "worker", "SUPER-3", false)
	if err != nil {
		t.Fatal(err)
	}
	if proxy.calls != 1 || proxy.key != "iter-1" || proxy.task != "SUPER-3" || proxy.epic != "SUPER-1" {
		t.Fatalf("proxy=%+v", proxy)
	}
	if result["task_id"] != "SUPER-3" || result["epic_id"] != "SUPER-1" || result["updated"] != 1 {
		t.Fatalf("result=%v", result)
	}
}

func TestSetCurrentTaskAttributionLookupErrorDoesNotMutateProxy(t *testing.T) {
	want := errors.New("native task denied")
	reader := &attributionTaskReader{err: map[string]error{"SUPER-9": want}}
	proxy := &attributionProxy{}
	_, err := setCurrentTaskAttribution(context.Background(), reader, proxy, "iter-1", "worker", "SUPER-9", false)
	if !errors.Is(err, want) || proxy.calls != 0 {
		t.Fatalf("error=%v proxy calls=%d, want denied/0", err, proxy.calls)
	}
}
