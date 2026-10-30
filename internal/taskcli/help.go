package taskcli

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

type commandHelp struct {
	summary, usage, help, arguments, examples string
}

var sharedHelp = map[string]commandHelp{
	"list": {"List and search tasks visible to the current actor", "list [flags]",
		"Agent mode lists the tasks the agent can see; operator mode lists every host task. Filters combine with AND; a comma-separated value matches any of its items. " +
			"--status matches the status show prints: the workflow status of a workflow task, the stored status of a flexible task; a category value also finds workflow tasks in that category. --category matches only the category. " +
			"--text searches the key, title, and description, ignoring case. --parent lists direct children and --subtree every descendant; they cannot be combined, and a task you cannot see gives an empty list. " +
			"--view defaults to active (open, in_progress, wait_customer) unless --status is given; closed is done and cancelled. " +
			"--sort tree keeps queue tree order; updated and created show the newest first; priority shows P0 first, then the newest. " +
			"A page holds --limit tasks (default 50, maximum 500). When more remain, a last line next: prints the command for the next page. --json prints the page with tasks and next_cursor.",
		"", "ttasks list --status plan,review\nttasks list --text 'login bug' --sort updated --limit 20\nttasks list --subtree DEV-12 --view all\nttasks list --waiting-for me"},
	"mine": {"List tasks visible to the current actor (alias of list)", "mine [flags]",
		"mine is an alias of list: the same flags, filters, and output. See ttasks list --help.", "", "ttasks mine --queue DEV --status in_progress\nttasks mine --waiting-for agent:worker"},
	"ready": {"List or claim eligible flexible tasks", "ready [flags]",
		"Ready tasks are open, unassigned, and unblocked. Agent mode may atomically claim work with --claim. Operator mode only lists it; its default limit is 50 (maximum 200).", "", "ttasks ready --queue DEV\nttasks ready --claim --idempotency-key claim-1"},
	"show": {"Inspect a task and its conversation", "show KEY",
		"Read task fields, comments, pending answers, relations, and descendant counts, subject to task visibility.", "KEY  Task key, for example DEV-12 (required)", "ttasks show DEV-12"},
	"create": {"Create a root task or child task", "create --title TEXT (--queue QUEUE | --parent KEY) [flags]",
		"Use --queue for a root or --parent for a child. In an agent's non-owned queue, omitting --assignee files an unassigned task: the filed response means it is recorded but no longer visible. An explicit assignee retains task ownership without exposing the queue; --group is forbidden there.", "", "ttasks create --queue DEV --title 'Investigate failure' --assignee worker --priority P1\nttasks create --parent DEV-12 --title 'Add regression test'"},
	"update": {"Change task fields", "update KEY [flags]",
		"Only supplied fields are updated. Empty --pull-request or --manual-block-reason clears that field. Statuses are open, in_progress, wait_customer, done, and cancelled.", "KEY  Task key (required)", "ttasks update DEV-12 --priority P0\nttasks update DEV-12 --pull-request=''"},
	"assign": {"Assign a flexible task to an agent", "assign KEY ASSIGNEE [flags]",
		"Changes task ownership without granting access to the rest of its queue.", "KEY  Task key (required)\nASSIGNEE  Existing agent name (required)", "ttasks assign DEV-12 worker"},
	"comment": {"Add a durable Markdown task comment", "comment KEY TEXT... [flags]\n       ttasks comment KEY --body TEXT [flags]\n       ttasks comment KEY --file PATH [flags]\n       ttasks comment KEY < FILE",
		"Pass comment text positionally, with --body, or with --file PATH, only one of them; without any, the text is read from stdin. Text from a file or stdin keeps quotes, backticks and newlines as written, so no shell quoting is needed; it is trimmed like --body and may be at most 64 KiB. Typed @agent:name and @user:login mentions request an answer; the mentioned principal's next comment resolves its own pending wait.", "KEY  Task key (required)\nTEXT  Markdown comment text; --body, --file or stdin when absent", "ttasks comment DEV-12 'Found the boundary'\nttasks comment DEV-12 --body 'Tests passed'\nttasks comment DEV-12 --file note.md\nttasks comment DEV-12 <<'EOF'\nIt's `fine`\nEOF"},
	"ask": {"Request a durable answer on a task", "ask KEY PRINCIPAL TEXT... [--idempotency-key KEY]\n       ttasks ask KEY PRINCIPAL --file PATH [--idempotency-key KEY]\n       ttasks ask KEY PRINCIPAL < FILE",
		"Use a task key and user:login or agent:name (a bare name means agent:name). It creates a principal wait; asking the customer moves assigned work to wait_customer. Pass the question as words or with --file PATH, not both; without either, it is read from stdin, at most 64 KiB, with quotes, backticks and newlines kept as written.",
		"KEY  Task key (required)\nPRINCIPAL  user:login or agent:name to answer (required)\nTEXT  Question text; --file or stdin when absent",
		"ttasks ask DEV-12 user:login 'Which behavior should win?'\nttasks ask DEV-12 user:login --file question.md"},
	"move": {"Reparent, reorder, or detach a task", "move KEY (--parent KEY | --before KEY | --to-root) [flags]",
		"Moves within the same queue. --before reorders within a priority bucket. --to-root detaches and cannot be combined with --parent or --before; detaching may remove the agent's inherited access.", "KEY  Task to move (required)", "ttasks move DEV-12 --parent DEV-3\nttasks move DEV-12 --to-root"},
	"block": {"Make one task depend on another", "block KEY --by BLOCKER [flags]",
		"Creates a directional blocks relation: BLOCKER must finish before KEY is ready. Cycles are rejected; write access to both tasks is required. --revision refers to the blocker task.", "KEY  Task being blocked (required)", "ttasks block DEV-12 --by DEV-2"},
	"relate": {"Link two related tasks", "relate KEY TARGET [flags]",
		"Adds a symmetric related relation without blocking either task. Requires write access to both endpoints.", "KEY  Source task key (required)\nTARGET  Related task key (required)", "ttasks relate DEV-12 DEV-9"},
	"done": {"Complete a flexible task", "done KEY [flags]",
		"Marks work done. Active descendants cause a conflict unless --complete-anyway is explicit.", "KEY  Task key (required)", "ttasks done DEV-12"},
	"advance": {"Declare an outcome for a workflow task", "advance KEY --outcome NAME [--from STATUS] [--message TEXT]",
		"Leaves the task's current workflow status by the named outcome. An agent may advance a task it holds in a pool status; the customer may advance a task in a customer status. Required artifacts must be set first. --from names the status you believe the task is in; when the task has moved on the advance is refused with status_changed, so a retry cannot apply twice. A transition with checks is verified before it applies: advance waits for the checks (up to their timeouts plus 30 seconds) and then prints the result. A rejection, a check that could not run, a cancelled request, or a wait that ran out exits non-zero; the output names the exact next command. --no-wait prints the pending request at once and exits 0. Prints the request state; a refusal exits non-zero.", "KEY  Task key (required)", "ttasks advance DEV-12 --outcome ready --from develop --message 'PR opened'\nttasks advance DEV-12 --outcome ready --no-wait"},
	"artifact_set": {"Set a workflow task artifact", "artifacts set KEY NAME [VALUE | --file PATH]",
		"Stores a new value for a declared artifact. Without VALUE the value is read from stdin; --file reads it from a file. The value is stored as given, trailing newline included, up to 64 KiB.", "KEY  Task key (required)\nNAME  Declared artifact name (required)\nVALUE  Artifact text; stdin when absent", "ttasks artifacts set DEV-12 plan 'step 1'\nttasks artifacts set DEV-12 summary --file summary.md"},
	"artifact_ls": {"List the current artifacts of a workflow task", "artifacts ls KEY",
		"Lists the current value of every artifact set on the task.", "KEY  Task key (required)", "ttasks artifacts ls DEV-12"},
	"artifact_show": {"Show a workflow task artifact and its history", "artifacts show KEY NAME",
		"Prints the current value of one artifact and every earlier value, newest first.", "KEY  Task key (required)\nNAME  Artifact name (required)", "ttasks artifacts show DEV-12 plan"},
	"workflow_get": {"Show where a workflow task is and what it may do next", "workflow get KEY",
		"Prints the workflow, status, category, owner, holder, the outcomes with their missing artifacts and checks, the current artifacts, and the last transition request. --json prints the whole view.", "KEY  Task key (required)", "ttasks workflow get DEV-12"},
	"workflow_runs": {"List the script runs of a workflow task", "workflow runs KEY",
		"Lists the checks and watch scripts the daemon ran for the task, newest first: id, kind, script, state, verdict, exit code, start time, and duration. Read a run's log with workflow log.", "KEY  Task key (required)", "ttasks workflow runs DEV-12"},
	"workflow_run_log": {"Read the log of a workflow script run", "workflow log KEY RUN [--max-bytes N]",
		"Prints the last bytes of the run's log, 64 KiB by default and at most 1 MiB. Queue secret values are replaced with [redacted]. When the log is longer, a first line on stderr says so.", "KEY  Task key (required)\nRUN  Run id, as listed by workflow runs or named in a failed advance (required)", "ttasks workflow log DEV-12 4\nttasks workflow log DEV-12 4 --max-bytes 4096"},
	"workflow_move": {"Move a workflow task to another status (operator-only)", "workflow move KEY --to STATUS [--reason TEXT]",
		"Moves the task to the named status of its workflow regardless of outcomes, and records the reason, if any. Operator-only: uses the host daemon as the customer actor.", "KEY  Task key (required)", "ttasks workflow move DEV-12 --to develop --reason 'review found a regression'"},
	"workflow_upgrade": {"Restart a workflow task at the version its queue is bound to (operator-only)", "workflow upgrade KEY [--reason TEXT]",
		"A task stays on the workflow version it was created with. Upgrade pins it to the version its queue is bound to now and starts it again at that version's initial status: the current status ends, a pending request is cancelled, a pause is cleared, and the status's scripts stop. Artifacts and pool holders stay. Refused when the task is closed, already on the queue's version, or the queue has no workflow, and when the new version needs a pool or secret the queue lacks. Operator-only: uses the host daemon as the customer actor.",
		"KEY  Task key (required)", "ttasks workflow upgrade DEV-12\nttasks workflow upgrade DEV-12 --reason 'adopt the new review step'"},
	"workflow_resume": {"Resolve the pause of a workflow task (operator-only)", "workflow resume KEY --decision continue|release",
		"A paused task waits for the customer's decision; a plain comment does not resume it. --decision continue resumes the task in its current status with the status counters reset; in a status owned by an agent pool the holder stays. --decision release, only in a status owned by an agent pool, takes the task from its holder and dispatches it to another member; with no other eligible member the task waits unassigned. To stop the task instead, run ttasks cancel KEY. Operator-only: uses the host daemon as the customer actor.",
		"KEY  Task key (required)", "ttasks workflow resume DEV-12 --decision continue\nttasks workflow resume DEV-12 --decision release"},
	"workflow_watch": {"Run the watch script of a workflow task now (operator-only)", "workflow watch KEY",
		"A status owned by a script runs its watch script on a schedule: again after every once a run ends. Watch starts a run of that script at once instead of at the next scheduled time; the run then ends like a scheduled one and sets the next time. The run waits for the workflow's watch_spacing and the limit of parallel scripts like any other. Refused with workflow_no_watch when the current status has no watch script, script_running while a run of the task is pending or running, and on a closed or paused task. Operator-only: uses the host daemon as the customer actor.",
		"KEY  Task key (required)", "ttasks workflow watch DEV-12"},
	"workflow_reassign": {"Hand the pool status of a workflow task to another member (operator-only)", "workflow reassign KEY --agent NAME",
		"Changes the agent of a task in a status owned by an agent pool. NAME must be a member of that pool that can work: enabled, not halted, and not on the bare image. The task goes to NAME at once and NAME becomes the sticky holder of the pool: when the task comes back to a status of that pool, NAME gets it again. The previous holder loses the task, and its open questions to the customer are resolved. The idle and rejected counters of the status are reset. Artifacts stay. Refused in a customer, script, or terminal status, on a paused task (use workflow resume), and while a transition request runs. Operator-only: uses the host daemon as the customer actor.",
		"KEY  Task key (required)", "ttasks workflow reassign DEV-12 --agent dev-2"},
	"workflow_retry": {"Run the checks of a failed transition request again (operator-only)", "workflow retry KEY [REQUEST]",
		"Repeats a transition request whose check could not run (a script error or an expired token, for example) as a new request with the same outcome, message and requester, and waits for its checks like advance. Without REQUEST it retries the task's latest request. Only the latest request of the current status can be retried, and only when it failed: a rejection is the holder's to fix, and a paused task must be resumed first. Operator-only: uses the host daemon as the customer actor.",
		"KEY  Task key (required)\nREQUEST  Transition request id, as shown by workflow get (default: the latest)", "ttasks workflow retry DEV-12\nttasks workflow retry DEV-12 41"},
	"cancel": {"Cancel a workflow task (operator-only)", "cancel KEY",
		"Cancels a task that follows a workflow. Operator-only: uses the host daemon as the customer actor.", "KEY  Task key (required)", "ttasks cancel DEV-12"},
	"requeue": {"Move a task to another queue (operator-only)", "requeue KEY --queue QUEUE",
		"A task key belongs to its queue, so the move creates a new task in QUEUE with the same title, description, priority, pull request, group, and comments, and prints it. The new task enters the workflow of QUEUE, if it has one, like any new task there. The old task is cancelled: its workflow stops, its running scripts are stopped, and its questions are resolved. The two tasks are related and each gets a comment that names the other. A task with open subtasks is refused with requeue_has_children. Operator-only: uses the host daemon as the customer actor.", "KEY  Task key (required)", "ttasks requeue DEV-12 --queue OPS"},
}

var helpGroups = map[string]string{
	"queue":          "Manage task queues, owners, and agent pools (operator-only)",
	"queue.pool":     "Bind existing agents to named queue pools, or delete a pool (operator-only)",
	"queue.trigger":  "Manage external events that create tasks (operator-only)",
	"queue.workflow": "Bind a workflow image to a queue (operator-only)",
	"queue.secret":   "Store secrets that workflow scripts of a queue receive (operator-only)",
	"queue.env":      "Store plain env vars that workflow scripts of a queue receive (operator-only)",
	"queue.source":   "Inspect the workflow sources that create tasks in a queue (operator-only)",
	"artifacts":      "Read and set workflow task artifacts",
	"workflow":       "Inspect, move, and resume workflow tasks",
}

var helpFlags = map[string]string{
	"queue":               "Queue prefix, for example DEV",
	"status":              "Task status: open, in_progress, wait_customer, done, or cancelled; list also takes a workflow status and comma-separated values",
	"category":            "Category: open, in_progress, wait_customer, done, or cancelled; comma-separated values",
	"assignee":            "Agent name to assign or filter by",
	"author":              "Author principal, user:login or agent:name (a bare name means agent:name)",
	"customer":            "Customer principal, user:login or agent:name (a bare name means agent:name)",
	"subtree":             "Task key whose descendants to list; excludes --parent",
	"blocked":             "true lists only blocked tasks, false only unblocked ones",
	"view":                "active (default), closed (done and cancelled), or all",
	"sort":                "tree (default), updated, created, or priority",
	"after":               "Task key to continue after, from the next: line or next_cursor",
	"text":                "Search text in the key, title, and description, ignoring case",
	"waiting-for":         "Filter pending answers by user:login or agent:name; me is the current agent",
	"limit":               "Maximum number of results (list: default 50, maximum 500; operator ready: default 50, maximum 200)",
	"claim":               "Atomically claim eligible flexible work (agent-only; boolean, default false)",
	"parent":              "Parent task key; create inherits its queue, list shows its direct children",
	"title":               "Task title (required for create)",
	"description":         "Task description as Markdown",
	"pull-request":        "Absolute http(s) pull request URL; an empty update clears it",
	"group":               "Task group; forbidden when filing into a queue the agent does not own",
	"priority":            "P0 Critical, P1 High, P2 Normal (create default), P3 Low; list takes comma-separated values",
	"manual-block-reason": "Manual blocker text; an empty update clears it",
	"revision":            "Expected task revision (block: blocker revision); operator mode fetches it if omitted",
	"body":                "Markdown comment text, instead of positional TEXT",
	"idempotency-key":     "Stable key reused for retries of the same intent",
	"before":              "Sibling task key to insert before, in the same priority bucket",
	"to-root":             "Detach from parent; excludes --parent and --before (boolean, default false)",
	"by":                  "Blocking task key (required)",
	"complete-anyway":     "Complete despite active descendants (boolean, default false)",
	"outcome":             "Outcome to declare, as listed by workflow get (required)",
	"from":                "Status you believe the task is in; refused with status_changed when it has moved on",
	"message":             "Message recorded with the transition",
	"no-wait":             "Print a pending request at once instead of waiting for its checks (boolean, default false)",
	"max-bytes":           "Largest log tail to read in bytes (default 65536, maximum 1048576)",
	"file":                "Read the text (artifact value, comment or question) from this file instead of stdin",
	"to":                  "Target workflow status id (required)",
	"reason":              "Why the task is moved",
	"decision":            "How to resolve the pause: continue or release (required)",
	"agent":               "Pool member that takes over the current status (required)",
}

const globalHelp = `Global flags:
  --help, -h   Show local help for a command or group; never execute it
  --help-json  Print the complete command tree as JSON (root only)
  --json       Print command results as JSON
  --version    Print the client build version (root only)`

// Help is resolved before parsing or socket selection in both modes.
func runTextHelp(args []string, prog string, stdout, stderr io.Writer) (int, bool) {
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
			printTaskCommandHelp(entry, prog, stdout)
			return 0, true
		}
		if n == len(args) || args[n] == "--help" || args[n] == "-h" || args[n] == "help" {
			if path == "" || helpGroups[path] != "" {
				printTaskGroupHelp(path, prog, entries, stdout)
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
		spec, _ := tasks.SpecFor(action)
		flags := commandFlags(spec)
		var names []string
		for name := range flags {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			value := " VALUE"
			if flags[name] {
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

func printTaskCommandHelp(entry commandHelp, prog string, out io.Writer) {
	var text strings.Builder
	fmt.Fprintf(&text, "Usage: ttasks %s\n\n%s\n\n%s\n", entry.usage, entry.summary, strings.TrimSpace(entry.help))
	if strings.TrimSpace(entry.arguments) != "" {
		fmt.Fprintf(&text, "\nArguments and flags:\n%s\n", strings.TrimSpace(entry.arguments))
	}
	fmt.Fprintf(&text, "\nExamples:\n%s\n\n%s\n", entry.examples, globalHelp)
	// The help texts name ttasks in usage lines, examples, and cross-references.
	fmt.Fprint(out, strings.ReplaceAll(text.String(), "ttasks ", prog+" "))
}

func printTaskGroupHelp(path, prog string, entries map[string]commandHelp, out io.Writer) {
	display := strings.ReplaceAll(path, ".", " ")
	if path == "" && prog != "ttasks" {
		fmt.Fprintf(out, "Usage: %s <command> [args] [flags]\n\nNative Tasks client: the commands, flags, and output of ttasks.\nAlways operator mode on the host Unix daemon socket, also for an agent:\nthe host daemon acts as the customer and every task is visible.\n", prog)
	} else if path == "" {
		fmt.Fprintln(out, "Usage: ttasks <command> [args] [flags]\n\nNative Tasks client (tariboy-tasks; ttasks is its alias).\nWith non-empty TARIBOY_TOOLS_SOCKET: identity-bound agent mode, no operator fallback.\nOtherwise: operator mode on the host Unix daemon socket.\nHelp is local and lists both modes; execution still enforces their permissions.")
	} else {
		fmt.Fprintf(out, "Usage: %s %s <command> [args] [flags]\n\n%s\n", prog, display, helpGroups[path])
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
	fmt.Fprintf(out, "\nExamples:\n  %s %s --help\n\n%s\n", prog, strings.TrimSpace(display+" "+names[0]), globalHelp)
}

var operatorExamples = map[string]string{
	"queue.list":           "queue list",
	"queue.create":         "queue create --prefix DEV --name Development --owners worker",
	"queue.get":            "queue get DEV",
	"queue.update":         "queue update DEV --name Development --revision 2",
	"queue.pool.set":       "queue pool set DEV developers --agents worker,reviewer --revision 0 --idempotency-key pool-1",
	"queue.pool.delete":    "queue pool delete DEV developers --revision 3 --idempotency-key pool-del-1",
	"queue.pool.list":      "queue pool list DEV",
	"queue.pool.get":       "queue pool get DEV developers",
	"queue.trigger.list":   "queue trigger list DEV",
	"queue.trigger.create": "queue trigger create DEV --pattern plugin:telegram --action create_task",
	"queue.trigger.delete": "queue trigger delete DEV 4",
	"queue.workflow.set":   "queue workflow set DEV development:latest --revision 0",
	"queue.workflow.get":   "queue workflow get DEV",
	"queue.workflow.clear": "queue workflow clear DEV --revision 2",
	"queue.secret.set":     "queue secret set DEV GH_TOKEN --value 'token'\nprintf '%s' \"$TOKEN\" | ttasks queue secret set DEV GH_TOKEN",
	"queue.secret.ls":      "queue secret ls DEV",
	"queue.secret.rm":      "queue secret rm DEV GH_TOKEN",
	"queue.env.set":        "queue env set DEV LOG_LEVEL debug\nqueue env set DEV BASE_URL --value 'https://example.test'\nqueue env set DEV CUSTOMER_AGENT_POOL approvers",
	"queue.env.ls":         "queue env ls DEV",
	"queue.env.rm":         "queue env rm DEV LOG_LEVEL",
	"queue.source.ls":      "queue source ls DEV",
	"queue.source.log":     "queue source log DEV 12\nqueue source log DEV 12 --max-bytes 4096",
	"events":               "events DEV-12 --after 42 --limit 20",
	"principals":           "principals",
	"activity":             "activity --since 2026-10-01T00:00:00Z --agent worker",
}

var operatorHelp = map[string]string{
	"queue.list":           "Lists host queues, including their names, owners, and responsible agents.",
	"queue.create":         "Creates a queue namespace for keys such as DEV-1. Owners are comma-separated existing agent names; the responsible agent handles triage.",
	"queue.get":            "Reads the queue's configuration and current revision before an update.",
	"queue.update":         "Changes only supplied queue fields. Pass the current revision from queue get; stale writes are rejected.",
	"queue.pool.set":       "Replaces pool membership with comma-separated existing agent names. Use revision 0 for a new pool.",
	"queue.pool.delete":    "Deletes a pool and its memberships; the agents stay. Refused with workflow_pool_empty while the queue's workflow, an unfinished task's workflow version, CUSTOMER_AGENT_POOL, or an open status visit uses the pool.",
	"queue.pool.list":      "Lists the queue's pools and explicit memberships.",
	"queue.pool.get":       "Reads a pool's membership and current revision before rebinding it.",
	"queue.trigger.list":   "Lists external channel triggers configured to create work in this queue.",
	"queue.trigger.create": "Creates a trigger for future plugin-produced events. --action must be create_task; internal agent/group/user/system namespaces are rejected. --correlation-key restricts matching to an exact key.",
	"queue.trigger.delete": "Removes a trigger by numeric id. Existing tasks created by that trigger remain.",
	"queue.workflow.set":   "Binds a published workflow image, name:tag, to the queue. A binding by a tag such as latest follows it: new tasks get the version the tag names. A binding by digest is pinned. Use revision 0 (the default) for a new binding, or the current revision from queue workflow get to rebind.",
	"queue.workflow.get":   "Shows the image a queue is bound to, the tag it follows, and the binding revision; fails with queue_workflow_not_found when the queue is unbound.",
	"queue.workflow.clear": "Unbinds the queue. Pass the current revision from queue workflow get. Tasks already running keep their workflow.",
	"queue.secret.set":     "Stores a secret that the scripts of the queue's workflow receive in their environment, replacing an earlier value. The value is read from --value, or from stdin when --value is absent; prefer stdin, since an argument shows in the process list and shell history. One trailing newline of stdin is stripped (unlike artifacts, which keep it), because a token piped in almost always carries one. A key is a name such as GH_TOKEN: letters, digits, and underscores, not starting with a digit or TARIBOY_. A value is non-empty UTF-8 text of at most 64 KiB. The value is never shown again.",
	"queue.secret.ls":      "Lists the keys of the queue's secrets and when each was set, and in global_env the secrets the bound workflow requires that the global env (the daemon environment after global-agent-shell.sh) holds. Values are never shown.",
	"queue.secret.rm":      "Removes a secret. Refused with workflow_secret_missing while the workflow bound to the queue requires the key and the global env does not hold it.",
	"queue.env.set":        "Stores a plain env var that the scripts of the queue's workflow receive in their environment, replacing an earlier value. The value is the third argument, --value, or stdin with one trailing newline stripped. A key follows the secret key rules and is either an env var or a secret: setting a secret's key here makes it an env var. The value is not secret: env ls and the Environment tab show it, and run logs do not redact it. Keep a token in queue secret set.",
	"queue.env.ls":         "Lists the plain env vars of the queue with their values and when each was set. Secrets are listed by queue secret ls.",
	"queue.env.rm":         "Removes a plain env var. Refused with workflow_secret_missing while the workflow bound to the queue requires the key and the global env does not hold it.",
	"queue.source.ls":      "Lists the sources of the workflow bound to the queue: script, every, when each runs next, how many runs failed in a row, the last run with its verdict, message, and the number of tasks it created, and the ids of its 20 newest runs, newest first, for queue source log.",
	"queue.source.log":     "Prints the last bytes of a source run's log, 64 KiB by default and at most 1 MiB. Queue secret values are replaced with [redacted]. Run ids come from queue source ls or the queue.source_failed event. A running run's log can be read before it finishes.",
	"events":               "Reads durable task events after the supplied sequence, bounded by --limit. Use the last sequence to resume.",
	"activity":             "Summarizes the work on tasks in a time window: AI requests tagged with a task and the task events, per task and, server-wide, per agent. --since and --until take RFC 3339 times; --agent keeps one agent's work.",
	"principals":           "Lists principals available for task assignment and typed user:login or agent:name mentions.",
}
