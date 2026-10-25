package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Catches a running daemon whose served spec differs from the committed
// snapshot, or whose responses do not match the spec. It runs only from
// scripts/tariboy-tasks-e2e.sh, which starts an isolated daemon and names
// the socket and the objects its scenario created. The variables carry no
// TARIBOY_ prefix because TestMain clears those.
func TestDaemonOpenAPIContract(t *testing.T) {
	sock := os.Getenv("OPENAPI_E2E_SOCKET")
	if sock == "" {
		t.Skip("set OPENAPI_E2E_SOCKET to the socket of an isolated daemon")
	}
	agentName, queue, task := os.Getenv("OPENAPI_E2E_AGENT"), os.Getenv("OPENAPI_E2E_QUEUE"), os.Getenv("OPENAPI_E2E_TASK")
	if agentName == "" || queue == "" || task == "" {
		t.Fatal("set OPENAPI_E2E_AGENT, OPENAPI_E2E_QUEUE and OPENAPI_E2E_TASK")
	}
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
	status, body := rawRequest(t, client, "GET", "http://tariboyd/api/openapi.json", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /api/openapi.json = %d %s", status, body)
	}
	var envelope struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	err := json.Unmarshal(body, &envelope)
	if err != nil {
		t.Fatal(err)
	}
	// The snapshot pins info.version; the daemon reports its build version.
	var info map[string]any
	if err := json.Unmarshal(envelope.Result["info"], &info); err != nil {
		t.Fatal(err)
	}
	info["version"] = openAPISnapshotVersion
	if envelope.Result["info"], err = json.Marshal(info); err != nil {
		t.Fatal(err)
	}
	served, err := json.Marshal(envelope.Result)
	if err != nil {
		t.Fatal(err)
	}
	served = indentOpenAPI(t, served)
	want, err := os.ReadFile(openAPISnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if diff := firstLineDiff(want, served); diff != "" {
		t.Fatalf("the daemon's OpenAPI spec differs from %s at %s", openAPISnapshot, diff)
	}
	validator := newOpenAPIValidator(t, served)
	routes := []struct{ route, url string }{
		{"/api/agents", "/api/agents"},
		{"/api/agents/{name}", "/api/agents/" + agentName},
		{"/api/agents/{name}/status", "/api/agents/" + agentName + "/status"},
		{"/api/agents/{name}/budget", "/api/agents/" + agentName + "/budget"},
		{"/api/agents/{name}/image", "/api/agents/" + agentName + "/image"},
		{"/api/task-queues/{queue}/pools", "/api/task-queues/" + queue + "/pools"},
		{"/api/task-queues/{queue}/workflow-triggers", "/api/task-queues/" + queue + "/workflow-triggers"},
		{"/api/task-queues/{queue}/secrets", "/api/task-queues/" + queue + "/secrets"},
		{"/api/task-queues/{queue}/env", "/api/task-queues/" + queue + "/env"},
		{"/api/task-queues/{queue}/sources", "/api/task-queues/" + queue + "/sources"},
		{"/api/tasks", "/api/tasks"},
		{"/api/tasks/{key}", "/api/tasks/" + task},
	}
	for _, r := range routes {
		status, body := rawRequest(t, client, "GET", "http://tariboyd"+r.url, nil)
		if status != http.StatusOK {
			t.Errorf("GET %s = %d %s", r.url, status, body)
			continue
		}
		if err := validator.validate("GET", r.route, status, body); err != nil {
			t.Errorf("%s: %v\nresponse: %s", r.url, err, abbreviate(body))
		}
	}
}

// abbreviate shortens a response body for a failure message.
func abbreviate(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) > 2000 {
		return string(body[:2000]) + "..."
	}
	return strings.TrimSpace(string(body))
}
