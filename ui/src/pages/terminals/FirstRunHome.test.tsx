import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import type {
  AgentActionResult,
  AgentRunResult,
  AgentView,
  CreateAgentSpec,
  HarnessModelCatalog,
  ImageManifest,
  ImageSummary,
  OnboardingStatus,
} from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { manifest, setupCreateAgentDialog } from "@/test/terminals/createAgentDialog";
import { imageSummary } from "@/test/fixtures/apiFixtures";
import { FirstRunHome } from "./FirstRunHome";

const calls = vi.hoisted(() => ({
  agentGetOn: vi.fn<(target: Daemon | null, name: string, action: string) => Promise<AgentView>>(),
  createAgent: vi.fn<(spec: CreateAgentSpec, target: Daemon | null) => Promise<AgentRunResult>>(),
  getHarnessModelsOn:
    vi.fn<(target: Daemon | null, harness: string) => Promise<HarnessModelCatalog>>(),
  imageManifestGet: vi.fn<(ref: string, target: Daemon | null) => Promise<ImageManifest>>(),
  listImages:
    vi.fn<(target: Daemon | null) => Promise<{ images: ImageSummary[]; count: number }>>(),
  startAgent: vi.fn<(name: string, target: Daemon | null) => Promise<AgentActionResult>>(),
  onboardingStatus: vi.fn<() => Promise<OnboardingStatus>>(),
  onboardingRetry: vi.fn<() => Promise<OnboardingStatus>>(),
}));
const { createAgent, startAgent, listImages, imageManifestGet, onboardingStatus, onboardingRetry } =
  calls;

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  const createApi: typeof actual.createApi = (target) => {
    const client = actual.createApi(target);
    const host = client.target;
    return {
      ...client,
      agents: {
        ...client.agents,
        createAgent: (spec: CreateAgentSpec) => calls.createAgent(spec, host),
        startAgent: (name: string) => calls.startAgent(name, host),
      },
      files: {
        ...client.files,
        fsList: async () => ({ path: "/", parent: "", entries: [{ name: "my-app", dir: true }] }),
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
      onboarding: { getStatus: calls.onboardingStatus, retry: calls.onboardingRetry },
    } as typeof client;
  };
  return { ...actual, createApi };
});

vi.mock("@/lib/daemons", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/daemons")>();
  return { ...actual, resolveDaemon: vi.fn() };
});

vi.mock("@/components/PathAutocomplete", () => ({
  PathAutocomplete: ({
    value,
    onChange,
    "aria-label": ariaLabel,
  }: {
    value: string;
    onChange: (value: string) => void;
    "aria-label"?: string;
  }) => <input aria-label={ariaLabel} value={value} onChange={(e) => onChange(e.target.value)} />,
}));

const status = (store: string, storeError?: string): OnboardingStatus => ({
  agents: 0,
  default_store: { state: store, name: "tariboy", source: "s", error: storeError },
  starter_image: { state: store === "registered" ? "ready" : "pending" },
  harnesses: [
    { type: "claude", found: true, install: "" },
    { type: "codex", found: false, install: "npm i -g @openai/codex" },
  ],
});

const renderHome = () => {
  const onCreated = vi.fn();
  render(
    <FirstRunHome
      hostId=""
      hosts={[{ id: "", label: "local", agents: [] }]}
      onCreated={onCreated}
    />,
  );
  return { onCreated };
};

beforeEach(() => {
  setupCreateAgentDialog(calls);
  listImages.mockResolvedValue({
    images: [
      imageSummary({ name: "basic", tag: "latest" }),
      imageSummary({ name: "bare", tag: "latest", bare: true }),
    ],
    count: 2,
  });
  imageManifestGet.mockImplementation(async (ref) =>
    ref.startsWith("bare")
      ? manifest({ name: "bare", tag: "latest", bare: true, harness: undefined })
      : manifest({ name: "basic", tag: "latest", harness: { type: "claude", interactive: true } }),
  );
  onboardingStatus.mockResolvedValue(status("registered"));
});

afterEach(() => vi.restoreAllMocks());

it("starts the first agent with one folder and one click", async () => {
  const { onCreated } = renderHome();
  expect(screen.getByRole("heading", { name: "Welcome to Tariboy" })).toBeInTheDocument();
  const ready = await screen.findByRole("list", { name: "Getting ready" });
  expect(within(ready).getByText("Tariboy catalog connected")).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Run your first agent" })).toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText("image")).toHaveValue("basic"));
  await waitFor(() => expect(screen.getByRole("radio", { name: "Claude Code" })).toBeChecked());

  fireEvent.change(screen.getByLabelText("folder"), { target: { value: "~/code/my-app" } });
  expect(screen.getByLabelText("name")).toHaveValue("my-app-basic");
  const start = screen.getByRole("button", { name: "Start agent" });
  await waitFor(() => expect(start).toBeEnabled());
  expect(start).toHaveAttribute("data-tour", "new-agent");
  fireEvent.click(start);

  await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
  expect(createAgent.mock.calls[0][0]).toMatchObject({
    image: "basic:latest",
    cwd: "~/code/my-app",
    harness: "claude",
    name: "my-app-basic",
    interactive: true,
  });
  await waitFor(() => expect(startAgent).toHaveBeenCalledWith("created", null));
  expect(onCreated).toHaveBeenCalledWith("", "created");
});

it("explains a failed catalog with Details and Retry and still starts a plain terminal", async () => {
  onboardingStatus.mockResolvedValue(status("failed", "git clone: network is unreachable"));
  onboardingRetry.mockResolvedValue(status("pending"));
  // The daemon's seeded bare:latest does not carry the bare flag.
  listImages.mockResolvedValue({
    images: [imageSummary({ name: "bare", tag: "latest", bare: false })],
    count: 1,
  });
  imageManifestGet.mockResolvedValue(manifest({ name: "bare", tag: "latest", harness: undefined }));
  renderHome();
  expect(await screen.findByText("Could not connect the Tariboy catalog")).toBeInTheDocument();
  expect(screen.getByText("Details")).toBeInTheDocument();
  expect(screen.getByText("git clone: network is unreachable")).toBeInTheDocument();
  expect(screen.getAllByRole("list", { name: "Getting ready" })).toHaveLength(1);

  await waitFor(() => expect(screen.getByLabelText("image")).toHaveValue("Plain terminal (bare)"));
  await waitFor(() =>
    expect(screen.queryByRole("radiogroup", { name: "Coding tool" })).not.toBeInTheDocument(),
  );
  fireEvent.change(screen.getByLabelText("folder"), { target: { value: "/my-app" } });
  const start = screen.getByRole("button", { name: "Start agent" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.click(start);
  await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
  expect(createAgent.mock.calls[0][0]).toMatchObject({ image: "bare:latest", name: "my-app-bare" });

  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await waitFor(() => expect(onboardingRetry).toHaveBeenCalledOnce());
  expect(await screen.findByText("Connecting the Tariboy catalog…")).toBeInTheDocument();
});
