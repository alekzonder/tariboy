package commands

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// crossStub records the advance input as JSON too.
type crossStub struct{ inputStub }

func (s *crossStub) Advance(_ context.Context, a tasks.Actor, key string, in tasks.AdvanceInput) (tasks.TransitionRequest, error) {
	s.input(a, "advance", key, in)
	return tasks.TransitionRequest{}, nil
}

// TestTaskPayloadDecodesAlikeOverHTTPAndTheSocket sends one payload to a task
// route and decodes it as the agent socket does: both must build the same
// service input.
func TestTaskPayloadDecodesAlikeOverHTTPAndTheSocket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action  string
		payload string
	}{
		{"list", `{"queue":" DEV ","status":"plan, review","category":["open"],"priority":"P0,P1","assignee":"w","group":"g","text":"t","parent":"DEV-1","sort":"updated","limit":10,"after":"DEV-9","blocked":"true","status_view":"all"}`},
		{"create", `{"queue":" DEV ","parent_key":"DEV-1","title":" T ","description":"D","pull_request":"https://example.test/pull/1","assignee":"w","group":"g","priority":"P1","idempotency_key":"k1"}`},
		{"update", `{"key":"DEV-1","title":" T ","pull_request":"","manual_block_reason":"","priority":"P0","revision":3}`},
		{"move", `{"key":"DEV-1","parent_key":" ","revision":"4"}`},
		{"done", `{"key":"DEV-1","revision":2,"complete_anyway":"true"}`},
		{"comment", `{"key":"DEV-1","body":" hello ","idempotency_key":"k1"}`},
		{"relate", `{"key":"DEV-1","target_key":"DEV-2","type":"related","revision":1,"idempotency_key":"k1"}`},
		{"advance", `{"key":"DEV-1","outcome":" ready ","message":" PR opened\n","from":" develop "}`},
	}
	stub := &crossStub{}
	server := api.NewServer(BuildRegistry(), &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Tasks: stub})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	for _, tt := range tests {
		var body map[string]any
		if err := json.Unmarshal([]byte(tt.payload), &body); err != nil {
			t.Fatal(err)
		}
		spec, _ := tasks.SpecFor(tt.action)
		route := strings.ReplaceAll(spec.Path, "{key}", "DEV-1")
		var httpBody any = body
		if spec.Method == "GET" {
			query := []string{}
			for name, value := range body {
				if text, ok := value.(string); ok {
					query = append(query, name+"="+strings.ReplaceAll(text, " ", "%20"))
				}
			}
			route, httpBody = route+"?"+strings.Join(query, "&"), nil
		}
		stub.calls = nil
		status, env := taskRequest(t, httpServer.Client(), spec.Method, httpServer.URL+route, httpBody)
		if status != 200 || len(stub.calls) != 1 {
			t.Fatalf("%s: %d %+v calls %q", tt.action, status, env.Error, stub.calls)
		}
		fromHTTP := strings.SplitN(stub.calls[0], " ", 3)[2]

		request := spec.New()
		tasks.Decode(body, request)
		method := "Input"
		if tt.action == "list" {
			method = "Filter"
		}
		fromSocket, _ := json.Marshal(reflect.ValueOf(request).MethodByName(method).Call(nil)[0].Interface())
		if tt.action == "list" {
			// The list query above carries only the text filters.
			var filter tasks.ListFilter
			_ = json.Unmarshal(fromSocket, &filter)
			filter.Categories, filter.Limit = nil, 0
			fromSocket, _ = json.Marshal(filter)
		}
		if fromHTTP != string(fromSocket) {
			t.Errorf("%s: HTTP input %s; socket input %s", tt.action, fromHTTP, fromSocket)
		}
	}
}
