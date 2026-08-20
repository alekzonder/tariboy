package tasks

import (
	"context"
	"testing"
)

func TestHubReplayFiltersAgentVisibilityAndResumesAfterSequence(t *testing.T) {
	svc := newTestService(t)
	hub := NewHub(svc)
	svc.SetHub(hub)
	ctx := context.Background()
	customer := CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "LIVE", Name: "Live"})
	visible, _ := svc.CreateTask(ctx, customer, CreateTaskInput{
		Queue: "LIVE", Title: "visible", Assignee: "alice",
	})
	_, _ = svc.CreateTask(ctx, customer, CreateTaskInput{
		Queue: "LIVE", Title: "hidden",
	})

	events, reset, err := hub.Replay(ctx, AgentActor("alice"), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if reset || len(events) != 1 || events[0].TaskKey != visible.Key {
		t.Fatalf("alice replay reset/events = %v/%#v", reset, events)
	}
	resumed, reset, err := hub.Replay(ctx, AgentActor("alice"), events[0].Sequence, 100)
	if err != nil {
		t.Fatal(err)
	}
	if reset || len(resumed) != 0 {
		t.Fatalf("resume returned reset/events = %v/%#v", reset, resumed)
	}
}

func TestHubSignalsSubscribersAfterCommittedMutation(t *testing.T) {
	svc := newTestService(t)
	hub := NewHub(svc)
	svc.SetHub(hub)
	wake, cancel := hub.Subscribe()
	defer cancel()
	ctx := context.Background()
	customer := CustomerActor("customer")
	_, _ = svc.CreateQueue(ctx, customer, CreateQueueInput{Prefix: "WAKE", Name: "Wake"})
	_, err := svc.CreateTask(ctx, customer, CreateTaskInput{Queue: "WAKE", Title: "wake"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
	default:
		t.Fatal("committed task mutation did not wake subscriber")
	}
}
