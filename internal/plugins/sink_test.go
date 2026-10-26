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

	"github.com/alekzonder/tariboy/internal/mail"
)

// recordingPlugin serves /health 200 and records the letters of /deliver.
func recordingPlugin(t *testing.T, sock string) *[]mail.Letter {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(sock), 0o700)
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	got := &[]mail.Letter{}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/deliver", func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			Letter mail.Letter `json:"letter"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &env)
		mu.Lock()
		*got = append(*got, env.Letter)
		mu.Unlock()
		w.WriteHeader(200)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return got
}

func TestDrainDeliversTheLetterAndClosesIt(t *testing.T) {
	t.Parallel()
	h, b, ps := newHost(t, nil)
	if err := ps.Upsert(sampleRecord("echo")); err != nil {
		t.Fatal(err)
	}
	sock := h.SocketPath("echo")
	got := recordingPlugin(t, sock)
	sent := sendToPlugin(t, b, "echo", "hey")
	h.drainOnce(context.Background(), "echo", "plugin:echo", NewClient(sock))
	if len(*got) != 1 || (*got)[0].ID != sent.ID || (*got)[0].Body != "hey" || (*got)[0].From != "agent:alice" {
		t.Fatalf("delivered = %+v", *got)
	}
	if open := openLetters(t, b, "plugin:echo"); len(open) != 0 {
		t.Fatalf("delivered letter still open: %+v", open)
	}
}

// A delivery that fails leaves the letter open, and each attempt counts, so a
// plugin that keeps failing gets the letter dead-lettered after five.
func TestDrainLeavesTheLetterOpenOnFailureAndDeadLettersIt(t *testing.T) {
	t.Parallel()
	h, b, ps := newHost(t, nil)
	if err := ps.Upsert(sampleRecord("echo")); err != nil {
		t.Fatal(err)
	}
	// No plugin server listening on the socket: every deliver fails.
	sent := sendToPlugin(t, b, "echo", "hey")
	client := NewClient(h.SocketPath("echo"))
	for range 5 {
		h.drainOnce(context.Background(), "echo", "plugin:echo", client)
	}
	if open := openLetters(t, b, "plugin:echo"); len(open) != 1 || open[0].Attempts != 5 {
		t.Fatalf("after five failed deliveries = %+v; want one open letter with five attempts", open)
	}
	h.drainOnce(context.Background(), "echo", "plugin:echo", client)
	got, _, err := b.Show("plugin:echo", sent.ID)
	if err != nil || got.DeadReason != mail.DeadAttempts {
		t.Fatalf("letter after a sixth drain = %+v, %v; want dead with reason attempts", got, err)
	}
}
