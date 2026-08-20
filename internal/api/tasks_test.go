package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/coder/websocket"
)

func TestTasksWebSocketRejectsUntrustedOriginWithoutBearer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := tasks.NewService(st.DB, "customer", time.Now)
	server := NewServer(registry.New(), &registry.Ctx{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	server.SetTasks(tasks.NewHub(svc), func() tasks.Actor {
		return tasks.CustomerActor("customer")
	})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	_, response, err := websocket.Dial(context.Background(), taskWSURL(httpServer.URL),
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil {
		t.Fatal("untrusted tokenless origin unexpectedly opened Tasks websocket")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("response = %#v, want HTTP 403", response)
	}

	_, response, err = websocket.Dial(context.Background(),
		taskWSURL(httpServer.URL)+"?token=bogus",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil {
		t.Fatal("unvalidated query token bypassed Tasks websocket origin check")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("bogus-token response = %#v, want HTTP 403", response)
	}
}

func taskWSURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + "/api/tasks/ws"
}

func readTaskHint(t *testing.T, ws *websocket.Conn) tasks.EventHint {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, raw, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var hint tasks.EventHint
	if err := json.Unmarshal(raw, &hint); err != nil {
		t.Fatal(err)
	}
	return hint
}

func TestWorkflowRouteTemplatesAppearInOpenAPIAndBindPathParameters(t *testing.T) {
	reg := registry.New()
	for _, route := range []registry.Command{
		{
			Path: "test.workflow.get", Summary: "Get workflow", CLIHidden: true,
			Args:         []registry.Arg{{Name: "version", Type: registry.Int, Required: true, Help: "Version"}, {Name: "verbose", Type: registry.Bool, Help: "Verbose view"}},
			ResultSchema: map[string]any{"type": "object", "required": []string{"name", "version"}},
			HTTP:         &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/workflows/{name}/versions/{version}"},
			Handler: func(_ *registry.Ctx, p registry.Params) (any, error) {
				return map[string]any{"name": p["name"], "version": p["version"]}, nil
			},
		},
		{
			Path: "test.workflow.action", Summary: "Workflow action", CLIHidden: true,
			Args:         []registry.Arg{{Name: "key", Required: true, Help: "Task key"}, {Name: "revision", Type: registry.Int, Required: true, Help: "Expected revision"}},
			ResultSchema: map[string]any{"type": "object", "required": []string{"key", "action"}},
			HTTP:         &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/tasks/{key}/workflow-override"},
			Handler: func(_ *registry.Ctx, p registry.Params) (any, error) {
				return map[string]any{"key": p["key"], "action": p["action"]}, nil
			},
		},
	} {
		if err := reg.Register(route); err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(reg, &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	resp, err := httpServer.Client().Get(httpServer.URL + "/api/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var document struct {
		Result struct {
			Paths map[string]map[string]any `json:"paths"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if document.Result.Paths["/api/workflows/{name}/versions/{version}"]["get"] == nil ||
		document.Result.Paths["/api/tasks/{key}/workflow-override"]["post"] == nil {
		t.Fatalf("workflow route templates missing from OpenAPI: %#v", document.Result.Paths)
	}
	action := document.Result.Paths["/api/tasks/{key}/workflow-override"]["post"].(map[string]any)
	if action["requestBody"] == nil || action["responses"] == nil || action["parameters"] == nil {
		t.Fatalf("typed workflow action operation = %#v", action)
	}
	getOperation := document.Result.Paths["/api/workflows/{name}/versions/{version}"]["get"].(map[string]any)
	parameters := getOperation["parameters"].([]any)
	byName := map[string]map[string]any{}
	for _, raw := range parameters {
		p := raw.(map[string]any)
		byName[p["name"].(string)] = p
	}
	if byName["name"]["in"] != "path" || byName["name"]["schema"].(map[string]any)["type"] != "string" ||
		byName["version"]["schema"].(map[string]any)["type"] != "integer" || byName["verbose"]["in"] != "query" {
		t.Fatalf("GET parameter inference = %#v", byName)
	}
	bodySchema := action["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	properties := bodySchema["properties"].(map[string]any)
	if properties["key"] != nil || properties["revision"].(map[string]any)["type"] != "integer" {
		t.Fatalf("POST body schema includes path or misses revision: %#v", bodySchema)
	}

	resp, err = httpServer.Client().Get(httpServer.URL + "/api/workflows/dev/versions/7")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Result map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Result["name"] != "dev" || envelope.Result["version"] != "7" {
		t.Fatalf("bound workflow path params = %#v", envelope.Result)
	}
}

func TestRegistryHTTPHandlerReceivesRequestCancellation(t *testing.T) {
	reg := registry.New()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	if err := reg.Register(registry.Command{
		Path: "test.request.context", Summary: "request context", CLIHidden: true,
		HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/test/request-context"},
		Handler: func(_ *registry.Ctx, p registry.Params) (any, error) {
			ctx := registry.RequestContext(p)
			close(started)
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		},
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(reg, &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/test/request-context", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		resp, _ := httpServer.Client().Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not observe request cancellation")
	}
	<-done
}

func TestTasksWebSocketReplaysLiveEventsAndResumesWithoutDuplicate(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := tasks.NewService(st.DB, "customer", time.Now)
	hub := tasks.NewHub(svc)
	svc.SetHub(hub)
	customer := tasks.CustomerActor("customer")
	ctx := context.Background()
	_, _ = svc.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: "WS", Name: "WS"})
	task, _ := svc.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "WS", Title: "before"})

	server := NewServer(registry.New(), &registry.Ctx{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	server.SetTasks(hub, func() tasks.Actor { return customer })
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	ws, _, err := websocket.Dial(ctx, taskWSURL(httpServer.URL)+"?after=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	queueHint := readTaskHint(t, ws)
	if queueHint.Kind != "task.queue_created" || queueHint.Queue != "WS" {
		t.Fatalf("queue hint = %#v", queueHint)
	}
	first := readTaskHint(t, ws)
	if first.TaskKey != task.Key || first.Kind != "task.created" {
		t.Fatalf("first hint = %#v", first)
	}
	title := "after"
	updated, err := svc.UpdateTask(ctx, customer, task.Key, tasks.UpdateTaskInput{
		Title: &title, Revision: task.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	live := readTaskHint(t, ws)
	if live.Sequence <= first.Sequence || live.TaskRevision != updated.Revision {
		t.Fatalf("live hint = %#v after %#v", live, first)
	}
	_ = ws.Close(websocket.StatusNormalClosure, "reconnect")

	resumed, _, err := websocket.Dial(ctx,
		taskWSURL(httpServer.URL)+"?after="+strconv.FormatInt(live.Sequence, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.CloseNow()
	title = "second reconnect event"
	updated, err = svc.UpdateTask(ctx, customer, task.Key, tasks.UpdateTaskInput{
		Title: &title, Revision: updated.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	next := readTaskHint(t, resumed)
	if next.Sequence <= live.Sequence || next.TaskRevision != updated.Revision {
		t.Fatalf("resumed hint = %#v; previous = %#v", next, live)
	}
}
