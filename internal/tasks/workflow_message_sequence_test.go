package tasks

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

// workflowBus is a bus wired the way the daemon wires it for workflow ingress.
func workflowBus(svc *Service) *bus.Bus {
	b := bus.New(&basestore.Store{DB: svc.db}, svc.clock)
	b.SetPublishTxHook(RecordWorkflowMessageSequence)
	b.SetMessageRetention(WorkflowMessageRetainedSQL)
	return b
}

func TestPublishAssignsMonotonicWorkflowIngressSequence(t *testing.T) {
	svc := newTestService(t)
	b := workflowBus(svc)
	for _, channel := range []string{"chat:z", "chat:a"} {
		if _, err := b.Publish(bus.Message{Channel: channel, Source: "plugin:test", Type: "event"}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.db.Query(`SELECT m.channel FROM task_workflow_message_sequence s JOIN messages m ON m.id=s.message_id ORDER BY s.sequence`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var channels []string
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			t.Fatal(err)
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(channels) != 2 || channels[0] != "chat:z" || channels[1] != "chat:a" {
		t.Fatalf("sequenced channels = %#v", channels)
	}
}
