package taskcli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type request struct {
	action  string
	payload map[string]any
}
type usageError struct{ message string }

func (e usageError) Error() string { return e.message }

var commonWork = map[string]bool{"task-revision": true, "assignment-revision": true, "idempotency-key": true}

func parse(argv []string) (request, error) {
	if len(argv) == 0 {
		return request{}, usageError{"tasks: a command is required"}
	}
	action, rest := argv[0], argv[1:]
	if action == "work" || action == "artifacts" || action == "observe" {
		if len(rest) == 0 {
			return request{}, usageError{fmt.Sprintf("tasks %s: a command is required", action)}
		}
		if action == "artifacts" {
			action = "artifact"
		}
		action += "_" + rest[0]
		rest = rest[1:]
	}
	allowed := map[string]map[string]bool{
		"mine": set("queue,status,assignee,text,waiting-for"), "ready": set("queue,limit,idempotency-key,claim"), "show": {},
		"create": set("queue,parent,title,description,pull-request,assignee,group,priority,idempotency-key"),
		"update": set("title,description,status,pull-request,assignee,manual-block-reason,priority,revision"), "assign": set("revision"),
		"comment": set("body,idempotency-key"), "ask": set("question,context,blocking-scope,anchor,suggested-answer,options,artifacts,task-revision,assignment-revision,idempotency-key"),
		"move": set("parent,before,to-root,revision"), "block": set("by,revision,idempotency-key"), "relate": set("revision,idempotency-key"), "done": set("revision,complete-anyway"),
		"work_next": set("queue,idempotency-key"), "work_show": commonWork, "work_complete": set("task-revision,assignment-revision,idempotency-key,outcome"), "work_release": commonWork,
		"artifact_add": set("task-revision,assignment-revision,idempotency-key,name,type,content,metadata"), "artifact_show": set("task"), "questions": {},
		"answer": set("task-revision,assignment-revision,idempotency-key,assignment,answer"), "observe_subscribe": set("task-revision,assignment-revision,idempotency-key,correlation-key,reaction"), "observe_list": {}, "observe_cancel": commonWork,
	}
	valid, ok := allowed[action]
	if !ok {
		return request{}, usageError{fmt.Sprintf("tasks: unknown command %q", action)}
	}
	flags, pos, err := parseFlags(rest, valid)
	if err != nil {
		return request{}, err
	}
	p := map[string]any{}
	copyFlag := func(name string, present bool) {
		if v, ok := flags[name]; ok && (present || v != "") {
			p[strings.ReplaceAll(name, "-", "_")] = v
		}
	}
	require := func(index int, label string) (string, error) {
		if len(pos) <= index || strings.TrimSpace(pos[index]) == "" {
			return "", usageError{fmt.Sprintf("tasks %s: %s is required", strings.ReplaceAll(action, "_", " "), label)}
		}
		return pos[index], nil
	}
	workFields := func() {
		for name := range commonWork {
			copyFlag(name, false)
		}
	}
	switch action {
	case "work_next":
		copyFlag("queue", false)
		copyFlag("idempotency-key", false)
		if _, ok := p["idempotency_key"]; !ok {
			return request{}, usageError{"tasks work next: --idempotency-key is required"}
		}
	case "work_show", "work_complete", "work_release":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["assignment_id"] = v
		workFields()
		if action == "work_complete" {
			copyFlag("outcome", false)
		}
	case "artifact_add":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["assignment_id"] = v
		for _, name := range []string{"task-revision", "assignment-revision", "idempotency-key", "name", "type"} {
			copyFlag(name, false)
		}
		copyFlag("content", true)
		if v, ok := flags["metadata"]; ok {
			var metadata any
			if err := json.Unmarshal([]byte(v), &metadata); err != nil {
				return request{}, usageError{fmt.Sprintf("tasks artifacts add: --metadata is not valid JSON: %v", err)}
			}
			p["metadata"] = metadata
		}
	case "artifact_show":
		a, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		id, e := require(1, "artifact id")
		if e != nil {
			return request{}, e
		}
		p["assignment_id"], p["artifact_id"] = a, id
		if v, ok := flags["task"]; ok && v != "" {
			p["task_key"] = v
		}
	case "questions":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["assignment_id"] = v
	case "answer":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["question_id"] = v
		if v, ok := flags["assignment"]; ok && v != "" {
			p["assignment_id"] = v
		}
		copyFlag("answer", false)
		workFields()
		action = "workflow_answer"
	case "observe_subscribe":
		a, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		pattern, e := require(1, "pattern")
		if e != nil {
			return request{}, e
		}
		p["assignment_id"], p["pattern"] = a, pattern
		workFields()
		copyFlag("correlation-key", false)
		copyFlag("reaction", false)
	case "observe_list":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["assignment_id"] = v
	case "observe_cancel":
		a, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		id, e := require(1, "subscription id")
		if e != nil {
			return request{}, e
		}
		p["assignment_id"], p["subscription_id"] = a, id
		workFields()
	case "mine":
		for name := range valid {
			copyFlag(name, false)
		}
	case "ready":
		for _, name := range []string{"queue", "limit", "idempotency-key"} {
			copyFlag(name, false)
		}
		if _, ok := flags["claim"]; ok {
			p["claim"] = true
		}
	case "show":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["key"] = v
	case "create":
		for name := range valid {
			copyFlag(name, false)
		}
		if v, ok := p["parent"].(string); ok {
			delete(p, "parent")
			p["parent_key"] = v
		}
		if _, ok := p["title"]; !ok {
			return request{}, usageError{"tasks create: --title is required"}
		}
		if _, q := p["queue"]; !q {
			if _, parent := p["parent_key"]; !parent {
				return request{}, usageError{"tasks create: --queue or --parent is required"}
			}
		}
	case "update":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["key"] = v
		for name := range valid {
			copyFlag(name, name == "manual-block-reason" || name == "pull-request")
		}
	case "assign":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		a, e := require(1, "assignee")
		if e != nil {
			return request{}, e
		}
		p["key"], p["assignee"] = v, a
		copyFlag("revision", false)
	case "comment":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		body := strings.TrimSpace(flags["body"])
		if body == "" {
			body = strings.TrimSpace(strings.Join(pos[1:], " "))
		}
		if body == "" {
			return request{}, usageError{"tasks comment: comment text is required"}
		}
		p["key"], p["body"] = v, body
		copyFlag("idempotency-key", false)
	case "ask":
		if _, workflow := flags["question"]; workflow {
			if len(pos) > 1 {
				return request{}, usageError{"tasks ask: workflow ask does not accept a positional principal or question"}
			}
			v, e := require(0, "task key")
			if e != nil {
				return request{}, e
			}
			p["assignment_id"] = v
			for _, name := range []string{"question", "context", "blocking-scope", "anchor", "suggested-answer", "task-revision", "assignment-revision", "idempotency-key"} {
				copyFlag(name, false)
			}
			if v, ok := flags["options"]; ok {
				p["options"] = csv(v)
			}
			if v, ok := flags["artifacts"]; ok {
				ids := []int{}
				for _, item := range csv(v) {
					id, e := strconv.Atoi(item)
					if e != nil {
						return request{}, usageError{"tasks ask: --artifacts must contain numeric ids"}
					}
					ids = append(ids, id)
				}
				p["artifact_attachments"] = ids
			}
			action = "workflow_ask"
		} else {
			for name := range flags {
				if name != "idempotency-key" {
					return request{}, usageError{fmt.Sprintf("tasks ask: --%s is a workflow-only flag", name)}
				}
			}
			v, e := require(0, "task key")
			if e != nil {
				return request{}, e
			}
			principal, e := require(1, "principal")
			if e != nil {
				return request{}, e
			}
			if len(pos) < 3 {
				return request{}, usageError{"tasks ask: principal and question are required"}
			}
			p["key"], p["principal"], p["body"] = v, principal, strings.Join(pos[2:], " ")
			copyFlag("idempotency-key", false)
		}
	case "move":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["key"] = v
		if v, ok := flags["parent"]; ok && v != "" {
			p["parent_key"] = v
		}
		if v, ok := flags["before"]; ok && v != "" {
			p["before_key"] = v
		}
		copyFlag("revision", false)
		if _, root := flags["to-root"]; root {
			if _, parent := p["parent_key"]; parent {
				return request{}, usageError{"tasks move: --to-root cannot be combined with --parent or --before"}
			}
			if _, before := p["before_key"]; before {
				return request{}, usageError{"tasks move: --to-root cannot be combined with --parent or --before"}
			}
			p["parent_key"] = ""
		} else {
			if _, parent := p["parent_key"]; !parent {
				if _, before := p["before_key"]; !before {
					return request{}, usageError{"tasks move: pass --parent, --before, or --to-root to detach"}
				}
			}
		}
	case "block":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		by := flags["by"]
		if by == "" {
			return request{}, usageError{"tasks block: --by is required"}
		}
		p["key"], p["blocker_key"] = v, by
		copyFlag("revision", false)
		copyFlag("idempotency-key", false)
	case "relate":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		target, e := require(1, "related task key")
		if e != nil {
			return request{}, e
		}
		p["key"], p["target_key"] = v, target
		copyFlag("revision", false)
		copyFlag("idempotency-key", false)
	case "done":
		v, e := require(0, "task key")
		if e != nil {
			return request{}, e
		}
		p["key"] = v
		copyFlag("revision", false)
		if _, ok := flags["complete-anyway"]; ok {
			p["complete_anyway"] = true
		}
	}
	if err := noExtra(action, pos); err != nil {
		return request{}, err
	}
	return request{action, p}, nil
}

func set(csv string) map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Split(csv, ",") {
		if name != "" {
			out[name] = true
		}
	}
	return out
}
func csv(value string) []string {
	out := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
func parseFlags(args []string, allowed map[string]bool) (map[string]string, []string, error) {
	flags := map[string]string{}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			pos = append(pos, arg)
			continue
		}
		name, value, has := strings.Cut(arg[2:], "=")
		if !allowed[name] {
			return nil, nil, usageError{fmt.Sprintf("unknown flag --%s", name)}
		}
		if !has {
			value = "true"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				value = args[i]
			}
		}
		flags[name] = value
	}
	return flags, pos, nil
}
func noExtra(action string, pos []string) error {
	limits := map[string]int{"mine": 0, "ready": 0, "show": 1, "create": 0, "update": 1, "assign": 2, "comment": -1, "ask": -1, "workflow_ask": 1, "move": 1, "block": 1, "relate": 2, "done": 1, "work_next": 0, "work_show": 1, "work_complete": 1, "work_release": 1, "artifact_add": 1, "artifact_show": 2, "questions": 1, "answer": 1, "workflow_answer": 1, "observe_subscribe": 2, "observe_list": 1, "observe_cancel": 2}
	if limit, ok := limits[action]; ok && limit >= 0 && len(pos) > limit {
		return usageError{fmt.Sprintf("tasks %s: unexpected argument: %s", strings.ReplaceAll(action, "_", " "), pos[limit])}
	}
	return nil
}
