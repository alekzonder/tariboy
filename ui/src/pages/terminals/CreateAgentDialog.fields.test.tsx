import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@/test/queryRender";
import type {
  AgentActionResult,
  AgentRunResult,
  AgentView,
  CreateAgentSpec,
  FsListing,
  HarnessModelCatalog,
  ImageManifest,
  ImageSummary,
  OnboardingStatus,
} from "@/types";
import type { Daemon } from "@/lib/daemons";
import {
  setupCreateAgentDialog,
  cloneSource,
  hosts,
  manifest,
  openAdvanced,
  renderDialog,
} from "@/test/terminals/createAgentDialog";
import { imageSummary } from "@/test/fixtures/apiFixtures";

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
  fsList: vi.fn<(path: string) => Promise<FsListing>>(),
}));
const { createAgent, listImages, onboardingStatus, fsList } = calls;

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
      files: { ...client.files, fsList: (path: string) => calls.fsList(path) },
      harnesses: {
        ...client.harnesses,
        getHarnessModels: (harness: string) => calls.getHarnessModelsOn(host, harness),
      },
      images: {
        ...client.images,
        listImages: () => calls.listImages(host),
        imageManifestGet: (ref: string) => calls.imageManifestGet(ref, host),
      },
      onboarding: { ...client.onboarding, getStatus: () => calls.onboardingStatus() },
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
    id,
    "aria-label": ariaLabel,
  }: {
    value: string;
    onChange: (value: string) => void;
    id?: string;
    "aria-label"?: string;
  }) => (
    <input
      id={id}
      aria-label={ariaLabel}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}));

const status = (found: string[], starter = "ready", store = "registered"): OnboardingStatus => ({
  agents: 0,
  default_store: { state: store, name: "tariboy", source: "s" },
  starter_image: { state: starter },
  harnesses: ["claude", "codex", "opencode", "cursor"].map((type) => ({
    type,
    found: found.includes(type),
    install: `npm i -g ${type}`,
  })),
});

const startButton = () => screen.getByRole("button", { name: "Start agent" });

beforeEach(() => {
  setupCreateAgentDialog(calls);
  onboardingStatus.mockResolvedValue(status(["claude", "codex"]));
  fsList.mockResolvedValue({ path: "/work", parent: "/", entries: [{ name: "app", dir: true }] });
});

afterEach(() => vi.restoreAllMocks());

it("shows only Folder, Image, Coding tool and Name on one host", async () => {
  renderDialog({ hosts: [hosts[1]], imageRef: undefined });
  await waitFor(() => expect(screen.getByLabelText("image")).toHaveValue("worker"));
  expect(screen.queryByLabelText("Run on")).not.toBeInTheDocument();
  expect(screen.getByLabelText("folder")).toBeInTheDocument();
  expect(screen.getByRole("radiogroup", { name: "Coding tool" })).toBeInTheDocument();
  expect(screen.getByLabelText("name")).toBeInTheDocument();
  expect(screen.queryByLabelText("alias")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("interval seconds")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Advanced" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
});

it("adds Run on with several hosts and keeps every other field under Advanced", async () => {
  renderDialog();
  expect(await screen.findByLabelText("Run on")).toBeInTheDocument();
  openAdvanced();
  for (const group of ["Identity", "Image", "Folder", "Coding tool", "Autopilot", "Tasks"]) {
    expect(screen.getByRole("region", { name: group })).toBeInTheDocument();
  }
  for (const label of [
    "alias",
    "team",
    "color",
    "notes",
    "version",
    "environment JSON",
    "interval seconds",
    "AI inactivity timeout seconds",
    "standing user prompt",
    "wait customer timeout seconds",
  ]) {
    expect(screen.getByLabelText(label)).toBeInTheDocument();
  }
  for (const name of [
    "Use a managed folder",
    "Headless (no terminal)",
    "Goal",
    "Create without starting",
  ]) {
    expect(screen.getByRole("switch", { name })).toBeInTheDocument();
  }
});

it("lists one image row per name with the newest tag, basic first and bare last", async () => {
  listImages.mockResolvedValue({
    images: [
      imageSummary({ name: "worker", tag: "v1", built_at: "2026-01-01T00:00:00Z" }),
      imageSummary({ name: "worker", tag: "v2", built_at: "2026-02-01T00:00:00Z" }),
      imageSummary({ name: "bare", tag: "latest", bare: true }),
      imageSummary({ name: "basic", tag: "latest", source: "tariboy/basic" }),
    ],
    count: 4,
  });
  renderDialog({ imageRef: undefined });
  await waitFor(() => expect(screen.getByLabelText("image")).toHaveValue("basic"));
  fireEvent.focus(screen.getByLabelText("image"));
  const options = within(screen.getByRole("listbox")).getAllByRole("option");
  expect(options.map((option) => option.getAttribute("aria-label"))).toEqual([
    "basic",
    "worker",
    "Plain terminal (bare)",
  ]);
  fireEvent.click(options[1]);
  openAdvanced();
  expect(screen.getByLabelText("version")).toHaveValue("v2");
});

it("keeps the form usable when the selected image is chosen again", async () => {
  renderDialog({ imageRef: undefined });
  fireEvent.change(screen.getByLabelText("folder"), { target: { value: "/work/app" } });
  await waitFor(() => expect(startButton()).toBeEnabled());
  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(screen.getByRole("option", { name: "worker" }));
  await waitFor(() => expect(startButton()).toBeEnabled());
});

it("pre-selects the detected coding tool over a missing image default", async () => {
  onboardingStatus.mockResolvedValue(status(["claude"]));
  renderDialog();
  await waitFor(() => expect(screen.getByRole("radio", { name: "Claude Code" })).toBeChecked());
  expect(screen.getByText("not found", { selector: "label:has(input[value=codex]) *" }));
});

it("blocks Start for a tool that is not installed and names the host", async () => {
  onboardingStatus.mockResolvedValue(status(["claude"]));
  renderDialog();
  fireEvent.change(screen.getByLabelText("folder"), { target: { value: "/work/app" } });
  await waitFor(() => expect(startButton()).toBeEnabled());
  fireEvent.click(screen.getByRole("radio", { name: "Codex" }));
  expect(await screen.findByText("Codex is not installed on prod")).toBeInTheDocument();
  expect(screen.getByText("To install it: npm i -g codex")).toBeInTheDocument();
  expect(startButton()).toBeDisabled();
});

it("requires a folder unless the managed folder is chosen", async () => {
  renderDialog();
  expect(await screen.findByText("Choose a folder")).toBeInTheDocument();
  openAdvanced();
  fireEvent.click(screen.getByRole("switch", { name: "Use a managed folder" }));
  await waitFor(() => expect(startButton()).toBeEnabled());
  expect(screen.getByLabelText("name")).toHaveValue("worker");
  fireEvent.click(startButton());
  await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
  expect(createAgent.mock.calls[0][0]).toMatchObject({ cwd: "", name: "worker" });
});

it("offers to create a missing folder and suggests a unique name", async () => {
  renderDialog({ hosts: [{ id: "d1", label: "prod", agents: ["new-worker"] }] });
  fireEvent.change(screen.getByLabelText("folder"), { target: { value: "/work/new" } });
  fireEvent.click(await screen.findByRole("button", { name: "Create it" }));
  expect(screen.getByText("The folder will be created.")).toBeInTheDocument();
  expect(fsList).toHaveBeenCalledWith("/work/");
  expect(screen.getByLabelText("name")).toHaveValue("new-worker-2");
  await waitFor(() => expect(startButton()).toBeEnabled());
  fireEvent.click(startButton());
  await waitFor(() => expect(createAgent).toHaveBeenCalledOnce());
  expect(createAgent.mock.calls[0][0]).toMatchObject({
    cwd: "/work/new",
    create_cwd: true,
    name: "new-worker-2",
  });
});

it("keeps the typed name when the folder changes", async () => {
  renderDialog();
  fireEvent.change(screen.getByLabelText("name"), { target: { value: "mine" } });
  fireEvent.change(screen.getByLabelText("folder"), { target: { value: "/work/app" } });
  await waitFor(() => expect(startButton()).toBeEnabled());
  expect(screen.getByLabelText("name")).toHaveValue("mine");
  expect(screen.queryByRole("button", { name: "Create it" })).not.toBeInTheDocument();
});

it("waits for the starter image only while it is chosen", async () => {
  onboardingStatus.mockResolvedValue(status(["claude"], "building"));
  listImages.mockResolvedValue({
    images: [imageSummary({ name: "bare", tag: "latest", bare: true })],
    count: 1,
  });
  renderDialog({ imageRef: undefined });
  await waitFor(() => expect(screen.getByLabelText("image")).toHaveValue("basic"));
  expect(screen.getByRole("list", { name: "Getting ready" })).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("folder"), { target: { value: "/work/app" } });
  expect(
    await screen.findByText("Waiting for the starter image…", { selector: "p" }),
  ).toBeVisible();
  expect(startButton()).toBeDisabled();

  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(screen.getByRole("option", { name: "Plain terminal (bare)" }));
  await waitFor(() => expect(startButton()).toBeEnabled());
});

it("opens Advanced in clone mode and suggests a free name", async () => {
  renderDialog({
    imageRef: undefined,
    hosts: [{ id: "d1", label: "prod", agents: ["source", "worker"] }],
    cloneSource: { hostId: "d1", agentName: "source", hostLabel: "prod" },
  });
  await waitFor(() => expect(screen.getByLabelText("alias")).toHaveValue(cloneSource.alias));
  expect(screen.getByRole("button", { name: "Advanced" })).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("switch", { name: "Use a managed folder" })).toBeChecked();
  expect(screen.getByLabelText("name")).toHaveValue("worker-2");
  expect(screen.getByRole("radio", { name: "Codex" })).toBeChecked();
});

const roleImages = () => {
  listImages.mockResolvedValue({
    images: [
      imageSummary({ name: "worker", tag: "v1" }),
      imageSummary({ name: "writer", tag: "v1" }),
      imageSummary({ name: "plain", tag: "v1" }),
      imageSummary({ name: "bare", tag: "latest", bare: true }),
    ],
    count: 4,
  });
  const roles: Record<string, string> = {
    worker: "# Reviewer\n\nYou **review** [pull requests](https://x.test).\n\n- Merge nothing",
    writer: "You write docs.",
    bare: "Never shown.",
  };
  calls.imageManifestGet.mockImplementation(async (ref) => {
    const name = ref.split(":")[0];
    return manifest({
      name,
      bare: name === "bare",
      ...(roles[name] ? { role: { text: roles[name] } } : {}),
    });
  });
};

const chooseImage = (name: string) => {
  fireEvent.focus(screen.getByLabelText("image"));
  fireEvent.click(screen.getByRole("option", { name }));
  fireEvent.blur(screen.getByLabelText("image"));
};

it("shows the selected image's role fragment under Image and opens the full role", async () => {
  roleImages();
  renderDialog();
  const fragment = await screen.findByText("You review pull requests.");
  expect(fragment).toHaveClass("line-clamp-2", "text-body", "text-muted-foreground");
  expect(screen.getByLabelText("image").getAttribute("aria-describedby")).toContain(fragment.id);
  expect(screen.queryByRole("heading", { name: "Reviewer" })).not.toBeInTheDocument();

  const show = screen.getByRole("button", { name: "Show role" });
  expect(show).toHaveAttribute("aria-expanded", "false");
  fireEvent.click(show);
  expect(show).toHaveAccessibleName("Hide role");
  expect(show).toHaveAttribute("aria-expanded", "true");
  const full = document.getElementById(show.getAttribute("aria-controls")!)!;
  expect(full).toHaveClass("max-h-60", "overflow-auto", "rounded-chip", "border", "p-3");
  expect(await within(full).findByRole("heading", { name: "Reviewer" })).toBeInTheDocument();

  fireEvent.click(show);
  expect(show).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("heading", { name: "Reviewer" })).not.toBeInTheDocument();
});

it("changes the fragment with the image and closes the full role", async () => {
  roleImages();
  renderDialog();
  fireEvent.click(await screen.findByRole("button", { name: "Show role" }));
  chooseImage("writer");
  expect(await screen.findByText("You write docs.")).toBeInTheDocument();
  expect(screen.queryByText("You review pull requests.")).not.toBeInTheDocument();
  // One short paragraph needs no full view.
  expect(screen.queryByRole("button", { name: /role$/ })).not.toBeInTheDocument();
});

it("adds nothing without ROLE.md, for bare, or when the manifest fails", async () => {
  roleImages();
  renderDialog({ imageRef: "plain:v1" });
  await waitFor(() =>
    expect(calls.imageManifestGet).toHaveBeenCalledWith("plain:v1", expect.anything()),
  );
  await waitFor(() => expect(screen.getByLabelText("image")).toHaveValue("plain"));
  expect(screen.queryByRole("button", { name: "Show role" })).not.toBeInTheDocument();
  expect(screen.getByLabelText("image")).not.toHaveAttribute("aria-describedby");

  chooseImage("Plain terminal (bare)");
  await waitFor(() =>
    expect(calls.imageManifestGet).toHaveBeenCalledWith("bare:latest", expect.anything()),
  );
  await screen.findByText("Terminal only");
  expect(screen.queryByText("Never shown.")).not.toBeInTheDocument();

  calls.imageManifestGet.mockRejectedValue(new Error("boom"));
  chooseImage("worker");
  expect(await screen.findByText(/Could not load image worker:v1/)).toBeInTheDocument();
  expect(screen.queryByText("You review pull requests.")).not.toBeInTheDocument();
});
