package tasks

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Spec is one native task command. Its Request points to the zero request
// struct whose field tags define every wire form of the command: the ttasks
// flags and positional arguments, the agent tools-socket body, and the
// operator HTTP arguments.
//
//	json:"key"       the socket and HTTP key; "-" for a command-line option
//	flag:"name"      the ttasks flag
//	pos:"N", "N+"    the Nth positional argument, or every one from N on
//	label:"text"     how a usage error names a positional argument
//	required:"true"  required on the command line and in HTTP; "cli" or
//	                 "http" requires it in one of them
//	help:"text"      the HTTP argument description
//	enum:"a,b"       the values the HTTP schema lists
//	raw:"true"       the text is kept as given instead of trimmed
//	clear:"true"     an empty flag value is sent, so it clears the field
//	min:"N" max:"N"  the bounds of a number given on the command line
//	http:"-"         the key is not an HTTP argument
//
// Field types are string, *string (omitted versus empty), int, int64, bool,
// *bool, and []string (comma-separated in a flag); another type panics when
// the fields are first read. A request with a Validate method checks its
// cross-field rules on the command line.
type Spec struct {
	Action  string   // the tools-socket action, such as artifact_set
	Words   []string // the ttasks command, such as artifacts set; none for a socket-only action
	Request any
	// Method and Path are the operator HTTP route; empty for a command
	// without a route of its own.
	Method, Path string
	// Via names the action whose route the operator client calls instead.
	Via          string
	OperatorOnly bool
	agent        agentRun
}

// New returns a pointer to a new zero request of the command.
func (s Spec) New() any { return reflect.New(reflect.TypeOf(s.Request).Elem()).Interface() }

var specs = []Spec{
	{Action: "list", Words: []string{"list"}, Request: &ListRequest{}, Method: "GET", Path: "/api/tasks", agent: onAgent((*TaskService).agentList)},
	{Action: "mine", Words: []string{"mine"}, Request: &ListRequest{}, Via: "list", agent: onAgent((*TaskService).agentList)},
	{Action: "ready", Words: []string{"ready"}, Request: &ReadyRequest{}, agent: onAgent((*TaskService).agentReady)},
	{Action: "show", Words: []string{"show"}, Request: &KeyRequest{}, Method: "GET", Path: "/api/tasks/{key}", agent: onAgent((*TaskService).agentShow)},
	{Action: "create", Words: []string{"create"}, Request: &CreateRequest{}, Method: "POST", Path: "/api/tasks", agent: onAgent((*TaskService).agentCreate)},
	{Action: "update", Words: []string{"update"}, Request: &UpdateRequest{}, Method: "PATCH", Path: "/api/tasks/{key}", agent: onAgent((*TaskService).agentUpdate)},
	{Action: "assign", Words: []string{"assign"}, Request: &AssignRequest{}, Via: "update", agent: onAgent((*TaskService).agentAssign)},
	{Action: "comment", Words: []string{"comment"}, Request: &CommentRequest{}, Method: "POST", Path: "/api/tasks/{key}/comments", agent: onAgent((*TaskService).agentComment)},
	{Action: "ask", Words: []string{"ask"}, Request: &AskRequest{}, Via: "comment", agent: onAgent((*TaskService).agentAsk)},
	{Action: "move", Words: []string{"move"}, Request: &MoveRequest{}, Method: "POST", Path: "/api/tasks/{key}/move", agent: onAgent((*TaskService).agentMove)},
	{Action: "relate", Words: []string{"relate"}, Request: &RelateRequest{}, Method: "POST", Path: "/api/tasks/{key}/relations", agent: onAgent((*TaskService).agentRelate)},
	{Action: "block", Words: []string{"block"}, Request: &BlockRequest{}, Via: "relate", agent: onAgent((*TaskService).agentBlock)},
	{Action: "done", Words: []string{"done"}, Request: &DoneRequest{}, Method: "POST", Path: "/api/tasks/{key}/complete", agent: onAgent((*TaskService).agentDone)},
	{Action: "advance", Words: []string{"advance"}, Request: &AdvanceRequest{}, Method: "POST", Path: "/api/tasks/{key}/advance", agent: onAgent((*TaskService).agentAdvance)},
	{Action: "artifact_set", Words: []string{"artifacts", "set"}, Request: &ArtifactSetRequest{}, Method: "PUT", Path: "/api/tasks/{key}/artifacts/{name}", agent: onAgent((*TaskService).agentArtifactSet)},
	{Action: "artifact_ls", Words: []string{"artifacts", "ls"}, Request: &KeyRequest{}, Method: "GET", Path: "/api/tasks/{key}/artifacts", agent: onAgent((*TaskService).agentArtifactList)},
	{Action: "artifact_show", Words: []string{"artifacts", "show"}, Request: &ArtifactRequest{}, Method: "GET", Path: "/api/tasks/{key}/artifacts/{name}", agent: onAgent((*TaskService).agentArtifactShow)},
	{Action: "workflow_get", Words: []string{"workflow", "get"}, Request: &KeyRequest{}, Method: "GET", Path: "/api/tasks/{key}/workflow", agent: onAgent((*TaskService).agentWorkflowGet)},
	{Action: "workflow_runs", Words: []string{"workflow", "runs"}, Request: &KeyRequest{}, Method: "GET", Path: "/api/tasks/{key}/workflow/runs", agent: onAgent((*TaskService).agentRuns)},
	{Action: "workflow_run_log", Words: []string{"workflow", "log"}, Request: &RunLogRequest{}, Method: "GET", Path: "/api/tasks/{key}/workflow/runs/{id}/log", agent: onAgent((*TaskService).agentRunLog)},
	{Action: "request_get", Request: &RequestRef{}, Method: "GET", Path: "/api/tasks/{key}/workflow/requests/{id}", agent: onAgent((*TaskService).agentRequestGet)},
	{Action: "workflow_move", Words: []string{"workflow", "move"}, Request: &WorkflowMoveRequest{}, Method: "POST", Path: "/api/tasks/{key}/workflow/move", OperatorOnly: true},
	{Action: "workflow_upgrade", Words: []string{"workflow", "upgrade"}, Request: &WorkflowUpgradeRequest{}, Method: "POST", Path: "/api/tasks/{key}/workflow/upgrade", OperatorOnly: true},
	{Action: "workflow_resume", Words: []string{"workflow", "resume"}, Request: &ResumeRequest{}, Method: "POST", Path: "/api/tasks/{key}/workflow/resume", OperatorOnly: true},
	{Action: "workflow_retry", Words: []string{"workflow", "retry"}, Request: &RetryRequest{}, Method: "POST", Path: "/api/tasks/{key}/workflow/requests/{id}/retry", OperatorOnly: true},
	{Action: "cancel", Words: []string{"cancel"}, Request: &KeyRequest{}, Method: "POST", Path: "/api/tasks/{key}/cancel", OperatorOnly: true},
	{Action: "requeue", Words: []string{"requeue"}, Request: &RequeueRequest{}, Method: "POST", Path: "/api/tasks/{key}/requeue", OperatorOnly: true},
}

// Specs returns the native task commands in their help order.
func Specs() []Spec { return append([]Spec(nil), specs...) }

// SpecFor returns the command of a tools-socket action.
func SpecFor(action string) (Spec, bool) {
	for _, spec := range specs {
		if spec.Action == action {
			return spec, true
		}
	}
	return Spec{}, false
}

// SpecForRoute returns the command that owns an operator HTTP route.
func SpecForRoute(method, path string) (Spec, bool) {
	for _, spec := range specs {
		if spec.Method == method && spec.Path == path {
			return spec, true
		}
	}
	return Spec{}, false
}

// FieldKind is the Go type of a request field.
type FieldKind int

const (
	KindString FieldKind = iota
	KindOptString
	KindInt
	KindInt64
	KindBool
	KindOptBool
	KindStrings
)

// Field is one request field as its tags describe it.
type Field struct {
	Name, Flag, Label, Help   string
	Pos                       int  // -1 when the field is not positional
	Rest                      bool // the field takes every positional from Pos on
	CLIRequired, HTTPRequired bool
	HTTP                      bool // the field is an HTTP argument
	Raw, Clear                bool
	Min, Max                  int64
	Enum                      []string
	Kind                      FieldKind
	index                     int
}

var fieldCache sync.Map // reflect.Type -> []Field

// Fields returns the fields of a request in declaration order. It panics on a
// field type the wire forms do not support; TestEveryTaskSpecBuilds reads
// every request so the panic shows in tests.
func Fields(request any) []Field {
	typ := reflect.TypeOf(request).Elem()
	if cached, ok := fieldCache.Load(typ); ok {
		return cached.([]Field)
	}
	fields := make([]Field, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		fields = append(fields, readField(typ.Field(i), i))
	}
	fieldCache.Store(typ, fields)
	return fields
}

var fieldKinds = map[reflect.Type]FieldKind{
	reflect.TypeOf(""): KindString, reflect.TypeOf((*string)(nil)): KindOptString,
	reflect.TypeOf(0): KindInt, reflect.TypeOf(int64(0)): KindInt64,
	reflect.TypeOf(false): KindBool, reflect.TypeOf((*bool)(nil)): KindOptBool,
	reflect.TypeOf([]string(nil)): KindStrings,
}

func readField(sf reflect.StructField, index int) Field {
	kind, ok := fieldKinds[sf.Type]
	if !ok {
		panic(fmt.Sprintf("tasks: request field %s has unsupported type %s", sf.Name, sf.Type))
	}
	tag := sf.Tag
	f := Field{Name: tag.Get("json"), Flag: tag.Get("flag"), Label: tag.Get("label"), Help: tag.Get("help"),
		Pos: -1, Raw: tag.Get("raw") == "true", Clear: tag.Get("clear") == "true", Kind: kind, index: index}
	if f.Name == "-" {
		f.Name = ""
	}
	f.HTTP = f.Name != "" && tag.Get("http") != "-"
	required := tag.Get("required")
	f.CLIRequired, f.HTTPRequired = required == "true" || required == "cli", required == "true" || required == "http"
	if pos, ok := tag.Lookup("pos"); ok {
		f.Rest = strings.HasSuffix(pos, "+")
		f.Pos = mustAtoi(sf.Name, strings.TrimSuffix(pos, "+"))
	}
	if enum := tag.Get("enum"); enum != "" {
		f.Enum = strings.Split(enum, ",")
	}
	if min := tag.Get("min"); min != "" {
		f.Min = int64(mustAtoi(sf.Name, min))
	}
	if max := tag.Get("max"); max != "" {
		f.Max = int64(mustAtoi(sf.Name, max))
	}
	return f
}

func mustAtoi(field, text string) int {
	n, err := strconv.Atoi(text)
	if err != nil {
		panic(fmt.Sprintf("tasks: request field %s has a bad tag value %q", field, text))
	}
	return n
}

func (f Field) value(request any) reflect.Value {
	return reflect.ValueOf(request).Elem().Field(f.index)
}

// IsZero reports whether the field of request holds no value.
func (f Field) IsZero(request any) bool { return f.value(request).IsZero() }

// text is the stored form of a string: trimmed unless the field is raw.
func (f Field) text(s string) string {
	if f.Raw {
		return s
	}
	return strings.TrimSpace(s)
}

// Set stores text, as typed on a command line, in the field of request. A
// flag's []string is comma-separated, an empty *string flag stays unset
// unless the field clears, and a positional *string is set even when empty.
// An error names the field as name.
func (f Field) Set(request any, text, name string) error {
	v := f.value(request)
	switch f.Kind {
	case KindString:
		v.SetString(f.text(text))
	case KindOptString:
		if s := f.text(text); s != "" || f.Clear || f.Pos >= 0 {
			v.Set(reflect.ValueOf(&s))
		}
	case KindInt, KindInt64:
		n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil || (f.Min != 0 && n < f.Min) || (f.Max != 0 && n > f.Max) {
			return fmt.Errorf("%s must be %s, got %q", name, f.numberRange(), text)
		}
		v.SetInt(n)
	case KindBool, KindOptBool:
		if text != "true" && text != "false" {
			return fmt.Errorf("%s must be true or false, got %q", name, text)
		}
		b := text == "true"
		if f.Kind == KindBool {
			v.SetBool(b)
		} else {
			v.Set(reflect.ValueOf(&b))
		}
	case KindStrings:
		v.Set(reflect.ValueOf(splitList(text)))
	}
	return nil
}

// SetWords stores the positional words of a Rest field.
func (f Field) SetWords(request any, words []string) {
	f.value(request).Set(reflect.ValueOf(append([]string(nil), words...)))
}

func (f Field) numberRange() string {
	switch {
	case f.Min != 0 && f.Max != 0:
		return fmt.Sprintf("a number from %d to %d", f.Min, f.Max)
	case f.Min == 1:
		return "a positive number"
	default:
		return "a whole number"
	}
}

// Payload is the tools-socket body of request: every keyed field that holds
// a value. A list travels comma-separated, as a flag gives it.
func Payload(request any) map[string]any {
	out := map[string]any{}
	for _, f := range Fields(request) {
		v := f.value(request)
		if f.Name == "" || v.IsZero() {
			continue
		}
		switch f.Kind {
		case KindOptString, KindOptBool:
			out[f.Name] = v.Elem().Interface()
		case KindStrings:
			out[f.Name] = strings.Join(v.Interface().([]string), ",")
		default:
			out[f.Name] = v.Interface()
		}
	}
	return out
}

// Decode fills request from a socket body or HTTP parameters. Like the
// registry.Params accessors it never fails: a value of the wrong type leaves
// the field zero. Text is trimmed unless the field is raw; a list may be a
// JSON array or a comma-separated string.
func Decode(body map[string]any, request any) {
	for _, f := range Fields(request) {
		raw, ok := body[f.Name]
		if f.Name == "" || !ok || raw == nil {
			continue
		}
		v := f.value(request)
		switch f.Kind {
		case KindString:
			s, _ := raw.(string)
			v.SetString(f.text(s))
		case KindOptString:
			if s, ok := raw.(string); ok {
				s = f.text(s)
				v.Set(reflect.ValueOf(&s))
			}
		case KindInt, KindInt64:
			n, _ := wholeNumber(raw)
			v.SetInt(n)
		case KindBool:
			b, _ := boolValue(raw)
			v.SetBool(b)
		case KindOptBool:
			if b, ok := boolValue(raw); ok {
				v.Set(reflect.ValueOf(&b))
			}
		case KindStrings:
			v.Set(reflect.ValueOf(listValue(raw)))
		}
	}
}

func splitList(text string) []string {
	var out []string
	for _, item := range strings.Split(text, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func listValue(raw any) []string {
	var items []string
	switch v := raw.(type) {
	case string:
		return splitList(v)
	case []string:
		items = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				items = append(items, s)
			}
		}
	}
	var out []string
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func wholeNumber(raw any) (int64, bool) {
	switch n := raw.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || math.Abs(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func boolValue(raw any) (bool, bool) {
	switch b := raw.(type) {
	case bool:
		return b, true
	case string:
		switch b {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return false, false
}

// agentRun runs a decoded request of the agent tools socket.
type agentRun func(context.Context, *TaskService, Actor, any) (any, error)

func onAgent[T any](run func(*TaskService, context.Context, Actor, *T) (any, error)) agentRun {
	return func(ctx context.Context, s *TaskService, actor Actor, request any) (any, error) {
		return run(s, ctx, actor, request.(*T))
	}
}
