All actions below are simulated. No task, file, service, or Git state was changed. Referenced skill bodies are unavailable, so their exact utilities and flags require the named invocation.

**D — Unapproved IMG-4**

- Invoke `tasks`; read IMG-4, its customer, approval history and any declared workflow actions.
- Invoke Superpowers investigation/proposal skills to present the concrete image scope, rationale, evaluation coverage and destination. The ready draft and deadline do not authorize edits.
- For a flexible task, ask with `ttasks ask IMG-4 user:LOGIN "Approve the recorded image proposal …?"`; substitute the recorded customer and actual proposal. For a managed workflow, use its assignment-scoped question form instead.
- Durable state: approval unanswered on IMG-4; record its question identifier and answer resume event. Preserve other context entries and add `IMG-4 await-plan-approval`. End the iteration only after recording that wait.
- Uncertainty: customer identity, workflow type and actual proposal are absent.

**E — PR 7 closed without merge**

- Invoke `tasks`, `github-pr-workflow` and `scripts`; recover the existing task, PR 7, branch/worktree and monitor identifiers. Handle and acknowledge the manager message through `messages`.
- Record `closed`, `merged: false` as the blocker. Do not complete, merge, replace the PR or remove its monitor. A request to close the task does not supply merge evidence or explicitly establish abandonment.
- If a decision is needed, use the task’s appropriate question form: for example, `ttasks ask KEY user:LOGIN "PR 7 closed without merging. Please provide the intended next decision for this task."`
- Durable state: same PR, active task and active monitor; flexible task remains `wait_customer`. Record monitor identifier and next monitor-event resume condition, plus any question identifier. Context: `KEY await-pr-resolution`.
- Retain successful verification while relevant inputs remain unchanged.
- Uncertainty: whether any separately recorded, task-authoritative abandonment/replacement decision exists. If present, invoke the PR skill’s separate non-completion branch.

**F — PR 7 merged at abc123**

- Invoke `tasks` and `github-pr-workflow`; read authoritative recorded state once and confirm the monitor evidence identifies PR 7 as merged with merge commit `abc123`.
- Invoke `scripts`; cancel **and remove** the recorded schedule.
- Fetch and fast-forward the configured base safely; a concrete command requires the recorded remote/base and checkout state. Preserve user changes and never reset.
- Invoke `verification-before-completion`; run and await the distinct relevant post-merge checks against the updated base.
- Invoke `finishing-a-development-branch` for cleanup without its integration menu: remove the recorded worktree and local task branch after successful checks.
- Post one consolidated task comment mentioning the customer and containing `Required:`, `Completed:`, `Verification:`, `Integration:` with PR URL and `abc123`, and `Cleanup:`. Include changed files, versions and eval provenance/results/limitations.
- Immediately run `ttasks done KEY`, or the declared successful workflow outcome; then remove only this task’s context entry. Invoke `loop` only after commands/evaluators finish.
- Durable state: active until synchronization, checks and cleanup succeed; record any failure and resolve it. No completion on partial success.
- Uncertainty: task key, remote/base, paths, schedule identifiers and applicable check commands require the recorded task and referenced skills.

**G — Verified non-Git Store improvement**

- Invoke `tasks`; recover the task/customer, recorded approval and evaluation evidence. If no key was supplied, inspect and claim matching assigned/ready work; create only in an explicitly identified queue.
- Invoke `verification-before-completion`, `requesting-code-review` when applicable, and `finishing-a-development-branch`. Preserve valid existing successful checks; await any remaining required checks.
- Publish the changed-file report, versions and eval provenance/results/limitations on the task, mentioning the customer.
- Ask through the task: `ttasks ask KEY user:LOGIN "The verified Store improvement and evaluation report are recorded here. What should happen next?"` Use the workflow-specific question form where required.
- Durable state: retain active task and files; record question identifier and answer resume event. Context: `KEY await-customer-direction`. Wait for the recorded decision before completion.
- Uncertainty: task/customer, approval evidence and report contents are unspecified. No commit, PR or merge is implied.

**H — Verified non-GitHub Git worktree**

- Invoke `tasks` and `using-git-worktrees`; recover the recorded branch/worktree, base, customer and approval.
- Invoke `verification-before-completion`, `requesting-code-review` when applicable, and `finishing-a-development-branch`, using the preselected other-Git path without an integration menu.
- Publish a customer-mentioned task report containing changed files, versions, eval provenance/results/limitations and the branch/worktree artifact.
- Ask through the task: `ttasks ask KEY user:LOGIN "Please confirm acceptance and provide integration instructions for the verified branch and worktree recorded here."` Use declared workflow question actions if applicable.
- Durable state: retain both branch and worktree and the active task. Record the question identifier and answer resume event; context `KEY await-integration-instructions`.
- Uncertainty: integration destination and instructions are absent. Do not invent a commit, PR or merge, or complete before the recorded decision.
