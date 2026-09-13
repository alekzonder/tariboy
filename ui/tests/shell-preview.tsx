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

const real = window.fetch;
window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = String(typeof input === "string" ? input : input instanceof URL ? input : input.url);
  const json = (result: unknown) => new Response(JSON.stringify({ ok: true, result }), { status: 200, headers: { "content-type": "application/json" } });
  if (url.includes("/api/agents")) return json({ agents: AGENTS, count: AGENTS.length });
  if (url.includes("/api/groups")) return json({ groups: [{ name: "core", lead: "builder", members: 2 }], count: 1 });
  if (url.includes("/api/daemon/config")) return json({});
  if (url.includes("/api/tasks")) return json({ tasks: [], count: 0 });
  if (url.includes("/status")) return json({ state: "running", messages_pending: 0 });
  if (url.includes("/api/")) return json({});
  return real(input, init);
}) as typeof fetch;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <MemoryRouter initialEntries={["/agents/local/builder/console"]}>
      <Routes><Route path="*" element={<App />} /></Routes>
    </MemoryRouter>
  </StrictMode>,
);
