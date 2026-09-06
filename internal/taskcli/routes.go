package taskcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alekzonder/tariboy/internal/cli"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

func runOperator(ctx context.Context, parsed request, caller Caller, jsonOut bool, stdout, stderr io.Writer) int {
	if strings.HasPrefix(parsed.action, "work_") || strings.HasPrefix(parsed.action, "artifact_") || strings.HasPrefix(parsed.action, "observe_") || parsed.action == "workflow_ask" || parsed.action == "workflow_answer" || parsed.action == "questions" {
		fmt.Fprintln(stderr, "tasks: leased workflow execution requires agent mode")
		return 2
	}
	method, route := "", ""
	body := parsed.payload
	key, _ := body["key"].(string)
	switch parsed.action {
	case "mine":
		method, route = "GET", "/api/tasks"
	case "ready":
		method, route = "GET", "/api/tasks"
		body["ready"] = "true"
		if claim, _ := body["claim"].(bool); claim {
			delete(body, "claim")
			raw, err := caller.Call(method, route, query(body))
			if err != nil {
				return operatorError(err, stderr)
			}
			key, revision, err := firstTask(raw)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			method, route, body = "POST", "/api/tasks/"+key+"/claim", map[string]any{"revision": revision}
		}
	case "show":
		method, route, body = "GET", "/api/tasks/"+key, nil
	case "create":
		method, route = "POST", "/api/tasks"
	case "update", "assign":
		if _, ok := body["revision"]; !ok {
			revision, code := revisionFor(caller, key, stderr)
			if code != 0 {
				return code
			}
			body["revision"] = revision
		}
		method, route = "PATCH", "/api/tasks/"+key
		delete(body, "key")
	case "comment":
		method, route = "POST", "/api/tasks/"+key+"/comments"
		delete(body, "key")
	case "ask":
		method, route = "POST", "/api/tasks/"+key+"/comments"
		body = map[string]any{"body": "@" + fmt.Sprint(body["principal"]) + " " + fmt.Sprint(body["body"]), "idempotency_key": body["idempotency_key"]}
	case "move":
		if _, ok := body["revision"]; !ok {
			revision, code := revisionFor(caller, key, stderr)
			if code != 0 {
				return code
			}
			body["revision"] = revision
		}
		method, route = "POST", "/api/tasks/"+key+"/move"
		delete(body, "key")
	case "block", "relate":
		if _, ok := body["revision"]; !ok {
			revision, code := revisionFor(caller, key, stderr)
			if code != 0 {
				return code
			}
			body["revision"] = revision
		}
		if parsed.action == "block" {
			body["target_key"], body["type"] = body["blocker_key"], "blocks"
			delete(body, "blocker_key")
		} else {
			body["type"] = "related"
		}
		method, route = "POST", "/api/tasks/"+key+"/relations"
		delete(body, "key")
	case "done":
		if _, ok := body["revision"]; !ok {
			revision, code := revisionFor(caller, key, stderr)
			if code != 0 {
				return code
			}
			body["revision"] = revision
		}
		method, route = "POST", "/api/tasks/"+key+"/complete"
		delete(body, "key")
	default:
		fmt.Fprintf(stderr, "tasks: unsupported operator command %s\n", parsed.action)
		return 2
	}
	var requestBody any = body
	if method == "GET" {
		requestBody = query(body)
	}
	raw, err := caller.Call(method, route, requestBody)
	if err != nil {
		return operatorError(err, stderr)
	}
	return printResult(parsed, raw, jsonOut, stdout, stderr)
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
		Revision int64 `json:"revision"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		fmt.Fprintln(stderr, err)
		return 0, 1
	}
	return value.Revision, 0
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

func operatorError(err error, stderr io.Writer) int {
	if client.IsDaemonDown(err) {
		fmt.Fprintln(stderr, "tariboyd is not running (start it with: tariboyd)")
		return 2
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 1
}

func isOperatorCommand(args []string) bool {
	return len(args) > 0 && map[string]bool{"queue": true, "workflows": true, "workflow": true, "events": true, "principals": true, "notifications": true}[args[0]]
}

func runOperatorCommand(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	resolved, err := paths.Resolve(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "tariboy-tasks: %v\n", err)
		return 2
	}
	reg := registry.New()
	groups := map[string]bool{}
	for _, command := range commands.TaskOperatorCommands() {
		command.Path = strings.TrimPrefix(command.Path, "tasks.")
		command.CLIHidden = false
		if err := reg.Register(command); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		parts := strings.Split(command.Path, ".")
		for i := 1; i < len(parts); i++ {
			groups[strings.Join(parts[:i], ".")] = true
		}
	}
	for group := range groups {
		_ = reg.RegisterGroup(group, "Manage native tasks")
	}
	return cli.Run(ctx, reg, args, newCaller(resolved.Socket()), nil, stdout, stderr)
}
