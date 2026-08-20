package plugins

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
)

// TestSinkSeedOnPublishAndEchoSuppression proves the concrete-channel sink model:
//
//   - a channel-sink whose declared subscribe is a glob (chat:*) never gets that
//     glob literally subscribed (the exact-match bus could never deliver to it);
//   - when the plugin publishes INBOUND to a concrete chat channel, the publish
//     handler seeds a concrete subscription for exactly that channel;
//   - a subsequent reply from ANOTHER source to that channel is drained to the
//     plugin's /deliver;
//   - the plugin's OWN inbound on the same channel is NOT echoed back to it.
//
// Together this is the fix for the dead sink: subscribe=[chat:*] used to be
// literal-subscribed and match nothing, so /deliver never ran.
func TestSinkSeedOnPublishAndEchoSuppression(t *testing.T) {
	h, b, _ := newHost(t, nil)
	// API shares the host's bus + token registry so a seed on publish is visible
	// to the host drainer.
	a := NewAPI(h.cfg.Tokens, b, slog.New(slog.NewTextHandler(io.Discard, nil)), func(string, string, string) {})
	tok, err := h.cfg.Tokens.Mint(Identity{
		Name:    "messenger",
		Publish: []string{"chat:*"},
		Sink:    []string{"chat:*"}, // glob sink surface, as a channel-sink declares
	})
	if err != nil {
		t.Fatal(err)
	}

	subscriber := "plugin:messenger"
	chatCh := "chat:messenger:c1"

	// A recording plugin server on the sink socket captures every /deliver.
	sock := h.SocketPath("messenger")
	got := recordingPlugin(t, sock)

	publish := func(body string) {
		t.Helper()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/plugin/publish", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		a.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("publish code = %d body=%s", rr.Code, rr.Body)
		}
	}

	// No sink subscription exists before any inbound flows through publish.
	if subs, _ := b.ListSubscriptions(subscriber); len(subs) != 0 {
		t.Fatalf("pre-publish subscriptions = %+v, want none", subs)
	}

	// 1) Plugin publishes inbound #1 -> seeds a CONCRETE subscription for chatCh.
	publish(`{"channel":"` + chatCh + `","type":"chat.msg","text":"inbound-1"}`)
	subs, err := b.ListSubscriptions(subscriber)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Channel != chatCh {
		t.Fatalf("seeded subscriptions = %+v, want one on %s", subs, chatCh)
	}

	// 2) Plugin publishes inbound #2 on the SAME channel. The subscription now
	// exists, so a delivery row is created for it — this is the echo candidate.
	publish(`{"channel":"` + chatCh + `","type":"chat.msg","text":"inbound-2"}`)

	// 3) A reply from ANOTHER source (an agent) to the same channel.
	if _, err := b.Publish(bus.Message{
		Channel: chatCh, Type: "chat.reply", Text: "agent-reply",
		Source: "agent:alice", ProducedByAgent: "alice",
	}); err != nil {
		t.Fatal(err)
	}

	// 4) Drain: only the agent reply reaches /deliver; the plugin's own inbound #2
	// is echo-suppressed (acked, never delivered).
	h.drainOnce(context.Background(), "messenger", subscriber, NewClient(sock))

	if len(*got) != 1 {
		t.Fatalf("delivered %d messages, want exactly 1 (the agent reply): %+v", len(*got), *got)
	}
	if (*got)[0].Text != "agent-reply" || (*got)[0].ProducedByPlugin != "" {
		t.Fatalf("delivered wrong message (echo leaked?): %+v", (*got)[0])
	}

	// 5) Nothing left pending: reply delivered+acked, inbound #2 acked as echo.
	if has, _ := b.HasPending(subscriber); has {
		t.Fatal("messages still pending after drain: echo not acked or reply not delivered")
	}
}

// TestSinkStartSkipsGlobSubscribe guards that startSink does NOT literally
// subscribe a glob entry (which the exact-match bus can never deliver to) while
// still subscribing a concrete entry alongside it.
func TestSinkStartSkipsGlobSubscribe(t *testing.T) {
	h, b, _ := newHost(t, nil)
	rec := Record{
		Name:  "messenger",
		Types: []string{"channel-sink"},
		Channels: Channels{
			Subscribe: []string{"chat:*", "chat:direct"},
		},
	}
	// t.Context() is cancelled at test end, stopping the background drainer
	// startSink spawns.
	h.startSink(t.Context(), rec, h.SocketPath("messenger"))

	subs, err := b.ListSubscriptions("plugin:messenger")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Channel != "chat:direct" {
		t.Fatalf("startSink subscriptions = %+v, want only the concrete chat:direct", subs)
	}
}
