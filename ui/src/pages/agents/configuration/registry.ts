// The Configuration page is one column of blocks over one page-wide draft.
// This module is the pure half of that page: which blocks exist, which fields
// they hold, how a field's text is normalized and checked, and how search picks
// blocks. Rendering and requests live elsewhere so this stays testable alone.

export type Values = Record<string, string>;

export type FieldKind =
  | "text"
  | "number"
  | "select"
  | "switch"
  | "textarea"
  | "preset"
  | "path"
  | "image"
  | "readonly"
  | "secret-value"
  | "secret-remove";

export interface ConfigField {
  // key is the draft key and the control's DOM id, so a failed save can move
  // focus to the control it belongs to.
  key: string;
  label: string;
  helper: string;
  kind: FieldKind;
  // Words a search should find that the label and helper do not contain.
  keywords?: string;
  options?: readonly string[];
  minimum?: number;
}

export interface ConfigBlock {
  id: string;
  title: string;
  description: string;
  keywords: string;
  fields: readonly ConfigField[];
}

const NEXT_ITERATION = "Takes effect on the next iteration.";
const NEXT_START = "Takes effect the next time the agent starts.";

export const BLOCKS: readonly ConfigBlock[] = [
  {
    id: "general",
    title: "General",
    description: "Image and working directory used for the next launch.",
    keywords: "identity location",
    fields: [
      { key: "image", label: "Agent image", kind: "image", helper: "Select an already-built image. It becomes active before the next iteration and does not change runtime settings.", keywords: "version pending build" },
      { key: "image-digest", label: "Image digest", kind: "readonly", helper: "Read only.", keywords: "sha hash" },
      { key: "cwd", label: "Working directory", kind: "path", helper: "Takes effect on the next start. Existing files stay where they are.", keywords: "cwd path folder directory location" },
    ],
  },
  {
    id: "runtime",
    title: "Runtime",
    description: "Harness, model and effort for this agent.",
    keywords: "llm ai",
    fields: [
      { key: "harness", label: "Harness", kind: "select", helper: NEXT_START, keywords: "cli claude codex opencode cursor" },
      { key: "model", label: "Model", kind: "preset", helper: NEXT_ITERATION, keywords: "llm" },
      { key: "effort", label: "Effort", kind: "preset", helper: NEXT_ITERATION, keywords: "reasoning thinking" },
      { key: "interactive", label: "Interactive (tmux TUI)", kind: "switch", helper: NEXT_START, keywords: "terminal tmux tui" },
    ],
  },
  {
    id: "loop",
    title: "Loop and timeouts",
    description: "When the agent runs and how it recovers.",
    keywords: "schedule iteration",
    fields: [
      { key: "interval", label: "Interval", kind: "text", helper: `Seconds. ${NEXT_ITERATION}`, keywords: "period seconds" },
      { key: "schedule", label: "Schedule (RRULE)", kind: "textarea", helper: "RFC 5545 RRULE, whole minutes, in the daemon host's time zone unless DTSTART names one. Works together with Interval. Empty means no schedule.", keywords: "rrule cron calendar time weekday day month dtstart" },
      { key: "timeout", label: "Timeout", kind: "text", helper: `Seconds. ${NEXT_ITERATION}`, keywords: "seconds" },
      { key: "hard-timeout", label: "Hard timeout", kind: "text", helper: `Seconds. ${NEXT_ITERATION}`, keywords: "kill seconds" },
      { key: "on-timeout", label: "On timeout", kind: "select", options: ["restart", "stop"], helper: NEXT_ITERATION, keywords: "policy" },
      { key: "on-error", label: "On error", kind: "select", options: ["restart", "stop"], helper: NEXT_ITERATION, keywords: "policy failure" },
      { key: "max-idle", label: "Maximum idle iterations", kind: "number", minimum: 0, helper: `0 means never. ${NEXT_ITERATION}`, keywords: "idle limit" },
      { key: "ai-stall-timeout", label: "AI inactivity timeout", kind: "number", minimum: 1, helper: "Seconds without an AI-proxy request before showing an informational error.", keywords: "stall proxy" },
    ],
  },
  {
    id: "goal",
    title: "Goal",
    description: "Choose whether this agent follows one sticky Native Task goal.",
    keywords: "task native",
    fields: [
      { key: "goal-enabled", label: "Enable Goal", kind: "switch", helper: "Select and deliver this agent's current Native Task goal." },
      { key: "goal-wait-customer-timeout", label: "Wait customer timeout seconds", kind: "number", minimum: 1, helper: "Use a positive whole number of seconds.", keywords: "customer wait" },
      { key: "goal-delivery-cooldown", label: "Goal delivery cooldown seconds", kind: "number", minimum: 1, helper: "Wait before sending another unprocessed Goal message.", keywords: "cooldown delay" },
      { key: "current-goal-task", label: "Current goal task", kind: "readonly", helper: "Selected by the daemon." },
    ],
  },
  {
    id: "messages",
    title: "Messages",
    description: "Control queue capacity and iteration batches.",
    keywords: "channels inbox queue",
    fields: [
      { key: "messages-batch", label: "Messages per iteration", kind: "number", minimum: 1, helper: "Maximum pending messages added to one iteration.", keywords: "batch" },
      { key: "messages-max-queue", label: "Maximum pending queue", kind: "number", minimum: 1, helper: "New deliveries beyond this limit enter DLQ.", keywords: "queue dlq limit" },
    ],
  },
  {
    id: "budgets",
    title: "Budgets (USD)",
    description: "Zero means Unlimited. Each configured calendar limit applies independently.",
    keywords: "money cost spend usd limit",
    fields: [
      { key: "budget-hour", label: "Hour budget", kind: "number", minimum: 0, helper: "" },
      { key: "budget-day", label: "Day budget", kind: "number", minimum: 0, helper: "" },
      { key: "budget-week", label: "Week budget", kind: "number", minimum: 0, helper: "" },
      { key: "budget-month", label: "Month budget", kind: "number", minimum: 0, helper: "" },
    ],
  },
  {
    id: "environment",
    title: "Environment",
    description: "Shell script and secrets for each iteration.",
    keywords: "bash env variables",
    fields: [
      { key: "shell-script", label: "Agent shell script", kind: "textarea", helper: "Bash commands sourced after the global script before this agent starts an iteration.", keywords: "bash export" },
      { key: "secrets-remove", label: "Secrets", kind: "secret-remove", helper: "Key names only; values are never shown.", keywords: "token password credential" },
      { key: "secret-key", label: "Secret key", kind: "text", helper: NEXT_ITERATION, keywords: "token" },
      { key: "secret-value", label: "Secret value", kind: "secret-value", helper: "Write-only. Cleared after it is saved.", keywords: "password" },
    ],
  },
  {
    id: "retention",
    title: "Retention and cleanup",
    description: "Keep the history you need, then review cleanup safely.",
    keywords: "prune history archive",
    fields: [
      { key: "keep-iterations", label: "Keep iterations", kind: "number", minimum: 0, helper: "" },
      { key: "keep-days", label: "Keep days", kind: "number", minimum: 0, helper: "" },
    ],
  },
];

const FIELDS = new Map(BLOCKS.flatMap((block) => block.fields.map((field) => [field.key, { field, block: block.id }] as const)));

export const blockOfKey = (key: string) => FIELDS.get(key)?.block ?? "";
export const fieldOfKey = (key: string) => FIELDS.get(key)?.field;

// Whole seconds and counts: a non-numeric draft is left alone so the server,
// not the page, decides that it is invalid.
const WHOLE: Record<string, number | undefined> = {
  interval: undefined,
  timeout: undefined,
  "hard-timeout": undefined,
  "max-idle": 0,
  "ai-stall-timeout": 1,
};
const VERBATIM = new Set(["shell-script", "secret-value"]);

// normalizeField maps raw input text to the value that is compared against
// the baseline AND sent to the server, so "30" and " 30 " are the same edit.
export function normalizeField(key: string, raw: string): string {
  if (VERBATIM.has(key)) return raw;
  const text = raw.trim();
  if (key in WHOLE) {
    const n = Number(text);
    if (text === "" || !Number.isFinite(n)) return text;
    const min = WHOLE[key];
    const truncated = Math.trunc(n);
    return String(min === undefined ? truncated : Math.max(min, truncated));
  }
  return text;
}

const positiveWhole = (message: string) => (value: string) =>
  Number.isInteger(Number(value)) && Number(value) > 0 ? "" : message;
const nonNegativeWhole = (value: string) =>
  value !== "" && Number.isInteger(Number(value)) && Number(value) >= 0 ? "" : "Enter zero or a positive whole number.";
const amount = (value: string) =>
  value !== "" && Number.isFinite(Number(value)) && Number(value) >= 0 ? "" : "Enter zero or a positive amount.";

const SECONDS = positiveWhole("Enter a positive whole number of seconds.");
const COUNT = positiveWhole("Enter a positive whole number.");
const VALIDATORS: Record<string, (value: string, values: Values) => string> = {
  "goal-wait-customer-timeout": SECONDS,
  "goal-delivery-cooldown": SECONDS,
  "messages-batch": COUNT,
  "messages-max-queue": COUNT,
  "budget-hour": amount,
  "budget-day": amount,
  "budget-week": amount,
  "budget-month": amount,
  "keep-iterations": nonNegativeWhole,
  "keep-days": nonNegativeWhole,
  "secret-key": (value, values) => (!value && values["secret-value"] ? "Enter a secret key." : ""),
  "secret-value": (value, values) => (!value && values["secret-key"]?.trim() ? "Enter a secret value." : ""),
};

// validateField checks a normalized value before anything is sent; "" is valid.
export function validateField(key: string, value: string, values: Values): string {
  return VALIDATORS[key]?.(value, values) ?? "";
}

const haystack = (...parts: (string | undefined)[]) => parts.filter(Boolean).join(" ").toLowerCase();

// filterBlocks returns the blocks a query selects, whole, in page order, and
// the fields that matched by themselves so the page can highlight them. Every
// word of the query must be found somewhere in one block.
export function filterBlocks(query: string, blocks: readonly ConfigBlock[]): { blocks: ConfigBlock[]; matched: Set<string> } {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  const matched = new Set<string>();
  if (words.length === 0) return { blocks: [...blocks], matched };
  const selected = blocks.filter((block) => {
    const own = haystack(block.title, block.description, block.keywords);
    const fieldTexts = block.fields.map((f) => haystack(f.label, f.helper, f.keywords, f.key));
    const all = [own, ...fieldTexts].join(" ");
    if (!words.every((word) => all.includes(word))) return false;
    block.fields.forEach((field, index) => {
      if (words.every((word) => fieldTexts[index].includes(word))) matched.add(field.key);
    });
    return true;
  });
  return { blocks: selected, matched };
}
