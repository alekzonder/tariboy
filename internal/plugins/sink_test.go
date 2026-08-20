package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
)

// recordingPlugin serves /health 200 and records /deliver bodies.
func recordingPlugin(t *testing.T, sock string) *[]MessageDTO {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(sock), 0o700)
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	got := &[]MessageDTO{}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/deliver", func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			Message MessageDTO `json:"message"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &env)
		mu.Lock()
		*got = append(*got, env.Message)
		mu.Unlock()
		w.WriteHeader(200)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return got
}

func TestSinkDrainDeliversAndAcks(t *testing.T) {
	h, b, _ := newHost(t, nil)
	sock := h.SocketPath("echo")
	got := recordingPlugin(t, sock)
	subscriber := "plugin:echo"
	if _, err := b.Subscribe(subscriber, "chat:in", bus.Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Publish(bus.Message{Channel: "chat:in", Type: "chat.msg", Text: "hey"}); err != nil {
		t.Fatal(err)
	}
	h.drainOnce(context.Background(), "echo", subscriber, NewClient(sock))
	if len(*got) != 1 || (*got)[0].Text != "hey" {
		t.Fatalf("delivered = %+v", *got)
	}
	// Acked -> no longer pending.
	if has, _ := b.HasPending(subscriber); has {
		t.Fatal("message still pending after successful deliver")
	}
}

// A kind=reply delivery must carry its routing fields (kind, in_reply_to,
// correlation_id) through to the plugin, so a channel-sink can implement the
// reply-forwarding contract (spec §6.4) — map the reply back to its external
// entity. Guards toDTO against silently dropping those fields.
func TestSinkDrainForwardsReplyRoutingFields(t *testing.T) {
	h, b, _ := newHost(t, nil)
	sock := h.SocketPath("echo")
	got := recordingPlugin(t, sock)
	subscriber := "plugin:echo"
	if _, err := b.Subscribe(subscriber, "chat:in", bus.Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Publish(bus.Message{
		Channel: "chat:in", Type: "chat.reply", Text: "answer",
		Kind: "reply", InReplyTo: "orig-1", CorrelationID: "corr-1",
		ProducedByAgent: "alice",
	}); err != nil {
		t.Fatal(err)
	}
	h.drainOnce(context.Background(), "echo", subscriber, NewClient(sock))
	if len(*got) != 1 {
		t.Fatalf("delivered %d messages, want 1: %+v", len(*got), *got)
	}
	d := (*got)[0]
	if d.Kind != "reply" || d.InReplyTo != "orig-1" || d.CorrelationID != "corr-1" {
		t.Fatalf("routing fields not forwarded: kind=%q in_reply_to=%q correlation_id=%q",
			d.Kind, d.InReplyTo, d.CorrelationID)
	}
}

func TestSinkDrainLeavesUnackedOnFailure(t *testing.T) {
	h, b, _ := newHost(t, nil)
	// No plugin server listening on the socket -> deliver fails.
	subscriber := "plugin:echo"
	b.Subscribe(subscriber, "chat:in", bus.Matcher{}, nil)
	b.Publish(bus.Message{Channel: "chat:in", Type: "chat.msg", Text: "hey"})
	h.drainOnce(context.Background(), "echo", subscriber, NewClient(h.SocketPath("echo")))
	// Delivery failed -> still pending for redelivery.
	if has, _ := b.HasPending(subscriber); !has {
		t.Fatal("failed delivery should leave the message pending")
	}
}
