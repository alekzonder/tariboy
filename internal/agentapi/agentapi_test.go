package agentapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestNativeTasksActionReturnsErrorDataToTheAgent(t *testing.T) {
	server := NewServer(Deps{
		Agent: "alice", Plugins: []string{"whoami", "loop", "messages", "tasks"},
		TaskAction: func(string, map[string]any) (any, error) {
			return nil, &tasks.Error{Status: http.StatusConflict, Code: "workflow_managed", Msg: "managed",
				Data: map[string]any{"status": "develop", "outcomes": []string{"ready"}}}
		},
	})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest("POST", "/tools/tasks/done", bytes.NewBufferString(`{"key":"DEV-1"}`)))
	var env struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || env.Error.Code != "workflow_managed" || env.Error.Details["status"] != "develop" {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func decode(t *testing.T, body []byte) (bool, map[string]any) {
	t.Helper()
	var env struct {
		OK     bool           `json:"ok"`
		Result map[string]any `json:"result"`
		Error  map[string]any `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	if !env.OK {
		return false, env.Error
	}
	return true, env.Result
}

func TestPluginGatingReadsCurrentImageCapabilities(t *testing.T) {
	plugins := []string{}
	server := NewServer(Deps{
		CurrentPlugins: func() []string { return plugins },
		SetStatus:      func(string) (map[string]any, error) { return map[string]any{"ok": true}, nil },
	})
	handler := server.Handler()
	request := func() int {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/status/set", bytes.NewBufferString(`{"message":"working"}`)))
		return rr.Code
	}
	if got := request(); got != http.StatusNotFound {
		t.Fatalf("disabled status=%d", got)
	}
	plugins = []string{"status"}
	if got := request(); got != http.StatusOK {
		t.Fatalf("enabled status=%d", got)
	}
}

func TestExplicitCorePluginsGateTheirCompleteRouteSurface(t *testing.T) {
	plugins := []string{}
	doneCalls := 0
	server := NewServer(Deps{
		CurrentPlugins:   func() []string { return plugins },
		CurrentIteration: func() string { return "iter-1" },
		SetDone:          func(string, bool) error { doneCalls++; return nil },
	})
	handler := server.Handler()
	tests := []struct {
		plugin string
		method string
		path   string
	}{
		{plugin: "whoami", method: http.MethodGet, path: "/tools/whoami"},
		{plugin: "loop", method: http.MethodPost, path: "/tools/loop/done"},
		{plugin: "loop", method: http.MethodPost, path: "/tools/loop/complete"},
		{plugin: "loop", method: http.MethodPost, path: "/tools/loop/control"},
		{plugin: "mail", method: http.MethodPost, path: "/tools/mail/inbox"},
		{plugin: "mail", method: http.MethodPost, path: "/tools/mail/send"},
		{plugin: "mail", method: http.MethodPost, path: "/tools/mail/reply"},
	}

	for _, tc := range tests {
		t.Run(tc.plugin+" "+tc.path, func(t *testing.T) {
			beforeDone := doneCalls
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{}`)))
			ok, body := decode(t, rr.Body.Bytes())
			if rr.Code != http.StatusNotFound || ok || body["code"] != "plugin_disabled" {
				t.Fatalf("disabled route response=%d %v", rr.Code, body)
			}
			if doneCalls != beforeDone {
				t.Fatalf("disabled route reached SetDone: calls %d -> %d", beforeDone, doneCalls)
			}

			plugins = []string{tc.plugin}
			rr = httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{}`)))
			if rr.Code == http.StatusNotFound {
				t.Fatalf("enabled route remained hidden: %s", rr.Body.String())
			}
			if (tc.path == "/tools/loop/done" || tc.path == "/tools/loop/complete") && doneCalls != beforeDone+1 {
				t.Fatalf("enabled loop completion did not reach SetDone: calls %d -> %d", beforeDone, doneCalls)
			}
			plugins = nil
		})
	}
}

func newTestServer(t *testing.T, plugins []string) (*Server, *string, string) {
	t.Helper()
	ctxPath := filepath.Join(t.TempDir(), "CONTEXT.md")
	current := "iter-1"
	var doneSet string
	s := NewServer(Deps{
		Agent: "smoke", Cwd: "/w", ContextPath: ctxPath, Plugins: plugins,
		CurrentIteration: func() string { return current },
		SetDone:          func(id string, _ bool) error { doneSet = id; return nil },
		Status:           func() (map[string]any, error) { return map[string]any{"state": "running"}, nil },
	})
	return s, &doneSet, ctxPath
}

func TestTaskCurrentRouteIsRemoved(t *testing.T) {
	s := NewServer(Deps{Agent: "smoke", ContextPath: filepath.Join(t.TempDir(), "CONTEXT.md"),
		Plugins: []string{"whoami", "loop", "messages", "current-task"}})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/tools/task/current", bytes.NewBufferString(`{"id":"x"}`)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	ok, e := decode(t, rr.Body.Bytes())
	if ok || e["code"] != "not_found" {
		t.Fatalf("error = %v", e)
	}
}

func TestGoalSetIsCapabilityGated(t *testing.T) {
	called := ""
	server := NewServer(Deps{
		Plugins: []string{"goal"},
		SetGoal: func(key string) (map[string]any, error) {
			called = key
			return map[string]any{"task_id": key, "epic_id": "T-1"}, nil
		},
	})
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tools/goal/set", bytes.NewBufferString(`{"key":"T-2"}`)))
	ok, result := decode(t, rr.Body.Bytes())
	if rr.Code != http.StatusOK || !ok || called != "T-2" || result["epic_id"] != "T-1" {
		t.Fatalf("enabled response=%d %v called=%q", rr.Code, result, called)
	}

	server.d.Plugins = nil
	rr = httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tools/goal/set", bytes.NewBufferString(`{"key":"T-2"}`)))
	if rr.Code != http.StatusNotFound || called != "T-2" {
		t.Fatalf("disabled response=%d called=%q", rr.Code, called)
	}
}

func TestNativeTasksActionIsCapabilityGatedAndScrubsForgedIdentity(t *testing.T) {
	disabled := NewServer(Deps{
		Agent: "alice", Plugins: []string{"whoami", "loop", "messages"},
		TaskAction: func(string, map[string]any) (any, error) {
			t.Fatal("disabled tasks action must not run")
			return nil, nil
		},
	})
	request := httptest.NewRequest("POST", "/tools/tasks/create",
		bytes.NewBufferString(`{"queue":"TEST","title":"work","author":"user:forged"}`))
	recorder := httptest.NewRecorder()
	disabled.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("disabled status = %d; want 404", recorder.Code)
	}

	var gotAction string
	var gotBody map[string]any
	enabled := NewServer(Deps{
		Agent: "alice", Plugins: []string{"whoami", "loop", "messages", "tasks"},
		TaskAction: func(action string, body map[string]any) (any, error) {
			gotAction, gotBody = action, body
			return map[string]any{"key": "TEST-1", "author": "agent:alice"}, nil
		},
	})
	request = httptest.NewRequest("POST", "/tools/tasks/create",
		bytes.NewBufferString(`{
			"queue":"TEST","title":"work",
			"author":"user:forged","actor":"user:forged","customer":"user:forged","iteration_id":"forged"
		}`))
	recorder = httptest.NewRecorder()
	enabled.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("enabled status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if gotAction != "create" || gotBody["queue"] != "TEST" {
		t.Fatalf("action/body = %q/%v", gotAction, gotBody)
	}
	for _, forbidden := range []string{"author", "actor", "customer", "principal", "iteration_id"} {
		if _, exists := gotBody[forbidden]; exists {
			t.Fatalf("forged identity %q reached task service: %v", forbidden, gotBody)
		}
	}
}

func TestMailActionAcceptsMessagesAliasAndScrubsIdentity(t *testing.T) {
	var gotAction string
	var gotBody map[string]any
	s := NewServer(Deps{Agent: "alice", Plugins: []string{"messages"},
		MailAction: func(action string, body map[string]any) (any, error) {
			gotAction, gotBody = action, body
			return map[string]any{"letters": []any{}}, nil
		},
	})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/tools/mail/send",
		bytes.NewBufferString(`{"to":"agent:bob","text":"x","from":"agent:forged","as":"user:forged","sender":"x","principal":"x"}`)))
	if rr.Code != http.StatusOK || gotAction != "send" || gotBody["to"] != "agent:bob" {
		t.Fatalf("send through the messages alias: status=%d action=%q body=%v", rr.Code, gotAction, gotBody)
	}
	for _, forged := range []string{"from", "as", "sender", "principal"} {
		if _, ok := gotBody[forged]; ok {
			t.Fatalf("forged identity %q reached the mail service: %v", forged, gotBody)
		}
	}
}

func TestMailActionAnswersMailErrors(t *testing.T) {
	s := NewServer(Deps{Agent: "alice", Plugins: []string{"mail"},
		MailAction: func(string, map[string]any) (any, error) {
			return nil, &mail.Error{Status: http.StatusConflict, Code: "answer_on_task", Msg: "answer on the task"}
		},
	})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/tools/mail/reply", bytes.NewBufferString(`{"id":"m-1","text":"x"}`)))
	ok, e := decode(t, rr.Body.Bytes())
	if ok || rr.Code != http.StatusConflict || e["code"] != "answer_on_task" {
		t.Fatalf("reply = %d %v", rr.Code, e)
	}
}

func TestWhoamiAndLoopDone(t *testing.T) {
	s, doneSet, _ := newTestServer(t, []string{"whoami", "loop", "messages"})
	h := s.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/tools/whoami", nil))
	ok, res := decode(t, rr.Body.Bytes())
	if !ok || res["agent"] != "smoke" || res["cwd"] != "/w" || res["iteration"] != "iter-1" {
		t.Fatalf("whoami = %v", res)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/loop/done", nil))
	if ok, _ := decode(t, rr.Body.Bytes()); !ok || *doneSet != "iter-1" {
		t.Fatalf("loop done not recorded: %q", *doneSet)
	}

	*doneSet = ""
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/loop/complete", nil))
	if ok, _ := decode(t, rr.Body.Bytes()); !ok || *doneSet != "iter-1" {
		t.Fatalf("loop complete not recorded: %q", *doneSet)
	}
}

func TestContextGatedByPlugin(t *testing.T) {
	// context plugin absent -> 404 plugin_disabled
	s, _, _ := newTestServer(t, []string{"whoami", "loop", "messages"})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/tools/context/get", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	ok, e := decode(t, rr.Body.Bytes())
	if ok || e["code"] != "plugin_disabled" {
		t.Fatalf("error = %v", e)
	}
}

func TestContextSetGet(t *testing.T) {
	s, _, ctxPath := newTestServer(t, []string{"whoami", "loop", "messages", "context"})
	h := s.Handler()

	body, _ := json.Marshal(map[string]string{"text": "remember this"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/context/set", bytes.NewReader(body)))
	if ok, _ := decode(t, rr.Body.Bytes()); !ok {
		t.Fatalf("context set failed: %s", rr.Body.String())
	}
	if data, _ := os.ReadFile(ctxPath); string(data) != "remember this" {
		t.Fatalf("CONTEXT.md = %q", data)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/tools/context/get", nil))
	ok, res := decode(t, rr.Body.Bytes())
	if !ok || res["text"] != "remember this" {
		t.Fatalf("context get = %v", res)
	}
}

func TestContextSetConcurrent(t *testing.T) {
	s, _, ctxPath := newTestServer(t, []string{"whoami", "loop", "messages", "context"})
	h := s.Handler()

	payloadA := strings.Repeat("A", 2048)
	payloadB := strings.Repeat("B", 2048)

	doSet := func(text string) {
		body, _ := json.Marshal(map[string]string{"text": text})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/context/set", bytes.NewReader(body)))
		if ok, _ := decode(t, rr.Body.Bytes()); !ok {
			t.Errorf("context set failed: %s", rr.Body.String())
		}
	}
	doGet := func() {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/tools/context/get", nil))
		ok, res := decode(t, rr.Body.Bytes())
		if !ok {
			t.Errorf("context get failed: %s", rr.Body.String())
			return
		}
		text, _ := res["text"].(string)
		if text != "" && text != payloadA && text != payloadB {
			t.Errorf("context get returned corrupted/partial content: len=%d", len(text))
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch i % 3 {
			case 0:
				doSet(payloadA)
			case 1:
				doSet(payloadB)
			case 2:
				doGet()
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(ctxPath)
	if err != nil {
		t.Fatalf("read CONTEXT.md: %v", err)
	}
	got := string(data)
	if got != payloadA && got != payloadB {
		t.Fatalf("final CONTEXT.md is neither full payload (corrupted/truncated): len=%d", len(got))
	}
}

func TestStatusRoute(t *testing.T) {
	s, _, _ := newTestServer(t, []string{"whoami", "loop", "messages", "status"})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/tools/status", nil))
	ok, res := decode(t, rr.Body.Bytes())
	if !ok || res["state"] != "running" {
		t.Fatalf("status = %v", res)
	}
}

func TestStatusSetRoute(t *testing.T) {
	var gotMsg string
	newSrv := func(plugins []string, withHook bool) *Server {
		d := Deps{
			Agent: "smoke", Plugins: plugins,
			CurrentIteration: func() string { return "iter-1" },
			SetDone:          func(string, bool) error { return nil },
			Status:           func() (map[string]any, error) { return map[string]any{"state": "running"}, nil },
		}
		if withHook {
			d.SetStatus = func(msg string) (map[string]any, error) {
				gotMsg = msg
				return map[string]any{"message": msg, "updated": "2026-07-09T10:00:00Z"}, nil
			}
		}
		return NewServer(d)
	}
	post := func(s *Server) (bool, map[string]any) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tools/status/set", strings.NewReader(`{"message":"cloning repo"}`))
		s.Handler().ServeHTTP(rr, req)
		return decode(t, rr.Body.Bytes())
	}

	// happy path: status plugin + hook present.
	ok, res := post(newSrv([]string{"whoami", "loop", "messages", "status"}, true))
	if !ok || gotMsg != "cloning repo" || res["message"] != "cloning repo" {
		t.Fatalf("status set = %v gotMsg=%q", res, gotMsg)
	}
	// gated: status plugin absent -> plugin_disabled.
	if ok, e := post(newSrv([]string{"whoami", "loop", "messages"}, true)); ok || e["code"] != "plugin_disabled" {
		t.Fatalf("expected plugin_disabled, got ok=%v e=%v", ok, e)
	}
	// unavailable: hook nil -> unavailable.
	if ok, e := post(newSrv([]string{"whoami", "loop", "messages", "status"}, false)); ok || e["code"] != "unavailable" {
		t.Fatalf("expected unavailable, got ok=%v e=%v", ok, e)
	}
}

func TestGroupToolsInfoStatusLoopAndObserve(t *testing.T) {
	var loopStarted, loopStopped string
	s := NewServer(Deps{
		Agent: "manager", Cwd: "/w", Plugins: []string{"whoami", "loop", "messages", "status"},
		CurrentIteration: func() string { return "iter-manager" },
		SetDone:          func(string, bool) error { return nil },
		Status:           func() (map[string]any, error) { return map[string]any{"state": "running"}, nil },
		GroupInfo: func() (map[string]any, error) {
			return map[string]any{
				"agent": "manager", "group": "dev-team", "lead": "manager", "role": "lead",
				"members": []string{"manager", "worker"}, "broadcast": "group:dev-team:broadcast",
				"inbox": "group:dev-team:inbox", "shared_dir": "/groups/dev-team/shared",
			}, nil
		},
		GroupStatus: func(member string) (map[string]any, error) {
			rows := []map[string]any{
				{"name": "manager", "role": "lead", "state": "running", "enabled": true, "status_message": "coordinating"},
				{"name": "worker", "role": "member", "state": "idle", "enabled": true, "status_message": "waiting"},
			}
			if member != "" {
				if member != "worker" {
					return nil, errors.New("agent is not in your group")
				}
				return map[string]any{"member": rows[1]}, nil
			}
			return map[string]any{"members": rows, "count": len(rows)}, nil
		},
		GroupObserve: func(member string, tail int) (map[string]any, error) {
			return map[string]any{"events": []map[string]any{
				{"seq": float64(2), "kind": "status", "source": "agent", "data": `{"message":"done"}`},
			}, "count": 1, "tail": tail}, nil
		},
		GroupLoop: func(member, action string) (map[string]any, error) {
			if action == "start" {
				loopStarted = member
			} else {
				loopStopped = member
			}
			return map[string]any{"member": member, "action": action}, nil
		},
		LoopControl: func(action string) (map[string]any, error) {
			return map[string]any{"agent": "manager", "action": action}, nil
		},
	})
	h := s.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/tools/group/info", nil))
	ok, res := decode(t, rr.Body.Bytes())
	if !ok || res["group"] != "dev-team" || res["role"] != "lead" {
		t.Fatalf("group info = %v", res)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/tools/group/status", nil))
	ok, res = decode(t, rr.Body.Bytes())
	if !ok || res["count"].(float64) != 2 {
		t.Fatalf("group status = %v", res)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/tools/group/status/worker", nil))
	ok, res = decode(t, rr.Body.Bytes())
	if !ok || res["member"].(map[string]any)["name"] != "worker" {
		t.Fatalf("group status worker = %v", res)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/tools/group/observe/worker?tail=7", nil))
	ok, res = decode(t, rr.Body.Bytes())
	if !ok || res["tail"].(float64) != 7 || res["count"].(float64) != 1 {
		t.Fatalf("group observe = %v", res)
	}

	body, _ := json.Marshal(map[string]any{"member": "worker", "action": "start"})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/group/loop", bytes.NewReader(body)))
	if ok, _ := decode(t, rr.Body.Bytes()); !ok || loopStarted != "worker" {
		t.Fatalf("group loop start failed: %s started=%q", rr.Body.String(), loopStarted)
	}
	body, _ = json.Marshal(map[string]any{"member": "worker", "action": "stop"})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/group/loop", bytes.NewReader(body)))
	if ok, _ := decode(t, rr.Body.Bytes()); !ok || loopStopped != "worker" {
		t.Fatalf("group loop stop failed: %s stopped=%q", rr.Body.String(), loopStopped)
	}

	body, _ = json.Marshal(map[string]any{"action": "start"})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/loop/control", bytes.NewReader(body)))
	ok, res = decode(t, rr.Body.Bytes())
	if !ok || res["action"] != "start" {
		t.Fatalf("loop control = %v", res)
	}
}

func TestGroupToolsUnavailable(t *testing.T) {
	s := NewServer(Deps{
		Agent: "solo", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "iter-1" },
		SetDone:          func(string, bool) error { return nil },
	})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/tools/group/info", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	ok, e := decode(t, rr.Body.Bytes())
	if ok || !strings.Contains(e["message"].(string), "group tools are not available") {
		t.Fatalf("error = %v", e)
	}
}

func TestScheduleAndScriptGated(t *testing.T) {
	// schedule/script absent from plugins -> 404 plugin_disabled.
	s := NewServer(Deps{Agent: "smoke", Plugins: []string{"whoami", "loop", "messages"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil }})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/tools/schedule/ls", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("schedule ls status = %d, want 404", rr.Code)
	}
}

func TestScheduleAddAndScriptRun(t *testing.T) {
	var ran string
	s := NewServer(Deps{
		Agent: "smoke", Plugins: []string{"whoami", "loop", "messages", "schedule", "scripts"},
		CurrentIteration: func() string { return "i" }, SetDone: func(string, bool) error { return nil },
		AddSchedule: func(kind, spec, to, channel, tpl string) (map[string]any, error) {
			return map[string]any{"id": "sch-1", "kind": kind, "to": to}, nil
		},
		ListSchedules: func() ([]map[string]any, error) { return []map[string]any{{"id": "sch-1"}}, nil },
		RunScriptOnce: func(in script.CreateOnce) (script.Definition, script.Run, error) {
			ran = in.Name
			return script.Definition{ID: "scr-1", Name: in.Name, Mode: script.ModeOnce, State: script.StateActive}, script.Run{ID: "srun-1", ScriptID: "scr-1", Status: script.RunPending}, nil
		},
		ListScripts: func() ([]script.Definition, error) { return []script.Definition{{ID: "scr-0", Name: "greet"}}, nil },
	})
	h := s.Handler()

	body, _ := json.Marshal(map[string]any{"kind": "cron", "spec": "*/5 * * * *"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/schedule/add", bytes.NewReader(body)))
	if ok, res := decode(t, rr.Body.Bytes()); !ok || res["id"] != "sch-1" {
		t.Fatalf("schedule add = %v", res)
	}

	rbody, _ := json.Marshal(map[string]any{"name": "greet", "description": "greet", "command": "echo hi"})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/script/run", bytes.NewReader(rbody)))
	if ok, _ := decode(t, rr.Body.Bytes()); !ok || ran != "greet" {
		t.Fatalf("script run not invoked: ran=%q", ran)
	}
}

func TestScriptToolsCreateAndCancel(t *testing.T) {
	var created script.CreateOnce
	var cancelled string
	s := NewServer(Deps{
		Agent: "smoke", Plugins: []string{"scripts"},
		RunScriptOnce: func(in script.CreateOnce) (script.Definition, script.Run, error) {
			created = in
			return script.Definition{ID: "scr-1", Agent: "smoke", Name: in.Name, Description: in.Description, Command: in.Command, Mode: script.ModeOnce, State: script.StateActive}, script.Run{ID: "srun-1", ScriptID: "scr-1", Agent: "smoke", Status: script.RunPending}, nil
		},
		CancelScriptTarget: func(id string) error { cancelled = id; return nil },
	})
	h := s.Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/script/run", strings.NewReader(`{"name":"ci","description":"check","command":"true"}`)))
	ok, res := decode(t, rr.Body.Bytes())
	if !ok || rr.Code != http.StatusOK || res["script"].(map[string]any)["id"] != "scr-1" || created.Command != "true" {
		t.Fatalf("add status=%d result=%v create=%#v", rr.Code, res, created)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/tools/script/cancel", strings.NewReader(`{"id":"scr-1"}`)))
	if ok, _ := decode(t, rr.Body.Bytes()); !ok || rr.Code != http.StatusOK || cancelled != "scr-1" {
		t.Fatalf("cancel status=%d id=%q", rr.Code, cancelled)
	}
}

func TestScriptToolsRejectActiveRemoval(t *testing.T) {
	s := NewServer(Deps{
		Agent: "smoke", Plugins: []string{"scripts"},
		RemoveScript: func(string) error { return script.ErrActive },
	})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/tools/script/rm", strings.NewReader(`{"id":"scr-1"}`)))
	if ok, res := decode(t, rr.Body.Bytes()); ok || rr.Code != http.StatusConflict || res["code"] != "script_active" {
		t.Fatalf("remove status=%d ok=%v result=%v", rr.Code, ok, res)
	}
}

func TestImageBuildGatedAndDispatched(t *testing.T) {
	var gotTag, gotPath string
	deps := Deps{
		Agent: "creator", Cwd: "/w",
		Plugins: []string{"whoami", "loop", "messages", "image-creator"},
		BuildImage: func(name, tag, path string) (map[string]any, error) {
			gotTag, gotPath = name+":"+tag, path
			return map[string]any{"name": "authored", "tag": "latest", "digest": "deadbeef", "layers": 3}, nil
		},
	}
	s := NewServer(deps)

	// Positive: an image-creator agent can build.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/tools/image/build",
		strings.NewReader(`{"name":"authored","path":"authored"}`))
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("build status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// An empty tag reaches the builder, which then publishes the source
	// image_version plus latest, exactly as the operator CLI does.
	if gotTag != "authored:" || gotPath != "authored" {
		t.Fatalf("BuildImage called with (%q,%q)", gotTag, gotPath)
	}
	if !strings.Contains(rec.Body.String(), "deadbeef") {
		t.Fatalf("digest not returned: %s", rec.Body.String())
	}

	// Negative (privilege boundary): a non-creator agent is refused.
	deps2 := Deps{
		Agent: "plain", Cwd: "/w",
		Plugins:    []string{"whoami", "loop", "messages"},
		BuildImage: func(name, tag, path string) (map[string]any, error) { t.Fatal("must not be called"); return nil, nil },
	}
	s2 := NewServer(deps2)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/tools/image/build",
		strings.NewReader(`{"name":"evil","path":"x"}`))
	s2.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != 404 {
		t.Fatalf("non-creator build status = %d, want 404 (plugin_disabled)", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "plugin_disabled") {
		t.Fatalf("expected plugin_disabled, got %s", rec2.Body.String())
	}
}

func TestRemovedMessageRoutesAreNotRegistered(t *testing.T) {
	s := NewServer(Deps{Plugins: []string{"messages", "mail"}})
	for _, route := range []struct {
		method string
		path   string
	}{
		{"POST", "/tools/chat/create"},
		{"GET", "/tools/chat/unanswered"},
		{"POST", "/tools/message/send"},
		{"GET", "/tools/message/ls"},
		{"POST", "/tools/message/processed"},
		{"POST", "/tools/request"},
		{"POST", "/tools/channel/subscribe"},
		{"GET", "/tools/sources"},
		{"POST", "/tools/group/send"},
		{"POST", "/tools/group/request"},
	} {
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, httptest.NewRequest(route.method, route.path, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want 404", route.method, route.path, rr.Code)
		}
	}
}

func TestServerHasHeaderTimeout(t *testing.T) {
	s := NewServer(Deps{})
	if s.http.ReadHeaderTimeout <= 0 || s.http.IdleTimeout <= 0 || s.http.MaxHeaderBytes <= 0 {
		t.Fatalf("agent API server has no header limits: %+v", s.http)
	}
}

func TestContextSetRejectsOversizedBody(t *testing.T) {
	s, _, ctxPath := newTestServer(t, []string{"whoami", "loop", "messages", "context"})
	body := `{"text":"` + strings.Repeat("x", maxBodyBytes) + `"}`
	for _, declared := range []bool{true, false} {
		req := httptest.NewRequest("POST", "/tools/context/set", strings.NewReader(body))
		if !declared {
			req.ContentLength = -1
		}
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rr.Body.String(), `"too_large"`) {
			t.Fatalf("declared=%v: status = %d, body = %.200s", declared, rr.Code, rr.Body.String())
		}
	}
	if _, err := os.Stat(ctxPath); !os.IsNotExist(err) {
		t.Fatalf("oversized body wrote CONTEXT.md: %v", err)
	}
}
