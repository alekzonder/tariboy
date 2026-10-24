import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import type { CreateAgentResult, CreateAgentSpec, ImageManifest, ImageRow } from "@/lib/api";
import type { AgentLifecycleResult, AgentView, HarnessCatalog } from "@/lib/types";
import type { Daemon } from "@/lib/daemons";
import { RUNTIME_PRESETS_STORAGE_KEY } from "@/lib/runtimePresets";
import { targetFor } from "@/lib/terminalsHost";
import { CreateAgentDialog } from "./CreateAgentDialog";
import { useApi } from "@/components/apiContext";
import {
  setupCreateAgentDialog,
  manifest,
  completeOrdinarySpec,
  renderDialog,
  setSwitch,
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
const { createAgent, imageManifestGet, startAgent } = calls;

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

describe.each([{ startNow: false }, { startNow: true }])(
  "bare terminal session with startNow=$startNow",
  ({ startNow }) => {
    it("keeps bare selected, locks its modes, and supports stopped or started creation", async () => {
      imageManifestGet.mockResolvedValue(
        manifest({
          name: "bare",
          tag: "latest",
          bare: true,
          harness: { type: "claude", interactive: false },
        }),
      );
      renderDialog({ imageRef: "bare:latest" });

      expect(await screen.findByText("Terminal only")).toBeInTheDocument();
      expect(screen.getByLabelText("image")).toHaveValue("bare:latest");
      expect(screen.getByRole("switch", { name: "Interactive" })).toBeChecked();
      expect(screen.getByRole("switch", { name: "Interactive" })).toBeDisabled();
      expect(screen.queryByRole("switch", { name: "Autopilot" })).not.toBeInTheDocument();
      expect(screen.getByText(/does not contain agent instructions or tools/i)).toBeInTheDocument();
      await setSwitch("Start now", startNow);

      fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
      await waitFor(() => expect(createAgent).toHaveBeenCalled());
      expect(createAgent).toHaveBeenCalledWith(
        completeOrdinarySpec({
          image: "bare:latest",
          harness: "claude",
          model: "",
          effort: "",
          interactive: true,
        }),
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
  startAgent
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
  expect(JSON.parse(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY) ?? "{}")).toMatchObject({
    codex: { models: ["private-model"] },
  });

  fireEvent.click(screen.getByRole("button", { name: "Retry start" }));
  await waitFor(() => expect(startAgent).toHaveBeenCalledTimes(2));
  expect(createAgent).toHaveBeenCalledOnce();
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

describe("Create agent blocker", () => {
  const clone = {
    imageRef: undefined,
    cloneSource: { hostId: "d1", agentName: "source", hostLabel: "prod" },
  };

  it("names a pending image manifest next to the disabled button", async () => {
    imageManifestGet.mockReturnValue(new Promise(() => {}));
    renderDialog(clone);

    expect(await screen.findByText("Loading image worker:v1 on prod…")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();
  });

  it("keeps a manifest failure visible across an image reload and retries it", async () => {
    imageManifestGet.mockRejectedValue(new Error("tunnel closed"));
    const props: ComponentProps<typeof CreateAgentDialog> = {
      open: true,
      hostId: "d1",
      hosts: [{ id: "d1", label: "prod", revision: "r1" }],
      onOpenChange: vi.fn(),
      onCreated: vi.fn(),
      ...clone,
    };
    const view = render(<CreateAgentDialog {...props} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /Could not load image worker:v1: .*tunnel closed/,
    );
    view.rerender(
      <CreateAgentDialog {...props} hosts={[{ id: "d1", label: "prod", revision: "r2" }]} />,
    );
    await waitFor(() => expect(imageManifestGet).toHaveBeenCalledTimes(2));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /Could not load image worker:v1: .*tunnel closed/,
    );
    expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();

    imageManifestGet.mockResolvedValue(manifest());
    fireEvent.click(screen.getByRole("button", { name: "Retry image" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Create agent" })).toBeEnabled());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("asks for an image when none is selected", async () => {
    renderDialog({ imageRef: "" });

    expect(await screen.findByText("Choose an image")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create agent" })).toBeDisabled();
  });
});

it("sends create_cwd only when the create directory switch is on", async () => {
  renderDialog();
  await screen.findByDisplayValue("o3");
  const createCwd = screen.getByRole("switch", {
    name: "Create directory if missing",
  });
  expect(createCwd).not.toBeChecked();
  expect(createCwd).toBeDisabled();

  fireEvent.change(screen.getByLabelText("cwd"), {
    target: { value: "/work/new" },
  });
  expect(createCwd).toBeEnabled();
  fireEvent.click(createCwd);
  expect(createCwd).toBeChecked();
  fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

  await waitFor(() => expect(createAgent).toHaveBeenCalled());
  expect(createAgent).toHaveBeenCalledWith(
    completeOrdinarySpec({ cwd: "/work/new", create_cwd: true }),
    targetFor("d1"),
  );
});
