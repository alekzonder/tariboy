package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const openAPIResource = "openapi.json"

// openAPIValidator checks HTTP responses against the schemas of a served
// OpenAPI spec. The spec is one JSON Schema resource, so component
// references resolve inside it.
type openAPIValidator struct {
	compiler *jsonschema.Compiler
	paths    map[string]map[string]any
	compiled map[string]*jsonschema.Schema
}

func newOpenAPIValidator(t *testing.T, spec []byte) *openAPIValidator {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(spec))
	if err != nil {
		t.Fatal(err)
	}
	doc = openAPINullable(doc)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(openAPIResource, doc); err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(spec, &parsed); err != nil {
		t.Fatal(err)
	}
	return &openAPIValidator{compiler: compiler, paths: parsed.Paths, compiled: map[string]*jsonschema.Schema{}}
}

// openAPINullable rewrites the OpenAPI 3.0 keyword nullable, which JSON
// Schema does not know, into a type and enum that admit null.
func openAPINullable(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = openAPINullable(item)
		}
		if v["nullable"] != true {
			return v
		}
		delete(v, "nullable")
		if typ, ok := v["type"].(string); ok {
			v["type"] = []any{typ, "null"}
		}
		if enum, ok := v["enum"].([]any); ok {
			v["enum"] = append(enum, nil)
		}
	case []any:
		for i, item := range v {
			v[i] = openAPINullable(item)
		}
	}
	return value
}

// validate checks body against the response schema the spec declares for
// method and route: the success envelope for 200, the error envelope for
// any other status.
func (v *openAPIValidator) validate(method, route string, status int, body []byte) error {
	operation := strings.ToLower(method)
	if _, ok := v.paths[route][operation]; !ok {
		return fmt.Errorf("the spec has no operation %s %s", method, route)
	}
	response := "default"
	if status == http.StatusOK {
		response = "200"
	}
	location := openAPIResource + "#/paths/" + jsonPointerEscape(route) + "/" + operation + "/responses/" + response + "/content/application~1json/schema"
	schema, ok := v.compiled[location]
	if !ok {
		var err error
		if schema, err = v.compiler.Compile(location); err != nil {
			return fmt.Errorf("compile %s %s response %s: %w", method, route, response, err)
		}
		v.compiled[location] = schema
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, route, err)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("%s %s status %d: %w", method, route, status, err)
	}
	return nil
}

// typedOperations returns "METHOD route" for each operation whose success
// result has a declared shape: a component reference, an array of one, or
// properties. A free
// object result is checked only at the envelope level.
func (v *openAPIValidator) typedOperations() []string {
	var out []string
	for route, operations := range v.paths {
		for method, raw := range operations {
			operation, _ := raw.(map[string]any)
			result := dig(operation, "responses", "200", "content", "application/json", "schema", "properties", "result")
			_, ref := result["$ref"]
			_, props := result["properties"]
			_, items := dig(result, "items")["$ref"]
			if ref || props || items {
				out = append(out, strings.ToUpper(method)+" "+route)
			}
		}
	}
	slices.Sort(out)
	return out
}

func dig(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		m, _ = m[key].(map[string]any)
	}
	return m
}

func jsonPointerEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// Catches a validator that accepts what the spec forbids: an extra field in
// a strict result, a missing required field, a wrong type, null outside a
// nullable field, and a malformed error envelope.
func TestOpenAPIValidator(t *testing.T) {
	t.Parallel()
	spec := []byte(`{
	  "openapi": "3.0.3",
	  "paths": {"/api/things/{name}": {"get": {"responses": {
	    "200": {"content": {"application/json": {"schema": {"type": "object", "required": ["ok", "result"],
	      "properties": {"ok": {"type": "boolean"}, "result": {"$ref": "#/components/schemas/Thing"}}}}}},
	    "default": {"content": {"application/json": {"schema": {"type": "object", "required": ["ok", "error"],
	      "properties": {"ok": {"type": "boolean"}, "error": {"type": "object", "required": ["code", "message"]}}}}}}
	  }}},
	  "/api/things": {"get": {"responses": {
	    "200": {"content": {"application/json": {"schema": {"type": "object", "required": ["ok", "result"],
	      "properties": {"ok": {"type": "boolean"}, "result": {"type": "array", "items": {"$ref": "#/components/schemas/Thing"}}}}}}}
	  }}}},
	  "components": {"schemas": {"Thing": {"type": "object", "additionalProperties": false, "required": ["name", "tags"],
	    "properties": {"name": {"type": "string"}, "tags": {"type": "array", "items": {"type": "string"}, "nullable": true}}}}}
	}`)
	v := newOpenAPIValidator(t, spec)
	route := "/api/things/{name}"
	cases := []struct {
		name   string
		status int
		body   string
		valid  bool
	}{
		{"matching result", 200, `{"ok":true,"result":{"name":"a","tags":["x"]}}`, true},
		{"null in a nullable field", 200, `{"ok":true,"result":{"name":"a","tags":null}}`, true},
		{"extra field", 200, `{"ok":true,"result":{"name":"a","tags":[],"extra":1}}`, false},
		{"missing required field", 200, `{"ok":true,"result":{"name":"a"}}`, false},
		{"wrong type", 200, `{"ok":true,"result":{"name":1,"tags":[]}}`, false},
		{"null outside a nullable field", 200, `{"ok":true,"result":{"name":null,"tags":[]}}`, false},
		{"error envelope", 404, `{"ok":false,"error":{"code":"not_found","message":"no thing"}}`, true},
		{"malformed error envelope", 404, `{"ok":false,"error":{"code":"not_found"}}`, false},
	}
	for _, tc := range cases {
		err := v.validate("GET", route, tc.status, []byte(tc.body))
		if tc.valid && err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("%s: accepted %s", tc.name, tc.body)
		}
	}
	if err := v.validate("POST", route, 200, []byte(`{}`)); err == nil {
		t.Error("an operation the spec lacks was accepted")
	}
	if err := v.validate("GET", "/api/things", 200, []byte(`{"ok":true,"result":[{"name":"a"}]}`)); err == nil {
		t.Error("an array item missing a required field was accepted")
	}
	if got := v.typedOperations(); !slices.Equal(got, []string{"GET /api/things", "GET " + route}) {
		t.Errorf("typedOperations = %v", got)
	}
}

// typedCall is one HTTP request of the response contract table.
type typedCall struct {
	method, route, url string
	body               any
}

// Catches a typed HTTP response that does not match the schema the served
// spec declares for it, and a typed operation the table does not call. The
// agents fixture and the native Tasks service back the calls; the workflow
// stub answers the routes that need a workflow engine.
func TestTypedResponsesMatchOpenAPI(t *testing.T) {
	c, fc, ref, _ := agentsFixture(t)
	svc := tasks.NewService(c.Store.DB, "customer", time.Now)
	c.Tasks = &workflowStub{TaskControl: svc}
	c.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	httpServer := httptest.NewServer(api.NewServer(BuildRegistry(), c).Handler())
	t.Cleanup(httpServer.Close)
	if _, err := c.Store.DB.Exec(`INSERT INTO agents(name, image_ref, image_digest) VALUES ('dev-a', 'basic:latest', 'digest')`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateQueue(t.Context(), tasks.CustomerActor("customer"), tasks.CreateQueueInput{Prefix: "FLOW", Name: "Flow"}); err != nil {
		t.Fatal(err)
	}
	validator := newOpenAPIValidator(t, servedOpenAPI(t))
	calls := append(agentTypedCalls(ref), taskTypedCalls()...)
	called := map[string]bool{}
	validateTypedCalls(t, httpServer, validator, calls, called, func(call typedCall) {
		fc.live = ""
		if call.url == "/api/agents/gamma" {
			fc.live = "error"
		}
	})
	for _, name := range slices.Sorted(maps.Keys(typedCallSets)) {
		t.Run(name, func(t *testing.T) {
			sc, setCalls := typedCallSets[name](t)
			if sc.Log == nil {
				sc.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
			}
			setServer := httptest.NewServer(api.NewServer(BuildRegistry(), sc).Handler())
			t.Cleanup(setServer.Close)
			validateTypedCalls(t, setServer, validator, setCalls, called, nil)
		})
	}
	for _, operation := range validator.typedOperations() {
		if !called[operation] {
			t.Errorf("typed operation %s has no call in TestTypedResponsesMatchOpenAPI: add one", operation)
		}
	}
}

// typedCallSet prepares the state one domain's typed responses need and
// returns the context to serve them from and the calls to validate. A domain
// adds its set to typedCallSets from an init function in its own test file.
type typedCallSet func(t *testing.T) (*registry.Ctx, []typedCall)

// typedCallSets holds the per-domain call sets of TestTypedResponsesMatchOpenAPI.
var typedCallSets = map[string]typedCallSet{}

// validateTypedCalls sends each call, records its operation as called, and
// checks the response against the spec; before runs ahead of each call.
func validateTypedCalls(t *testing.T, server *httptest.Server, validator *openAPIValidator, calls []typedCall, called map[string]bool, before func(typedCall)) {
	t.Helper()
	for _, call := range calls {
		if before != nil {
			before(call)
		}
		status, body := rawRequest(t, server.Client(), call.method, server.URL+call.url, call.body)
		if status != http.StatusOK {
			t.Errorf("%s %s = %d %s; the table expects a success", call.method, call.url, status, body)
			continue
		}
		called[call.method+" "+call.route] = true
		if err := validator.validate(call.method, call.route, status, body); err != nil {
			t.Errorf("%s: %v\nresponse: %s", call.url, err, body)
		}
	}
}

func agentTypedCalls(ref image.Ref) []typedCall {
	return []typedCall{
		{"GET", "/api/agents", "/api/agents", nil},
		{"GET", "/api/agents/{name}", "/api/agents/alpha", nil},
		{"GET", "/api/agents/{name}", "/api/agents/beta", nil},
		{"GET", "/api/agents/{name}", "/api/agents/gamma", nil},
		{"GET", "/api/agents/{name}/status", "/api/agents/alpha/status", nil},
		{"GET", "/api/agents/{name}/status", "/api/agents/beta/status", nil},
		{"GET", "/api/agents/{name}/budget", "/api/agents/alpha/budget", nil},
		{"POST", "/api/agents/{name}/budget", "/api/agents/beta/budget", map[string]any{"hour_usd": "1.5", "day_usd": "2", "week_usd": "3", "month_usd": "4"}},
		{"GET", "/api/agents/{name}/image", "/api/agents/alpha/image", nil},
		{"GET", "/api/agents/{name}/image", "/api/agents/beta/image", nil},
		{"POST", "/api/agents/{name}/image", "/api/agents/alpha/image", map[string]any{"image": ref.String()}},
		{"DELETE", "/api/agents/{name}/image", "/api/agents/alpha/image", nil},
		{"POST", "/api/agents/{name}/goal-enabled", "/api/agents/alpha/goal-enabled", map[string]any{"enabled": false}},
		{"POST", "/api/agents/{name}/goal-wait-customer-timeout", "/api/agents/alpha/goal-wait-customer-timeout", map[string]any{"seconds": 120}},
		{"POST", "/api/agents/{name}/goal-delivery-cooldown", "/api/agents/alpha/goal-delivery-cooldown", map[string]any{"seconds": 90}},
		{"POST", "/api/agents", "/api/agents", map[string]any{"image": "basic:latest", "name": "delta", "goal_enabled": false}},
		{"POST", "/api/agents/{name}/stop", "/api/agents/alpha/stop", nil},
		{"POST", "/api/agents/{name}/start", "/api/agents/alpha/start", nil},
		{"POST", "/api/agents/{name}/restart", "/api/agents/alpha/restart", nil},
		{"POST", "/api/agents/{name}/kill", "/api/agents/alpha/kill", nil},
		{"POST", "/api/agents/{name}/reprovision", "/api/agents/alpha/reprovision", nil},
		{"DELETE", "/api/agents/{name}", "/api/agents/beta?purge=true", nil},
	}
}

func taskTypedCalls() []typedCall {
	return []typedCall{
		{"PATCH", "/api/task-queues/{queue}/pools/{pool}", "/api/task-queues/FLOW/pools/developers", map[string]any{"agents": []string{"dev-a"}, "revision": 0, "idempotency_key": "pool"}},
		{"GET", "/api/task-queues/{queue}/pools/{pool}", "/api/task-queues/FLOW/pools/developers", nil},
		{"GET", "/api/task-queues/{queue}/pools", "/api/task-queues/FLOW/pools", nil},
		{"POST", "/api/task-queues/{queue}/workflow-triggers", "/api/task-queues/FLOW/workflow-triggers", map[string]any{"pattern": "plugin:*", "action": "create_task"}},
		{"GET", "/api/task-queues/{queue}/workflow-triggers", "/api/task-queues/FLOW/workflow-triggers", nil},
		{"PUT", "/api/task-queues/{queue}/workflow", "/api/task-queues/FLOW/workflow", map[string]any{"ref": "flow:1.0.0"}},
		{"GET", "/api/task-queues/{queue}/workflow", "/api/task-queues/FLOW/workflow", nil},
		{"DELETE", "/api/task-queues/{queue}/workflow", "/api/task-queues/FLOW/workflow?revision=1", nil},
		{"PUT", "/api/task-queues/{queue}/secrets/{key}", "/api/task-queues/FLOW/secrets/GH_TOKEN", map[string]any{"value": "s"}},
		{"GET", "/api/task-queues/{queue}/secrets", "/api/task-queues/FLOW/secrets", nil},
		{"DELETE", "/api/task-queues/{queue}/secrets/{key}", "/api/task-queues/FLOW/secrets/GH_TOKEN", nil},
		{"PUT", "/api/task-queues/{queue}/env/{key}", "/api/task-queues/FLOW/env/LOG_LEVEL", map[string]any{"value": "debug"}},
		{"GET", "/api/task-queues/{queue}/env", "/api/task-queues/FLOW/env", nil},
		{"DELETE", "/api/task-queues/{queue}/env/{key}", "/api/task-queues/FLOW/env/LOG_LEVEL", nil},
		{"GET", "/api/task-queues/{queue}/sources", "/api/task-queues/FLOW/sources", nil},
		{"GET", "/api/task-queues/{queue}/source-runs/{id}/log", "/api/task-queues/FLOW/source-runs/3/log", nil},
		{"POST", "/api/tasks/{key}/advance", "/api/tasks/FLOW-1/advance", map[string]any{"outcome": "ready"}},
		{"PUT", "/api/tasks/{key}/artifacts/{name}", "/api/tasks/FLOW-1/artifacts/plan", map[string]any{"value": "p"}},
		{"GET", "/api/tasks/{key}/artifacts", "/api/tasks/FLOW-1/artifacts", nil},
		{"GET", "/api/tasks/{key}/artifacts/{name}", "/api/tasks/FLOW-1/artifacts/plan", nil},
		{"GET", "/api/tasks/{key}/workflow", "/api/tasks/FLOW-1/workflow", nil},
		{"GET", "/api/tasks/{key}/workflow/requests/{id}", "/api/tasks/FLOW-1/workflow/requests/7", nil},
		{"POST", "/api/tasks/{key}/workflow/requests/{id}/retry", "/api/tasks/FLOW-1/workflow/requests/7/retry", nil},
		{"GET", "/api/tasks/{key}/workflow/runs", "/api/tasks/FLOW-1/workflow/runs", nil},
		{"GET", "/api/tasks/{key}/workflow/runs/{id}", "/api/tasks/FLOW-1/workflow/runs/4", nil},
		{"GET", "/api/tasks/{key}/workflow/runs/{id}/log", "/api/tasks/FLOW-1/workflow/runs/4/log", nil},
		{"POST", "/api/tasks/{key}/workflow/move", "/api/tasks/FLOW-1/workflow/move", map[string]any{"to": "review", "reason": "r"}},
		{"POST", "/api/tasks/{key}/workflow/resume", "/api/tasks/FLOW-1/workflow/resume", map[string]any{"decision": "continue"}},
		{"POST", "/api/tasks/{key}/workflow/watch", "/api/tasks/FLOW-1/workflow/watch", nil},
		{"POST", "/api/tasks/{key}/cancel", "/api/tasks/FLOW-1/cancel", nil},
		{"POST", "/api/tasks/{key}/requeue", "/api/tasks/FLOW-1/requeue", map[string]any{"queue": "FLOW", "revision": 1}},
	}
}

// rawRequest sends a JSON request and returns the status and the raw body.
func rawRequest(t *testing.T, client *http.Client, method, url string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}
