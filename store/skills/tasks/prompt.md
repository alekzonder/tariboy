## Native Tasks
Tasks are the durable source of truth for work, decomposition, ownership,
questions, and answers. Use the bare `tasks` command; this is Tariboy's
native task system.

Choose the `tasks ask` form from the context you actually have; the flexible
and workflow forms are mutually exclusive.

For a flexible task without a work packet, ask with:
    tasks ask <TASK-KEY> user:<login>|agent:<name> <TEXT>
This posts the question and creates the durable answer wait. It requires
neither an assignment ID nor revisions. A plain comment is not a substitute
for this question.

For a workflow-managed task with a work packet, use only the assignment-scoped
form: its packet is your complete and least-privilege context; use only its
declared actions, tools, outcomes, and channel patterns:
    tasks work next
    tasks work show <assignment-id>
    tasks artifacts add <assignment-id> --name <name> --type <type> --content <value>
    tasks ask <assignment-id> --question <text> --context <why> --blocking-scope assignment
    tasks questions <assignment-id>
    tasks answer <question-id> --assignment <assignment-id> --answer <text>
    tasks observe subscribe <assignment-id> <channel-pattern> --reaction wake_current
    tasks work complete <assignment-id> --outcome <allowed-outcome>
Raw channel subscription commands are always denied during managed work; use
`tasks observe` so the subscription remains assignment-scoped. Direct message and
group-request commands are denied unless the workflow explicitly grants them.

Inspect your work and claim an unblocked item:
    tasks mine
    tasks ready
    tasks ready --claim
    tasks show TEST-12

Create and delegate durable work:
    tasks create --queue TEST --title "Investigate failure"
    tasks create --parent TEST-12 --title "Add regression test"
    tasks assign TEST-12 worker

Keep every decision, question, and answer on the task:
    tasks comment TEST-12 "Found the failing boundary"
    tasks ask TEST-12 user:login "Which behavior should win?"
    tasks ask TEST-12 agent:worker "Can you verify this?"

Update status as work advances and close only completed work:
    tasks update TEST-12 --status in_progress
    tasks done TEST-12

Decompose substantial work before delegating it. Never invent another
principal's identity: the daemon records tasks and comments under your
authenticated agent name.
