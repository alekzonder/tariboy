// The Settings page is four groups and one Advanced disclosure over one
// page-wide draft. This module is the pure half of that page: which sections
// exist, which fields they hold, and how a field's text is normalized and
// checked. Rendering and requests live elsewhere so this stays testable alone.

export type Values = Record<string, string>;

type FieldKind =
  | "text"
  | "number"
  | "select"
  | "switch"
  | "textarea"
  | "schedule"
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
  options?: readonly string[];
  minimum?: number;
}

export interface ConfigSection {
  id: string;
  title: string;
  // An advanced section renders inside the collapsed Advanced disclosure.
  advanced: boolean;
  fields: readonly ConfigField[];
}

const NEXT_ITERATION = "Takes effect on the next iteration.";
const NEXT_START = "Takes effect the next time the agent starts.";

export const SECTIONS: readonly ConfigSection[] = [
  {
    id: "model",
    title: "Model",
    advanced: false,
    fields: [
      { key: "harness", label: "Coding tool", kind: "select", helper: NEXT_START },
      { key: "model", label: "Model", kind: "preset", helper: "" },
      { key: "effort", label: "Effort", kind: "preset", helper: "" },
    ],
  },
  {
    id: "folder",
    title: "Working folder",
    advanced: false,
    fields: [
      {
        key: "cwd",
        label: "Working directory",
        kind: "path",
        helper: "Takes effect on the next start. Existing files stay where they are.",
      },
      {
        key: "cwd-create",
        label: "Create directory if missing",
        kind: "switch",
        helper: "Make the working directory and its parents when it does not exist.",
      },
    ],
  },
  {
    id: "autopilot",
    title: "Autopilot",
    advanced: false,
    fields: [
      { key: "interval", label: "Minutes between runs", kind: "text", helper: "" },
      {
        key: "timeout",
        label: "Stop a run after (minutes)",
        kind: "text",
        helper: `0 means no limit. ${NEXT_ITERATION}`,
      },
      { key: "interactive", label: "Interactive terminal", kind: "switch", helper: NEXT_START },
    ],
  },
  {
    id: "spending",
    title: "Spending limit",
    advanced: false,
    fields: [
      { key: "budget-day", label: "Per day", kind: "number", minimum: 0, helper: "" },
      { key: "budget-month", label: "Per month", kind: "number", minimum: 0, helper: "" },
    ],
  },
  {
    id: "image",
    title: "Image",
    advanced: true,
    fields: [
      {
        key: "image",
        label: "Agent image",
        kind: "image",
        helper:
          "Select an already-built image. It becomes active before the next iteration and does not change runtime settings.",
      },
      { key: "image-digest", label: "Image digest", kind: "readonly", helper: "Read only." },
    ],
  },
  {
    id: "schedule",
    title: "Schedule and timeouts",
    advanced: true,
    fields: [
      {
        key: "schedule",
        label: "Schedule",
        kind: "schedule",
        helper:
          "Times are in the daemon host's time zone, not the browser's. Works together with Autopilot. Custom RRULE takes any RFC 5545 rule; None means no schedule.",
      },
      {
        key: "hard-timeout",
        label: "Hard timeout",
        kind: "text",
        helper: `Seconds. ${NEXT_ITERATION}`,
      },
      {
        key: "on-timeout",
        label: "On timeout",
        kind: "select",
        options: ["restart", "stop"],
        helper: NEXT_ITERATION,
      },
      {
        key: "on-error",
        label: "On error",
        kind: "select",
        options: ["restart", "stop"],
        helper: NEXT_ITERATION,
      },
      {
        key: "max-idle",
        label: "Maximum idle iterations",
        kind: "number",
        minimum: 0,
        helper: `0 means never. ${NEXT_ITERATION}`,
      },
      {
        key: "ai-stall-timeout",
        label: "AI inactivity timeout",
        kind: "number",
        minimum: 1,
        helper: "Seconds without an AI-proxy request before showing an informational error.",
      },
    ],
  },
  {
    id: "goal",
    title: "Goal",
    advanced: true,
    fields: [
      {
        key: "goal-enabled",
        label: "Enable Goal",
        kind: "switch",
        helper: "Select and deliver this agent's current Native Task goal.",
      },
      {
        key: "goal-wait-customer-timeout",
        label: "Wait customer timeout seconds",
        kind: "number",
        minimum: 1,
        helper: "Use a positive whole number of seconds.",
      },
      {
        key: "goal-delivery-cooldown",
        label: "Goal delivery cooldown seconds",
        kind: "number",
        minimum: 1,
        helper: "Wait before sending another unprocessed Goal message.",
      },
      {
        key: "current-goal-task",
        label: "Current goal task",
        kind: "readonly",
        helper: "Selected by the daemon.",
      },
    ],
  },
  {
    id: "mail",
    title: "Mail",
    advanced: true,
    fields: [
      {
        key: "messages-batch",
        label: "Mail: letters per iteration",
        kind: "number",
        minimum: 1,
        helper: "Most open inbox letters added to one iteration.",
      },
      {
        key: "messages-max-queue",
        label: "Mail: maximum open inbox",
        kind: "number",
        minimum: 1,
        helper: "A letter beyond this limit is dead on arrival; requeue it from Mailbox, Archive.",
      },
    ],
  },
  {
    id: "budgets",
    title: "Hour and week budgets (USD)",
    advanced: true,
    fields: [
      { key: "budget-hour", label: "Hour budget", kind: "number", minimum: 0, helper: "" },
      { key: "budget-week", label: "Week budget", kind: "number", minimum: 0, helper: "" },
    ],
  },
  {
    id: "environment",
    title: "Environment",
    advanced: true,
    fields: [
      {
        key: "shell-script",
        label: "Agent shell script",
        kind: "textarea",
        helper:
          "Bash commands sourced after the global script before this agent starts an iteration.",
      },
      {
        key: "secrets-remove",
        label: "Secrets",
        kind: "secret-remove",
        helper: "Key names only; values are never shown.",
      },
      { key: "secret-key", label: "Secret key", kind: "text", helper: NEXT_ITERATION },
      {
        key: "secret-value",
        label: "Secret value",
        kind: "secret-value",
        helper: "Write-only. Cleared after it is saved.",
      },
    ],
  },
  {
    id: "retention",
    title: "Retention and cleanup",
    advanced: true,
    fields: [
      { key: "keep-iterations", label: "Keep iterations", kind: "number", minimum: 0, helper: "" },
      { key: "keep-days", label: "Keep days", kind: "number", minimum: 0, helper: "" },
    ],
  },
];

const FIELDS = new Map(
  SECTIONS.flatMap((section) =>
    section.fields.map((field) => [field.key, { field, section }] as const),
  ),
);

export const sectionOfKey = (key: string) => FIELDS.get(key)?.section;
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
  value !== "" && Number.isInteger(Number(value)) && Number(value) >= 0
    ? ""
    : "Enter zero or a positive whole number.";
const amount = (value: string) =>
  value !== "" && Number.isFinite(Number(value)) && Number(value) >= 0
    ? ""
    : "Enter zero or a positive amount.";

// The minutes boxes pass text they cannot convert through, so it lands here.
const minutes = (message: string) => (value: string) => (nonNegativeWhole(value) ? message : "");

const SECONDS = positiveWhole("Enter a positive whole number of seconds.");
const COUNT = positiveWhole("Enter a positive whole number.");
const VALIDATORS: Record<string, (value: string, values: Values) => string> = {
  interval: minutes("Enter the minutes between runs."),
  timeout: minutes("Enter the minutes, or 0 for no limit."),
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
  "secret-value": (value, values) =>
    !value && values["secret-key"]?.trim() ? "Enter a secret value." : "",
};

// validateField checks a normalized value before anything is sent; "" is valid.
export function validateField(key: string, value: string, values: Values): string {
  return VALIDATORS[key]?.(value, values) ?? "";
}
