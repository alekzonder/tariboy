import type { ComponentProps } from "react";
import type { Mock } from "vitest";
import { expect, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import type { CreateAgentResult, ImageManifest, ImageRow } from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import { targetFor } from "@/lib/terminalsHost";
import type { AgentLifecycleResult, AgentView, HarnessCatalog } from "@/lib/types";
import { clearHarnessCatalogCache } from "@/hooks/useHarnessCatalog";
import { CreateAgentDialog } from "@/pages/terminals/CreateAgentDialog";

/** The mocked api calls every CreateAgentDialog test file records; each file
 *  creates them in its own `vi.hoisted` block, because Vitest hoists only
 *  within the test file itself. */
export interface CreateAgentDialogCalls {
  agentGetOn: Mock<(...args: never[]) => Promise<AgentView>>;
  createAgent: Mock<(...args: never[]) => Promise<CreateAgentResult>>;
  getHarnessModelsOn: Mock<(target: never, harness: string) => Promise<HarnessCatalog>>;
  imageManifestGet: Mock<(...args: never[]) => Promise<ImageManifest>>;
  listImages: Mock<(...args: never[]) => Promise<{ images: ImageRow[]; count: number }>>;
  startAgent: Mock<(...args: never[]) => Promise<AgentLifecycleResult>>;
}

export const hosts = [
  { id: "", label: "local" },
  { id: "d1", label: "prod" },
];

export const manifest = (overrides: Partial<ImageManifest> = {}): ImageManifest => ({
  schema_version: 1,
  name: "worker",
  tag: "v1",
  built_at: "2026-07-29T00:00:00Z",
  parents: null,
  plugins: null,
  requires_secrets: null,
  harness: {
    type: "codex",
    model: "o3",
    effort: "high",
    interactive: false,
  },
  env: null,
  policy: {},
  layers: null,
  ...overrides,
  skills: overrides.skills ?? null,
});

export const cloneSource: AgentView = {
  name: "source",
  image: "worker:v1",
  digest: "sha256",
  state: "stopped",
  cwd: "/managed/source/workdir",
  configured_cwd: "",
  harness: "codex",
  model: "gpt-5",
  effort: "high",
  interactive: true,
  enabled: false,
  interval_s: 12,
  timeout_s: 34,
  hard_timeout_s: 56,
  on_timeout: "stop",
  on_error: "restart",
  max_idle_iterations: 7,
  ai_stall_timeout_s: 420,
  user_prompt: "standing prompt",
  env: { CSV: "a,b", EQ: "a=b", LINES: "one\ntwo" },
  plugins: ["context", "custom"],
  messages_batch: 8,
  messages_max_queue: 900,
  group: "reviewers",
  alias: "Source alias",
  notes: "source notes",
  color: "#123abc",
  goal_enabled: false,
  goal_wait_customer_timeout_s: 120,
  goal_delivery_cooldown_s: 60,
  current_goal_task_key: "TARI-43",
};

export const completeOrdinarySpec = (overrides: Record<string, unknown> = {}) => ({
  image: "worker:v1",
  cwd: "",
  harness: "codex",
  model: "o3",
  effort: "high",
  interactive: true,
  env: {},
  plugins: [],
  interval_s: 0,
  timeout_s: 7200,
  hard_timeout_s: 10800,
  on_timeout: "restart",
  on_error: "restart",
  max_idle_iterations: 0,
  ai_stall_timeout_s: 1800,
  user_prompt: "",
  messages_batch: 10,
  messages_max_queue: 100,
  group: "",
  alias: "",
  notes: "",
  color: "",
  goal_enabled: true,
  goal_wait_customer_timeout_s: 300,
  goal_delivery_cooldown_s: 60,
  ...overrides,
});

export function renderDialog(props: Partial<ComponentProps<typeof CreateAgentDialog>> = {}) {
  const onCreated = vi.fn();
  const onOpenChange = vi.fn();
  const view = render(
    <CreateAgentDialog
      open
      hostId="d1"
      imageRef="worker:v1"
      hosts={hosts}
      onOpenChange={onOpenChange}
      onCreated={onCreated}
      {...props}
    />,
  );
  return { ...view, onCreated, onOpenChange };
}

export async function setSwitch(name: string, value: boolean) {
  const control = await screen.findByRole("switch", { name });
  if (control.getAttribute("aria-checked") === String(!value)) fireEvent.click(control);
}

export async function selectHarness(value: string) {
  const control = screen.getByRole("combobox", { name: "harness" });
  await waitFor(() => expect(control).toBeEnabled());
  fireEvent.change(control, { target: { value } });
  expect(control).toHaveValue(value);
}

const harnessCatalogs: Record<string, HarnessCatalog> = {
  codex: {
    harness: "codex",
    models: [
      { id: "gpt-6-astra", efforts: ["low", "ultra"] },
      { id: "gpt-5.6-luna", efforts: ["low", "max"] },
    ],
    efforts: ["low", "max", "ultra"],
    error: "",
  },
  claude: {
    harness: "claude",
    models: [{ id: "fable" }, { id: "opus" }, { id: "opus[1m]" }],
    efforts: ["low", "medium", "high"],
    error: "",
  },
};

export function setupCreateAgentDialog({
  agentGetOn,
  createAgent,
  getHarnessModelsOn,
  imageManifestGet,
  listImages,
  startAgent,
}: CreateAgentDialogCalls) {
  vi.clearAllMocks();
  localStorage.clear();
  clearHarnessCatalogCache();
  getHarnessModelsOn.mockImplementation(
    async (_target, harness) =>
      harnessCatalogs[harness] ?? {
        harness,
        models: [],
        efforts: [],
        error: "",
      },
  );
  vi.mocked(resolveDaemon).mockImplementation(async (id) => targetFor(id));
  listImages.mockResolvedValue({
    images: [
      { name: "worker", tag: "v1", bare: false },
      { name: "bare", tag: "latest", bare: true },
    ],
    count: 2,
  });
  imageManifestGet.mockResolvedValue(manifest());
  agentGetOn.mockResolvedValue(cloneSource);
  createAgent.mockResolvedValue({
    name: "created",
    state: "stopped",
  });
  startAgent.mockResolvedValue({ name: "created", action: "start" });
}
