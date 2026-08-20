# Development Task Workflow

Every customer request must be represented by a Native Task, performed in its
own Git worktree, and completed through the applicable Superpowers workflow.
These requirements are mandatory even when the request is urgent or asks to
skip process.

These project rules take precedence when a packaged skill offers a conflicting
default: customer questions and approvals happen only in Native Task comments,
worktree isolation is pre-approved and has no in-place fallback, and local
merge into `main` with cleanup is the predetermined completion choice. The
state-based verification freshness defined below also overrides any packaged
rule that requires an unchanged successful check to be rerun merely because a
new message, iteration, completion claim, or integration action began.

Following the spirit while violating the exact context or continuation
contract below is still a workflow violation.

## Goal and Progress Contract

The agent's goal is to deliver the customer's requested outcome and drive its
Native Task through this entire workflow to the terminal `done` state. Tool
calls, checks, progress comments, handoffs, `i-am-done`, and the end of an
iteration are intermediate mechanics. None of them satisfies the goal while
the Native Task remains active.

After every action, choose the next state by this contract:

1. If the task meets its acceptance criteria, execute every remaining step in
   section 5, including `tasks done` and context cleanup.
2. If the next workflow action can be executed now, execute it immediately.
3. When a specific unresolved object prevents that next action, wait for that
   object by the rule below. Record its stable identifier and the event that
   will resume work in the Native Task.

A valid wait object is an unanswered task question, a named external workflow
event or subscription, a live terminal session/process or subagent handle, or
an active durable script/run whose result is required. Read its authoritative
state once before waiting. A live terminal command or subagent must be awaited
to its terminal result in the current iteration; it never justifies
`i-am-done`. End an iteration with an active task only for a recorded question,
external event/subscription, or durable script that is designed to deliver its
answer or result in a later iteration. A command that returned its final output
and exit status without a live session ID is finished. If no valid wait object
exists, there is nothing to wait for: repair any stale `wait-*` context slug
and continue the workflow. Do not poll status, scripts, processes, or task
state hoping that an event appears. After the one authoritative state read, do
not read it again before the recorded resume event; use the object's event or
wait mechanism instead. Awaiting a live terminal session or subagent through
its blocking or event-driven wait mechanism is not polling.

Run the verification required by the current workflow stage once for each
unchanged state. A successful result satisfies that verification point until
relevant code, configuration, dependencies, environment, or the required
stage changes. Branch verification and post-merge verification are distinct
required stages. Repeat a check only after such a change, a concrete failure
or incomplete result, or an explicit requirement of the active workflow.
"Fresh evidence" does not mean rerunning the same successful check before
each subsequent action.

## 1. Establish the Native Task

Invoke `using-superpowers` first. Then, before planning, editing files, or
running any other task-specific command:

1. Read any supplied task key or workflow assignment with `tasks show <key>` or
   `tasks work show <assignment-id>`.
2. If no task is supplied, inspect `tasks mine` and the relevant queue's ready
   work. Claim the request's existing task when one matches.
3. If no task matches, create a root Native Task in the queue identified by the
   request or runtime context and assign it to the current agent. Record the
   resulting task key.
4. For flexible tasks, set the task to `in_progress`. For workflow-managed
   tasks, use only the actions and outcomes allowed by the work packet.

Do not perform the work without a task key or assignment. Keep the Native Task
as the durable source of truth: add concise comments for important findings,
decisions, progress, verification results, and integration status.

Use `tools context get` only to recover pointers to active work. Context is an
index, not a task report. Its entire content consists of one line per active
task with exactly two whitespace-separated fields:

```text
<TASK-KEY> <next-action-slug>
```

The slug is one lowercase hyphenated token naming only the next immediately
executable action, for example `DEV-417 commit` or `DEV-417 wait-answer`. Put
requirements, findings, decisions, test output, progress summaries, status,
branch and worktree details, blockers, history, and later workflow steps in the
Native Task, never in context. After the active-task set or a next action
changes, read the current value with `tools context get`, preserve the other
active-task lines, and replace the document with
`tools context set "<minimal active-task lines>"`. Remove a completed task's
line; use `tools context set ""` when none remain.

Before every non-empty `tools context set`, validate the complete payload: each
line must match `^[A-Z][A-Z0-9]*-[0-9]+ [a-z][a-z0-9-]*$`. If any line does not
match, move its information to the Native Task and reduce the context line to
the two-field form before setting it. For one active task whose next action is
commit, the complete call is exactly `tools context set "DEV-417 commit"`.

| Native Task state | Complete context entry |
|---|---|
| Active; commit is next | `DEV-417 commit` |
| Waiting for a customer answer | `DEV-417 wait-answer` |
| Complete | No line for `DEV-417` |

A handoff therefore has two writes: the full handoff as a Native Task comment,
then only the matching two-field pointer in context.

Never guess a queue. If the request, runtime context, and the agent's visible
or responsible queues do not identify exactly one appropriate queue, do not
ask a question and do not begin the task. Return the pre-task intake error
`Native Task intake blocked: resubmit the request with a queue.` The next
request must supply the queue so the agent can create the Native Task before
any customer question or task work. This is the only communication allowed
without a Native Task because no task can exist without a queue.

## 2. Communicate Through the Task

Ask every customer question through the Native Task. For a flexible task, use
`tasks ask <key> user:<login> "<question>"`; this posts the question as a task
comment and records a durable answer wait. For a workflow assignment, use its
assignment-scoped `tasks ask` command with the required question, context, and
blocking scope.

Do not duplicate a question in chat or accept a chat reply as the decision.
Resume only from the answer recorded on the Native Task, then summarize the
decision in a task comment.

A task comment is a durable checkpoint, not a handoff and not a reason to end
the iteration. Immediately after every non-blocking comment, perform the next
required workflow action in the same iteration. Do not return a progress-only
final response while the Native Task is active and has an actionable next
step, including after implementation, focused tests, documentation checks, or
any other partial verification.

An iteration may end with an active task only for a cross-iteration wait named
in the Goal and Progress Contract: a recorded customer answer, external
workflow event/subscription, or required durable script/run. It may also end
when one of the four stop conditions in the active Superpowers workflow
applies and leaves no immediately executable action. Before ending, inspect
the Native Task state: if the next action can be performed now, continue; if it
cannot, record the exact wait or blocker in the task and keep only
`<TASK-KEY> <next-action-slug>` in context. Time pressure, token pressure,
context compaction, a detailed checkpoint, or a request to leave a handoff are
not stop conditions. Returning `STOP` while also naming an immediately
executable next action is a process violation: execute that action instead.

| Rationalization | Required action |
|---|---|
| "A detailed context is safer for compaction" | Put the detail in the Native Task; context remains `<TASK-KEY> <next-action-slug>`. |
| "The manager asked for a handoff and STOP" | Write the handoff to the Native Task and continue its immediately executable next action. |
| "The progress comment is a good stopping point" | A comment is only a checkpoint; continue while the task is actionable. |
| "The turn or token budget is nearly exhausted" | Continue the workflow; resource pressure does not create a wait state. |
| "The parent or next iteration can continue" | The agent assigned to the Native Task owns the next action; do it now. |
| "Fresh evidence means rerun before every next step" | One successful check satisfies its unchanged verification point; rerun only for a changed state, failed/incomplete result, or distinct required stage. |
| "A valid `wait-*` slug proves something is pending" | Name the live object, stable identifier, and resume event; otherwise repair the stale slug and continue. |
| "A live session lets me call `i-am-done` and resume later" | Await terminal sessions and subagents to their terminal result in this iteration. Only durable or external wait objects may cross iterations. |

Red flags: a context line containing prose, status, a semicolon, or multiple
workflow steps; `STOP` paired with an available next action; a final response
that only repeats a task comment; handing an actionable task to a parent or a
future iteration; repeated checks against unchanged state; polling without a
named wait object; or `i-am-done` while a terminal session or subagent is live.
On any red flag, correct the state and continue before returning.

## 3. Use One Git Worktree per Task

After establishing the task and before making any task-specific file change,
use `using-git-worktrees` to create or enter a dedicated branch and worktree.
Use a name derived from the task key, such as `dev-123-short-description`.

- One Native Task has exactly one active worktree and branch.
- Never implement a task directly in the main checkout or on `main`.
- Reuse a worktree only when it belongs to the same task.
- Preserve unrelated and pre-existing changes.
- Comment on the task with the branch name and worktree path.
- If isolation cannot be created safely, record the blocker on the task and
  stop; do not fall back to editing the shared checkout.

The customer's use of this image pre-approves worktree creation. Do not ask for
separate worktree consent, and do not use the in-place fallback described by
`using-git-worktrees` when creation is declined or sandbox-blocked.

## 4. Follow the Superpowers Flow

After Native Task intake, continue the `using-superpowers` flow by invoking
every skill whose trigger matches the work. Read each selected skill before
acting and obey its gates.

- For new features and behavior changes, use `brainstorming` before
  implementation. Override that skill's chat channel: put every clarifying
  question, proposed design, and approval request in Native Task comments, and
  wait for approval recorded there. Use `writing-plans` when the brainstorming
  flow classifies the change as architectural or otherwise requires a written
  plan.
- Execute an approved written plan with `subagent-driven-development` or
  `executing-plans`, as appropriate.
- Implement features and bug fixes with `test-driven-development`: write the
  failing test, observe the expected failure, implement the minimum change,
  then observe the passing test.
- For bugs, use `systematic-debugging` before proposing or implementing a fix.
  Determine the root cause and reproduce it with a failing regression test.
- Use `receiving-code-review` when acting on review feedback and
  `requesting-code-review` before integration when its trigger applies.
- Use `verification-before-completion` before any completion claim or
  integration action, applying the state-based freshness contract above.
  Inspect and report the successful output and exit status for the current
  verification point; rerun it only when that contract requires a rerun.

Do not bypass a skill because the customer describes the task as simple,
obvious, or urgent.

## 5. Integrate, Clean Up, and Complete

Use `finishing-a-development-branch` after implementation and verification.
Do not present that skill's integration menu: this project preselects its local
merge option with `main` as the required base branch. The required outcome for
a completed task is a local merge into `main`, not an unmerged branch:

1. Commit only the task's intended changes in its worktree.
2. Run the complete relevant verification suite on the task branch.
3. Merge the task branch into `main` according to repository conventions.
4. Run the relevant verification suite again on the resulting `main`.
5. Remove the task worktree and delete the merged task branch.
6. Add one consolidated final Native Task comment. Earlier progress comments
   do not replace it. The comment contains these labeled parts, in order:
   - `Required:` the customer's requested outcome and acceptance criteria.
   - `Completed:` the concrete behavior and files/components changed.
   - `Verification:` every final verification command and its result, including
     the post-merge run on `main`.
   - `Integration:` the merge result and commit.
   - `Cleanup:` confirmation that the task worktree and merged branch were
     removed, plus any remaining follow-up; write `none` when there is none.
7. Immediately run `tasks done <key>` or complete the workflow assignment with
   its declared successful outcome. A technically finished task must be closed
   in the same iteration; do not leave it active for a parent, later turn, or
   another event. After the final comment succeeds, `tasks done` is the next
   command: do not return a response or perform an unrelated action between
   them. "Close next turn" is not a valid completion state.
8. Read context with `tools context get`, remove the completed task's line, and
   replace context with the remaining minimal lines; use `tools context set ""`
   when no active tasks remain.

If merge, verification, or cleanup fails, keep the Native Task active, post the
exact failure as a task comment, and continue the workflow. Never mark a task
done while its changes remain unmerged or its worktree remains active. A final
report without the following `tasks done` is also incomplete.
