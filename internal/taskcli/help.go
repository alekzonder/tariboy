package taskcli

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/registry"
)

type commandHelp struct {
	summary, usage, help, arguments, examples string
}

const workflowRevisions = " --task-revision 3 --assignment-revision 2 --idempotency-key example-1"

var sharedHelp = map[string]commandHelp{
	"mine": {"List tasks visible to the current actor", "mine [flags]",
		"Agent mode lists accessible work; operator mode lists host tasks. Filters narrow the result; --waiting-for selects a principal whose answer is pending.", "", "ttasks mine --queue DEV --status in_progress\nttasks mine --waiting-for agent:worker"},
	"ready": {"List or claim eligible flexible tasks", "ready [flags]",
		"Ready tasks are open, unassigned, unblocked, and not workflow-managed. Agent mode may atomically claim work with --claim. Operator mode only lists it; its default limit is 50 (maximum 200).", "", "ttasks ready --queue DEV\nttasks ready --claim --idempotency-key claim-1"},
	"show": {"Inspect a task and its conversation", "show KEY",
		"Read task fields, comments, pending answers, relations, and descendant counts, subject to task visibility.", "KEY  Task key, for example DEV-12 (required)", "ttasks show DEV-12"},
	"create": {"Create a root task or child task", "create --title TEXT (--queue QUEUE | --parent KEY) [flags]",
		"Use --queue for a root or --parent for a child. In an agent's non-owned queue, omitting --assignee files an unassigned task: the filed response means it is recorded but no longer visible. An explicit assignee retains task ownership without exposing the queue; --group is forbidden there.", "", "ttasks create --queue DEV --title 'Investigate failure' --assignee worker --priority P1\nttasks create --parent DEV-12 --title 'Add regression test'"},
	"update": {"Change task fields", "update KEY [flags]",
		"Only supplied fields are updated. Empty --pull-request or --manual-block-reason clears that field. Flexible statuses are open, in_progress, wait_customer, done, and cancelled; workflow-managed status changes must follow the work packet.", "KEY  Task key (required)", "ttasks update DEV-12 --priority P0\nttasks update DEV-12 --pull-request=''"},
	"assign": {"Assign a flexible task to an agent", "assign KEY ASSIGNEE [flags]",
		"Changes task ownership without granting access to the rest of its queue. Workflow phase ownership is leased through work next instead.", "KEY  Task key (required)\nASSIGNEE  Existing agent name (required)", "ttasks assign DEV-12 worker"},
	"comment": {"Add a durable Markdown task comment", "comment KEY TEXT... [flags]\n       ttasks comment KEY --body TEXT [flags]",
		"Pass comment text positionally or with --body, never both. Typed @agent:name and @user:login mentions request an answer; the mentioned principal's next comment resolves its own pending wait.", "KEY  Task key (required)\nTEXT  Markdown comment text (required unless --body is supplied)", "ttasks comment DEV-12 'Found the boundary'\nttasks comment DEV-12 --body 'Tests passed'"},
	"ask": {"Request a durable answer on flexible or workflow work", "ask KEY PRINCIPAL TEXT... [--idempotency-key KEY]\n       ttasks ask ASSIGNMENT --question TEXT --context TEXT --blocking-scope SCOPE [flags]",
		"Flexible form: use a task key and user:login or agent:name (a bare name means agent:name). It creates a principal wait; asking the customer moves assigned flexible work to wait_customer. No assignment or revisions are needed.\nWorkflow form (agent-only): --question selects an assignment-scoped question routed by the pinned workflow. --context, --blocking-scope, both revisions and --idempotency-key are required. Do not pass a positional principal or question. Only holds allowed by the packet are accepted.",
		"KEY  Flexible task key\nPRINCIPAL  user:login or agent:name to answer\nTEXT  Flexible question text\nASSIGNMENT  Leased workflow assignment id (not a task key)",
		"ttasks ask DEV-12 user:login 'Which behavior should win?'\nttasks ask 17 --question 'Keep compatibility?' --context 'The old API accepts empty values' --blocking-scope assignment" + workflowRevisions},
	"move": {"Reparent, reorder, or detach a task", "move KEY (--parent KEY | --before KEY | --to-root) [flags]",
		"Moves within the same queue. --before reorders within a priority bucket. --to-root detaches and cannot be combined with --parent or --before; detaching may remove the agent's inherited access.", "KEY  Task to move (required)", "ttasks move DEV-12 --parent DEV-3\nttasks move DEV-12 --to-root"},
	"block": {"Make one task depend on another", "block KEY --by BLOCKER [flags]",
		"Creates a directional blocks relation: BLOCKER must finish before KEY is ready. Cycles are rejected; write access to both tasks is required. --revision refers to the blocker task.", "KEY  Task being blocked (required)", "ttasks block DEV-12 --by DEV-2"},
	"relate": {"Link two related tasks", "relate KEY TARGET [flags]",
		"Adds a symmetric related relation without blocking either task. Requires write access to both endpoints.", "KEY  Source task key (required)\nTARGET  Related task key (required)", "ttasks relate DEV-12 DEV-9"},
	"done": {"Complete a flexible task", "done KEY [flags]",
		"Marks work done. Active descendants cause a conflict unless --complete-anyway is explicit. Workflow-managed work must use work complete with a declared outcome.", "KEY  Task key (required)", "ttasks done DEV-12"},
	"work_next": {"Claim one eligible workflow assignment", "work next --idempotency-key KEY [--queue QUEUE]",
		"Agent-only: atomically leases work to the current running iteration and returns its work packet, allowed actions, inputs, outcomes, and revisions. Reuse the retry key for the same claim intent.", "", "ttasks work next --queue DEV --idempotency-key iter-42-claim"},
	"work_show": {"Read an assignment's current work packet", "work show ASSIGNMENT [flags]",
		"Agent-only: reads the permitted packet, including current revisions, artifacts, questions, and observations. This is a read; revision and retry flags are not required.", "ASSIGNMENT  Workflow assignment id, not a task key (required)", "ttasks work show 17"},
	"work_complete": {"Report a declared workflow outcome", "work complete ASSIGNMENT --outcome OUTCOME [flags]",
		"Agent-only: completes the current lease and evaluates the pinned workflow. Required output artifacts must exist and --outcome must be allowed by the packet. Both revisions and --idempotency-key are required.", "ASSIGNMENT  Leased workflow assignment id (required)", "ttasks work complete 17 --outcome implemented" + workflowRevisions},
	"work_release": {"Release a workflow lease for bounded retry", "work release ASSIGNMENT [flags]",
		"Agent-only: relinquishes the current attempt through the workflow's retry/exhaustion policy. Both revisions and --idempotency-key are required.", "ASSIGNMENT  Leased workflow assignment id (required)", "ttasks work release 17" + workflowRevisions},
	"artifact_add": {"Attach an immutable workflow output", "artifacts add ASSIGNMENT --name NAME --type TYPE [flags]",
		"Agent-only: records a named artifact for this assignment. Use a name declared by the packet and type markdown, json, file, commit, or url. Both revisions and --idempotency-key are required. File content is a reference, not an upload.", "ASSIGNMENT  Leased workflow assignment id (required)", `ttasks artifacts add 17 --name implementation --type commit --content '{"repository":"example/project","ref":"abc123"}'` + workflowRevisions},
	"artifact_show": {"Read one artifact visible to an assignment", "artifacts show ASSIGNMENT ARTIFACT --task KEY",
		"Agent-only: reads an artifact within the packet's access boundary. --task is required to identify its task; it does not grant additional access.", "ASSIGNMENT  Workflow assignment id (required)\nARTIFACT  Numeric artifact id (required)", "ttasks artifacts show 17 9 --task DEV-12"},
	"questions": {"List questions for a workflow assignment", "questions ASSIGNMENT",
		"Agent-only: reads assignment-scoped questions and their answer state. For flexible task waits and comments, use show KEY instead.", "ASSIGNMENT  Workflow assignment id (required)", "ttasks questions 17"},
	"answer": {"Answer a workflow question", "answer QUESTION --assignment ASSIGNMENT --answer TEXT [flags]",
		"Agent-only: the leased question/manager assignment records an answer and releases the associated hold. --assignment, --answer, both revisions and --idempotency-key are required. Answer flexible waits with comment KEY TEXT instead.", "QUESTION  Numeric workflow question id (required)", "ttasks answer 6 --assignment 21 --answer 'Reject empty owners'" + workflowRevisions},
	"observe_subscribe": {"Subscribe an assignment to allowed observations", "observe subscribe ASSIGNMENT PATTERN [flags]",
		"Agent-only: subscribes through the packet's channel policy. Matching events become durable observations; late events are recorded only. Both revisions and --idempotency-key are required.", "ASSIGNMENT  Leased workflow assignment id (required)\nPATTERN  Channel pattern allowed by the packet (required)", "ttasks observe subscribe 17 logs:dev --reaction wake_current" + workflowRevisions},
	"observe_list": {"List an assignment's observation subscriptions", "observe list ASSIGNMENT",
		"Agent-only: reads the assignment's subscriptions without creating or cancelling them.", "ASSIGNMENT  Workflow assignment id (required)", "ttasks observe list 17"},
	"observe_cancel": {"Cancel an assignment's observation subscription", "observe cancel ASSIGNMENT SUBSCRIPTION [flags]",
		"Agent-only: stops future matching deliveries for this subscription. Both revisions and --idempotency-key are required.", "ASSIGNMENT  Leased workflow assignment id (required)\nSUBSCRIPTION  Numeric subscription id (required)", "ttasks observe cancel 17 4" + workflowRevisions},
}

var helpGroups = map[string]string{
	"work":              "Claim, inspect, complete, and release workflow assignments (agent-only)",
	"artifacts":         "Write and inspect immutable workflow outputs (agent-only)",
	"observe":           "Manage assignment-scoped event subscriptions (agent-only)",
	"queue":             "Manage task queues, owners, and workflow bindings (operator-only)",
	"queue.workflow":    "Activate and inspect a queue's published workflow (operator-only)",
	"queue.pool":        "Bind existing agents to logical workflow pools (operator-only)",
	"queue.trigger":     "Manage external events that create workflow tasks (operator-only)",
	"workflows":         "Create drafts, validate, and publish workflow versions (operator-only)",
	"workflow":          "Inspect a task's workflow execution and history (operator-only)",
	"workflow.artifact": "Inspect individual workflow artifacts (operator-only)",
	"workflow.question": "Inspect individual workflow questions (operator-only)",
	"notifications":     "Read and dismiss customer task notifications (operator-only)",
}

var helpFlags = map[string]string{
	"queue":               "Queue prefix, for example DEV",
	"status":              "Task status: open, in_progress, wait_customer, done, or cancelled",
	"assignee":            "Agent name to assign or filter by",
	"text":                "Search text in tasks",
	"waiting-for":         "Filter pending answers by user:login or agent:name",
	"limit":               "Maximum number of results (operator ready: default 50, maximum 200)",
	"claim":               "Atomically claim eligible flexible work (agent-only; boolean, default false)",
	"parent":              "Parent task key; create inherits its queue",
	"title":               "Task title (required for create)",
	"description":         "Task description as Markdown",
	"pull-request":        "Absolute http(s) pull request URL; an empty update clears it",
	"group":               "Task group; forbidden when filing into a queue the agent does not own",
	"priority":            "P0 Critical, P1 High, P2 Normal (create default), P3 Low",
	"manual-block-reason": "Manual blocker text; an empty update clears it",
	"revision":            "Expected task revision (block: blocker revision); operator mode fetches it if omitted",
	"body":                "Markdown comment text, instead of positional TEXT",
	"question":            "Workflow question text; selects workflow ask (required for that form)",
	"context":             "Background needed to answer (required for workflow ask)",
	"blocking-scope":      "none, assignment, or requirement (required for workflow ask; must be allowed by packet)",
	"anchor":              "Optional reference identifying the subject of a workflow question",
	"suggested-answer":    "Suggested workflow answer",
	"options":             "Comma-separated workflow answer choices",
	"artifacts":           "Comma-separated numeric artifact ids attached to the question",
	"task-revision":       "Current workflow task revision from the work packet; required for workflow mutations",
	"assignment-revision": "Current assignment revision from the work packet; required for workflow mutations",
	"idempotency-key":     "Stable key reused for retries of the same intent; required for workflow claims and mutations",
	"before":              "Sibling task key to insert before, in the same priority bucket",
	"to-root":             "Detach from parent; excludes --parent and --before (boolean, default false)",
	"by":                  "Blocking task key (required)",
	"complete-anyway":     "Complete despite active descendants (boolean, default false)",
	"outcome":             "Outcome declared by the work packet (required)",
	"name":                "Artifact name declared by the work packet (required)",
	"type":                "Artifact type: markdown, json, file, commit, or url (required)",
	"content":             "Type-specific content: Markdown (may be empty), JSON object, clean relative file path, commit JSON with repository/ref strings, or absolute http(s) URL without credentials",
	"metadata":            "Artifact metadata as a JSON object",
	"task":                "Task key containing the artifact (required)",
	"assignment":          "Leased question/manager assignment id (required)",
	"answer":              "Workflow answer text (required)",
	"correlation-key":     "Restrict observations to this exact correlation key",
	"reaction":            "Policy-allowed reaction: record_only (default), wake_current, hold_assignment, or create_requirement",
}

const globalHelp = `Global flags:
  --help, -h   Show local help for a command or group; never execute it
  --help-json  Print the complete command tree as JSON (root only)
  --json       Print command results as JSON
  --version    Print the client build version (root only)`

// Help is resolved before parsing or socket selection in both modes.
func runTextHelp(args []string, stdout, stderr io.Writer) (int, bool) {
	explicit := len(args) == 0 || slices.Contains(args, "--help") || slices.Contains(args, "-h")
	if len(args) > 0 && args[0] == "help" {
		explicit, args = true, args[1:]
	}
	path := strings.Join(args, ".")
	if !explicit {
		if _, group := helpGroups[path]; !group {
			return 0, false
		}
	}
	reg, err := taskOperatorRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1, true
	}
	entries := taskHelpEntries(reg)
	for n := 0; n <= len(args); n++ {
		path = strings.Join(args[:n], ".")
		if entry, ok := entries[path]; ok {
			printTaskCommandHelp(entry, stdout)
			return 0, true
		}
		if n == len(args) || args[n] == "--help" || args[n] == "-h" || args[n] == "help" {
			if path == "" || helpGroups[path] != "" {
				printTaskGroupHelp(path, entries, stdout)
				return 0, true
			}
			break
		}
	}
	fmt.Fprintf(stderr, "tasks: unknown help command %q\n", strings.Join(args, " "))
	return 2, true
}

func taskHelpEntries(reg *registry.Registry) map[string]commandHelp {
	entries := map[string]commandHelp{}
	for action, entry := range sharedHelp {
		var names []string
		for name := range taskCommandFlags()[action] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			value := " VALUE"
			if boolFlags[name] {
				value = ""
			}
			entry.arguments += fmt.Sprintf("\n--%s%s  %s", name, value, helpFlags[name])
		}
		entries[strings.Join(sharedHelpPath(action), ".")] = entry
	}
	for _, command := range reg.Commands() {
		entry := commandHelp{summary: command.Summary + " (operator-only)", usage: strings.ReplaceAll(command.Path, ".", " "), help: command.Help}
		entry.help += operatorHelp[command.Path] + "\nOperator-only: uses the host daemon as the customer actor. Agent mode cannot execute this command. Arguments may be given with their named flags; required values are shown below."
		for _, arg := range command.Args {
			name := arg.Flag
			if name == "" {
				name = arg.Name
			}
			value, required := " VALUE", ""
			if arg.Type == registry.Bool {
				value = ""
			}
			if arg.Required {
				entry.usage += " --" + name + value
				required = " (required)"
			}
			entry.arguments += fmt.Sprintf("\n--%s%s  %s%s", name, value, arg.Help, required)
		}
		entry.usage += " [flags]"
		entry.examples = "ttasks " + operatorExamples[command.Path]
		entries[command.Path] = entry
	}
	return entries
}

func printTaskCommandHelp(entry commandHelp, out io.Writer) {
	fmt.Fprintf(out, "Usage: ttasks %s\n\n%s\n\n%s\n", entry.usage, entry.summary, strings.TrimSpace(entry.help))
	if strings.TrimSpace(entry.arguments) != "" {
		fmt.Fprintf(out, "\nArguments and flags:\n%s\n", strings.TrimSpace(entry.arguments))
	}
	fmt.Fprintf(out, "\nExamples:\n%s\n\n%s\n", entry.examples, globalHelp)
}

func printTaskGroupHelp(path string, entries map[string]commandHelp, out io.Writer) {
	display := strings.ReplaceAll(path, ".", " ")
	if path == "" {
		fmt.Fprintln(out, "Usage: ttasks <command> [args] [flags]\n\nNative Tasks client (tariboy-tasks; ttasks is its alias).\nWith non-empty TARIBOY_TOOLS_SOCKET: identity-bound agent mode, no operator fallback.\nOtherwise: operator mode on the host Unix daemon socket.\nHelp is local and lists both modes; execution still enforces their permissions.")
	} else {
		fmt.Fprintf(out, "Usage: ttasks %s <command> [args] [flags]\n\n%s\n", display, helpGroups[path])
	}
	children := map[string]string{}
	prefix := path
	if prefix != "" {
		prefix += "."
	}
	add := func(key, summary string) {
		if !strings.HasPrefix(key, prefix) {
			return
		}
		name := strings.TrimPrefix(key, prefix)
		if name != "" && !strings.Contains(name, ".") {
			children[name] = summary
		}
	}
	for key, summary := range helpGroups {
		add(key, summary)
	}
	for key, entry := range entries {
		add(key, entry.summary)
	}
	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintln(out, "\nCommands:")
	for _, name := range names {
		fmt.Fprintf(out, "  %-18s %s\n", name, children[name])
	}
	fmt.Fprintf(out, "\nExamples:\n  ttasks %s --help\n\n%s\n", strings.TrimSpace(display+" "+names[0]), globalHelp)
}

var operatorExamples = map[string]string{
	"queue.list":             "queue list",
	"queue.create":           "queue create --prefix DEV --name Development --owners worker",
	"queue.get":              "queue get DEV",
	"queue.update":           "queue update DEV --name Development --revision 2",
	"queue.workflow.set":     "queue workflow set DEV --workflow-version-id 12 --revision 0 --idempotency-key bind-1",
	"queue.workflow.get":     "queue workflow get DEV",
	"queue.pool.set":         "queue pool set DEV developers --agents worker,reviewer --revision 0 --idempotency-key pool-1",
	"queue.pool.list":        "queue pool list DEV",
	"queue.pool.get":         "queue pool get DEV developers",
	"queue.trigger.list":     "queue trigger list DEV",
	"queue.trigger.create":   "queue trigger create DEV --pattern external:incidents --action create_task",
	"queue.trigger.delete":   "queue trigger delete DEV 4",
	"workflows.create":       `workflows create --definition '{"name":"single-change","version":1,"initial_status":"implement","statuses":[{"id":"implement","requirements":[{"id":"code","pool":"developers","dispatch":"claim_one","outcomes":["completed"]}],"transitions":[{"when":"code.completed","to":"done"}]},{"id":"done","terminal":true,"requirements":[],"transitions":[]}]}'`,
	"workflows.versions":     "workflows versions development",
	"workflows.get":          "workflows get development 2",
	"workflows.validate":     "workflows validate development 2",
	"workflows.publish":      "workflows publish development 2",
	"workflow.get":           "workflow get DEV-12",
	"workflow.packets":       "workflow packets DEV-12",
	"workflow.assignments":   "workflow assignments DEV-12",
	"workflow.artifacts":     "workflow artifacts DEV-12 --assignment-id 17",
	"workflow.artifact.get":  "workflow artifact get DEV-12 9 --assignment-id 17",
	"workflow.questions":     "workflow questions DEV-12",
	"workflow.question.get":  "workflow question get DEV-12 6",
	"workflow.subscriptions": "workflow subscriptions DEV-12 --assignment-id 17",
	"workflow.events":        "workflow events DEV-12 --after 42 --limit 20",
	"events":                 "events DEV-12 --after 42 --limit 20",
	"principals":             "principals",
	"notifications.list":     "notifications list --include-dismissed",
	"notifications.read":     "notifications read 1",
	"notifications.dismiss":  "notifications dismiss 1",
}

var operatorHelp = map[string]string{
	"queue.list":             "Lists host queues, including their names, owners, and responsible agents.",
	"queue.create":           "Creates a queue namespace for keys such as DEV-1. Owners are comma-separated existing agent names; the responsible agent handles triage.",
	"queue.get":              "Reads the queue's configuration and current revision before an update.",
	"queue.update":           "Changes only supplied queue fields. Pass the current revision from queue get; stale writes are rejected.",
	"queue.workflow.set":     "Activates a published workflow version for future tasks. Existing tasks keep their pinned version. Referenced pools must be non-empty; use revision 0 for the first binding.",
	"queue.workflow.get":     "Reads the active workflow binding and its revision for this queue.",
	"queue.pool.set":         "Replaces pool membership with comma-separated existing agent names. Use revision 0 for the first binding; an active referenced pool cannot be emptied. Future executions use the new membership.",
	"queue.pool.list":        "Lists the queue's logical workflow pools and explicit memberships.",
	"queue.pool.get":         "Reads a pool's membership and current revision before rebinding it.",
	"queue.trigger.list":     "Lists external channel triggers configured to create work in this queue.",
	"queue.trigger.create":   "Creates a trigger for future plugin-produced events. --action must be create_task; internal agent/group/user/system namespaces are rejected. --correlation-key restricts matching to an exact key.",
	"queue.trigger.delete":   "Removes a trigger by numeric id. Existing tasks created by that trigger remain.",
	"workflows.create":       "Creates or replaces an unpublished draft using a complete JSON workflow definition. Published name/version pairs are immutable; use a higher version to change them.",
	"workflows.versions":     "Lists draft and published versions of the named workflow.",
	"workflows.get":          "Reads one named integer workflow version and its definition.",
	"workflows.validate":     "Returns validation findings with codes, paths, and messages. Inspect the valid field and findings before publishing.",
	"workflows.publish":      "Validates the draft and makes this version immutable. Activate it separately with queue workflow set.",
	"workflow.get":           "Reads the task's full workflow execution state, including its pinned version and current status.",
	"workflow.packets":       "Reads operator-visible work packet history for the task.",
	"workflow.assignments":   "Lists task assignment history, owners, leases, and attempts.",
	"workflow.artifacts":     "Lists immutable task artifacts; --assignment-id narrows the result to one assignment.",
	"workflow.artifact.get":  "Reads a task artifact by numeric id, optionally scoped to an assignment.",
	"workflow.questions":     "Lists workflow questions and answers for a task, optionally filtered by assignment.",
	"workflow.question.get":  "Reads one workflow question by numeric id within the task.",
	"workflow.subscriptions": "Lists observation subscriptions for the required assignment within this task.",
	"workflow.events":        "Replays workflow.* events after a sequence. Observation events remain in ordinary task events; use events for the complete history.",
	"events":                 "Reads durable task events after the supplied sequence, bounded by --limit. Use the last sequence to resume.",
	"principals":             "Lists principals available for task assignment and typed user:login or agent:name mentions.",
	"notifications.list":     "Lists customer task notifications. Dismissed entries are excluded unless --include-dismissed is supplied.",
	"notifications.read":     "Marks one customer notification read without changing the task or answering its question.",
	"notifications.dismiss":  "Dismisses one customer notification; it remains accessible with notifications list --include-dismissed.",
}
