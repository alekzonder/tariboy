package commands

import (
	"errors"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
)

func ctxWithBus(t *testing.T) (*registry.Ctx, *bus.Bus) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/x.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	b := bus.New(s, time.Now)
	return &registry.Ctx{Store: s, Bus: b}, b
}

func TestChannelLsAndMessageSend(t *testing.T) {
	c, b := ctxWithBus(t)
	// operator publish creates the channel and the message.
	res, err := h(t, "message.send")(c, registry.Params{"channel": "chat:room", "type": "note", "text": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["channel"] != "chat:room" {
		t.Fatalf("send = %v", res)
	}
	// the published message carries operator attribution.
	tail, _ := b.Tail("chat:room", 10)
	if len(tail) != 1 || tail[0].Source != "operator" || tail[0].Text != "hi" {
		t.Fatalf("tail = %+v", tail)
	}
	// channel ls shows it.
	ls, err := h(t, "channel.ls")(c, registry.Params{})
	if err != nil || ls.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("ls = %v err=%v", ls, err)
	}
	// channel tail (non-follow) returns the message.
	tl, err := h(t, "channel.tail")(c, registry.Params{"channel": "chat:room"})
	if err != nil || tl.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("tail cmd = %v err=%v", tl, err)
	}
}

func TestAgentSubscriptions(t *testing.T) {
	c, b := ctxWithBus(t)
	// One agent subscribes to two channels; a second agent to a third. The
	// endpoint must return only the queried agent's channels (deduped), never
	// leaking another agent's subscriptions — that leak is exactly the "bound to
	// every agent" UI illusion this endpoint replaces.
	for _, ch := range []string{"group:dev:broadcast", "agent:worker:inbox"} {
		if _, err := b.Subscribe("worker", ch, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Subscribe("other", "chat:messenger:x", nil, nil); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "agent.subscriptions")(c, registry.Params{"name": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["count"].(int) != 2 {
		t.Fatalf("count = %v", m)
	}
	got := map[string]string{}
	for _, ch := range m["channels"].([]map[string]any) {
		got[ch["name"].(string)] = ch["kind"].(string)
	}
	if got["group:dev:broadcast"] != "group_broadcast" {
		t.Fatalf("broadcast kind = %q (%v)", got["group:dev:broadcast"], got)
	}
	if _, leaked := got["chat:messenger:x"]; leaked {
		t.Fatalf("leaked another agent's channel: %v", got)
	}
}

func TestAgentSubscriptionsProtectedFlag(t *testing.T) {
	c, b := ctxWithBus(t)
	for _, ch := range []string{"agent:worker:inbox", "group:dev:inbox", "chat:messenger:x"} {
		if _, err := b.Subscribe("worker", ch, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	res, err := h(t, "agent.subscriptions")(c, registry.Params{"name": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range res.(map[string]any)["channels"].([]map[string]any) {
		got[r["name"].(string)] = r["protected"].(bool)
	}
	want := map[string]bool{"agent:worker:inbox": true, "group:dev:inbox": true, "chat:messenger:x": false}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("protected[%q] = %v, want %v", name, got[name], w)
		}
	}
}

func TestAgentSubscribeUnsubscribe(t *testing.T) {
	c, b := ctxWithBus(t)
	agent.NewStore(c.Store).Create(agent.Agent{Name: "worker", ImageRef: "i:latest"})

	// subscribe: happy path creates the subscription.
	res, err := h(t, "agent.subscribe")(c, registry.Params{"name": "worker", "channel": "chat:messenger:x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["channel"] != "chat:messenger:x" {
		t.Fatalf("subscribe = %v", res)
	}
	subs, _ := b.ListSubscriptions("worker")
	if len(subs) != 1 || subs[0].Channel != "chat:messenger:x" {
		t.Fatalf("subs = %+v", subs)
	}

	// unknown agent → code not_found.
	if _, err := h(t, "agent.subscribe")(c, registry.Params{"name": "ghost", "channel": "chat:x"}); !isCode(err, "not_found") {
		t.Fatalf("unknown agent err = %v, want not_found", err)
	}
	// invalid channel → code bad_channel.
	if _, err := h(t, "agent.subscribe")(c, registry.Params{"name": "worker", "channel": "System:X"}); !isCode(err, "bad_channel") {
		t.Fatalf("bad channel err = %v, want bad_channel", err)
	}

	// unsubscribe: removes the ad-hoc subscription.
	if _, err := h(t, "agent.unsubscribe")(c, registry.Params{"name": "worker", "channel": "chat:messenger:x"}); err != nil {
		t.Fatalf("unsubscribe err = %v", err)
	}
	if s, _ := b.ListSubscriptions("worker"); len(s) != 0 {
		t.Fatalf("subs after unsubscribe = %+v", s)
	}

	// unsubscribe a protected channel → code protected_subscription (guard not bypassable).
	b.Subscribe("worker", "group:dev:inbox", nil, nil)
	if _, err := h(t, "agent.unsubscribe")(c, registry.Params{"name": "worker", "channel": "group:dev:inbox"}); !isCode(err, "protected_subscription") {
		t.Fatalf("protected unsubscribe err = %v, want protected_subscription", err)
	}
}

// TestAgentSubscribeParams proves the operator subscribe endpoint forwards
// type/matcher/params (spec §5.3): a parameterized subscribe routes through
// SubscribeParams, producing a subscription that carries a watch and the params
// so a provider can act on it. This is the daemon half compose's object-form
// subscribe entries drive.
func TestAgentSubscribeParams(t *testing.T) {
	c, b := ctxWithBus(t)
	agent.NewStore(c.Store).Create(agent.Agent{Name: "worker", ImageRef: "i:latest"})

	// params + matcher arrive as decoded JSON objects (the REST/compose body
	// shape); type as a comma list.
	res, err := h(t, "agent.subscribe")(c, registry.Params{
		"name":    "worker",
		"channel": "plugin:issue-provider",
		"type":    "run.finished",
		"matcher": map[string]any{"data.status": "failed"},
		"params":  map[string]any{"query": "Q1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["watch"] == nil || m["watch"] == "" {
		t.Fatalf("parameterized subscribe returned no watch: %v", m)
	}

	subs, _ := b.ListSubscriptions("worker")
	if len(subs) != 1 {
		t.Fatalf("want 1 sub, got %+v", subs)
	}
	if subs[0].Watch == "" {
		t.Fatalf("stored sub carries no watch: %+v", subs[0])
	}
	if subs[0].Params["query"] != "Q1" {
		t.Fatalf("stored sub dropped params: %+v", subs[0])
	}

	// Re-subscribing the identical params is idempotent (same watch -> same row).
	if _, err := h(t, "agent.subscribe")(c, registry.Params{
		"name": "worker", "channel": "plugin:issue-provider",
		"type": "run.finished", "matcher": map[string]any{"data.status": "failed"},
		"params": map[string]any{"query": "Q1"},
	}); err != nil {
		t.Fatal(err)
	}
	if subs, _ := b.ListSubscriptions("worker"); len(subs) != 1 {
		t.Fatalf("re-subscribe created a duplicate: %+v", subs)
	}

	// A malformed params JSON string (CLI-flag shape) is a loud bad_params error.
	if _, err := h(t, "agent.subscribe")(c, registry.Params{
		"name": "worker", "channel": "plugin:issue-provider", "params": "{not json",
	}); !isCode(err, "bad_params") {
		t.Fatalf("bad params err = %v, want bad_params", err)
	}
}

// isCode reports whether err is an api.UserError carrying the given code.
func isCode(err error, code string) bool {
	var ue api.UserError
	return errors.As(err, &ue) && ue.Code == code
}

// TestChannelLsListsBoundIdleChannel proves dev-t-dbu.1: a channel materialized
// by a plugin bind (EnsureChannel) with NO message and NO subscription still
// appears in channel.ls (GET /api/channels), so the agent Subscribe picker can
// offer a bound-but-idle chat. The row is marked by its kind ("chat").
func TestChannelLsListsBoundIdleChannel(t *testing.T) {
	c, b := ctxWithBus(t)
	if err := b.EnsureChannel("chat:messenger:idle"); err != nil {
		t.Fatal(err)
	}
	ls, err := h(t, "channel.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	m := ls.(map[string]any)
	if m["count"].(int) != 1 {
		t.Fatalf("ls count = %v, want 1 (bound-but-idle channel missing)", m["count"])
	}
	row := m["channels"].([]map[string]any)[0]
	if row["name"] != "chat:messenger:idle" || row["kind"] != "chat" {
		t.Fatalf("listed channel = %v, want chat:messenger:idle of kind chat", row)
	}
}

func TestChannelInspect(t *testing.T) {
	c, b := ctxWithBus(t)
	b.Publish(bus.Message{Channel: "chat:room", Type: "x", Text: "a"})
	insp, err := h(t, "channel.inspect")(c, registry.Params{"channel": "chat:room"})
	if err != nil || insp.(map[string]any)["messages"].(int) != 1 {
		t.Fatalf("inspect = %v err=%v", insp, err)
	}
}
