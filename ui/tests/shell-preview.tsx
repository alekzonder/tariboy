// Console shell preview fixture: the real App over a stubbed daemon, for
// eyeballing the style layer (and for tests/shell-preview-shot.mjs). Not part
// of any test run — it is a development harness, served by
// vite.workspace-test.config.ts like the other fixtures in this directory.
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import App from "@/App";
import "@/index.css";

const AGENTS = [
  { name: "builder", image: "worker:v2", state: "running", harness: "claude", loop_enabled: true, group: "core", interactive: true, current_goal_task_key: "TB-142", cwd: "/home/agent/github/tariboy" },
  { name: "reviewer", image: "worker:v2", state: "running", harness: "claude", loop_enabled: false, group: "core", interactive: true },
  { name: "packager", image: "bare:latest", state: "failed", harness: "codex", loop_enabled: false, group: null, interactive: true },
  { name: "docs-bot", image: "bare:latest", state: "stopped", harness: "codex", loop_enabled: false, group: null, interactive: false },
];

const TASKS = [
  { key: "TB-142", queue: "TB", parent_key: "", position: 1, priority: "P1", title: "Migrate store schema", description: "", status: "in_progress", author: "customer:ops", customer: "customer:ops", group: "", assignee: "agent:builder", manual_block_reason: "", blocked: false, revision: 1, created_at: "2026-09-13T09:10:00Z", updated_at: "2026-09-13T14:02:00Z", completed_at: "" },
  { key: "TB-143", queue: "TB", parent_key: "TB-142", position: 1, priority: "P2", title: "Dump current schema", description: "", status: "done", author: "customer:ops", customer: "customer:ops", group: "", assignee: "agent:builder", manual_block_reason: "", blocked: false, revision: 1, created_at: "2026-09-13T09:12:00Z", updated_at: "2026-09-13T13:44:00Z", completed_at: "2026-09-13T13:44:00Z" },
  { key: "TB-144", queue: "TB", parent_key: "TB-142", position: 2, priority: "P1", title: "Write migration", description: "", status: "wait_customer", author: "customer:ops", customer: "customer:ops", group: "", assignee: "agent:builder", manual_block_reason: "", blocked: false, revision: 1, created_at: "2026-09-13T09:14:00Z", updated_at: "2026-09-13T14:02:00Z", completed_at: "" },
  { key: "TB-146", queue: "TB", parent_key: "TB-144", position: 1, priority: "P2", title: "Generate SQL", description: "", status: "done", author: "customer:ops", customer: "customer:ops", group: "", assignee: "agent:reviewer", manual_block_reason: "", blocked: false, revision: 1, created_at: "2026-09-13T09:20:00Z", updated_at: "2026-09-13T13:58:00Z", completed_at: "2026-09-13T13:58:00Z" },
  { key: "TB-147", queue: "TB", parent_key: "TB-144", position: 2, priority: "P2", title: "Verify rollback", description: "", status: "open", author: "customer:ops", customer: "customer:ops", group: "", assignee: "agent:reviewer", manual_block_reason: "", blocked: true, revision: 1, created_at: "2026-09-13T09:22:00Z", updated_at: "2026-09-12T18:00:00Z", completed_at: "" },
  { key: "TB-145", queue: "TB", parent_key: "TB-142", position: 3, priority: "P3", title: "Update docs", description: "", status: "cancelled", author: "customer:ops", customer: "customer:ops", group: "", assignee: "agent:docs-bot", manual_block_reason: "", blocked: false, revision: 1, created_at: "2026-09-08T09:22:00Z", updated_at: "2026-09-08T11:00:00Z", completed_at: "" },
];

// Dark mode is a class on <html> (ThemeProvider does this in the real entry).
if (new URLSearchParams(location.search).get("theme") === "dark") {
  document.documentElement.classList.add("dark");
}

const real = window.fetch;
window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = String(typeof input === "string" ? input : input instanceof URL ? input : input.url);
  const json = (result: unknown) => new Response(JSON.stringify({ ok: true, result }), { status: 200, headers: { "content-type": "application/json" } });
  if (url.includes("/api/agents")) return json({ agents: AGENTS, count: AGENTS.length });
  if (url.includes("/api/groups")) return json({ groups: [{ name: "core", lead: "builder", members: 2 }], count: 1 });
  if (url.includes("/api/daemon/config")) return json({});
  if (url.includes("/api/task-queues")) return json({ queues: [{ prefix: "TB", title: "Tariboy", description: "", next_number: 148 }], count: 1 });
  if (url.includes("/api/task-notifications")) return json({ notifications: [], count: 0 });
  if (url.includes("/api/tasks")) return json({ tasks: TASKS, count: TASKS.length, sequence: 10 });
  if (url.includes("/status")) return json({ state: "running", messages_pending: 0 });
  if (url.includes("/api/")) return json({});
  return real(input, init);
}) as typeof fetch;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <MemoryRouter initialEntries={[new URLSearchParams(location.search).get("route") ?? "/agents/local/builder/console"]}>
      <Routes><Route path="*" element={<App />} /></Routes>
    </MemoryRouter>
  </StrictMode>,
);
