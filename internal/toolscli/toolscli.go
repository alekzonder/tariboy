// Package toolscli is the command surface of tariboy-tools: a thin client of
// the per-agent tools socket (spec §8).
package toolscli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/version"
)

const helpText = `tariboy-tools — the agent command surface

Usage: tools <group> <command> [args]

Commands:
  loop done            Signal this iteration is finished (i-am-done)
  loop start|stop      Enable or disable your own loop
  whoami               Print agent, cwd, current iteration and both versions
  context get          Print the durable working memory (CONTEXT.md)
  context set <text>   Overwrite the durable working memory
  status               Print agent state and your current status message
  status set <text>    Set your status message ("what I'm doing now")
  task current <id>    Tag this iteration with a native task (attributes AI usage)
  task current --clear Clear the current-task tag
  tasks mine [--status S] [--queue Q] [--waiting-for me]
  tasks ready [--queue Q] [--claim]
  tasks show KEY
  tasks create --queue Q|--parent KEY --title TEXT [--priority P0|P1|P2|P3]
  tasks update KEY [--status S] [--title TEXT] [--assignee AGENT] [--priority P0|P1|P2|P3]
  tasks assign KEY AGENT
  tasks comment KEY TEXT
  tasks ask KEY agent:NAME|user:LOGIN TEXT
  tasks move KEY [--parent KEY] [--before KEY] [--to-root]
  tasks block KEY --by BLOCKER
  tasks relate KEY OTHER
  tasks done KEY [--complete-anyway]
  tasks work next [--queue Q] --idempotency-key KEY
  tasks work show|complete|release ASSIGNMENT [workflow flags]
  tasks artifacts add ASSIGNMENT --name N --type T --content VALUE
  tasks artifacts show ASSIGNMENT ARTIFACT --task KEY
  tasks ask ASSIGNMENT --question Q --context C --blocking-scope none|assignment|requirement
  tasks questions ASSIGNMENT
  tasks answer QUESTION --assignment ASSIGNMENT --answer TEXT
  tasks observe subscribe|list|cancel ASSIGNMENT [PATTERN|SUBSCRIPTION]
  group info
  group status [member]
  group send <member> --text TEXT
  group request <member> --text TEXT [--deadline DURATION]
  group observe <member> [--tail N]
  group loop start|stop <member>
  message send --channel C [--type T] [--subject k=v,..] [--text .. | --data JSON]
  message ls [--all]   List your inbox (pending; --all adds archive+dlq)
  message processed ID <result...>   Ack a message with a mandatory result
  message reply ID [--text ..] [--data JSON]   Reply to a message (auto-processes)
  message dlq          List your dead-lettered messages
  message dlq requeue ID   Requeue a dead-lettered message
  request --channel C --text .. [--deadline DURATION]   Send a request (§4.2)
  channel subscribe C [--matcher JSON] [--type globs] [--params JSON]
  channel unsubscribe ID   By subscription id (or channel name to drop all own subs)
  channel ls           List your subscriptions
  sources              List available channels
  schedule add --kind cron|oneshot --spec S [--channel C] [--message JSON]
  schedule ls          List your schedules
  schedule cancel ID
  script run NAME [--description TEXT] -- COMMAND
  script schedule NAME --every SECONDS [--quiet-exit CODE] [--description TEXT] -- COMMAND
  script rerun SCRIPT_ID
  script ls
  script runs SCRIPT_ID
  script logs RUN_ID
  script cancel SCRIPT_OR_RUN_ID
  script rm SCRIPT_ID
  image build --name NAME [--tag TAG] --path DIR   Author+build a new image (image-creator only)
  judge iterations search --agent A --judge-group G [--group G] [--since T] [--until T] [--status S] [--limit N]
  judge run create --request-file F --selector JSON --judges a,b --summary-agent A [--judges-per-iteration N] [--judge-group G]
  judge run inspect RUN
  judge work claim [--run RUN]
  judge evidence search --assignment ID --artifact K [--query Q] [--cursor C]
  judge evidence get --assignment ID --artifact K --locator JSON
  judge analysis submit --assignment ID --file result.json
  judge summary claim RUN
  judge summary inputs RUN [--cursor C]
  judge summary submit RUN --file summary.json
  judge run cancel RUN
  judge work retry RUN
  help                 Show this help

The socket is taken from $TARIBOY_TOOLS_SOCKET.`

// Run dispatches one tools invocation. sock is the agent socket path (may be
// empty for `help`).
func Run(sock string, args []string, out, errOut io.Writer) int {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(out, helpText)
		return 0
	}
	if sock == "" {
		fmt.Fprintln(errOut, "tools: TARIBOY_TOOLS_SOCKET is not set (are you running inside an agent?)")
		return 2
	}
	c := client.New(sock)
	sc := newArgScan(args)

	method, route, body, wantsText := "", "", any(nil), ""
	switch {
	case args[0] == "whoami":
		method, route = "GET", "/tools/whoami"
	case args[0] == "status" && len(args) >= 2 && args[1] == "set":
		if len(args) < 3 {
			fmt.Fprintln(errOut, "tools status set: <message...> required")
			return 2
		}
		method, route, body = "POST", "/tools/status/set", map[string]string{"message": sc.text(2)}
	case args[0] == "status":
		method, route = "GET", "/tools/status"
	case args[0] == "loop" && len(args) >= 2 && args[1] == "done":
		// `i-am-done --idle` (forwarded verbatim by the shim) self-declares the
		// iteration idle; a plain `loop done` stays productive.
		flags, _ := sc.flags(2)
		method, route = "POST", "/tools/loop/done"
		if v, ok := flags.Lookup("idle"); ok {
			// Honor the parsed value: `--idle=false` / `--idle 0` self-declares
			// the pass productive, while `--idle` / `--idle=true` / `--idle 1`
			// stays idle. Bare `--idle` is captured as "true" by the parser, so
			// presence still means idle; an unparseable value falls back to idle.
			idle := true
			if b, err := strconv.ParseBool(v); err == nil {
				idle = b
			}
			body = map[string]any{"idle": idle}
		}
	case args[0] == "loop" && len(args) >= 2 && (args[1] == "start" || args[1] == "stop"):
		method, route, body = "POST", "/tools/loop/control", map[string]any{"action": args[1]}
	case args[0] == "task" && len(args) >= 2 && args[1] == "current":
		// `tools task current <task-key>` tags this iteration; `--clear` drops it.
		flags, pos := sc.flags(2)
		if flags.Has("clear") {
			method, route, body = "POST", "/tools/task/current", map[string]any{"clear": true}
		} else {
			if len(pos) == 0 {
				fmt.Fprintln(errOut, "tools task current: <task-key> is required (or pass --clear)")
				return 2
			}
			method, route, body = "POST", "/tools/task/current", map[string]any{"id": pos[0]}
		}
	case args[0] == "context" && len(args) >= 2 && args[1] == "get":
		method, route = "GET", "/tools/context/get"
		wantsText = "text"
	case args[0] == "context" && len(args) >= 2 && args[1] == "set":
		method, route = "POST", "/tools/context/set"
		body = map[string]string{"text": sc.text(2)}
	case args[0] == "sources":
		method, route = "GET", "/tools/sources"
	case args[0] == "group" && len(args) >= 2 && args[1] == "info":
		method, route = "GET", "/tools/group/info"
	case args[0] == "group" && len(args) >= 2 && args[1] == "status":
		if len(args) >= 3 {
			method, route = "GET", "/tools/group/status/"+args[2]
		} else {
			method, route = "GET", "/tools/group/status"
		}
	case args[0] == "group" && len(args) >= 2 && (args[1] == "send" || args[1] == "request"):
		flags, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintf(errOut, "tools group %s: <member> is required\n", args[1])
			return 2
		}
		if flags.Get("text") == "" {
			fmt.Fprintf(errOut, "tools group %s: --text is required\n", args[1])
			return 2
		}
		m := map[string]any{"member": pos[0], "text": flags.Get("text")}
		if flags.Get("deadline") != "" {
			m["deadline"] = flags.Get("deadline")
		}
		method, route, body = "POST", "/tools/group/"+args[1], m
	case args[0] == "group" && len(args) >= 2 && args[1] == "observe":
		flags, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools group observe: <member> is required")
			return 2
		}
		route = "/tools/group/observe/" + pos[0]
		if flags.Get("tail") != "" {
			route += "?tail=" + flags.Get("tail")
		}
		method = "GET"
	case args[0] == "group" && len(args) >= 3 && args[1] == "loop" && (args[2] == "start" || args[2] == "stop"):
		_, pos := sc.flags(3)
		if len(pos) == 0 {
			fmt.Fprintf(errOut, "tools group loop %s: <member> is required\n", args[2])
			return 2
		}
		method, route, body = "POST", "/tools/group/loop", map[string]any{"member": pos[0], "action": args[2]}
	case args[0] == "message" && len(args) >= 2 && args[1] == "send":
		flags, _ := sc.flags(2)
		if flags.Get("channel") == "" {
			fmt.Fprintln(errOut, "tools message send: --channel is required")
			return 2
		}
		m := map[string]any{"channel": flags.Get("channel"), "type": flags.Get("type"), "text": flags.Get("text")}
		if flags.Get("subject") != "" {
			m["subject"] = subjectMap(flags.Get("subject"))
		}
		if flags.Get("data") != "" {
			var d any
			if err := json.Unmarshal([]byte(flags.Get("data")), &d); err != nil {
				fmt.Fprintf(errOut, "tools message send: --data is not valid JSON: %v\n", err)
				return 2
			}
			m["data"] = d
		}
		method, route, body = "POST", "/tools/message/send", m
	case args[0] == "message" && len(args) >= 2 && args[1] == "ls":
		flags, _ := sc.flags(2)
		route = "/tools/message/ls"
		if flags.Has("all") {
			route += "?all=true"
		}
		method = "GET"
	case args[0] == "message" && len(args) >= 2 && args[1] == "processed":
		// message processed <id> <result...> — id positional, result is the rest.
		flags, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools message processed: <id> is required")
			return 2
		}
		result := strings.Join(pos[1:], " ")
		if flags.Get("result") != "" { // allow --result "..." as an alternative
			result = flags.Get("result")
		}
		if strings.TrimSpace(result) == "" {
			fmt.Fprintln(errOut, `tools message processed: a result is required (tools message processed <id> "<result>")`)
			return 2
		}
		method, route, body = "POST", "/tools/message/processed", map[string]any{"id": pos[0], "result": result}
	case args[0] == "message" && len(args) >= 2 && args[1] == "reply":
		flags, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools message reply: <id> is required")
			return 2
		}
		m := map[string]any{"id": pos[0], "text": flags.Get("text"), "type": flags.Get("type")}
		if flags.Get("data") != "" {
			var d any
			if err := json.Unmarshal([]byte(flags.Get("data")), &d); err != nil {
				fmt.Fprintf(errOut, "tools message reply: --data is not valid JSON: %v\n", err)
				return 2
			}
			m["data"] = d
		}
		method, route, body = "POST", "/tools/message/reply", m
	case args[0] == "message" && len(args) >= 3 && args[1] == "dlq" && args[2] == "requeue":
		_, pos := sc.flags(3)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools message dlq requeue: <id> is required")
			return 2
		}
		method, route, body = "POST", "/tools/message/dlq/requeue", map[string]any{"id": pos[0]}
	case args[0] == "message" && len(args) >= 2 && args[1] == "dlq":
		method, route = "GET", "/tools/message/dlq"
	case args[0] == "request":
		flags, _ := sc.flags(1)
		if flags.Get("channel") == "" {
			fmt.Fprintln(errOut, "tools request: --channel is required")
			return 2
		}
		m := map[string]any{"channel": flags.Get("channel"), "text": flags.Get("text")}
		if flags.Get("deadline") != "" {
			m["deadline"] = flags.Get("deadline")
		}
		method, route, body = "POST", "/tools/request", m
	case args[0] == "channel" && len(args) >= 2 && args[1] == "subscribe":
		flags, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools channel subscribe: <channel> is required")
			return 2
		}
		m := map[string]any{"channel": pos[0], "type": flags.Get("type")}
		if flags.Get("matcher") != "" {
			var mt map[string]string
			if err := json.Unmarshal([]byte(flags.Get("matcher")), &mt); err != nil {
				fmt.Fprintf(errOut, "tools channel subscribe: --matcher is not valid JSON: %v\n", err)
				return 2
			}
			m["matcher"] = mt
		}
		if flags.Get("params") != "" {
			var pj map[string]any
			if err := json.Unmarshal([]byte(flags.Get("params")), &pj); err != nil {
				fmt.Fprintf(errOut, "tools channel subscribe: --params is not valid JSON object: %v\n", err)
				return 2
			}
			m["params"] = pj
		}
		method, route, body = "POST", "/tools/channel/subscribe", m
	case args[0] == "channel" && len(args) >= 2 && args[1] == "unsubscribe":
		_, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools channel unsubscribe: <id> is required")
			return 2
		}
		method, route, body = "POST", "/tools/channel/unsubscribe", map[string]any{"id": pos[0]}
	case args[0] == "channel" && len(args) >= 2 && args[1] == "ls":
		method, route = "GET", "/tools/channel/ls"
	case args[0] == "schedule" && len(args) >= 2 && args[1] == "add":
		flags, _ := sc.flags(2)
		m := map[string]any{"kind": flags.Get("kind"), "spec": flags.Get("spec"), "channel": flags.Get("channel")}
		if flags.Get("message") != "" {
			var msg any
			if err := json.Unmarshal([]byte(flags.Get("message")), &msg); err != nil {
				fmt.Fprintf(errOut, "tools schedule add: --message is not valid JSON: %v\n", err)
				return 2
			}
			m["message"] = msg
		}
		method, route, body = "POST", "/tools/schedule/add", m
	case args[0] == "schedule" && len(args) >= 2 && args[1] == "ls":
		method, route = "GET", "/tools/schedule/ls"
	case args[0] == "schedule" && len(args) >= 2 && args[1] == "cancel":
		_, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools schedule cancel: <id> is required")
			return 2
		}
		method, route, body = "POST", "/tools/schedule/cancel", map[string]any{"id": pos[0]}
	case args[0] == "script" && len(args) >= 2 && args[1] == "run":
		m, err := parseScriptCommand(sc, args, false)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
		method, route, body = "POST", "/tools/script/run", m
	case args[0] == "script" && len(args) >= 2 && args[1] == "schedule":
		m, err := parseScriptCommand(sc, args, true)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
		method, route, body = "POST", "/tools/script/schedule", m
	case args[0] == "script" && len(args) >= 2 && args[1] == "ls":
		method, route = "GET", "/tools/script/ls"
	case args[0] == "script" && len(args) >= 2 && args[1] == "rerun":
		_, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools script rerun: <script-id> is required")
			return 2
		}
		method, route, body = "POST", "/tools/script/rerun", map[string]any{"id": pos[0]}
	case args[0] == "script" && len(args) >= 2 && args[1] == "runs":
		_, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools script runs: <script-id> is required")
			return 2
		}
		method, route = "GET", "/tools/script/runs/"+pos[0]
	case args[0] == "script" && len(args) >= 2 && args[1] == "logs":
		_, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools script logs: <run-id> is required")
			return 2
		}
		method, route = "GET", "/tools/script/logs/"+pos[0]
	case args[0] == "script" && len(args) >= 2 && args[1] == "cancel":
		_, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools script cancel: <id> is required")
			return 2
		}
		method, route, body = "POST", "/tools/script/cancel", map[string]any{"id": pos[0]}
	case args[0] == "script" && len(args) >= 2 && args[1] == "rm":
		_, pos := sc.flags(2)
		if len(pos) == 0 {
			fmt.Fprintln(errOut, "tools script rm: <id> is required")
			return 2
		}
		method, route, body = "POST", "/tools/script/rm", map[string]any{"id": pos[0]}
	case args[0] == "image" && len(args) >= 2 && args[1] == "build":
		flags, _ := sc.flags(2)
		if flags.Get("name") == "" || flags.Get("path") == "" {
			fmt.Fprintln(errOut, "tools image build: --name and --path are required")
			return 2
		}
		tag := flags.Get("tag")
		if tag == "" {
			tag = "latest"
		}
		method, route, body = "POST", "/tools/image/build", map[string]any{"name": flags.Get("name"), "tag": tag, "path": flags.Get("path")}
	case args[0] == "judge":
		var err error
		method, route, body, err = judgeCommand(sc.from(1))
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
	case args[0] == "tasks":
		var err error
		method, route, body, err = nativeTasksCommand(sc.from(1))
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
	default:
		fmt.Fprintf(errOut, "tools: unknown command %q (try 'tools help')\n", strings.Join(args, " "))
		return 2
	}

	// One check for every command: a flag no branch read is not understood by
	// this build. Fail loudly, before any request goes out.
	if flag, ok := sc.unknownFlag(); ok {
		fmt.Fprintf(errOut, "tools %s: unknown flag %s (tariboy-tools %s does not know it)\n",
			sc.command(), flag, version.Version)
		return 2
	}

	raw, err := c.Call(method, route, body)
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok {
			// The agentapi plugin_disabled message is already human-readable.
			fmt.Fprintf(errOut, "%s\n", apiErr.Msg)
			return 1
		}
		if client.IsDaemonDown(err) {
			fmt.Fprintln(errOut, "tools: agent socket is not reachable")
			return 2
		}
		fmt.Fprintf(errOut, "tools: %v\n", err)
		return 1
	}
	// whoami is the first command run when tools misbehave, so it names the
	// client build too — the daemon can only report its own (SUPER-224 §4).
	if route == "/tools/whoami" {
		raw = withClientVersion(raw)
	}
	if wantsText != "" {
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			if s, ok := m[wantsText].(string); ok {
				fmt.Fprintln(out, s)
				return 0
			}
		}
	}
	if route == "/tools/sources" && renderSources(raw, out) {
		return 0
	}
	if jsonOutput {
		fmt.Fprintln(out, string(raw))
		return 0
	}
	printResult(raw, out)
	if route == "/tools/tasks/create" {
		printFiledReportNote(raw, out)
	}
	return 0
}

func parseScriptCommand(sc *argScan, args []string, scheduled bool) (map[string]any, error) {
	action := "run"
	if scheduled {
		action = "schedule"
	}
	if len(args) < 5 || strings.TrimSpace(args[2]) == "" {
		return nil, fmt.Errorf("tools script %s: NAME [options] -- COMMAND required", action)
	}
	separator := -1
	for i := 3; i < len(args); i++ {
		if args[i] == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return nil, fmt.Errorf("tools script %s: -- separator before COMMAND is required", action)
	}
	flags, positionals := sc.flagsRange(3, separator)
	if len(positionals) != 0 {
		return nil, fmt.Errorf("tools script %s: unexpected argument %q before --", action, positionals[0])
	}
	command := sc.verbatim(separator + 1)
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("tools script %s: command is required after --", action)
	}
	name := args[2]
	description := flags.Get("description")
	if description == "" {
		description = name
	}
	result := map[string]any{"name": name, "description": description, "command": command}
	if !scheduled {
		return result, nil
	}
	every := flags.Get("every")
	interval, err := strconv.Atoi(every)
	if err != nil || interval <= 0 {
		return nil, errors.New("tools script schedule: --every must be a positive number of seconds")
	}
	result["interval_seconds"] = interval
	if value := flags.Get("quiet-exit"); value != "" {
		code, err := strconv.Atoi(value)
		if err != nil || code < 0 || code > 255 {
			return nil, errors.New("tools script schedule: --quiet-exit must be between 0 and 255")
		}
		result["quiet_exit"] = code
	}
	return result, nil
}

func nativeTasksCommand(sc *argScan) (method, route string, body any, err error) {
	args := sc.argv
	if len(args) == 0 {
		return "", "", nil, fmt.Errorf("tasks: a command is required")
	}
	action := args[0]
	if action == "work" || action == "artifacts" || action == "observe" {
		if len(args) < 2 {
			return "", "", nil, fmt.Errorf("tasks %s: a command is required", action)
		}
		action += "_" + args[1]
		if strings.HasPrefix(action, "artifacts_") {
			action = "artifact_" + strings.TrimPrefix(action, "artifacts_")
		}
		sc = sc.from(1)
		args = sc.argv
	}
	flags, pos := sc.flags(1)
	payload := map[string]any{}
	requireKey := func() (string, error) {
		if len(pos) == 0 || strings.TrimSpace(pos[0]) == "" {
			return "", fmt.Errorf("tasks %s: task key is required", action)
		}
		return pos[0], nil
	}
	copyFlag := func(name, target string) {
		if flags.Get(name) != "" {
			payload[target] = flags.Get(name)
		}
	}
	copyPresentFlag := func(name, target string) {
		if value, ok := flags.Lookup(name); ok {
			payload[target] = value
		}
	}
	switch action {
	case "work_next":
		copyFlag("queue", "queue")
		copyFlag("idempotency-key", "idempotency_key")
		if payload["idempotency_key"] == nil {
			return "", "", nil, fmt.Errorf("tasks work next: --idempotency-key is required")
		}
	case "work_show", "work_complete", "work_release":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["assignment_id"] = id
		copyFlag("task-revision", "task_revision")
		copyFlag("assignment-revision", "assignment_revision")
		copyFlag("idempotency-key", "idempotency_key")
		if action == "work_complete" {
			copyFlag("outcome", "outcome")
		}
	case "artifact_add":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["assignment_id"] = id
		copyFlag("task-revision", "task_revision")
		copyFlag("assignment-revision", "assignment_revision")
		copyFlag("idempotency-key", "idempotency_key")
		copyFlag("name", "name")
		copyFlag("type", "type")
		copyPresentFlag("content", "content")
		if flags.Get("metadata") != "" {
			var value map[string]any
			if e := json.Unmarshal([]byte(flags.Get("metadata")), &value); e != nil {
				return "", "", nil, fmt.Errorf("tasks artifacts add: --metadata is not valid JSON: %v", e)
			}
			payload["metadata"] = value
		}
	case "artifact_show":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		if len(pos) < 2 {
			return "", "", nil, fmt.Errorf("tasks artifacts show: artifact id is required")
		}
		payload["assignment_id"], payload["artifact_id"] = id, pos[1]
		copyFlag("task", "task_key")
	case "questions":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["assignment_id"] = id
	case "answer":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["question_id"] = id
		copyFlag("assignment", "assignment_id")
		copyFlag("answer", "answer")
		copyFlag("task-revision", "task_revision")
		copyFlag("assignment-revision", "assignment_revision")
		copyFlag("idempotency-key", "idempotency_key")
		action = "workflow_answer"
	case "observe_subscribe":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		if len(pos) < 2 {
			return "", "", nil, fmt.Errorf("tasks observe subscribe: pattern is required")
		}
		payload["assignment_id"], payload["pattern"] = id, pos[1]
		copyFlag("correlation-key", "correlation_key")
		copyFlag("reaction", "reaction")
		copyFlag("task-revision", "task_revision")
		copyFlag("assignment-revision", "assignment_revision")
		copyFlag("idempotency-key", "idempotency_key")
	case "observe_list":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["assignment_id"] = id
	case "observe_cancel":
		id, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		if len(pos) < 2 {
			return "", "", nil, fmt.Errorf("tasks observe cancel: subscription id is required")
		}
		payload["assignment_id"], payload["subscription_id"] = id, pos[1]
		copyFlag("task-revision", "task_revision")
		copyFlag("assignment-revision", "assignment_revision")
		copyFlag("idempotency-key", "idempotency_key")
	case "mine":
		copyFlag("queue", "queue")
		copyFlag("status", "status")
		copyFlag("assignee", "assignee")
		copyFlag("text", "text")
		copyFlag("waiting-for", "waiting_for")
	case "ready":
		copyFlag("queue", "queue")
		copyFlag("limit", "limit")
		copyFlag("idempotency-key", "idempotency_key")
		if flags.Has("claim") {
			payload["claim"] = true
		}
	case "show":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["key"] = key
	case "create":
		copyFlag("queue", "queue")
		copyFlag("parent", "parent_key")
		copyFlag("title", "title")
		copyFlag("description", "description")
		copyFlag("assignee", "assignee")
		copyFlag("group", "group")
		copyFlag("priority", "priority")
		copyFlag("idempotency-key", "idempotency_key")
		if payload["title"] == nil {
			return "", "", nil, fmt.Errorf("tasks create: --title is required")
		}
		if payload["queue"] == nil && payload["parent_key"] == nil {
			return "", "", nil, fmt.Errorf("tasks create: --queue or --parent is required")
		}
	case "update":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["key"] = key
		copyFlag("title", "title")
		copyFlag("description", "description")
		copyFlag("status", "status")
		copyFlag("assignee", "assignee")
		copyPresentFlag("manual-block-reason", "manual_block_reason")
		copyFlag("priority", "priority")
		copyFlag("revision", "revision")
	case "assign":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		if len(pos) < 2 {
			return "", "", nil, fmt.Errorf("tasks assign: assignee is required")
		}
		payload["key"], payload["assignee"] = key, pos[1]
		copyFlag("revision", "revision")
	case "comment":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		text := strings.TrimSpace(strings.Join(pos[1:], " "))
		if flags.Get("body") != "" {
			text = flags.Get("body")
		}
		if text == "" {
			return "", "", nil, fmt.Errorf("tasks comment: comment text is required")
		}
		payload["key"], payload["body"] = key, text
		copyFlag("idempotency-key", "idempotency_key")
	case "ask":
		if flags.Get("question") != "" {
			id, e := requireKey()
			if e != nil {
				return "", "", nil, e
			}
			payload["assignment_id"] = id
			copyFlag("question", "question")
			copyFlag("context", "context")
			copyFlag("blocking-scope", "blocking_scope")
			copyFlag("anchor", "anchor")
			copyFlag("suggested-answer", "suggested_answer")
			if raw := flags.Get("options"); raw != "" {
				payload["options"] = splitCSV(raw)
			}
			if raw := flags.Get("artifacts"); raw != "" {
				ids := []int64{}
				for _, value := range splitCSV(raw) {
					id, parseErr := strconv.ParseInt(value, 10, 64)
					if parseErr != nil {
						return "", "", nil, fmt.Errorf("tasks ask: --artifacts must contain numeric ids")
					}
					ids = append(ids, id)
				}
				payload["artifact_attachments"] = ids
			}
			copyFlag("task-revision", "task_revision")
			copyFlag("assignment-revision", "assignment_revision")
			copyFlag("idempotency-key", "idempotency_key")
			action = "workflow_ask"
			break
		}
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		if len(pos) < 3 {
			return "", "", nil, fmt.Errorf("tasks ask: principal and question are required")
		}
		payload["key"], payload["principal"] = key, pos[1]
		payload["body"] = strings.Join(pos[2:], " ")
		copyFlag("idempotency-key", "idempotency_key")
	case "move":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["key"] = key
		copyFlag("parent", "parent_key")
		copyFlag("before", "before_key")
		copyFlag("revision", "revision")
		if flags.Has("to-root") {
			if payload["parent_key"] != nil || payload["before_key"] != nil {
				return "", "", nil, fmt.Errorf("tasks move: --to-root cannot be combined with --parent or --before")
			}
			// Detaching is the absence of a parent, so an explicit flag is the only way to
			// tell "move it out of its tree" apart from a typo that dropped --parent.
			payload["parent_key"] = ""
		} else if payload["parent_key"] == nil && payload["before_key"] == nil {
			return "", "", nil, fmt.Errorf("tasks move: pass --parent, --before, or --to-root to detach")
		}
	case "block":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		if flags.Get("by") == "" {
			return "", "", nil, fmt.Errorf("tasks block: --by is required")
		}
		payload["key"], payload["blocker_key"] = key, flags.Get("by")
		copyFlag("revision", "revision")
		copyFlag("idempotency-key", "idempotency_key")
	case "relate":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		if len(pos) < 2 {
			return "", "", nil, fmt.Errorf("tasks relate: related task key is required")
		}
		payload["key"], payload["target_key"] = key, pos[1]
		copyFlag("revision", "revision")
		copyFlag("idempotency-key", "idempotency_key")
	case "done":
		key, e := requireKey()
		if e != nil {
			return "", "", nil, e
		}
		payload["key"] = key
		copyFlag("revision", "revision")
		if flags.Has("complete-anyway") {
			payload["complete_anyway"] = true
		}
	default:
		return "", "", nil, fmt.Errorf("tasks: unknown command %q", action)
	}
	return "POST", "/tools/tasks/" + action, payload, nil
}

// renderSources pretty-prints `tools sources` as one channel per line, appending
// provider annotations (provider=, params: {...}, help) for provider channels
// (spec §6.1). Returns false if the payload isn't the expected shape so the
// caller falls back to the generic renderer.
func renderSources(raw json.RawMessage, out io.Writer) bool {
	var m struct {
		Channels []struct {
			Name     string   `json:"name"`
			Kind     string   `json:"kind"`
			Provider string   `json:"provider"`
			Params   []string `json:"params"`
			Help     string   `json:"help"`
		} `json:"channels"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	if len(m.Channels) == 0 {
		fmt.Fprintln(out, "no channels")
		return true
	}
	for _, c := range m.Channels {
		parts := []string{c.Name, c.Kind}
		if c.Provider != "" {
			parts = append(parts, "provider="+c.Provider)
		}
		if len(c.Params) > 0 {
			parts = append(parts, "params: {"+strings.Join(c.Params, ",")+"}")
		}
		if c.Help != "" {
			parts = append(parts, c.Help)
		}
		fmt.Fprintln(out, strings.Join(parts, "  "))
	}
	return true
}

// withClientVersion folds this binary's version into a whoami result so both
// the plain and the --json rendering show it. A result that is not an object,
// or one that already carries the key, is returned untouched.
func withClientVersion(raw json.RawMessage) json.RawMessage {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return raw
	}
	if _, ok := m["client_version"]; !ok {
		m["client_version"] = version.Version
	}
	merged, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return merged
}

// printFiledReportNote explains a task filed into a queue the caller does not run. Without
// it the next `tasks show` on that key answers not_found, which reads as a failure and
// invites filing the same report again.
func printFiledReportNote(raw json.RawMessage, out io.Writer) {
	var m struct {
		Key   string `json:"key"`
		Queue string `json:"queue"`
		Filed bool   `json:"filed"`
	}
	if json.Unmarshal(raw, &m) != nil || !m.Filed {
		return
	}
	fmt.Fprintf(out, "\n%s is filed into queue %s: no assignee, and no longer visible to you.\n",
		m.Key, m.Queue)
	fmt.Fprintln(out, "Whoever runs that queue triages it. It is recorded — do not file it again.")
}

func printResult(raw json.RawMessage, out io.Writer) {
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(out, "%s: %v\n", k, m[k])
		}
		return
	}
	fmt.Fprintln(out, strings.Trim(string(raw), "\"\n"))
}

// judgeCommand translates the deliberately small judge CLI surface into the
// generic authenticated action endpoint.  Agent and iteration identity are
// intentionally absent: the daemon derives both from the tools socket.
func judgeCommand(sc *argScan) (method, route string, body any, err error) {
	args := sc.argv
	if len(args) < 2 {
		return "", "", nil, fmt.Errorf("tools judge: a command is required")
	}
	flags, pos := sc.flags(2)
	action := ""
	request := func(flag string) (string, error) {
		if strings.TrimSpace(flags.Get(flag)) == "" {
			return "", fmt.Errorf("tools judge %s %s: --%s is required", args[0], args[1], flag)
		}
		return flags.Get(flag), nil
	}
	positional := func() (string, error) {
		if len(pos) == 0 || strings.TrimSpace(pos[0]) == "" {
			return "", fmt.Errorf("tools judge %s %s: run id is required", args[0], args[1])
		}
		return pos[0], nil
	}
	switch args[0] + " " + args[1] {
	case "iterations search":
		action = "iterations.search"
		agent, e := request("agent")
		if e != nil {
			return "", "", nil, e
		}
		judgeGroup, e := request("judge-group")
		if e != nil {
			return "", "", nil, e
		}
		selector := map[string]any{"agents": []string{agent}}
		if flags.Get("group") != "" {
			selector["group"] = flags.Get("group")
		}
		if flags.Get("since") != "" {
			selector["since"] = flags.Get("since")
		}
		if flags.Get("until") != "" {
			selector["until"] = flags.Get("until")
		}
		if flags.Get("status") != "" {
			selector["statuses"] = splitCSV(flags.Get("status"))
		}
		if flags.Get("limit") != "" {
			n, e := strconv.Atoi(flags.Get("limit"))
			if e != nil {
				return "", "", nil, fmt.Errorf("tools judge iterations search: --limit must be an integer")
			}
			selector["limit"] = n
		}
		body = map[string]any{"judge_group": judgeGroup, "selector": selector}
	case "run create":
		action = "run.create"
		file, e := request("request-file")
		if e != nil {
			return "", "", nil, e
		}
		criteria, e := os.ReadFile(file)
		if e != nil {
			return "", "", nil, fmt.Errorf("tools judge run create: read request file: %w", e)
		}
		selectorText, e := request("selector")
		if e != nil {
			return "", "", nil, e
		}
		selector, e := jsonObject(selectorText)
		if e != nil {
			return "", "", nil, fmt.Errorf("tools judge run create: --selector is not valid JSON object: %w", e)
		}
		judges, e := request("judges")
		if e != nil {
			return "", "", nil, e
		}
		summary, e := request("summary-agent")
		if e != nil {
			return "", "", nil, e
		}
		b := map[string]any{"original_request": string(criteria), "selector": selector, "judge_agents": splitCSV(judges), "summary_agent": summary}
		if flags.Get("judge-group") != "" {
			b["judge_group"] = flags.Get("judge-group")
		}
		if flags.Get("judges-per-iteration") != "" {
			n, e := strconv.Atoi(flags.Get("judges-per-iteration"))
			if e != nil {
				return "", "", nil, fmt.Errorf("tools judge run create: --judges-per-iteration must be an integer")
			}
			b["judges_per_iteration"] = n
		}
		body = b
	case "run inspect", "run cancel", "work retry", "summary claim":
		id, e := positional()
		if e != nil {
			return "", "", nil, e
		}
		action = strings.ReplaceAll(args[0]+"."+args[1], " ", ".")
		body = map[string]any{"run_id": id}
	case "work claim":
		action = "work.claim"
		body = map[string]any{}
		if flags.Get("run") != "" {
			body.(map[string]any)["run_id"] = flags.Get("run")
		}
	case "evidence search":
		action = "evidence.search"
		assignment, e := request("assignment")
		if e != nil {
			return "", "", nil, e
		}
		artifact, e := request("artifact")
		if e != nil {
			return "", "", nil, e
		}
		b := map[string]any{"assignment_id": assignment, "artifacts": []string{artifact}}
		if flags.Get("query") != "" {
			b["query"] = flags.Get("query")
		}
		if flags.Get("cursor") != "" {
			b["cursor"] = flags.Get("cursor")
		}
		body = b
	case "evidence get":
		action = "evidence.get"
		assignment, e := request("assignment")
		if e != nil {
			return "", "", nil, e
		}
		artifact, e := request("artifact")
		if e != nil {
			return "", "", nil, e
		}
		locator, e := jsonObjectFlag(flags, "locator")
		if e != nil {
			return "", "", nil, fmt.Errorf("tools judge evidence get: %w", e)
		}
		body = map[string]any{"assignment_id": assignment, "artifact": artifact, "locator": locator}
	case "analysis submit":
		action = "analysis.submit"
		assignment, e := request("assignment")
		if e != nil {
			return "", "", nil, e
		}
		result, raw, e := jsonFile(flags, "file")
		if e != nil {
			return "", "", nil, fmt.Errorf("tools judge analysis submit: %w", e)
		}
		body = map[string]any{"assignment_id": assignment, "result": result, "raw_submission": raw}
	case "summary inputs":
		id, e := positional()
		if e != nil {
			return "", "", nil, e
		}
		action = "summary.inputs"
		b := map[string]any{"run_id": id}
		if flags.Get("cursor") != "" {
			b["cursor"] = flags.Get("cursor")
		}
		body = b
	case "summary submit":
		id, e := positional()
		if e != nil {
			return "", "", nil, e
		}
		action = "summary.submit"
		result, raw, e := jsonFile(flags, "file")
		if e != nil {
			return "", "", nil, fmt.Errorf("tools judge summary submit: %w", e)
		}
		body = map[string]any{"run_id": id, "result": result, "raw_submission": raw}
	default:
		return "", "", nil, fmt.Errorf("tools judge: unknown command %q", strings.Join(args[:2], " "))
	}
	return "POST", "/tools/judge/action/" + action, body, nil
}

func jsonObject(text string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, err
	}
	if v == nil {
		return nil, fmt.Errorf("must be an object")
	}
	return v, nil
}

func jsonObjectFlag(flags *flagSet, name string) (map[string]any, error) {
	text := flags.Get(name)
	if text == "" {
		return nil, fmt.Errorf("--%s is required", name)
	}
	v, err := jsonObject(text)
	if err != nil {
		return nil, fmt.Errorf("--%s is not valid JSON object: %w", name, err)
	}
	return v, nil
}

func jsonFile(flags *flagSet, name string) (map[string]any, string, error) {
	file := flags.Get(name)
	if file == "" {
		return nil, "", fmt.Errorf("--%s is required", name)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", name, err)
	}
	v, err := jsonObject(string(raw))
	if err != nil {
		return nil, "", fmt.Errorf("%s is not valid JSON object: %w", file, err)
	}
	return v, string(raw), nil
}

// isFlagToken reports whether an argv token is a --flag. The bare `--` is not:
// it is the end-of-flags separator of `script run` or `script schedule`.
func isFlagToken(a string) bool { return strings.HasPrefix(a, "--") && a != "--" }

// argScan is one invocation's argv plus a record of which --flag tokens a
// command actually read. Parsing alone does not mark a flag as read; only a
// Get/Lookup/Has for that name does. Whatever is still unread once dispatch has
// built the request is a flag this build of the client does not understand, and
// Run turns that into an error before any daemon call (SUPER-224: a silently
// dropped flag is what made a client/daemon version drift invisible).
//
// The check is one place, at the end of Run, and it is unavoidable by
// construction: a new subcommand cannot obtain flags without going through this
// scan, and a subcommand that ignores its arguments entirely still leaves the
// tokens unread. Sub-scans share the parent's record through the same backing
// array, so the tasks/judge builders are covered by the same single check.
type argScan struct {
	argv []string
	read []bool
}

func newArgScan(args []string) *argScan {
	return &argScan{argv: args, read: make([]bool, len(args))}
}

// from returns a scan over argv[n:] sharing this scan's read record.
func (s *argScan) from(n int) *argScan {
	if n > len(s.argv) {
		n = len(s.argv)
	}
	return &argScan{argv: s.argv[n:], read: s.read[n:]}
}

// text joins argv[n:] as free text (a status message, a context body). Flag
// tokens inside it stay unread on purpose, so `tools status set x --bogus` is
// an error rather than a status message with a typo baked into it.
func (s *argScan) text(n int) string {
	if n >= len(s.argv) {
		return ""
	}
	return strings.Join(s.argv[n:], " ")
}

// verbatim joins argv[n:] and marks its flags read: the tail after `--` is
// another program's command line, so its flags are none of our business.
func (s *argScan) verbatim(n int) string {
	for i := n; i < len(s.argv); i++ {
		s.read[i] = true
	}
	return s.text(n)
}

// unknownFlag returns the first --flag token no command branch ever read.
func (s *argScan) unknownFlag() (string, bool) {
	for i, a := range s.argv {
		if !s.read[i] && isFlagToken(a) {
			return a, true
		}
	}
	return "", false
}

// command names the invoked command for diagnostics — the leading non-flag
// tokens, at most two ("tasks update", "status set", "request").
func (s *argScan) command() string {
	var parts []string
	for _, a := range s.argv {
		if strings.HasPrefix(a, "--") || len(parts) == 2 {
			break
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// flags parses argv[from:] into a flagSet plus the positionals.
func (s *argScan) flags(from int) (*flagSet, []string) {
	return s.flagsRange(from, len(s.argv))
}

// flagsRange parses argv[from:to], splitting --flag(=value|space value) pairs
// from positionals. Values are not marked read: only a flag's own token can be
// unknown, and a value never starts with "--" unless it was written as
// --flag=--value, which stays part of the flag token.
func (s *argScan) flagsRange(from, to int) (*flagSet, []string) {
	f := &flagSet{scan: s, value: map[string]string{}, at: map[string][]int{}}
	var pos []string
	if from > len(s.argv) {
		from = len(s.argv)
	}
	if to > len(s.argv) {
		to = len(s.argv)
	}
	for i := from; i < to; i++ {
		a := s.argv[i]
		if strings.HasPrefix(a, "--") {
			name := a[2:]
			if eq := strings.Index(name, "="); eq >= 0 {
				f.set(name[:eq], name[eq+1:], i)
				continue
			}
			if i+1 < to && !strings.HasPrefix(s.argv[i+1], "--") {
				f.set(name, s.argv[i+1], i)
				i++
			} else {
				f.set(name, "true", i)
			}
			continue
		}
		pos = append(pos, a)
	}
	return f, pos
}

// flagSet is one command's parsed flags. Reading a flag by name is what marks
// its argv token as understood — see argScan.
type flagSet struct {
	scan  *argScan
	value map[string]string
	at    map[string][]int // argv indices of every occurrence, for the read mark
}

func (f *flagSet) set(name, value string, at int) {
	f.value[name] = value
	f.at[name] = append(f.at[name], at)
}

// Lookup returns a flag's value and whether it was given, marking it read.
func (f *flagSet) Lookup(name string) (string, bool) {
	for _, i := range f.at[name] {
		f.scan.read[i] = true
	}
	v, ok := f.value[name]
	return v, ok
}

// Get returns a flag's value ("" when absent), marking it read.
func (f *flagSet) Get(name string) string { v, _ := f.Lookup(name); return v }

// Has reports whether a flag was given, marking it read.
func (f *flagSet) Has(name string) bool { _, ok := f.Lookup(name); return ok }

// splitCSV splits a comma list into trimmed, non-empty items (e.g. --members a,b).
func splitCSV(csv string) []string {
	var out []string
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func subjectMap(csv string) map[string]any {
	out := map[string]any{}
	for _, kv := range strings.Split(csv, ",") {
		if kv = strings.TrimSpace(kv); kv == "" {
			continue
		}
		if i := strings.Index(kv, "="); i >= 0 {
			out[kv[:i]] = kv[i+1:]
		}
	}
	return out
}
