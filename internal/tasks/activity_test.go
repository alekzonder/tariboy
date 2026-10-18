package tasks

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestTaskActivitySummarizesEventsInWindow(t *testing.T) {
	svc, customer, _ := engineFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return now }

	worked := mustCreateDev(t, svc, customer, "worked")
	quiet := mustCreateDev(t, svc, customer, "quiet")
	holder := holderOf(t, svc, worked, "developers")
	if holder == "" {
		t.Fatal("worked task has no developer holder")
	}
	if _, err := svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "OTHER", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "OTHER", Title: "hidden"})
	if err != nil {
		t.Fatal(err)
	}

	since := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	now = since.Add(time.Minute)
	if _, err := svc.AddComment(ctx, AgentActor(holder), worked.Key, AddCommentInput{Body: "progress"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Advance(ctx, AgentActor(holder), worked.Key, AdvanceInput{Outcome: "ready"}); err != nil {
		t.Fatal(err)
	}
	fresh := mustCreateDev(t, svc, customer, "fresh")
	now = since.Add(2 * time.Hour)
	if _, err := svc.AddComment(ctx, customer, quiet.Key, AddCommentInput{Body: "too late"}); err != nil {
		t.Fatal(err)
	}

	window := ActivityFilter{
		Since: since.Format(time.RFC3339Nano), Until: since.Add(time.Hour).Format(time.RFC3339Nano),
		Agent: holder, Keys: []string{quiet.Key, hidden.Key},
	}
	got, err := svc.TaskActivity(ctx, customer, window)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("agent activity = %#v", got)
	}
	byKey := map[string]TaskActivity{}
	for _, row := range got {
		byKey[row.Task.Key] = row
	}
	row := byKey[worked.Key]
	if row.Comments != 1 || !reflect.DeepEqual(row.Outcomes, []string{"ready"}) ||
		!reflect.DeepEqual(row.EnteredStatuses, []string{"review"}) || !reflect.DeepEqual(row.Agents, []string{holder}) ||
		row.FirstAt != "2026-07-31T10:01:00.000Z" || row.LastAt != "2026-07-31T10:01:00.000Z" || row.Completed {
		t.Fatalf("worked row = %#v", row)
	}
	// A key reported by AI usage appears even without events by the agent.
	if row := byKey[quiet.Key]; row.Task.Title != "quiet" || row.Comments != 0 || row.LastAt != "" {
		t.Fatalf("quiet row = %#v", row)
	}
	if _, ok := byKey[fresh.Key]; ok {
		t.Fatal("a task without the agent's work is listed")
	}

	// Without an agent every actor's events count; the agent list keeps agents only.
	window.Agent, window.Keys = "", nil
	got, err = svc.TaskActivity(ctx, customer, window)
	if err != nil {
		t.Fatal(err)
	}
	byKey = map[string]TaskActivity{}
	for _, row := range got {
		byKey[row.Task.Key] = row
	}
	if len(got) != 2 || len(byKey[fresh.Key].Agents) != 0 ||
		!reflect.DeepEqual(byKey[fresh.Key].EnteredStatuses, []string{"develop"}) {
		t.Fatalf("server activity = %#v", got)
	}

	// An agent caller never sees a task outside its visibility.
	got, err = svc.TaskActivity(ctx, AgentActor(holder), ActivityFilter{Keys: []string{hidden.Key}})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range got {
		if row.Task.Key == hidden.Key {
			t.Fatalf("hidden task listed: %#v", got)
		}
	}
}

func TestTaskActivityMarksTasksCompletedInWindow(t *testing.T) {
	svc, customer, _ := engineFixture(t)
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return now }
	task := mustCreateDev(t, svc, customer, "finish")
	enter(t, svc, task.Key, "done", "merged")

	got, err := svc.TaskActivity(context.Background(), customer, ActivityFilter{
		Since: "2026-07-31T10:00:00Z", Until: "2026-07-31T11:00:00Z",
	})
	if err != nil || len(got) != 1 || !got[0].Completed {
		t.Fatalf("completed activity = %#v, %v", got, err)
	}
}
