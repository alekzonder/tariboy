# Tasks UI selection and comment order Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve the task a user explicitly selected during real-time refreshes and show its comments newest-first by default with an oldest-first control.

**Architecture:** `TasksWorkspace` keeps the intended key in a ref before every asynchronous load, so real-time refreshes fetch the current intended task rather than a stale render value. `TaskDetail` owns only a local display-order state and passes a copied, reversed comment list to the existing comments renderer.

**Tech Stack:** React, TypeScript, Vitest, Testing Library.

**Spec:** `docs/superpowers/specs/2026-08-30-tasks-ui-selection-comments-design.md`

## Global Constraints

- Keep the API's existing oldest-first chronological comments response unchanged.
- Default the UI display to newest-first; do not persist the control or issue another API request.
- Reuse the existing focused Tasks workspace test suite and make no new dependency.

---

## File structure

- Modify: `ui/src/pages/tasks/TasksWorkspace.tsx` — synchronously retain the intended selection through detail and real-time loads.
- Modify: `ui/src/pages/tasks/TaskDetail.tsx` — local comment order control and display-only comment reversal.
- Modify: `ui/src/pages/tasks/TasksWorkspace.test.tsx` — regression coverage for refresh races and both comment orders.

### Task 1: Stable selected task key

**Files:**
- Modify: `ui/src/pages/tasks/TasksWorkspace.tsx`
- Test: `ui/src/pages/tasks/TasksWorkspace.test.tsx`

**Interfaces:**
- Produces: `selectedKeyRef.current`, synchronously assigned by `loadDetail(key)` before starting requests.
- Consumes: existing `loadDetail(key)` and `refreshFromRealtimeHint()` callbacks.

- [ ] **Step 1: Write the failing refresh-race test**

```tsx
const first = deferred<TaskDetail>()
api.getTask.mockImplementation((key: string) => key === "TEST-1" ? first.promise : Promise.resolve(secondDetail))
await user.click(await screen.findByRole("button", { name: /Ship native tasks/ }))
await user.click(screen.getByRole("button", { name: /Desktop tree/ }))
await act(async () => taskSocket.options?.onHint({ sequence: 11 }))
first.resolve(detail)
expect(await screen.findByRole("heading", { name: "TEST-2" })).toBeInTheDocument()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ui && npm test -- TasksWorkspace.test.tsx -t "keeps the latest task selected during a real-time refresh"`

Expected: FAIL because a refresh closure may load the previous key.

- [ ] **Step 3: Write minimal implementation**

```tsx
const selectedKeyRef = useRef("")
const loadDetail = useCallback(async (key: string) => {
  selectedKeyRef.current = key
  const request = ++detailRequestRef.current
  // accept response only when selectedKeyRef.current === key
}, [target])
```

Read `selectedKeyRef.current` inside `refreshFromRealtimeHint`; clear it with the existing close and failed-load state.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ui && npm test -- TasksWorkspace.test.tsx -t "keeps the latest task selected during a real-time refresh"`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/src/pages/tasks/TasksWorkspace.tsx ui/src/pages/tasks/TasksWorkspace.test.tsx
git commit -m "fix: preserve selected task during refresh"
```

### Task 2: Display-only comment order

**Files:**
- Modify: `ui/src/pages/tasks/TaskDetail.tsx`
- Test: `ui/src/pages/tasks/TasksWorkspace.test.tsx`

**Interfaces:**
- Consumes: `detail.comments` in API chronological order.
- Produces: a `Comment order` select with `newest` and `oldest` values passed to `TaskComments` as an ordered copy.

- [ ] **Step 1: Write the failing comment-order test**

```tsx
api.getTask.mockResolvedValue({ ...detail, comments: [oldComment, newComment] })
await user.click(await screen.findByRole("button", { name: /Ship native tasks/ }))
expect(screen.getAllByTestId("task-comment")[0]).toHaveTextContent("Newest")
await user.selectOptions(screen.getByLabelText("Comment order"), "oldest")
expect(screen.getAllByTestId("task-comment")[0]).toHaveTextContent("Oldest")
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ui && npm test -- TasksWorkspace.test.tsx -t "shows newest comments first and can switch to oldest first"`

Expected: FAIL because no comment-order control exists.

- [ ] **Step 3: Write minimal implementation**

```tsx
const [commentOrder, setCommentOrder] = useState<"newest" | "oldest">("newest")
const comments = commentOrder === "newest" ? [...detail.comments].reverse() : detail.comments
```

Render the native `<select aria-label="Comment order">` beside the comments heading and pass `comments` to `TaskComments`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ui && npm test -- TasksWorkspace.test.tsx -t "shows newest comments first and can switch to oldest first"`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/src/pages/tasks/TaskDetail.tsx ui/src/pages/tasks/TasksWorkspace.test.tsx
git commit -m "feat: add task comment order control"
```

### Task 3: Integrated verification

**Files:**
- Verify: `ui/src/pages/tasks/TasksWorkspace.test.tsx`

- [ ] **Step 1: Run the focused workspace suite**

Run: `cd ui && npm test -- TasksWorkspace.test.tsx`

Expected: PASS.

- [ ] **Step 2: Run repository checks before integration**

Run: `make check && git diff --check`

Expected: PASS with no whitespace errors.

- [ ] **Step 3: Inspect the complete diff before the final commit**

Run: `git diff --check && git diff --cached && git status --short`

Expected: only the planned UI, test, spec, and plan changes.
