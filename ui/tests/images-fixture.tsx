import { useState } from "react";
import { createRoot } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import BuiltImages from "@/pages/images/BuiltImages";
import ImagesPage from "@/pages/ImagesPage";
import ImageTemplate from "@/pages/images/ImageTemplate";
import ImageOverview from "@/pages/ImageOverview";
import { ImageLayout } from "@/components/ImageLayout";
import AgentConfigurationTab from "@/pages/agents/AgentConfigurationTab";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import type { AgentView } from "@/lib/types";
import { DaemonProvider } from "@/components/DaemonProvider";
import { Toaster } from "@/components/ui/sonner";
import "@/index.css";

declare global {
  interface Window {
    __imageApplyRef?: string;
    __imageBuild?: unknown;
    __imageTransferRequests?: Array<{ method: string; url: string }>;
  }
}

const envelope = (result: unknown) => new Response(JSON.stringify({ ok: true, result }), {
  status: 200,
  headers: { "content-type": "application/json" },
});

const failure = (status: number, message: string) => new Response(JSON.stringify({
  ok: false,
  error: { code: "image_ref_conflict", message },
}), {
  status,
  headers: { "content-type": "application/json" },
});

const transferHosts = [
  { id: "source", label: "Source server", baseURL: "https://source.tariboy.test", state: "ready" },
  { id: "conflict", label: "Conflict target", baseURL: "https://conflict.tariboy.test", state: "ready" },
  { id: "ready", label: "Ready target", baseURL: "https://ready.tariboy.test", state: "ready" },
  { id: "present", label: "Already present target", baseURL: "https://present.tariboy.test", state: "ready" },
  { id: "unavailable", label: "Unavailable target", baseURL: "https://unavailable.tariboy.test", state: "error" },
];

let pendingRef = "";
let activated = false;
const agentView: AgentView = {
  name: "worker", image: "basic:latest", digest: "basic-digest", state: "stopped", cwd: "/srv/worker",
  harness: "codex", model: "", effort: "", interactive: false, loop_enabled: false, enabled: false,
  interval_s: 0, timeout_s: 60, hard_timeout_s: 120, on_timeout: "restart", on_error: "restart",
  max_idle_iterations: 0, user_prompt: "", env: {}, plugins: [], group: null, alias: "", notes: "",
};

window.fetch = async (input, init) => {
  const requestURL = new URL(String(input), window.location.origin);
  const path = requestURL.pathname;
  const method = init?.method ?? "GET";
  const recordTransferRequest = () => {
    window.__imageTransferRequests ??= [];
    window.__imageTransferRequests.push({ method, url: requestURL.toString() });
  };
  if (path === "/api/images/validate" && method === "POST") {
    return envelope({ valid: true, schema_version: 2, plugins: ["loop"], template: { schema_version: 2, sha256: "template-sha", entries: [
      { kind: "runtime", runtime: "identity" },
      { kind: "file", source: "$CURRENT_VERSION_STORE/skills/loop/prompt.md", category: "current_version_store", archive_path: "prompt/layers/001-loop.md", size: 42, sha256: "layer-sha" },
      { kind: "runtime", runtime: "context" },
    ] } });
  }
  if (path === "/api/images/build" && method === "POST") {
    window.__imageBuild = JSON.parse(String(init.body));
    return envelope({ name: "browser-built", tag: "latest", digest: "built-digest", layers: 1 });
  }
  if (path === "/api/images") {
    return envelope({ images: [
      { name: "reviewer", tag: "v3", bare: false, exportable: true, source_cwd: "/srv/images/reviewer", source_available: true },
      { name: "browser-built", tag: "latest", bare: false, exportable: true, source_cwd: "/srv/images/browser-built", source_available: true },
      { name: "basic", tag: "latest", bare: false, exportable: false },
      { name: "imported", tag: "v1", bare: false, exportable: true },
      { name: "missing", tag: "v1", bare: false, exportable: true, source_cwd: "/srv/images/missing", source_available: false },
    ] });
  }
  if (path === "/api/images/reviewer%3Av3/template") {
    return envelope({ schema_version: 2, sha256: "template-sha", entries: [
      { kind: "runtime", runtime: "identity" }, { kind: "runtime", runtime: "context" },
    ] });
  }
  if (path === "/api/images/reviewer%3Av3/provenance") {
    return envelope({ ref: "reviewer:v3", digest: "reviewer-digest", source_cwd: "/srv/images/reviewer", source_available: true });
  }
  if (path === "/api/images/reviewer%3Av3") {
    return envelope({ schema_version: 2, name: "reviewer", tag: "v3", digest: "reviewer-digest", built_at: "2026-08-17T00:00:00Z", parents: [], plugins: [], requires_secrets: [], env: {}, layers: [], prompt_template_sha256: "template-sha" });
  }
  if (path === "/api/images/reviewer%3Av3/export") {
    if (requestURL.origin === "https://source.tariboy.test") recordTransferRequest();
    return new Response(new Blob(["portable-image"]), { status: 200, headers: { "content-type": "application/gzip" } });
  }
  if (path === "/api/image-imports" && method === "POST") {
    if (requestURL.origin === "https://ready.tariboy.test") {
      recordTransferRequest();
      return envelope({ import_id: "ready-import", ref: "reviewer:v3", digest: "ready-digest" });
    }
    if (requestURL.origin === "https://present.tariboy.test") {
      recordTransferRequest();
      return envelope({ import_id: "present-import", ref: "reviewer:v3", digest: "present-digest" });
    }
    if (requestURL.origin === "https://conflict.tariboy.test") {
      recordTransferRequest();
      return envelope({ import_id: "conflict-import", ref: "reviewer:v3", digest: "conflict-digest" });
    }
    return envelope({ import_id: "browser-import", ref: "reviewer:v3", digest: "digest" });
  }
  if (path === "/api/image-imports/ready-import/apply" && method === "POST") {
    recordTransferRequest();
    return envelope({ ref: "reviewer:v3" });
  }
  if (path === "/api/image-imports/present-import/apply" && method === "POST") {
    recordTransferRequest();
    return envelope({ ref: "reviewer:v3", reused: true });
  }
  if (path === "/api/image-imports/conflict-import/apply" && method === "POST") {
    recordTransferRequest();
    return failure(409, "target ref conflicts");
  }
  if (path === "/api/image-imports/browser-import/apply" && method === "POST") {
    const body = JSON.parse(String(init.body)) as { ref?: string };
    window.__imageApplyRef = body.ref;
    return envelope({ ref: body.ref });
  }
  if (path === "/api/agents/worker/image" && method === "POST") {
    pendingRef = (JSON.parse(String(init.body)) as { image: string }).image;
    return envelope({ name: "worker", current: { ref: "basic:latest", digest: "basic-digest" }, pending: { ref: pendingRef, digest: "built-digest", error: "" } });
  }
  if (path === "/api/agents/worker/image") {
    const current = activated ? { ref: "browser-built:latest", digest: "built-digest" } : { ref: "basic:latest", digest: "basic-digest" };
    return envelope({ name: "worker", current, pending: { ref: activated ? "" : pendingRef, digest: activated ? "" : "built-digest", error: "" } });
  }
  if (path === "/api/agents/worker/secrets") return envelope({ keys: [] });
  if (path === "/api/agents/worker/retention") return envelope({ keep_iterations: 20, keep_days: 30, max_bytes: 0, archive: false });
  if (path === "/api/agents/worker" || path.startsWith("/api/agents/worker/")) {
    return envelope(activated ? { ...agentView, image: "browser-built:latest", digest: "built-digest" } : agentView);
  }
  throw new Error(`unexpected fixture request ${method} ${requestURL.toString()}`);
};

export function AgentFixture() {
  const [generation, setGeneration] = useState(0);
  return <AgentNameContext.Provider value="worker">
    <AgentStatusContext.Provider value={{ status: null, refresh: async () => {} }}>
      <button type="button" onClick={() => { activated = true; pendingRef = ""; setGeneration((value) => value + 1); }}>Simulate next iteration</button>
      <div key={generation}><AgentConfigurationTab target={null} refresh={() => {}} /></div>
    </AgentStatusContext.Provider>
  </AgentNameContext.Provider>;
}

const mode = new URLSearchParams(window.location.search).get("mode") ?? "built";
if (mode === "transfer") {
  localStorage.setItem("tariboy_daemons", JSON.stringify(transferHosts));
  localStorage.setItem("tariboy_active_daemon", "source");
  for (const host of transferHosts) sessionStorage.setItem(`tariboy_daemon_token_${host.id}`, `${host.id}-token`);
}
let content = <BuiltImages hostId="" basePath="/servers/local/images" />;
if (mode === "build") content = <DaemonProvider><ImagesPage hostId="" basePath="/servers/local/images" /></DaemonProvider>;
if (mode === "detail") content = <Routes><Route path="/servers/local/images/:name/:tag" element={<ImageLayout hostId="" basePath="/servers/local/images" />}><Route index element={<ImageOverview />} /><Route path="template" element={<ImageTemplate />} /></Route></Routes>;
if (mode === "agent") content = <AgentFixture />;
if (mode === "transfer") content = <DaemonProvider><BuiltImages hostId="source" basePath="/servers/source/images" /></DaemonProvider>;
createRoot(document.getElementById("root")!).render(
  <MemoryRouter initialEntries={mode === "detail" ? ["/servers/local/images/reviewer/v3"] : ["/"]}>
    <main className={mode === "build" ? "h-screen overflow-hidden" : "p-4"}>{content}</main>
    <Toaster richColors />
  </MemoryRouter>,
);
