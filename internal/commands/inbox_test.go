package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
)

// seedInbox subscribes worker to a channel and publishes msgs into it, returning
// the published messages (a delivery row per message lands in worker's inbox).
func seedInbox(t *testing.T, b *bus.Bus, agent, channel string, texts ...string) []bus.Message {
	t.Helper()
	if _, err := b.Subscribe(agent, channel, nil, nil); err != nil {
		t.Fatal(err)
	}
	var out []bus.Message
	for _, tx := range texts {
		m, err := b.Publish(bus.Message{Channel: channel, Type: "note", Text: tx, Source: "operator"})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func TestAgentInboxListAndStatus(t *testing.T) {
	c, b := ctxWithBus(t)
	msgs := seedInbox(t, b, "worker", "chat:room", "one", "two")

	// pending: both, newest first.
	res, err := h(t, "agent.inbox.ls")(c, registry.Params{"name": "worker", "status": "pending"})
	if err != nil {
		t.Fatal(err)
	}
	rows := res.(map[string]any)["messages"].([]map[string]any)
	if len(rows) != 2 {
		t.Fatalf("pending rows = %d, want 2", len(rows))
	}
	if rows[0]["text"] != "two" || rows[1]["text"] != "one" {
		t.Fatalf("not newest-first: %v", rows)
	}
	if rows[0]["attempts"] == nil || rows[0]["dlq"] != false {
		t.Fatalf("missing delivery state: %v", rows[0])
	}

	// process the older one → pending drops to 1, processed shows 1 with prefix.
	if _, err := h(t, "agent.inbox.processed")(c, registry.Params{
		"name": "worker", "id": msgs[0].ID, "result": "looked at it"}); err != nil {
		t.Fatal(err)
	}
	pend, _ := h(t, "agent.inbox.ls")(c, registry.Params{"name": "worker", "status": "pending"})
	if n := pend.(map[string]any)["count"].(int); n != 1 {
		t.Fatalf("pending after processed = %d, want 1", n)
	}
	proc, _ := h(t, "agent.inbox.ls")(c, registry.Params{"name": "worker", "status": "processed"})
	prows := proc.(map[string]any)["messages"].([]map[string]any)
	if len(prows) != 1 || prows[0]["result"] != "operator: looked at it" {
		t.Fatalf("processed row = %v", prows)
	}
	if prows[0]["processed_at"] == "" || prows[0]["processed_at"] == nil {
		t.Fatalf("processed_at not set: %v", prows[0])
	}

	// default status (empty) → all.
	all, _ := h(t, "agent.inbox.ls")(c, registry.Params{"name": "worker"})
	if n := all.(map[string]any)["count"].(int); n != 2 {
		t.Fatalf("all count = %d, want 2", n)
	}
}

func TestAgentInboxProcessedValidation(t *testing.T) {
	c, b := ctxWithBus(t)
	msgs := seedInbox(t, b, "worker", "chat:room", "x")

	// empty result → missing_result.
	if _, err := h(t, "agent.inbox.processed")(c, registry.Params{
		"name": "worker", "id": msgs[0].ID, "result": "   "}); !isCode(err, "missing_result") {
		t.Fatalf("empty result err = %v, want missing_result", err)
	}
	// message not in this agent's inbox → not_found.
	if _, err := h(t, "agent.inbox.processed")(c, registry.Params{
		"name": "ghost", "id": msgs[0].ID, "result": "r"}); !isCode(err, "not_found") {
		t.Fatalf("ghost processed err = %v, want not_found", err)
	}
}

func TestAgentInboxReply(t *testing.T) {
	c, b := ctxWithBus(t)
	msgs := seedInbox(t, b, "worker", "chat:room", "please answer")

	res, err := h(t, "agent.inbox.reply")(c, registry.Params{
		"name": "worker", "id": msgs[0].ID, "text": "here you go"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["replied"] != true || m["in_reply_to"] != msgs[0].ID {
		t.Fatalf("reply = %v", m)
	}
	// the reply was published on the original's channel (Source=operator → channel).
	tail, _ := b.Tail("chat:room", 10)
	var found bool
	for _, tm := range tail {
		if tm.Kind == "reply" && tm.InReplyTo == msgs[0].ID && tm.Text == "here you go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("threaded reply not on channel: %+v", tail)
	}
	// the row is auto-processed with operator attribution.
	proc, _ := h(t, "agent.inbox.ls")(c, registry.Params{"name": "worker", "status": "processed"})
	prows := proc.(map[string]any)["messages"].([]map[string]any)
	if len(prows) != 1 || prows[0]["result"] != "operator replied: "+m["id"].(string) {
		t.Fatalf("reply did not auto-process with attribution: %v", prows)
	}
	// replying to an unknown message → not_found.
	if _, err := h(t, "agent.inbox.reply")(c, registry.Params{
		"name": "worker", "id": "nope", "text": "x"}); !isCode(err, "not_found") {
		t.Fatalf("reply unknown err = %v, want not_found", err)
	}
}

func TestAgentInboxRequeue(t *testing.T) {
	c, b := ctxWithBus(t)
	msgs := seedInbox(t, b, "worker", "chat:room", "x")

	if _, err := h(t, "agent.inbox.requeue")(c, registry.Params{
		"name": "worker", "id": msgs[0].ID}); err != nil {
		t.Fatalf("requeue err = %v", err)
	}
	// unknown message in this agent's inbox → not_found.
	if _, err := h(t, "agent.inbox.requeue")(c, registry.Params{
		"name": "worker", "id": "nope"}); !isCode(err, "not_found") {
		t.Fatalf("requeue unknown err = %v, want not_found", err)
	}
}

func TestChannelWatches(t *testing.T) {
	c, b := ctxWithBus(t)
	params := map[string]any{"q": "status:open"}
	if _, err := b.SubscribeParams("worker", "issue-provider:issues", nil, nil, params); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SubscribeParams("other", "issue-provider:issues", nil, nil, params); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "channel.watches")(c, registry.Params{"channel": "issue-provider:issues"})
	if err != nil {
		t.Fatal(err)
	}
	rows := res.(map[string]any)["watches"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("watches = %d, want 1 (same params dedup): %v", len(rows), rows)
	}
	subs := rows[0]["subscribers"].([]string)
	if len(subs) != 2 {
		t.Fatalf("subscribers = %v, want worker+other", subs)
	}
	if p, _ := rows[0]["params"].(map[string]any); p["q"] != "status:open" {
		t.Fatalf("params = %v", rows[0]["params"])
	}
}
