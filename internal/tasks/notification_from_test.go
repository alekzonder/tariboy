package tasks

import (
	"context"
	"encoding/json"
	"testing"
)

// A notification carries the principal that caused it, so the customer channel
// can be split back into one chat per agent. The author rides in data.from
// because source stays "system:tasks" for the bus fan-out author exclusion.
func TestNotificationOutboxRecordsItsAuthor(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusInProgress)
	if _, err := svc.AddComment(context.Background(), AgentActor("worker"), task.Key, AddCommentInput{
		Body: "@user:customer choose one", IdempotencyKey: "ask-1",
	}); err != nil {
		t.Fatal(err)
	}
	var channel, raw string
	if err := svc.db.QueryRow(`
		SELECT channel, data FROM task_notification_outbox
		WHERE message_type = 'task.question'`).Scan(&channel, &raw); err != nil {
		t.Fatal(err)
	}
	if channel != "user:customer" {
		t.Fatalf("channel = %q, want user:customer", channel)
	}
	data := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	if data["from"] != "agent:worker" {
		t.Fatalf("data.from = %v, want agent:worker", data["from"])
	}
}
