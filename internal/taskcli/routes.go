package taskcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/cli"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// runOperator calls the operator HTTP route of a command, taken from the
// tasks.Specs table: the path parameters come from the payload and the rest is
// the body, or the query of a GET.
func runOperator(ctx context.Context, parsed request, caller Caller, jsonOut bool, stdout, stderr io.Writer) int {
	if parsed.action == "ready" {
		if claim, _ := parsed.payload["claim"].(bool); claim {
			fmt.Fprintln(stderr, "tasks ready --claim requires agent mode")
			return 2
		}
		return runReady(parsed, caller, jsonOut, stdout, stderr)
	}
	route := parsed.spec
	if route.Via != "" {
		route, _ = tasks.SpecFor(route.Via)
	}
	key, _ := parsed.payload["key"].(string)
	body := operatorBody(parsed)
	if code := fillOperatorBody(parsed, caller, body, stderr); code != 0 {
		return code
	}
	path := route.Path
	for segment := range strings.SplitSeq(route.Path, "/") {
		if name, ok := strings.CutPrefix(segment, "{"); ok {
			name = strings.TrimSuffix(name, "}")
			path = strings.Replace(path, segment, url.PathEscape(fmt.Sprint(body[name])), 1)
			delete(body, name)
		}
	}
	var requestBody any = body
	if route.Method == "GET" {
		requestBody = query(body)
	}
	raw, err := caller.Call(route.Method, path, requestBody)
	if err != nil {
		return operatorErrorFor(err, key, stderr)
	}
	if (parsed.action == "advance" || parsed.action == "workflow_retry") && !parsed.noWait {
		return waitForAdvance(ctx, parsed, raw, func(id int64) (json.RawMessage, error) {
			return caller.Call("GET", "/api/tasks/"+url.PathEscape(key)+"/workflow/requests/"+strconv.FormatInt(id, 10), map[string]string{})
		}, func(err error) int { return operatorErrorFor(err, key, stderr) }, jsonOut, stdout, stderr)
	}
	return printResult(parsed, raw, jsonOut, stdout, stderr)
}

// operatorBody is the payload in the shape of the route the command calls:
// ask is a comment that mentions the principal, block a blocks relation of
// the blocker, and relate a related relation.
func operatorBody(parsed request) map[string]any {
	body := maps.Clone(parsed.payload)
	switch parsed.action {
	case "ask":
		principal := strings.TrimSpace(fmt.Sprint(body["principal"]))
		if !strings.Contains(principal, ":") {
			principal = "agent:" + principal
		}
		return map[string]any{"key": body["key"], "body": "@" + principal + "\n\n" + fmt.Sprint(body["body"]), "idempotency_key": body["idempotency_key"]}
	case "block":
		body["key"], body["target_key"], body["type"] = body["blocker_key"], body["key"], "blocks"
		delete(body, "blocker_key")
	case "relate":
		body["type"] = "related"
	}
	return body
}

// fillOperatorBody adds what the daemon needs and the command left out: the
// current revision of the task the route names, and the latest transition
// request of a retry.
func fillOperatorBody(parsed request, caller Caller, body map[string]any, stderr io.Writer) int {
	key, _ := body["key"].(string)
	if _, given := body["revision"]; !given && hasRevision(parsed.spec) {
		revision, code := revisionFor(caller, key, stderr)
		if code != 0 {
			return code
		}
		body["revision"] = revision
	}
	if _, given := body["id"]; given || parsed.action != "workflow_retry" {
		return 0
	}
	// Without an id the retry names the task's latest request; the daemon
	// refuses it unless it failed.
	raw, err := caller.Call("GET", "/api/tasks/"+url.PathEscape(key)+"/workflow", map[string]string{})
	if err != nil {
		return operatorErrorFor(err, key, stderr)
	}
	var view tasks.WorkflowView
	if json.Unmarshal(raw, &view) != nil || view.LastRequest == nil {
		fmt.Fprintf(stderr, "tasks workflow retry: %s has no transition request\n", key)
		return 1
	}
	body["id"] = view.LastRequest.ID
	return 0
}

// hasRevision reports whether a command takes the expected task revision.
func hasRevision(spec tasks.Spec) bool {
	for _, f := range tasks.Fields(spec.Request) {
		if f.Name == "revision" {
			return true
		}
	}
	return false
}

func query(body map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range body {
		out[key] = fmt.Sprint(value)
	}
	return out
}

func revisionFor(caller Caller, key string, stderr io.Writer) (int64, int) {
	raw, err := caller.Call("GET", "/api/tasks/"+key, nil)
	if err != nil {
		return 0, operatorError(err, stderr)
	}
	var value struct {
		Task struct {
			Revision int64 `json:"revision"`
		} `json:"task"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		fmt.Fprintln(stderr, err)
		return 0, 1
	}
	return value.Task.Revision, 0
}

func runReady(parsed request, caller Caller, jsonOut bool, stdout, stderr io.Writer) int {
	limit := 50
	if n, _ := parsed.payload["limit"].(int); n > 0 && n <= 200 {
		limit = n
	}
	readyQuery := map[string]string{"status": "open", "blocked": "false", "limit": "500"}
	if queue, _ := parsed.payload["queue"].(string); queue != "" {
		readyQuery["queue"] = queue
	}
	ready := []json.RawMessage{}
	for {
		raw, err := caller.Call("GET", "/api/tasks", readyQuery)
		if err != nil {
			return operatorError(err, stderr)
		}
		var page struct {
			Tasks      []json.RawMessage `json:"tasks"`
			NextCursor string            `json:"next_cursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, rawTask := range page.Tasks {
			var task struct {
				Category          string `json:"category"`
				WorkflowDigest    string `json:"workflow_digest"`
				Assignee          string `json:"assignee"`
				ManualBlockReason string `json:"manual_block_reason"`
				Blocked           bool   `json:"blocked"`
			}
			if err := json.Unmarshal(rawTask, &task); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			// "status" is the workflow status for a workflow task; the category is
			// the lifecycle state. The daemon assigns workflow work, so a task with a
			// workflow is never ready to claim.
			if task.Category == "open" && task.WorkflowDigest == "" && task.Assignee == "" && task.ManualBlockReason == "" && !task.Blocked {
				ready = append(ready, rawTask)
				if len(ready) == limit {
					result, _ := json.Marshal(ready)
					return printResult(parsed, result, jsonOut, stdout, stderr)
				}
			}
		}
		if page.NextCursor == "" {
			break
		}
		readyQuery["after"] = page.NextCursor
	}
	result, _ := json.Marshal(ready)
	return printResult(parsed, result, jsonOut, stdout, stderr)
}

func firstTask(raw json.RawMessage) (string, int64, error) {
	var value struct {
		Tasks []struct {
			Key      string `json:"key"`
			Revision int64  `json:"revision"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", 0, err
	}
	if len(value.Tasks) == 0 {
		return "", 0, fmt.Errorf("tasks ready: no eligible task to claim")
	}
	return value.Tasks[0].Key, value.Tasks[0].Revision, nil
}

func operatorError(err error, stderr io.Writer) int { return operatorErrorFor(err, "", stderr) }

// operatorErrorFor is operatorError for a command on task key, so a
// workflow_managed refusal can name the exact advance command.
func operatorErrorFor(err error, key string, stderr io.Writer) int {
	if client.IsDaemonDown(err) {
		fmt.Fprintln(stderr, "tariboyd is not running (start it with: tariboyd)")
		return 2
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		printWorkflowManagedHint(apiErr, key, stderr)
	}
	return 1
}

func isOperatorCommand(args []string) bool {
	return len(args) > 0 && operatorRoots[args[0]]
}

var operatorRoots = map[string]bool{"queue": true, "events": true, "principals": true, "notifications": true, "activity": true}

func runOperatorCommand(ctx context.Context, args []string, m mode, stdout, stderr io.Writer) int {
	socket, err := m.socket()
	if err != nil {
		fmt.Fprintf(stderr, "tariboy-tasks: %v\n", err)
		return 2
	}
	reg, err := taskOperatorRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return cli.Run(ctx, reg, args, newCaller(socket), nil, stdout, stderr)
}

func runHelpJSON(ctx context.Context, stdout, stderr io.Writer) int {
	reg, err := taskOperatorRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_ = ctx
	tree := reg.Tree()
	entries := taskHelpEntries(reg)
	for _, command := range reg.Commands() {
		node := tree
		for _, part := range strings.Split(command.Path, ".") {
			node = node[part].(map[string]any)
		}
		entry := entries[command.Path]
		node["summary"], node["help"], node["usage"], node["examples"] = entry.summary, entry.help, entry.usage, strings.Split(entry.examples, "\n")
	}
	for _, spec := range tasks.Specs() {
		if len(spec.Words) > 0 {
			insertSharedHelp(tree, spec.Words, commandFlags(spec), sharedHelp[spec.Action])
		}
	}
	encoded, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}

func sharedHelpPath(action string) []string {
	if spec, ok := tasks.SpecFor(action); ok && len(spec.Words) > 0 {
		return spec.Words
	}
	return []string{action}
}

func insertSharedHelp(tree map[string]any, path []string, flags map[string]bool, help commandHelp) {
	node := tree
	for i, segment := range path[:len(path)-1] {
		child, ok := node[segment].(map[string]any)
		if !ok {
			child = map[string]any{"summary": helpGroups[strings.Join(path[:i+1], ".")]}
			node[segment] = child
		}
		node = child
	}
	values := make([]string, 0, len(flags))
	descriptions := map[string]string{}
	for flag := range flags {
		values = append(values, flag)
		descriptions[flag] = helpFlags[flag]
	}
	sort.Strings(values)
	node[path[len(path)-1]] = map[string]any{
		"summary": help.summary, "flags": values, "flag_help": descriptions,
		"help": help.help, "usage": help.usage, "arguments": help.arguments, "examples": strings.Split(help.examples, "\n"),
	}
}

func taskOperatorRegistry() (*registry.Registry, error) {
	reg := registry.New()
	groups := map[string]bool{}
	for _, command := range commands.TaskOperatorCommands() {
		command.Path = strings.TrimPrefix(command.Path, "tasks.")
		if !operatorRoots[strings.Split(command.Path, ".")[0]] {
			continue
		}
		command.CLIHidden = false
		if err := reg.Register(command); err != nil {
			return nil, err
		}
		parts := strings.Split(command.Path, ".")
		for i := 1; i < len(parts); i++ {
			groups[strings.Join(parts[:i], ".")] = true
		}
	}
	for group := range groups {
		if err := reg.RegisterGroup(group, helpGroups[group]); err != nil {
			return nil, err
		}
	}
	return reg, nil
}
