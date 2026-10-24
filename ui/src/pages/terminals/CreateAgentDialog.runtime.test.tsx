import type { ComponentProps } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import type { CreateAgentResult, CreateAgentSpec, ImageManifest, ImageRow } from "@/lib/api";
import type { AgentLifecycleResult, AgentView, HarnessCatalog } from "@/lib/types";
import { resolveDaemon, type Daemon } from "@/lib/daemons";
import { RUNTIME_PRESETS_STORAGE_KEY } from "@/lib/runtimePresets";
import { targetFor } from "@/lib/terminalsHost";
import { CreateAgentDialog } from "./CreateAgentDialog";
import { useApi } from "@/components/apiContext";
import {
  setupCreateAgentDialog,
  manifest,
  completeOrdinarySpec,
  renderDialog,
  selectHarness,
} from "@/test/terminals/createAgentDialog";

// The dialog's requests, each recorded with the host of the client that sent
// it, so a test can assert which host a request was pinned to.
const calls = vi.hoisted(() => ({
  agentGetOn: vi.fn<(target: Daemon | null, name: string, action: string) => Promise<AgentView>>(),
  createAgent:
    vi.fn<(spec: CreateAgentSpec, target: Daemon | null) => Promise<CreateAgentResult>>(),
  getHarnessModelsOn: vi.fn<(target: Daemon | null, harness: string) => Promise<HarnessCatalog>>(),
  imageManifestGet: vi.fn<(ref: string, target: Daemon | null) => Promise<ImageManifest>>(),
  listImages: vi.fn<(target: Daemon | null) => Promise<{ images: ImageRow[]; count: number }>>(),
  startAgent: vi.fn<(name: string, target: Daemon | null) => Promise<AgentLifecycleResult>>(),
}));
const { createAgent, getHarnessModelsOn, imageManifestGet, listImages, startAgent } = calls;

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
  await waitFor(() => expect(imageManifestGet).toHaveBeenLastCalledWith("worker:v1", null));
  await waitFor(() => expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled());
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

  await waitFor(() => expect(resolveDaemon).toHaveBeenCalledTimes(2));
  expect(listImages).toHaveBeenLastCalledWith(targetFor("d1"));
  expect(imageManifestGet).toHaveBeenLastCalledWith("worker:v1", targetFor("d1"));
  await waitFor(() => expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled());
});

it("shows image defaults as editable values and submits the complete defaults", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");

  expect(screen.getByLabelText("harness")).toHaveValue("codex");
  expect(screen.getByLabelText("model")).toHaveValue("o3");
  expect(screen.getByLabelText("effort")).toHaveValue("high");
  fireEvent.focus(screen.getByLabelText("model"));
  expect(await screen.findByRole("option", { name: "gpt-6-astra" })).toBeVisible();
  expect(screen.getByRole("group", { name: "From harness" })).toContainElement(
    screen.getByRole("option", { name: "gpt-5.6-luna" }),
  );
  expect(screen.getByRole("group", { name: "Saved" })).toContainElement(
    screen.getByRole("option", { name: "o3" }),
  );
  expect(getHarnessModelsOn).toHaveBeenCalledWith(targetFor("d1"), "codex");
  fireEvent.blur(screen.getByLabelText("model"));

  expect(screen.getByLabelText("environment JSON")).toHaveValue("{}");
  // The manifest says interactive: false, but new agents default to interactive.
  expect(screen.getByRole("switch", { name: "Interactive" })).toBeChecked();

  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
  await waitFor(() => expect(createAgent).toHaveBeenCalled());
  expect(createAgent).toHaveBeenCalledWith(completeOrdinarySpec(), targetFor("d1"));
});

it("submits edited runtime values with the complete configuration", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  await selectHarness("claude");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "opus" },
  });
  fireEvent.change(screen.getByLabelText("effort"), {
    target: { value: "medium" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

  await waitFor(() => expect(createAgent).toHaveBeenCalled());
  expect(createAgent).toHaveBeenCalledWith(
    completeOrdinarySpec({
      harness: "claude",
      model: "opus",
      effort: "medium",
    }),
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
  expect(await screen.findByRole("option", { name: "opus[1m]" })).toBeVisible();
  expect(screen.getByRole("group", { name: "Saved" })).toContainElement(
    screen.getByRole("option", { name: "private-model" }),
  );
  expect(screen.queryByRole("option", { name: "gpt-6-astra" })).not.toBeInTheDocument();
});

it("offers the efforts of the selected harness model", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "gpt-5.6-luna" },
  });
  fireEvent.blur(screen.getByLabelText("model"));

  fireEvent.focus(screen.getByLabelText("effort"));

  const harness = await screen.findByRole("group", { name: "From harness" });
  expect(harness).toContainElement(screen.getByRole("option", { name: "max" }));
  expect(screen.queryByRole("option", { name: "ultra" })).not.toBeInTheDocument();
});

it("keeps runtime drafts when the same selected image is reloaded", async () => {
  const { rerender } = renderDialog({
    hosts: [
      { id: "", label: "local" },
      { id: "d1", label: "prod", revision: "initial" },
    ],
  });
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "private-model" },
  });
  fireEvent.change(screen.getByLabelText("effort"), {
    target: { value: "ultra-custom" },
  });

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
  imageManifestGet.mockResolvedValueOnce(manifest()).mockResolvedValueOnce(
    manifest({
      harness: {
        type: "claude",
        model: "claude-opus-4-8",
        effort: "max",
        interactive: true,
      },
    }),
  );
  renderDialog({
    hosts: [
      { id: "", label: "local" },
      { id: "d1", label: "prod" },
      { id: "d2", label: "staging" },
    ],
  });
  await screen.findByDisplayValue("o3");
  fireEvent.change(screen.getByLabelText("model"), {
    target: { value: "private-model" },
  });

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

  expect(JSON.parse(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY) ?? "{}")).toEqual({
    codex: {
      models: ["private-model"],
      efforts: ["ultra-custom"],
    },
  });
});

it("does not remember custom runtime values when creation fails", async () => {
  createAgent.mockRejectedValueOnce(new Error("rejected"));
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
