package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/operations"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/coder/websocket"
)

// One socket per host tells the UI which operation changed, so an open
// operation page refreshes its status and log at once.
func TestOperationsWebSocketStreamsOperationHints(t *testing.T) {
	t.Parallel()
	httpServer, hub := messagesWSServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/operations/ws"
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			hub.Emit(events.Event{Agent: "worker", Type: "iteration", Time: "t"})
			hub.Emit(events.Event{Type: operations.EventType, Time: "t", Data: map[string]any{"id": int64(4), "status": "running"}})
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	for range 2 {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var hint map[string]any
		if err := json.Unmarshal(raw, &hint); err != nil {
			t.Fatal(err)
		}
		if hint["id"] != float64(4) || hint["status"] != "running" || hint["ts"] != "t" {
			t.Fatalf("hint = %#v", hint)
		}
	}
}

func TestOperationLogDownload(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	st := storetest.OpenAt(t, filepath.Join(base, "state.db"))
	opStore := &operations.Store{DB: st.DB, Dir: filepath.Join(base, "operations")}
	runner := operations.NewRunner(context.Background(), opStore, nil, nil)
	t.Cleanup(runner.Shutdown)
	h, err := runner.Start(operations.Spec{Kind: "image.build", Title: "image build demo"}, func(ctx context.Context, l *operations.Log) (any, error) {
		l.Printf("step one")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := NewServer(registry.New(), &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Operations: runner})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	resp, err := http.Get(httpServer.URL + "/api/operations/1/log/download")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "step one\n") ||
		resp.Header.Get("Content-Disposition") != `attachment; filename="operation-1.log"` {
		t.Fatalf("download = %d %q %v", resp.StatusCode, body, resp.Header)
	}
	if err := os.Remove(opStore.LogPath(1)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/operations/1/log/download", "/api/operations/x/log/download"} {
		resp, err := http.Get(httpServer.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("%s = %d", path, resp.StatusCode)
		}
	}
}
