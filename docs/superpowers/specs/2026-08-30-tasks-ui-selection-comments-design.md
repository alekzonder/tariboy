# Tasks UI selection and comment order

## Goal

Keep the task explicitly chosen by a user or notification selected while task
data refreshes, and make comment order controllable in the task detail panel.

## Scope

- Record the intended task key synchronously before starting its detail request.
- Let refreshes update data without replacing that intended selection.
- Show comments newest-first by default, with a local oldest-first toggle.
- Preserve the API's existing oldest-first chronological response and its data
  contract.

## Design

`TasksWorkspace` owns the selection. Its synchronous ref becomes the source of
truth for selection during asynchronous detail loads and refreshes. A response
may populate detail only when it still belongs to that key; stale responses do
not change the selection.

`TaskDetail` owns a small display-only order state. It defaults to newest-first
and reverses a copy of the received comments for display. The control does not
persist and does not issue a new API request.

## Verification

Focused UI tests cover a refresh racing a notification/task selection and both
rendered comment orders. The existing UI checks and documentation build verify
the integrated change.
