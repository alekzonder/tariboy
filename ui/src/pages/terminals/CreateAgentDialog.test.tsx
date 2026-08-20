import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { CreateAgentDialog } from "./CreateAgentDialog";
import {
  createAgent,
  imageManifestGet,
  listImages,
  startAgent,
  type ImageManifest,
} from "@/lib/api";
import { targetFor } from "@/lib/terminalsHost";
import { resolveDaemon } from "@/lib/daemons";
import { RUNTIME_PRESETS_STORAGE_KEY } from "@/lib/runtimePresets";

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    createAgent: vi.fn(),
    imageManifestGet: vi.fn(),
    listImages: vi.fn(),
    startAgent: vi.fn(),
  };
});

vi.mock("@/lib/daemons", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/daemons")>();
  return { ...actual, resolveDaemon: vi.fn() };
});

vi.mock("@/components/PathAutocomplete", () => ({
  PathAutocomplete: ({
    value,
    onChange,
    id,
    daemon,
    "aria-label": ariaLabel,
  }: {
    value: string;
    onChange: (value: string) => void;
    id?: string;
    daemon?: { id?: string } | null;
    "aria-label"?: string;
  }) => (
    <input
      id={id}
      aria-label={ariaLabel}
      data-host={daemon?.id ?? "local"}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}));

const hosts = [
  { id: "", label: "local" },
  { id: "d1", label: "prod" },
];

const manifest = (overrides: Partial<ImageManifest> = {}): ImageManifest => ({
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
  evals: null,
  layers: null,
  ...overrides,
	 skills: overrides.skills ?? null,
});

function renderDialog(props: Partial<ComponentProps<typeof CreateAgentDialog>> = {}) {
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

async function setSwitch(name: string, value: boolean) {
  const control = await screen.findByRole("switch", { name });
  if (control.getAttribute("aria-checked") === String(!value)) fireEvent.click(control);
}

async function selectHarness(value: string) {
  const control = screen.getByRole("combobox", { name: "harness" });
  await waitFor(() => expect(control).toBeEnabled());
  fireEvent.change(control, { target: { value } });
  expect(control).toHaveValue(value);
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  vi.mocked(resolveDaemon).mockImplementation(async (id) => targetFor(id));
  vi.mocked(listImages).mockResolvedValue({
    images: [
      { name: "worker", tag: "v1", bare: false },
      { name: "bare", tag: "latest", bare: true },
    ],
    count: 2,
  });
  vi.mocked(imageManifestGet).mockResolvedValue(manifest());
  vi.mocked(createAgent).mockResolvedValue({ name: "created", state: "stopped" });
  vi.mocked(startAgent).mockResolvedValue({ name: "created", action: "start" });
});

afterEach(() => vi.restoreAllMocks());

describe.each([
  { startNow: false, interactive: false, autopilot: false },
  { startNow: false, interactive: false, autopilot: true },
  { startNow: false, interactive: true, autopilot: false },
  { startNow: false, interactive: true, autopilot: true },
  { startNow: true, interactive: false, autopilot: false },
  { startNow: true, interactive: false, autopilot: true },
  { startNow: true, interactive: true, autopilot: false },
  { startNow: true, interactive: true, autopilot: true },
])("creation matrix $startNow/$interactive/$autopilot", ({ startNow, interactive, autopilot }) => {
  it("creates stopped with explicit independent controls and starts only on request", async () => {
    const { onCreated } = renderDialog();
    await screen.findByDisplayValue("worker:v1");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled(),
    );
    await setSwitch("Start now", startNow);
    await setSwitch("Interactive", interactive);
    await setSwitch("Autopilot", autopilot);

    fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

    await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
    expect(createAgent).toHaveBeenCalledWith(
      { image: "worker:v1", interactive, loop: autopilot },
      targetFor("d1"),
    );
    if (startNow) {
      await waitFor(() =>
        expect(startAgent).toHaveBeenCalledWith("created", targetFor("d1")),
      );
    } else {
      expect(startAgent).not.toHaveBeenCalled();
    }
    expect(onCreated).toHaveBeenCalledWith("d1", "created");
  });
});

it("uses a newly selected host for image/default loading, cwd, creation, and start", async () => {
  renderDialog();

  expect(await screen.findByLabelText("cwd")).toHaveAttribute("data-host", "d1");
  expect(listImages).toHaveBeenCalledWith(targetFor("d1"));
  expect(imageManifestGet).toHaveBeenCalledWith("worker:v1", targetFor("d1"));

  fireEvent.click(screen.getByRole("combobox", { name: "Host" }));
  fireEvent.click(await screen.findByRole("option", { name: "local" }));
  await waitFor(() => expect(listImages).toHaveBeenLastCalledWith(null));
  expect(screen.getByLabelText("cwd")).toHaveAttribute("data-host", "local");
  expect(screen.getByLabelText("image")).toHaveValue("");

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "worker:v1" }));
  await waitFor(() =>
    expect(imageManifestGet).toHaveBeenLastCalledWith("worker:v1", null),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
  await waitFor(() => expect(createAgent).toHaveBeenCalled());
  expect(createAgent).toHaveBeenCalledWith(expect.any(Object), null);
  expect(startAgent).toHaveBeenCalledWith("created", null);
});

it("retries a cold remote Run Agent selection when the host becomes ready", async () => {
  vi.mocked(resolveDaemon)
    .mockRejectedValueOnce(new Error("host d1 is not ready"))
    .mockResolvedValueOnce(targetFor("d1"));
  const props: ComponentProps<typeof CreateAgentDialog> = {
    open: true,
    hostId: "d1",
    imageRef: "worker:v1",
    hosts: [{ id: "d1", label: "prod", revision: "pending" }],
    onOpenChange: vi.fn(),
    onCreated: vi.fn(),
  };
  const view = render(<CreateAgentDialog {...props} />);

  expect(await screen.findByRole("alert")).toHaveTextContent(/host d1 is not ready/i);
  expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();

  view.rerender(
    <CreateAgentDialog
      {...props}
      hosts={[{ id: "d1", label: "prod", revision: "ready:http://127.0.0.1:19000" }]}
    />,
  );

  await waitFor(() =>
    expect(resolveDaemon).toHaveBeenCalledTimes(2),
  );
  expect(listImages).toHaveBeenLastCalledWith(targetFor("d1"));
  expect(imageManifestGet).toHaveBeenLastCalledWith("worker:v1", targetFor("d1"));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled(),
  );
});

it("shows image defaults as editable values but omits untouched overrides", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");

  expect(screen.getByLabelText("harness")).toHaveValue("codex");
  expect(screen.getByLabelText("model")).toHaveValue("o3");
  expect(screen.getByLabelText("effort")).toHaveValue("high");
  fireEvent.focus(screen.getByLabelText("model"));
  expect(screen.getByRole("option", { name: "gpt-5" })).toBeVisible();
  expect(screen.getByRole("option", { name: "o3" })).toBeVisible();
  fireEvent.blur(screen.getByLabelText("model"));

  fireEvent.click(screen.getByRole("button", { name: "Advanced overrides" }));
  expect(screen.getByLabelText("env (K=V,K=V)")).toBeVisible();

  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
  await waitFor(() => expect(createAgent).toHaveBeenCalled());
  expect(createAgent).toHaveBeenCalledWith(
    { image: "worker:v1", interactive: false, loop: true },
    targetFor("d1"),
  );
});

it("sends only image defaults that the user overrides", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  await selectHarness("claude");
  fireEvent.change(screen.getByLabelText("model"), { target: { value: "opus" } });
  fireEvent.change(screen.getByLabelText("effort"), { target: { value: "medium" } });
  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

  await waitFor(() => expect(createAgent).toHaveBeenCalled());
  expect(createAgent).toHaveBeenCalledWith(
    {
      image: "worker:v1",
      harness: "claude",
      model: "opus",
      effort: "medium",
      interactive: false,
      loop: true,
    },
    targetFor("d1"),
  );
});

it("changes suggestions with harness without clearing a custom model", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "private-model" },
  });

  await selectHarness("claude");

  expect(screen.getByLabelText("model")).toHaveValue("private-model");
  fireEvent.focus(screen.getByLabelText("model"));
  expect(screen.getByRole("option", { name: "claude-opus-4-8" })).toBeVisible();
  expect(screen.getByRole("option", { name: "private-model" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "gpt-5" })).not.toBeInTheDocument();
});

it("keeps runtime drafts when the same selected image is reloaded", async () => {
  const { rerender } = renderDialog({
    hosts: [
      { id: "", label: "local" },
      { id: "d1", label: "prod", revision: "initial" },
    ],
  });
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), { target: { value: "private-model" } });
  fireEvent.change(screen.getByLabelText("effort"), { target: { value: "ultra-custom" } });

  rerender(
    <CreateAgentDialog
      open
      hostId="d1"
      imageRef="worker:v1"
      hosts={[
        { id: "", label: "local" },
        { id: "d1", label: "prod", revision: "ready" },
      ]}
      onOpenChange={vi.fn()}
      onCreated={vi.fn()}
    />,
  );

  await waitFor(() => expect(imageManifestGet).toHaveBeenCalledTimes(2));
  expect(screen.getByLabelText("model")).toHaveValue("private-model");
  expect(screen.getByLabelText("effort")).toHaveValue("ultra-custom");
});

it("initializes defaults again when the same image is selected on another host", async () => {
  vi.mocked(imageManifestGet)
    .mockResolvedValueOnce(manifest())
    .mockResolvedValueOnce(manifest({
      harness: { type: "claude", model: "claude-opus-4-8", effort: "max", interactive: true },
    }));
  renderDialog({
    hosts: [
      { id: "", label: "local" },
      { id: "d1", label: "prod" },
      { id: "d2", label: "staging" },
    ],
  });
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), { target: { value: "private-model" } });

  fireEvent.click(screen.getByRole("combobox", { name: "Host" }));
  fireEvent.click(await screen.findByRole("option", { name: "staging" }));
  await waitFor(() => expect(screen.getByLabelText("image")).toHaveValue(""));
  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(await screen.findByRole("option", { name: "worker:v1" }));

  expect(await screen.findByLabelText("harness")).toHaveValue("claude");
  expect(screen.getByLabelText("model")).toHaveValue("claude-opus-4-8");
  expect(screen.getByLabelText("effort")).toHaveValue("max");
});

it("remembers unknown model and effort after successful creation", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "private-model" },
  });
  fireEvent.change(screen.getByLabelText("effort"), {
    target: { value: "ultra-custom" },
  });

  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
  await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());

  expect(
    JSON.parse(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY) ?? "{}"),
  ).toEqual({
    codex: {
      models: ["private-model"],
      efforts: ["ultra-custom"],
    },
  });
});

it("does not remember custom runtime values when creation fails", async () => {
  vi.mocked(createAgent).mockRejectedValueOnce(new Error("rejected"));
  renderDialog();
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "private-model" },
  });
  fireEvent.change(screen.getByLabelText("effort"), {
    target: { value: "ultra-custom" },
  });

  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("rejected");
  expect(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY)).toBeNull();
});

describe.each([{ startNow: false }, { startNow: true }])(
  "bare terminal session with startNow=$startNow",
  ({ startNow }) => {
    it("keeps bare selected, locks its modes, and supports stopped or started creation", async () => {
      vi.mocked(imageManifestGet).mockResolvedValue(manifest({
        name: "bare",
        tag: "latest",
        bare: true,
        harness: { type: "claude", interactive: false },
      }));
      renderDialog({ imageRef: "bare:latest" });

      expect(await screen.findByText("Terminal only")).toBeInTheDocument();
      expect(screen.getByLabelText("image")).toHaveValue("bare:latest");
      expect(screen.getByRole("switch", { name: "Interactive" })).toBeChecked();
      expect(screen.getByRole("switch", { name: "Interactive" })).toBeDisabled();
      expect(screen.getByRole("switch", { name: "Autopilot" })).not.toBeChecked();
      expect(screen.getByRole("switch", { name: "Autopilot" })).toBeDisabled();
      expect(screen.getByText(/does not contain agent instructions or tools/i))
        .toBeInTheDocument();
      await setSwitch("Start now", startNow);

      fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
      await waitFor(() => expect(createAgent).toHaveBeenCalled());
      expect(createAgent).toHaveBeenCalledWith(
        { image: "bare:latest", interactive: true, loop: false },
        targetFor("d1"),
      );
      if (startNow) {
        expect(startAgent).toHaveBeenCalledWith("created", targetFor("d1"));
      } else {
        expect(startAgent).not.toHaveBeenCalled();
      }
    });
  },
);

it("keeps the created stopped agent and retries only start after a start failure", async () => {
  vi.mocked(startAgent)
    .mockRejectedValueOnce(new Error("daemon unavailable"))
    .mockResolvedValueOnce({ name: "created", action: "start" });
  const { onCreated, onOpenChange } = renderDialog();
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "private-model" },
  });

  const create = screen.getByRole("button", { name: "Create agent" });
  await waitFor(() => expect(create).toBeEnabled());
  fireEvent.click(create);

  expect(await screen.findByRole("alert")).toHaveTextContent(
    /agent created but could not be started.*daemon unavailable/i,
  );
  expect(onCreated).toHaveBeenCalledWith("d1", "created");
  expect(onOpenChange).not.toHaveBeenCalledWith(false);
  expect(
    JSON.parse(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY) ?? "{}"),
  ).toMatchObject({ codex: { models: ["private-model"] } });

  fireEvent.click(screen.getByRole("button", { name: "Retry start" }));
  await waitFor(() => expect(startAgent).toHaveBeenCalledTimes(2));
  expect(createAgent).toHaveBeenCalledOnce();
  expect(onOpenChange).toHaveBeenCalledWith(false);
});
