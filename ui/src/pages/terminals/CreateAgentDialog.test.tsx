import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@/test/queryRender";
import type {
  AgentActionResult,
  AgentRunResult,
  AgentView,
  CreateAgentSpec,
  HarnessModelCatalog,
  ImageManifest,
  ImageSummary,
} from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { targetFor } from "@/lib/terminalsHost";
import { useApi } from "@/components/apiContext";
import {
  setupCreateAgentDialog,
  hosts,
  manifest,
  cloneSource,
  completeOrdinarySpec,
  renderDialog,
  setSwitch,
} from "@/test/terminals/createAgentDialog";
import { imageSummary } from "@/test/fixtures/apiFixtures";

// The dialog's requests, each recorded with the host of the client that sent
// it, so a test can assert which host a request was pinned to.
const calls = vi.hoisted(() => ({
  agentGetOn: vi.fn<(target: Daemon | null, name: string, action: string) => Promise<AgentView>>(),
  createAgent: vi.fn<(spec: CreateAgentSpec, target: Daemon | null) => Promise<AgentRunResult>>(),
  getHarnessModelsOn:
    vi.fn<(target: Daemon | null, harness: string) => Promise<HarnessModelCatalog>>(),
  imageManifestGet: vi.fn<(ref: string, target: Daemon | null) => Promise<ImageManifest>>(),
  listImages:
    vi.fn<(target: Daemon | null) => Promise<{ images: ImageSummary[]; count: number }>>(),
  startAgent: vi.fn<(name: string, target: Daemon | null) => Promise<AgentActionResult>>(),
}));
const { agentGetOn, createAgent, imageManifestGet, listImages, startAgent } = calls;

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  const createApi: typeof actual.createApi = (target) => {
    const client = actual.createApi(target);
    const host = client.target;
    return {
      ...client,
      agents: {
        ...client.agents,
        getAgent: (name: string) => calls.agentGetOn(host, name, ""),
        createAgent: (spec: CreateAgentSpec) => calls.createAgent(spec, host),
        startAgent: (name: string) => calls.startAgent(name, host),
      },
      harnesses: {
        ...client.harnesses,
        getHarnessModels: (harness: string) => calls.getHarnessModelsOn(host, harness),
      },
      images: {
        ...client.images,
        listImages: () => calls.listImages(host),
        imageManifestGet: (ref: string) => calls.imageManifestGet(ref, host),
      },
    } as typeof client;
  };
  return { ...actual, createApi };
});

vi.mock("@/lib/daemons", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/daemons")>();
  return { ...actual, resolveDaemon: vi.fn() };
});

vi.mock("@/components/PathAutocomplete", () => ({
  PathAutocomplete: function PathAutocomplete({
    value,
    onChange,
    id,
    "aria-label": ariaLabel,
  }: {
    value: string;
    onChange: (value: string) => void;
    id?: string;
    "aria-label"?: string;
  }) {
    // Lists directories on the server of the surrounding ApiProvider.
    const daemon = useApi().target;
    return (
      <input
        id={id}
        aria-label={ariaLabel}
        data-host={daemon?.id ?? "local"}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    );
  },
}));

beforeEach(() => setupCreateAgentDialog(calls));

afterEach(() => vi.restoreAllMocks());

it("loads a complete clone draft from the explicit source host and submits every field", async () => {
  const { onCreated } = renderDialog({
    imageRef: undefined,
    cloneSource: { hostId: "d1", agentName: "source", hostLabel: "prod" },
  });

  expect(await screen.findByRole("heading", { name: "Clone agent" })).toBeInTheDocument();
  expect(screen.getByText(/source.*prod/i)).toBeInTheDocument();
  expect(agentGetOn).toHaveBeenCalledWith(targetFor("d1"), "source", "");
  expect(screen.getByLabelText("name")).toHaveValue("");
  expect(screen.getByLabelText("cwd")).toHaveValue("");
  await waitFor(() => expect(screen.getByLabelText("alias")).toHaveValue("Source alias"));
  expect(screen.getByLabelText("group")).toHaveValue("reviewers");
  expect(screen.getByLabelText("color")).toHaveValue("#123abc");
  expect(screen.getByLabelText("notes")).toHaveValue("source notes");
  expect(screen.getByLabelText("model")).toHaveValue("gpt-5");
  expect(screen.getByLabelText("environment JSON")).toHaveValue(
    '{\n  "CSV": "a,b",\n  "EQ": "a=b",\n  "LINES": "one\\ntwo"\n}',
  );
  expect(screen.getByText("context")).toBeInTheDocument();
  expect(screen.getByLabelText("interval seconds")).toHaveValue(12);
  expect(screen.getByLabelText("soft timeout seconds")).toHaveValue(34);
  expect(screen.getByLabelText("hard timeout seconds")).toHaveValue(56);
  expect(screen.getByLabelText("timeout policy")).toHaveValue("stop");
  expect(screen.getByLabelText("error policy")).toHaveValue("restart");
  expect(screen.getByLabelText("maximum idle iterations")).toHaveValue(7);
  expect(screen.getByLabelText("standing user prompt")).toHaveValue("standing prompt");
  expect(screen.getByLabelText("message batch size")).toHaveValue(8);
  expect(screen.getByLabelText("maximum queued messages")).toHaveValue(900);
  expect(screen.getByRole("switch", { name: "Goal" })).not.toBeChecked();
  expect(screen.getByLabelText("wait customer timeout seconds")).toHaveValue(120);
  expect(screen.getByRole("switch", { name: "Interactive" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Start now" })).not.toBeChecked();

  fireEvent.change(screen.getByLabelText("name"), {
    target: { value: "copy" },
  });
  const create = screen.getByRole("button", { name: "Create agent" });
  await waitFor(() => expect(create).toBeEnabled());
  fireEvent.click(create);

  await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
  expect(createAgent).toHaveBeenCalledWith(
    {
      image: "worker:v1",
      name: "copy",
      cwd: "",
      harness: "codex",
      model: "gpt-5",
      effort: "high",
      interactive: true,
      env: { CSV: "a,b", EQ: "a=b", LINES: "one\ntwo" },
      plugins: ["context", "custom"],
      interval_s: 12,
      timeout_s: 34,
      hard_timeout_s: 56,
      on_timeout: "stop",
      on_error: "restart",
      max_idle_iterations: 7,
      ai_stall_timeout_s: 420,
      user_prompt: "standing prompt",
      messages_batch: 8,
      messages_max_queue: 900,
      group: "reviewers",
      alias: "Source alias",
      notes: "source notes",
      color: "#123abc",
      goal_enabled: false,
      goal_wait_customer_timeout_s: 120,
      goal_delivery_cooldown_s: 60,
    },
    targetFor("d1"),
  );
  expect(startAgent).not.toHaveBeenCalled();
  expect(onCreated).toHaveBeenCalledWith("d1", "created");
});

it("retries a failed source load without allowing a partial clone", async () => {
  agentGetOn
    .mockRejectedValueOnce(new Error("source unavailable"))
    .mockResolvedValueOnce(cloneSource);
  renderDialog({
    imageRef: undefined,
    cloneSource: { hostId: "d1", agentName: "source", hostLabel: "prod" },
  });

  expect(await screen.findByRole("alert")).toHaveTextContent("source unavailable");
  expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();
  expect(createAgent).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "Retry source" }));

  await waitFor(() => expect(agentGetOn).toHaveBeenCalledTimes(2));
  expect(await screen.findByLabelText("alias")).toHaveValue("Source alias");
  await waitFor(() => expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled());
});

it("requires a daemon with the complete clone projection", async () => {
  const { configured_cwd: _, ...oldProjection } = cloneSource;
  agentGetOn.mockResolvedValueOnce(oldProjection as AgentView);
  renderDialog({
    imageRef: undefined,
    cloneSource: { hostId: "d1", agentName: "source", hostLabel: "prod" },
  });

  expect(await screen.findByRole("alert")).toHaveTextContent(/update.*host.*complete clone/i);
  expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();
  expect(createAgent).not.toHaveBeenCalled();
});

it("retains clone drafts while target host and image readiness change", async () => {
  const source = { ...cloneSource, interactive: false };
  agentGetOn.mockResolvedValueOnce(source);
  renderDialog({
    imageRef: undefined,
    hosts: [...hosts, { id: "d2", label: "staging" }],
    cloneSource: { hostId: "d1", agentName: "source", hostLabel: "prod" },
  });

  expect(await screen.findByLabelText("alias")).toHaveValue("Source alias");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "clone-model" },
  });

  fireEvent.click(screen.getByRole("combobox", { name: "Host" }));
  fireEvent.click(await screen.findByRole("option", { name: "staging" }));
  expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();
  await waitFor(() =>
    expect(imageManifestGet).toHaveBeenLastCalledWith("worker:v1", targetFor("d2")),
  );
  expect(screen.getByLabelText("alias")).toHaveValue("Source alias");
  expect(screen.getByLabelText("model")).toHaveValue("clone-model");

  imageManifestGet.mockResolvedValueOnce(
    manifest({
      name: "bare",
      tag: "latest",
      bare: true,
      harness: { type: "codex", interactive: false },
    }),
  );
  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "bare:latest" }));
  expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();
  expect(await screen.findByText("Terminal only")).toBeInTheDocument();
  expect(screen.getByRole("switch", { name: "Interactive" })).toBeChecked();
  expect(screen.getByLabelText("alias")).toHaveValue("Source alias");
  expect(screen.getByLabelText("model")).toHaveValue("clone-model");

  imageManifestGet.mockResolvedValueOnce(manifest());
  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "worker:v1" }));
  await waitFor(() => expect(screen.queryByText("Terminal only")).not.toBeInTheDocument());
  expect(screen.getByRole("switch", { name: "Interactive" })).not.toBeChecked();
});

it("keeps schema-v1 plugins editable and makes schema-v2 plugins image-owned", async () => {
  listImages.mockResolvedValue({
    images: [
      imageSummary({ name: "worker", tag: "v1", bare: false }),
      imageSummary({ name: "worker", tag: "v2", bare: false }),
    ],
    count: 2,
  });
  imageManifestGet.mockImplementation(async (ref) =>
    ref === "worker:v2"
      ? manifest({
          schema_version: 2,
          tag: "v2",
          plugins: [{ name: "image-plugin" }],
        })
      : manifest(),
  );
  renderDialog({
    imageRef: undefined,
    cloneSource: { hostId: "d1", agentName: "source", hostLabel: "prod" },
  });

  const pluginInput = await screen.findByRole("textbox", {
    name: "plugin name",
  });
  fireEvent.change(pluginInput, { target: { value: "draft-plugin" } });
  fireEvent.click(screen.getByRole("button", { name: "Add plugin" }));
  expect(screen.getByText("draft-plugin")).toBeInTheDocument();

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "worker:v2" }));
  expect(await screen.findByText("image-plugin")).toBeInTheDocument();
  expect(screen.getByText(/plugins are owned by this schema-v2 image/i)).toBeInTheDocument();
  expect(screen.queryByRole("textbox", { name: "plugin name" })).not.toBeInTheDocument();
  expect(screen.queryByText("draft-plugin")).not.toBeInTheDocument();

  fireEvent.change(screen.getByLabelText("name"), {
    target: { value: "copy" },
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
  await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
  expect(createAgent).toHaveBeenCalledWith(
    expect.not.objectContaining({ plugins: expect.anything() }),
    targetFor("d1"),
  );
});

it("keeps Interactive on for a schema-v2 image after a bare image was selected", async () => {
  listImages.mockResolvedValue({
    images: [
      imageSummary({ name: "bare", tag: "latest", bare: true }),
      imageSummary({ name: "worker", tag: "v2", bare: false }),
    ],
    count: 2,
  });
  imageManifestGet.mockImplementation(async (ref) =>
    ref === "worker:v2"
      ? manifest({ schema_version: 2, tag: "v2" })
      : manifest({
          name: "bare",
          tag: "latest",
          bare: true,
          harness: { type: "codex", interactive: true },
        }),
  );
  renderDialog({ imageRef: "bare:latest" });
  expect(await screen.findByText("Terminal only")).toBeInTheDocument();
  expect(screen.getByRole("switch", { name: "Interactive" })).toBeChecked();

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "worker:v2" }));
  await waitFor(() => expect(screen.queryByText("Terminal only")).not.toBeInTheDocument());
  expect(screen.getByRole("switch", { name: "Interactive" })).toBeChecked();
});

it("rejects invalid environment JSON before creation", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("environment JSON"), {
    target: { value: '{"COUNT": 3}' },
  });
  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Environment must be a JSON object whose values are strings",
  );
  expect(createAgent).not.toHaveBeenCalled();
});

it("rejects non-integral numeric configuration before creation", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("interval seconds"), {
    target: { value: "1.5" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Interval seconds must be a whole number of at least 0",
  );
  expect(createAgent).not.toHaveBeenCalled();
});

describe.each([
  { startNow: false, interactive: false },
  { startNow: false, interactive: true },
  { startNow: true, interactive: false },
  { startNow: true, interactive: true },
])("creation matrix $startNow/$interactive", ({ startNow, interactive }) => {
  it("creates stopped without a loop flag and starts only on request", async () => {
    const { onCreated } = renderDialog();
    await screen.findByDisplayValue("worker:v1");
    await waitFor(() => expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled());
    await setSwitch("Start now", startNow);
    await setSwitch("Interactive", interactive);

    fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

    await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
    expect(createAgent).toHaveBeenCalledWith(
      completeOrdinarySpec({ interactive }),
      targetFor("d1"),
    );
    expect(createAgent.mock.calls[0][0]).not.toHaveProperty("loop");
    if (startNow) {
      await waitFor(() => expect(startAgent).toHaveBeenCalledWith("created", targetFor("d1")));
    } else {
      expect(startAgent).not.toHaveBeenCalled();
    }
    expect(onCreated).toHaveBeenCalledWith("d1", "created");
  });
});
