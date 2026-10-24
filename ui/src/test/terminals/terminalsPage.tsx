import type { Mock } from "vitest";
import { expect, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import { CustomerQuestionNotificationsContext } from "@/components/customerQuestionNotificationsContext";
import { fetchAllAgents } from "@/lib/aggregate";
import type { ImageManifest, ImageRow } from "@/lib/api";
import { addDaemon } from "@/lib/daemons";
import type { AgentLifecycleResult } from "@/lib/types";
import { UI_MODE_KEY } from "@/lib/uiMode";
import { resetWorkspacesForTest } from "@/lib/workspaces";
import { SidebarStateProvider } from "@/pages/terminals/SidebarStateProvider";
import TerminalsPage from "@/pages/terminals/TerminalsPage";
import { TitlebarSlotContext } from "@/pages/terminals/titlebarSlotContext";
import { RoutedLocation } from "./RoutedLocation";

/** The mocked api calls every TerminalsPage test file records; each file
 *  creates them in its own `vi.hoisted` block, because Vitest hoists only
 *  within the test file itself. Mocks that are not set up here are left to
 *  the tests that read them. */
export interface TerminalsPageCalls {
  agentGetOn: Mock<(...args: never[]) => Promise<unknown>>;
  apiOn: Mock<(...args: never[]) => Promise<unknown>>;
  imageManifestGet: Mock<(...args: never[]) => Promise<ImageManifest>>;
  listImages: Mock<(...args: never[]) => Promise<{ images: ImageRow[]; count: number }>>;
  startAgent: Mock<(...args: never[]) => Promise<AgentLifecycleResult>>;
}

export const cloneProjection = {
  name: "source",
  image: "worker:v1",
  digest: "sha256",
  state: "stopped",
  cwd: "/managed/source",
  configured_cwd: "",
  harness: "codex",
  model: "gpt-5",
  effort: "high",
  interactive: true,
  enabled: false,
  interval_s: 0,
  timeout_s: 0,
  hard_timeout_s: 0,
  on_timeout: "restart",
  on_error: "restart",
  max_idle_iterations: 0,
  user_prompt: "",
  env: {},
  plugins: [],
  messages_batch: 10,
  messages_max_queue: 1000,
  group: null,
  alias: "",
  notes: "",
  color: "",
} as const;

export function setupTerminalsPage({
  agentGetOn,
  apiOn,
  imageManifestGet,
  listImages,
  startAgent,
}: TerminalsPageCalls) {
  localStorage.clear();
  sessionStorage.clear();
  resetWorkspacesForTest();
  // Terminal navigation is Expert behaviour; Simple opens Chat.
  localStorage.setItem(UI_MODE_KEY, "expert");
  apiOn.mockReset().mockResolvedValue(undefined);
  vi.mocked(fetchAllAgents).mockResolvedValue([
    {
      host: { id: "", label: "This daemon (local)" },
      agents: [
        {
          name: "a1",
          image: "bare:latest",
          state: "running",
          harness: "claude",
          group: null,
          interactive: true,
        },
        {
          name: "a2",
          image: "img",
          state: "running",
          harness: "claude",
          group: null,
          interactive: false,
        },
      ],
    },
    { host: { id: "d1", label: "prod" }, agents: [], error: "unauthorized: nope" },
  ]);
  listImages.mockResolvedValue({
    images: [{ name: "worker", tag: "v1", bare: false }],
    count: 1,
  });
  imageManifestGet.mockResolvedValue({
    schema_version: 1,
    name: "worker",
    tag: "v1",
    built_at: "2026-07-29T00:00:00Z",
    parents: null,
    plugins: null,
    skills: null,
    requires_secrets: null,
    harness: { type: "codex", interactive: false },
    env: null,
    policy: {},
    layers: null,
  });
  startAgent.mockResolvedValue({ name: "new-agent", action: "start" });
  agentGetOn.mockResolvedValue(cloneProjection);
}

export function cleanupTerminalsPage() {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
}

export function renderAt(path: string, attention: ReadonlyMap<string, number> = new Map()) {
  // Stands in for the App titlebar slot the workspace switcher portals into.
  const slot = document.body.appendChild(document.createElement("div"));
  return render(
    <TitlebarSlotContext.Provider value={slot}>
      <DaemonProvider>
        <CustomerQuestionNotificationsContext.Provider
          value={{ attention, refreshHost: async () => {} }}
        >
          <MemoryRouter initialEntries={[path]}>
            <SidebarStateProvider>
              <RoutedLocation />
              <Routes>
                <Route path="/" element={<TerminalsPage />} />
                <Route path="/terminals" element={<TerminalsPage />} />
                <Route path="/terminals/:hostId/:agent" element={<TerminalsPage />} />
                <Route path="/agents/:hostId/:agent/:tab" element={<TerminalsPage />} />
                <Route path="/agents/:hostId/teams/:team" element={<TerminalsPage />} />
                <Route path="/servers/:hostId/tasks" element={<TerminalsPage />} />
              </Routes>
            </SidebarStateProvider>
          </MemoryRouter>
        </CustomerQuestionNotificationsContext.Provider>
      </DaemonProvider>
    </TitlebarSlotContext.Provider>,
  );
}

export function mockRowRects(tops: Record<string, number>) {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    const top = tops[this.getAttribute("aria-label") ?? ""] ?? 0;
    return {
      left: 0,
      top,
      width: 100,
      height: 20,
      right: 100,
      bottom: top + 20,
      x: 0,
      y: top,
      toJSON: () => ({}),
    };
  });
}

export async function keyboardMove(label: string, direction: "ArrowUp" | "ArrowDown") {
  const row = screen.getByRole("button", { name: label });
  fireEvent.keyDown(row, { code: "Space" });
  await waitFor(() => expect(row).toHaveAttribute("aria-pressed", "true"));
  fireEvent.keyDown(document, { code: direction });
  fireEvent.keyDown(document, { code: "Space" });
  await waitFor(() => expect(row).not.toHaveAttribute("aria-pressed"));
}

/** Register "prod" in the real (jsdom) registry under the id the mocked
 *  fetchAllAgents reports, so the manage menu operates on a real entry. */
export async function registerProd(): Promise<string> {
  const meta = await addDaemon({ label: "prod", baseURL: "http://127.0.0.1:9992", token: "tok" });
  vi.mocked(fetchAllAgents).mockResolvedValue([
    { host: { id: "", label: "This daemon (local)" }, agents: [] },
    { host: { id: meta.id, label: "prod" }, agents: [] },
  ]);
  return meta.id;
}

// Radix opens a DropdownMenu on pointerdown, which jsdom's fireEvent.click
// does not synthesize — drive it from the keyboard instead.
export async function openManageMenu(): Promise<void> {
  fireEvent.keyDown(await screen.findByRole("button", { name: "manage prod" }), { key: "Enter" });
}

/** Open the Servers tab before mounting — the tab the sidebar remembers is the
 *  one the server sections, host status and drag-and-drop live in. */
export function serversTab() {
  localStorage.setItem("terminals:sidebar-tab:v1", "servers");
}

/** Open the Agents tab before mounting — the flat list across servers. */
export function agentsTab() {
  localStorage.setItem("terminals:sidebar-tab:v1", "agents");
}

/** Radix activates a tab on mousedown, which fireEvent.click does not send. */
export function pickSidebarTab(name: string) {
  fireEvent.mouseDown(screen.getByRole("tab", { name }), { button: 0 });
}
