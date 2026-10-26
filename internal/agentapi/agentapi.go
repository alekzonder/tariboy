// Package agentapi serves the per-agent capability socket (spec §8): loop done,
// whoami, context get/set, status. Routes are gated by the agent's plugins.
package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/tidy"
	"github.com/alekzonder/tariboy/internal/version"
)

type Deps struct {
	// Log receives the full text of an error the agent is answered with only a
	// generic 500. Nil discards it.
	Log         *slog.Logger
	Agent       string
	Cwd         string
	ContextPath string
	Plugins     []string
	// CurrentPlugins overrides the startup snapshot when image activation
	// changes capabilities between iterations.
	CurrentPlugins   func() []string
	CurrentIteration func() string
	SetDone          func(iterationID string, productive bool) error
	Status           func() (map[string]any, error)
	// SetStatus records the agent-authored status message and returns the saved
	// {message, updated}. Gated by the `status` plugin. Nil yields unavailable.
	SetStatus func(message string) (map[string]any, error)
	// SetGoal selects an assigned active Native Task and updates the current
	// iteration's live AI-proxy attribution. Nil yields unavailable.
	SetGoal func(key string) (map[string]any, error)

	// MailAction runs one /tools/mail/<action> call as this agent. Nil yields
	// unavailable.
	MailAction func(action string, body map[string]any) (any, error)

	// Schedule/script surface (OPTIONAL plugins: gated).
	// AddSchedule's to is the recipient principal; channel is the legacy
	// field older skills send, mapped to a recipient by the daemon.
	AddSchedule        func(kind, spec, to, channel, template string) (map[string]any, error)
	ListSchedules      func() ([]map[string]any, error)
	CancelSchedule     func(id string) error
	RunScriptOnce      func(script.CreateOnce) (script.Definition, script.Run, error)
	ScheduleScript     func(script.CreateSchedule) (script.Definition, script.Run, error)
	RerunScript        func(scriptID string) (script.Run, error)
	ListScripts        func() ([]script.Definition, error)
	ListScriptRuns     func(scriptID string) ([]script.Run, error)
	GetScriptRun       func(runID string) (script.Run, error)
	LogScriptRun       func(runID string) (string, error)
	CancelScriptTarget func(id string) error
	RemoveScript       func(id string) error

	// Image authoring surface (OPTIONAL image-creator capability: gated). Nil
	// yields unavailable. BuildImage authors + builds a new image from a
	// Tariboyfile at the given path and calls image.Build.
	BuildImage func(name, tag, path string) (map[string]any, error)

	// Group coordination surface. These callbacks are daemon-wired so an agent
	// can coordinate only within its own group through the per-agent tools socket.
	GroupInfo    func() (map[string]any, error)
	GroupStatus  func(member string) (map[string]any, error)
	GroupObserve func(member string, tail int) (map[string]any, error)
	GroupLoop    func(member, action string) (map[string]any, error)
	LoopControl  func(action string) (map[string]any, error)
	// TaskAction is the native Tasks surface. The daemon binds the caller from
	// this socket's Agent field; request bodies never carry identity.
	TaskAction func(action string, body map[string]any) (any, error)
}

type Server struct {
	d     Deps
	http  *http.Server
	ctxMu sync.Mutex // guards ContextPath (CONTEXT.md) reads/writes
}

func NewServer(d Deps) *Server {
	s := &Server{d: d}
	s.http = api.NewHTTPServer(s.Handler())
	return s
}

func (s *Server) has(plugin string) bool {
	plugins := s.d.Plugins
	if s.d.CurrentPlugins != nil {
		plugins = s.d.CurrentPlugins()
	}
	for _, p := range plugins {
		// messages is the old name of mail, kept for images built before it.
		if p == plugin || (plugin == "mail" && p == "messages") {
			return true
		}
	}
	return false
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Built-in capability routes are explicit, including the historical core.
	mux.HandleFunc("GET /tools/whoami", s.gated("whoami", func(w http.ResponseWriter, r *http.Request) {
		api.WriteOK(w, map[string]any{
			"agent": s.d.Agent, "cwd": s.d.Cwd, "iteration": s.d.CurrentIteration(),
			// The daemon's own build, so the Whoami skill can print it next to the
			// version of the client that asked (SUPER-224 §4).
			"daemon_version": version.Version,
		})
	}))
	completeLoop := func(w http.ResponseWriter, r *http.Request) {
		id := s.d.CurrentIteration()
		if id == "" {
			api.WriteErr(w, http.StatusConflict, "no_iteration", "no iteration is currently running")
			return
		}
		// `i-am-done --idle` marks the iteration idle (not productive); a plain
		// done leaves it productive. productive = !idle.
		var body struct {
			Idle bool `json:"idle"`
		}
		if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
			return
		}
		if err := s.d.SetDone(id, !body.Idle); err != nil {
			s.fail(w, r, err)
			return
		}
		api.WriteOK(w, map[string]any{"done": true, "iteration": id, "productive": !body.Idle})
	}
	mux.HandleFunc("POST /tools/loop/done", s.gated("loop", completeLoop))
	// `complete` is the socket-native spelling for harnesses that are themselves
	// running under a supervising agent. It has the same authenticated,
	// per-agent semantics as the CLI-facing `loop done` route, without invoking
	// an ambient i-am-done command in the parent process tree.
	mux.HandleFunc("POST /tools/loop/complete", s.gated("loop", completeLoop))
	mux.HandleFunc("POST /tools/loop/control", s.gated("loop", s.loopControl))

	// Mail capability surface; images that still list the old messages
	// capability reach it through the alias in has.
	mux.HandleFunc("POST /tools/mail/{action}", s.gated("mail", s.mailAction))

	// Optional routes: gated by plugin membership.
	mux.HandleFunc("GET /tools/context/get", s.gated("context", s.contextGet))
	mux.HandleFunc("POST /tools/context/set", s.gated("context", s.contextSet))
	mux.HandleFunc("GET /tools/status", s.gated("status", s.status))
	mux.HandleFunc("POST /tools/status/set", s.gated("status", s.statusSet))

	// Schedule/script surface (OPTIONAL plugins: gated).
	mux.HandleFunc("POST /tools/schedule/add", s.gated("schedule", s.scheduleAdd))
	mux.HandleFunc("GET /tools/schedule/ls", s.gated("schedule", s.scheduleLs))
	mux.HandleFunc("POST /tools/schedule/cancel", s.gated("schedule", s.scheduleCancel))
	mux.HandleFunc("POST /tools/script/run", s.gated("scripts", s.scriptRunOnce))
	mux.HandleFunc("POST /tools/script/schedule", s.gated("scripts", s.scriptSchedule))
	mux.HandleFunc("POST /tools/script/rerun", s.gated("scripts", s.scriptRerun))
	mux.HandleFunc("GET /tools/script/ls", s.gated("scripts", s.scriptLs))
	mux.HandleFunc("GET /tools/script/runs/{id}", s.gated("scripts", s.scriptRuns))
	mux.HandleFunc("GET /tools/script/logs/{id}", s.gated("scripts", s.scriptLogs))
	mux.HandleFunc("POST /tools/script/cancel", s.gated("scripts", s.scriptCancel))
	mux.HandleFunc("POST /tools/script/rm", s.gated("scripts", s.scriptRemove))
	mux.HandleFunc("POST /tools/image/build", s.gated("image-creator", s.imageBuild))
	mux.HandleFunc("POST /tools/goal/set", s.gated("goal", s.goalSet))
	mux.HandleFunc("POST /tools/tasks/{action}", s.gated("tasks", s.nativeTaskAction))
	mux.HandleFunc("GET /tools/group/info", s.groupInfo)
	mux.HandleFunc("GET /tools/group/status", s.groupStatus)
	mux.HandleFunc("GET /tools/group/status/{member}", s.groupStatus)
	mux.HandleFunc("GET /tools/group/observe/{member}", s.groupObserve)
	mux.HandleFunc("POST /tools/group/loop", s.groupLoop)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		api.WriteErr(w, http.StatusNotFound, "not_found", "unknown agent route "+r.Method+" "+r.URL.Path)
	})
	// One wrapper for the whole agent-facing surface: every response carries the
	// daemon's version, so a shim pinned to an older build can notice the drift.
	return api.VersionHeader(mux)
}

func (s *Server) goalSet(w http.ResponseWriter, r *http.Request) {
	if s.d.SetGoal == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "Goal selection is unavailable")
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	body.Key = strings.TrimSpace(body.Key)
	if body.Key == "" {
		api.WriteErr(w, http.StatusBadRequest, "missing_key", "key is required")
		return
	}
	result, err := s.d.SetGoal(body.Key)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, result)
}

func (s *Server) nativeTaskAction(w http.ResponseWriter, r *http.Request) {
	if s.d.TaskAction == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "native Tasks service is unavailable")
		return
	}
	body := map[string]any{}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	for _, key := range []string{"author", "actor", "customer", "principal_identity", "iteration", "iteration_id"} {
		delete(body, key)
	}
	// `principal` is an action argument for `ask`, never caller identity. Keep it
	// only for that action; scrub it everywhere else.
	action := r.PathValue("action")
	if action != "ask" {
		delete(body, "principal")
	}
	result, err := s.d.TaskAction(action, body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, result)
}

func (s *Server) loopControl(w http.ResponseWriter, r *http.Request) {
	if s.d.LoopControl == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "loop control is not available")
		return
	}
	var body struct{ Action string }
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if body.Action != "start" && body.Action != "stop" {
		api.WriteErr(w, http.StatusBadRequest, "bad_action", "action must be start or stop")
		return
	}
	res, err := s.d.LoopControl(body.Action)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, res)
}

func (s *Server) gated(plugin string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.has(plugin) {
			api.WriteErr(w, http.StatusNotFound, "plugin_disabled",
				"command not available: plugin '"+plugin+"' not enabled for this agent")
			return
		}
		h(w, r)
	}
}

// fail answers err through the shared error mapper, so a domain error gets the
// same status and code as on the operator API and any other error is a 500
// whose text stays in the daemon log.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	api.WriteError(w, s.d.Log, "agent "+s.d.Agent+" "+r.Method+" "+r.URL.Path, err)
}

func (s *Server) contextGet(w http.ResponseWriter, r *http.Request) {
	s.ctxMu.Lock()
	data, err := os.ReadFile(s.d.ContextPath)
	s.ctxMu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"text": string(data)})
}

func (s *Server) contextSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if err := s.writeContextAtomic([]byte(body.Text)); err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"saved": true})
}

// writeContextAtomic writes data to ContextPath atomically: it creates a
// temp file in the same directory, writes and closes it, then renames it
// over the destination. This avoids concurrent readers ever observing a
// partially-written or corrupted CONTEXT.md, and the mutex serializes
// concurrent writers so the final content is always exactly one full write.
func (s *Server) writeContextAtomic(data []byte) error {
	s.ctxMu.Lock()
	defer s.ctxMu.Unlock()

	dir := filepath.Dir(s.d.ContextPath)
	tmp, err := os.CreateTemp(dir, ".context-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.d.ContextPath)
}

// mailAction is the agent mail surface (mailbox design §3): the sender and the
// mailbox are always this socket's agent, never a body field.
func (s *Server) mailAction(w http.ResponseWriter, r *http.Request) {
	if s.d.MailAction == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "mail is unavailable")
		return
	}
	body := map[string]any{}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	for _, key := range []string{"from", "as", "sender", "principal"} {
		delete(body, key)
	}
	result, err := s.d.MailAction(r.PathValue("action"), body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, result)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	out, err := s.d.Status()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, out)
}

func (s *Server) statusSet(w http.ResponseWriter, r *http.Request) {
	if s.d.SetStatus == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "status set is not available")
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	out, err := s.d.SetStatus(body.Message)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, out)
}

func (s *Server) scheduleAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind, Spec, To, Channel string
		Message                 json.RawMessage `json:"message"`
	}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	tpl := "{}"
	if len(body.Message) > 0 {
		tpl = string(body.Message)
	}
	res, err := s.d.AddSchedule(body.Kind, body.Spec, body.To, body.Channel, tpl)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, res)
}

func (s *Server) scheduleLs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.d.ListSchedules()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"schedules": rows, "count": len(rows)})
}

func (s *Server) scheduleCancel(w http.ResponseWriter, r *http.Request) {
	var body struct{ ID string }
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if err := s.d.CancelSchedule(body.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"cancelled": body.ID})
}

func (s *Server) scriptRunOnce(w http.ResponseWriter, r *http.Request) {
	if s.d.RunScriptOnce == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	var body script.CreateOnce
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if body.Description == "" {
		body.Description = body.Name
	}
	definition, run, err := s.d.RunScriptOnce(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"script": scriptView(definition), "run": scriptRunView(run)})
}

func (s *Server) scriptSchedule(w http.ResponseWriter, r *http.Request) {
	if s.d.ScheduleScript == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	var body script.CreateSchedule
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if body.Description == "" {
		body.Description = body.Name
	}
	definition, run, err := s.d.ScheduleScript(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"script": scriptView(definition), "run": scriptRunView(run)})
}

func (s *Server) scriptLs(w http.ResponseWriter, r *http.Request) {
	if s.d.ListScripts == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	records, err := s.d.ListScripts()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows := make([]map[string]any, 0, len(records))
	for _, definition := range records {
		rows = append(rows, scriptView(definition))
	}
	api.WriteOK(w, map[string]any{"scripts": rows, "count": len(rows)})
}

func (s *Server) scriptRerun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if s.d.RerunScript == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	run, err := s.d.RerunScript(body.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, scriptRunView(run))
}

func (s *Server) scriptRuns(w http.ResponseWriter, r *http.Request) {
	if s.d.ListScriptRuns == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	runs, err := s.d.ListScriptRuns(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, scriptRunView(run))
	}
	api.WriteOK(w, map[string]any{"runs": rows, "count": len(rows)})
}

func (s *Server) scriptLogs(w http.ResponseWriter, r *http.Request) {
	if s.d.GetScriptRun == nil || s.d.LogScriptRun == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	run, err := s.d.GetScriptRun(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	logText, err := s.d.LogScriptRun(run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"run": scriptRunView(run), "log": logText})
}

func (s *Server) scriptCancel(w http.ResponseWriter, r *http.Request) {
	var body struct{ ID string }
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if s.d.CancelScriptTarget == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	if err := s.d.CancelScriptTarget(body.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"id": body.ID, "cancelled": true})
}

func (s *Server) scriptRemove(w http.ResponseWriter, r *http.Request) {
	var body struct{ ID string }
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if s.d.RemoveScript == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "scripts are not available")
		return
	}
	if err := s.d.RemoveScript(body.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, map[string]any{"id": body.ID, "removed": true})
}

func scriptView(definition script.Definition) map[string]any {
	row := map[string]any{"id": definition.ID, "agent": definition.Agent, "name": definition.Name, "description": definition.Description,
		"command": definition.Command, "mode": definition.Mode, "interval_seconds": definition.IntervalSeconds, "state": definition.State,
		"created_at": definition.CreatedAt, "next_run_at": definition.NextRunAt}
	if definition.LatestRun != nil {
		row["latest_run"] = scriptRunView(*definition.LatestRun)
	}
	return row
}

func scriptRunView(run script.Run) map[string]any {
	row := map[string]any{"id": run.ID, "script_id": run.ScriptID, "agent": run.Agent, "status": run.Status,
		"created_at": run.CreatedAt, "started_at": run.StartedAt, "finished_at": run.FinishedAt, "log_path": run.LogPath}
	if run.PID != nil {
		row["pid"] = *run.PID
	}
	if run.ExitCode != nil {
		row["exit_code"] = *run.ExitCode
	}
	return row
}

func (s *Server) imageBuild(w http.ResponseWriter, r *http.Request) {
	if s.d.BuildImage == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "image build is not available")
		return
	}
	var body struct{ Name, Tag, Path string }
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if body.Name == "" || body.Path == "" {
		api.WriteErr(w, http.StatusBadRequest, "missing_args", "name and path are required")
		return
	}
	// An empty tag is passed through: the builder then publishes the source
	// image_version plus latest, exactly as the operator CLI does.
	res, err := s.d.BuildImage(body.Name, body.Tag, body.Path)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, res)
}

func (s *Server) groupInfo(w http.ResponseWriter, r *http.Request) {
	if s.d.GroupInfo == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "group tools are not available")
		return
	}
	res, err := s.d.GroupInfo()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, res)
}

func (s *Server) groupStatus(w http.ResponseWriter, r *http.Request) {
	if s.d.GroupStatus == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "group tools are not available")
		return
	}
	res, err := s.d.GroupStatus(r.PathValue("member"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, res)
}

func (s *Server) groupObserve(w http.ResponseWriter, r *http.Request) {
	if s.d.GroupObserve == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "group tools are not available")
		return
	}
	tail := 80
	if raw := r.URL.Query().Get("tail"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			api.WriteErr(w, http.StatusBadRequest, "bad_tail", "tail must be a positive integer")
			return
		}
		tail = n
	}
	res, err := s.d.GroupObserve(r.PathValue("member"), tail)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, res)
}

func (s *Server) groupLoop(w http.ResponseWriter, r *http.Request) {
	if s.d.GroupLoop == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "group tools are not available")
		return
	}
	var body struct{ Member, Action string }
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	if body.Member == "" {
		api.WriteErr(w, http.StatusBadRequest, "missing_member", "member is required")
		return
	}
	if body.Action != "start" && body.Action != "stop" {
		api.WriteErr(w, http.StatusBadRequest, "bad_action", "action must be start or stop")
		return
	}
	res, err := s.d.GroupLoop(body.Member, body.Action)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, res)
}

// Listen binds the unix socket. Split out from Serve so callers can bind
// synchronously (surfacing a bind failure as an error) before serving in a
// goroutine, instead of losing the error inside a fire-and-forget Serve.
func (s *Server) Listen(sock string) (net.Listener, error) {
	tidy.Remove(nil, sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// ServeListener serves HTTP on an already-bound listener until Shutdown.
func (s *Server) ServeListener(ln net.Listener) error {
	err := s.http.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Serve binds and serves in one call (Listen + ServeListener).
func (s *Server) Serve(sock string) error {
	ln, err := s.Listen(sock)
	if err != nil {
		return err
	}
	return s.ServeListener(ln)
}

func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

// maxBodyBytes bounds every agent tool request body.
const maxBodyBytes = 1 << 20
