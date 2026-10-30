// Pure first-run rules: the "Getting ready" checklist, the image and coding
// tool choices of the New agent form, and the daemon banner filter.
import type { DaemonStatus, ImageSummary, OnboardingHarness, OnboardingStatus } from "@/lib/api";

export const HARNESS_LABELS: Record<string, string> = {
  claude: "Claude Code",
  codex: "Codex",
  opencode: "OpenCode",
  cursor: "Cursor",
};

/** The coding tools Desktop offers, in detection order. `stub` stays API-only. */
export const DESKTOP_HARNESSES = Object.keys(HARNESS_LABELS);

export const harnessLabel = (type: string) => HARNESS_LABELS[type] ?? type;

export const STARTER_IMAGE = "basic";
const PLAIN_TERMINAL = "bare";

export type ChecklistState = "done" | "working" | "failed" | "warn";

export interface ChecklistRow {
  key: "catalog" | "starter" | "tool";
  state: ChecklistState;
  text: string;
  /** The raw daemon error, shown under "Details". */
  error?: string;
  /** Install hints of the coding tools that were not found. */
  install?: OnboardingHarness[];
}

function catalogRow({ default_store: store }: OnboardingStatus): ChecklistRow {
  switch (store.state) {
    case "registered":
      return { key: "catalog", state: "done", text: "Tariboy catalog connected" };
    case "removed":
      return {
        key: "catalog",
        state: "warn",
        text: "Tariboy catalog removed. Add it again on the Stores page.",
      };
    case "failed":
      return {
        key: "catalog",
        state: "failed",
        text: "Could not connect the Tariboy catalog",
        error: store.error,
      };
    default:
      return { key: "catalog", state: "working", text: "Connecting the Tariboy catalog…" };
  }
}

function starterRow({ starter_image: starter }: OnboardingStatus): ChecklistRow {
  switch (starter.state) {
    case "ready":
    case "skipped":
      return { key: "starter", state: "done", text: `Starter image "${STARTER_IMAGE}" ready` };
    case "failed":
      return {
        key: "starter",
        state: "failed",
        text: `Could not build the starter image "${STARTER_IMAGE}"`,
        error: starter.error,
      };
    default:
      return { key: "starter", state: "working", text: "Building the starter image…" };
  }
}

const listLabels = (tools: OnboardingHarness[]) =>
  tools.map((tool) => harnessLabel(tool.type)).join(", ");

function toolRow({ harnesses }: OnboardingStatus): ChecklistRow {
  const found = harnesses.filter((tool) => tool.found);
  const missing = harnesses.filter((tool) => !tool.found);
  if (found.length === 0) {
    return {
      key: "tool",
      state: "warn",
      text: `Coding tool: none found. Install ${listLabels(missing)}.`,
      install: missing,
    };
  }
  const text = missing.length
    ? `Coding tool: ${listLabels(found)} found. ${listLabels(missing)} not found.`
    : `Coding tool: ${listLabels(found)} found.`;
  return { key: "tool", state: "done", text, install: missing };
}

/** The three "Getting ready" rows of one host. */
export function checklistRows(status: OnboardingStatus): ChecklistRow[] {
  // A removed catalog builds nothing, so the starter row would wait forever.
  const starter =
    status.default_store.state === "removed" && status.starter_image.state === "pending"
      ? null
      : starterRow(status);
  return [catalogRow(status), ...(starter ? [starter] : []), toolRow(status)];
}

/** Whether a row can still change by itself, so the status is worth polling. */
export function onboardingSettled(status: OnboardingStatus): boolean {
  if (status.default_store.state === "pending") return false;
  const starter = status.starter_image.state;
  if (starter === "building") return false;
  // The starter waits for the catalog; once the catalog is final it is final too.
  return !(starter === "pending" && status.default_store.state === "registered");
}

/** Why the starter image cannot be chosen yet, while the daemon still makes it. */
export function starterWaitReason(status: OnboardingStatus | null): string | null {
  if (!status) return null;
  const starter = status.starter_image.state;
  if (starter === "ready" || starter === "skipped" || starter === "failed") return null;
  if (status.default_store.state === "pending") return "Waiting for the Tariboy catalog…";
  if (starter === "building" || status.default_store.state === "registered") {
    return "Waiting for the starter image…";
  }
  return null;
}

export interface ImageChoice {
  name: string;
  label: string;
  /** The newest built tag, as `name:tag`. */
  ref: string;
  version: string;
  source: string;
  /** Every built tag of the name, newest first. */
  tags: string[];
  /** Set while the image is not built yet but the daemon is making it. */
  waiting?: string;
}

const newest = (a: ImageSummary, b: ImageSummary) =>
  b.built_at.localeCompare(a.built_at) || b.tag.localeCompare(a.tag);

const choiceRank = (name: string) =>
  name === STARTER_IMAGE ? 0 : name === PLAIN_TERMINAL ? 2 : 1;

/** One choice per image name: `basic` first, others A–Z, `bare` last. */
export function imageChoices(images: ImageSummary[], waiting: string | null = null): ImageChoice[] {
  const byName = new Map<string, ImageSummary[]>();
  for (const image of images) byName.set(image.name, [...(byName.get(image.name) ?? []), image]);
  const choices: ImageChoice[] = [...byName.entries()].map(([name, list]) => {
    const sorted = [...list].sort(newest);
    const top = sorted[0];
    return {
      name,
      label: name === PLAIN_TERMINAL ? "Plain terminal (bare)" : name,
      ref: `${name}:${top.tag}`,
      version: top.image_version ?? "",
      source: top.source ?? "",
      tags: sorted.map((image) => image.tag),
    };
  });
  if (waiting && !byName.has(STARTER_IMAGE)) {
    choices.push({
      name: STARTER_IMAGE,
      label: STARTER_IMAGE,
      ref: `${STARTER_IMAGE}:latest`,
      version: "",
      source: "",
      tags: [],
      waiting,
    });
  }
  return choices.sort(
    (a, b) => choiceRank(a.name) - choiceRank(b.name) || a.name.localeCompare(b.name),
  );
}

/** The image's own harness when it is installed, else the first installed one. */
export function pickHarness(imageDefault: string, harnesses: OnboardingHarness[] | null): string {
  if (!harnesses) return imageDefault;
  const found = new Set(harnesses.filter((tool) => tool.found).map((tool) => tool.type));
  if (imageDefault && found.has(imageDefault)) return imageDefault;
  return DESKTOP_HARNESSES.find((type) => found.has(type)) ?? imageDefault;
}

const slug = (value: string) =>
  value
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "-")
    .replace(/^-+|-+$/g, "");

/** `<folder basename>-<image name>`, unique among the host's agents. */
export function suggestAgentName(folder: string, imageName: string, taken: string[]): string {
  const base = folder.replace(/[\\/]+$/, "").split(/[\\/]/).pop() ?? "";
  const stem = slug([base, imageName].filter(Boolean).join("-")) || "agent";
  const used = new Set(taken);
  if (!used.has(stem)) return stem;
  let n = 2;
  while (used.has(`${stem}-${n}`)) n += 1;
  return `${stem}-${n}`;
}

const HARNESS_PURPOSE = "at least one agent harness";

/** The banner text of a daemon status, without the coding tool entry: a
 *  missing coding tool is shown in the checklist, never as a red banner. */
export function bannerMessage(status: DaemonStatus): string | null {
  if (status.state !== "error") return null;
  const tools = status.missing_tools ?? [];
  if (!tools.some((tool) => tool.purpose === HARNESS_PURPOSE)) return status.message;
  const others = tools.filter((tool) => tool.purpose !== HARNESS_PURPOSE);
  if (others.length === 0) return null;
  return `missing required tools: ${others.map((tool) => tool.name).join(", ")}; install them on the daemon host`;
}
