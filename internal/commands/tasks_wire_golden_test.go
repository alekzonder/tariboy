package commands

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// inputStub records the task service input each task route builds, as JSON.
type inputStub struct{ workflowStub }

func (s *inputStub) input(a tasks.Actor, name, key string, in any) {
	raw, _ := json.Marshal(in)
	s.record(a, "%s %s %s", name, key, raw)
}
func (s *inputStub) ListTasks(_ context.Context, a tasks.Actor, in tasks.ListFilter) (tasks.TaskPage, error) {
	s.input(a, "list", "", in)
	return tasks.TaskPage{}, nil
}
func (s *inputStub) CreateTask(_ context.Context, a tasks.Actor, in tasks.CreateTaskInput) (tasks.Task, error) {
	s.input(a, "create", "", in)
	return tasks.Task{}, nil
}
func (s *inputStub) GetTask(_ context.Context, a tasks.Actor, key string) (tasks.TaskDetail, error) {
	s.input(a, "get", key, nil)
	return tasks.TaskDetail{}, nil
}
func (s *inputStub) UpdateTask(_ context.Context, a tasks.Actor, key string, in tasks.UpdateTaskInput) (tasks.Task, error) {
	s.input(a, "update", key, in)
	return tasks.Task{}, nil
}
func (s *inputStub) MoveTask(_ context.Context, a tasks.Actor, key string, in tasks.MoveInput) (tasks.Task, error) {
	s.input(a, "move", key, in)
	return tasks.Task{}, nil
}
func (s *inputStub) CompleteTask(_ context.Context, a tasks.Actor, key string, in tasks.CompleteInput) (tasks.Task, error) {
	s.input(a, "complete", key, in)
	return tasks.Task{}, nil
}
func (s *inputStub) AddComment(_ context.Context, a tasks.Actor, key string, in tasks.AddCommentInput) (tasks.CommentResult, error) {
	s.input(a, "comment", key, in)
	return tasks.CommentResult{}, nil
}
func (s *inputStub) AddRelation(_ context.Context, a tasks.Actor, key string, in tasks.RelationInput) (tasks.Relation, error) {
	s.input(a, "relation", key, in)
	return tasks.Relation{}, nil
}

// TestTaskRoutesGoldenInputs pins the service input each native task route
// builds from its HTTP parameters.
func TestTaskRoutesGoldenInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method, route string
		body          any
		want          string
	}{
		{"GET", "/api/tasks?queue=DEV&status=plan,%20review&category=open&priority=P0,P1&assignee=w&author=user:me&customer=user:you&group=g&text=%20t%20&waiting_for=me&scope_agent=a&parent=DEV-1&subtree=DEV-2&sort=updated&limit=10&after=DEV-9&blocked=false&status_view=all", nil, "user:customer|list  {\"Queue\":\"DEV\",\"Status\":\"\",\"Statuses\":[\"plan\",\"review\"],\"Categories\":[\"open\"],\"Priorities\":[\"P0\",\"P1\"],\"StatusView\":\"all\",\"Assignee\":\"w\",\"Author\":\"user:me\",\"Customer\":\"user:you\",\"Group\":\"g\",\"Text\":\"t\",\"WaitingFor\":\"me\",\"ScopeAgent\":\"a\",\"ParentKey\":\"DEV-1\",\"SubtreeKey\":\"DEV-2\",\"Sort\":\"updated\",\"Blocked\":false,\"Limit\":10,\"AfterKey\":\"DEV-9\"}"},
		{"POST", "/api/tasks", map[string]any{"queue": " DEV ", "parent_key": "DEV-1", "title": " T ", "description": "D", "pull_request": "https://example.test/pull/1", "assignee": "w", "group": "g", "priority": "P1", "idempotency_key": "k1"}, "user:customer|create  {\"queue\":\"DEV\",\"parent_key\":\"DEV-1\",\"title\":\"T\",\"description\":\"D\",\"pull_request\":\"https://example.test/pull/1\",\"assignee\":\"w\",\"group\":\"g\",\"priority\":\"P1\",\"idempotency_key\":\"k1\"}"},
		{"GET", "/api/tasks/DEV-1", nil, "user:customer|get DEV-1 null"},
		{"PATCH", "/api/tasks/DEV-1", map[string]any{"title": " T ", "description": "D", "status": "done", "pull_request": "", "assignee": "w", "manual_block_reason": "", "priority": "P0", "revision": 3}, "user:customer|update DEV-1 {\"title\":\" T \",\"description\":\"D\",\"status\":\"done\",\"pull_request\":\"\",\"assignee\":\"w\",\"manual_block_reason\":\"\",\"priority\":\"P0\",\"revision\":3}"},
		{"PATCH", "/api/tasks/DEV-1", map[string]any{"assignee": "worker", "revision": 2}, "user:customer|update DEV-1 {\"title\":null,\"description\":null,\"status\":null,\"pull_request\":null,\"assignee\":\"worker\",\"manual_block_reason\":null,\"priority\":null,\"revision\":2}"},
		{"POST", "/api/tasks/DEV-1/move", map[string]any{"parent_key": "", "revision": 4}, "user:customer|move DEV-1 {\"parent_key\":\"\",\"before_key\":\"\",\"revision\":4}"},
		{"POST", "/api/tasks/DEV-1/move", map[string]any{"parent_key": "DEV-2", "before_key": " DEV-3 ", "revision": "4"}, "user:customer|move DEV-1 {\"parent_key\":\"DEV-2\",\"before_key\":\"DEV-3\",\"revision\":4}"},
		{"POST", "/api/tasks/DEV-1/complete", map[string]any{"revision": 2, "complete_anyway": true}, "user:customer|complete DEV-1 {\"revision\":2,\"complete_anyway\":true}"},
		{"POST", "/api/tasks/DEV-1/complete", map[string]any{"revision": 2, "complete_anyway": "true"}, "user:customer|complete DEV-1 {\"revision\":2,\"complete_anyway\":true}"},
		{"POST", "/api/tasks/DEV-1/comments", map[string]any{"body": " hello ", "idempotency_key": "k1"}, "user:customer|comment DEV-1 {\"body\":\"hello\",\"idempotency_key\":\"k1\"}"},
		{"POST", "/api/tasks/DEV-2/relations", map[string]any{"target_key": "DEV-1", "type": "blocks", "revision": 1, "idempotency_key": "k1"}, "user:customer|relation DEV-2 {\"target_key\":\"DEV-1\",\"type\":\"blocks\",\"revision\":1,\"idempotency_key\":\"k1\"}"},
		{"POST", "/api/tasks/DEV-1/advance", map[string]any{"outcome": " ready ", "message": " PR opened\n", "from": " develop "}, "user:customer|advance DEV-1 \"ready\" \" PR opened\\n\" from=\"develop\""},
		{"PUT", "/api/tasks/DEV-1/artifacts/plan", map[string]any{"value": " step 1\n"}, "user:customer|artifact_set DEV-1 plan \" step 1\\n\""},
		{"GET", "/api/tasks/DEV-1/artifacts", nil, "user:customer|artifact_ls DEV-1"},
		{"GET", "/api/tasks/DEV-1/artifacts/plan", nil, "user:customer|artifact_show DEV-1 plan"},
		{"GET", "/api/tasks/DEV-1/workflow", nil, "user:customer|workflow_get DEV-1"},
		{"GET", "/api/tasks/DEV-1/workflow/runs", nil, "user:customer|runs DEV-1"},
		{"GET", "/api/tasks/DEV-1/workflow/runs/4/log?max_bytes=100", nil, "user:customer|run_log DEV-1 4 max=100"},
		{"GET", "/api/tasks/DEV-1/workflow/requests/41", nil, "user:customer|request_get DEV-1 41"},
		{"POST", "/api/tasks/DEV-1/workflow/requests/41/retry", map[string]any{}, "user:customer|request_retry DEV-1 41"},
		{"POST", "/api/tasks/DEV-1/workflow/move", map[string]any{"to": " develop ", "reason": " regression "}, "user:customer|workflow_move DEV-1 develop \"regression\""},
		{"POST", "/api/tasks/DEV-1/workflow/resume", map[string]any{"decision": " release "}, "user:customer|workflow_resume DEV-1 release"},
		{"POST", "/api/tasks/DEV-1/cancel", map[string]any{}, "user:customer|cancel DEV-1"},
	}
	stub := &inputStub{}
	server := api.NewServer(BuildRegistry(), &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Tasks: stub})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	for _, tt := range tests {
		stub.calls = nil
		status, env := taskRequest(t, httpServer.Client(), tt.method, httpServer.URL+tt.route, tt.body)
		if status != 200 || len(stub.calls) != 1 || stub.calls[0] != tt.want {
			t.Errorf("%s %s = %d %+v calls %q; want %q", tt.method, tt.route, status, env.Error, stub.calls, tt.want)
		}
	}
	// The list filters are declared arguments, so a malformed one is refused
	// before the service is called.
	stub.calls = nil
	for _, query := range []string{"blocked=maybe", "limit=x"} {
		status, env := taskRequest(t, httpServer.Client(), "GET", httpServer.URL+"/api/tasks?"+query, nil)
		if status != 400 || env.Error == nil || env.Error.Code != "invalid_param" || len(stub.calls) != 0 {
			t.Errorf("%s = %d %+v calls %q; want invalid_param", query, status, env.Error, stub.calls)
		}
	}
}
