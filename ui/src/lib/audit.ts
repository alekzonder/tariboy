import type { AuditLogEvent, Call } from "@/lib/api";

export type { AuditLogEvent };

// parseData turns the JSON-string data field back into an object. On any parse
// failure it returns {} so callers never throw on a malformed record.
export function parseData(data: string): Record<string, unknown> {
  try {
    const v = JSON.parse(data);
    return v && typeof v === "object" ? (v as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function truncate(s: string, n = 120): string {
  return s.length > n ? s.slice(0, n) + "…" : s;
}

export type Tone = "neutral" | "success" | "warn" | "error";

// tone maps a record to a semantic accent used for the left bar. Cosmetic only.
export function tone(kind: string, data: string): Tone {
  if (kind === "error" || kind.endsWith("_error")) return "error";
  if (kind === "iteration_finished" || kind === "iteration_done") {
    const st = parseData(data).status;
    return typeof st === "string" && st.endsWith("_error") ? "error" : "success";
  }
  if (kind.startsWith("budget_")) return "warn";
  return "neutral";
}

export const TONE_BAR: Record<Tone, string> = {
  neutral: "border-l-transparent",
  success: "border-l-green-500",
  warn: "border-l-amber-500",
  error: "border-l-red-500",
};

// decodeJSONStrings walks a value and replaces any string that is itself a JSON
// object/array (e.g. harness_output's data.line) with the parsed value, so the
// pretty view shows real nested JSON instead of an escaped string. Non-JSON
// strings, numbers, and plain scalars are left untouched.
function decodeJSONStrings(v: unknown): unknown {
  if (typeof v === "string") {
    const t = v.trim();
    if (t.startsWith("{") || t.startsWith("[")) {
      try {
        const parsed = JSON.parse(t);
        if (parsed && typeof parsed === "object") return decodeJSONStrings(parsed);
      } catch {
        /* keep the raw string */
      }
    }
    return v;
  }
  if (Array.isArray(v)) return v.map(decodeJSONStrings);
  if (v && typeof v === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, val] of Object.entries(v as Record<string, unknown>))
      out[k] = decodeJSONStrings(val);
    return out;
  }
  return v;
}

// pretty renders the full record as indented JSON, with data parsed back into a
// nested object and any JSON-string fields (data.line) decoded recursively so
// harness JSON shows as real nested JSON, not an escaped string.
export function pretty(e: AuditLogEvent): string {
  const record = {
    seq: e.seq,
    at: e.at,
    kind: e.kind,
    source: e.source,
    data: decodeJSONStrings(parseData(e.data)),
  };
  return JSON.stringify(record, null, 2);
}

// isBoundary marks the iteration start/finish records that get an accent row.
export function isBoundary(kind: string): boolean {
  return kind === "iteration_started" || kind === "iteration_finished";
}

// hhmmss extracts HH:MM:SS from an ISO timestamp; on any parse failure it
// returns the raw value so the row still renders.
export function hhmmss(at: string): string {
  const m = /T(\d{2}:\d{2}:\d{2})/.exec(at);
  return m ? m[1] : at;
}

// salientArg picks the most informative field of a tool_use input, by tool name,
// falling back to the first string field and finally to compact JSON.
export function salientArg(name: string, input: Record<string, unknown>): string {
  const pick = (k: string) => (typeof input[k] === "string" ? (input[k] as string) : "");
  switch (name) {
    case "Bash":
      return pick("command");
    case "Read":
    case "Edit":
    case "Write":
    case "NotebookEdit":
      return pick("file_path");
    case "Grep":
    case "Glob":
      return pick("pattern");
    case "Task":
      return pick("description");
    case "Skill":
      return pick("skill");
  }
  for (const v of Object.values(input)) if (typeof v === "string") return v;
  return JSON.stringify(input);
}

export interface Descriptor {
  icon: string;
  label: string;
  preview: string;
  tone: Tone;
}

// resultText flattens a tool_result content (string, or an array of {text}
// blocks) into a single preview string.
function resultText(content: unknown): string {
  if (typeof content === "string") return content;
  if (Array.isArray(content))
    return content
      .map((b) =>
        b && typeof b === "object" && typeof (b as Record<string, unknown>).text === "string"
          ? ((b as Record<string, unknown>).text as string)
          : "",
      )
      .join(" ")
      .trim();
  return "";
}

type Inner = Record<string, unknown>;

// firstBlock returns the first content block of a stream-json line message, or
// undefined when the message has no content array.
function firstBlock(inner: Inner): Record<string, unknown> | undefined {
  const content = (inner.message as Record<string, unknown> | undefined)?.content;
  return Array.isArray(content) ? (content[0] as Record<string, unknown>) : undefined;
}

// lookup reads an own entry of a dispatch table, so keys such as "constructor"
// never resolve to Object.prototype members.
function lookup<T>(table: Record<string, T>, key: unknown): T | undefined {
  return typeof key === "string" && Object.hasOwn(table, key) ? table[key] : undefined;
}

function assistantDescriptor(inner: Inner): Descriptor {
  const block = firstBlock(inner);
  if (block?.type === "text")
    return {
      icon: "💬",
      label: "assistant",
      preview: truncate(String(block.text ?? "")),
      tone: "neutral",
    };
  if (block?.type === "tool_use") {
    const name = String(block.name ?? "tool");
    const input = (block.input && typeof block.input === "object" ? block.input : {}) as Record<
      string,
      unknown
    >;
    return {
      icon: "🔧",
      label: name,
      preview: truncate(salientArg(name, input)),
      tone: "neutral",
    };
  }
  // thinking or unknown assistant block — collapseThinking folds thinking away.
  return { icon: "🧠", label: "thinking", preview: "", tone: "neutral" };
}

function userDescriptor(inner: Inner): Descriptor {
  const block = firstBlock(inner);
  const isErr = block?.is_error === true;
  return {
    icon: isErr ? "❌" : "✅",
    label: "result",
    preview: truncate(resultText(block?.content)),
    tone: isErr ? "error" : "neutral",
  };
}

function systemDescriptor(inner: Inner): Descriptor {
  const st = inner.subtype;
  if (st === "init")
    return {
      icon: "⚙",
      label: "session",
      preview: truncate(`${inner.model ?? "?"} · ${inner.cwd ?? "?"}`),
      tone: "neutral",
    };
  return {
    icon: "🪝",
    label: "hook",
    preview: truncate(String(inner.hook_name ?? st ?? "")),
    tone: "neutral",
  };
}

function rateLimitDescriptor(inner: Inner): Descriptor {
  const status = String(
    (inner.rate_limit_info as Record<string, unknown> | undefined)?.status ?? "",
  );
  return {
    icon: "⏳",
    label: "rate limit",
    preview: status,
    tone: status === "allowed" ? "neutral" : "warn",
  };
}

const HARNESS_DESCRIPTORS: Record<string, (inner: Inner) => Descriptor> = {
  assistant: assistantDescriptor,
  user: userDescriptor,
  system: systemDescriptor,
  rate_limit_event: rateLimitDescriptor,
};

// harnessDescriptor maps a decoded stream-json line to a descriptor.
function harnessDescriptor(inner: Inner): Descriptor {
  const handler = lookup(HARNESS_DESCRIPTORS, inner.type);
  if (handler) return handler(inner);
  return {
    icon: "·",
    label: String(inner.type ?? "harness"),
    preview: truncate(JSON.stringify(inner)),
    tone: "neutral",
  };
}

// EventFields is the parsed data of one audit event plus a string-field reader.
interface EventFields {
  e: AuditLogEvent;
  d: Record<string, unknown>;
  s: (k: string) => string;
}

function harnessOutputDescriptor({ s }: EventFields): Descriptor {
  try {
    const inner = JSON.parse(s("line")) as Inner;
    return harnessDescriptor(inner);
  } catch {
    return { icon: "·", label: "harness", preview: truncate(s("line")), tone: "neutral" };
  }
}

function iterationFinishedDescriptor({ e, s }: EventFields): Descriptor {
  return {
    icon: "⏹",
    label: "iteration",
    preview: s("error") ? `${s("status")}: ${truncate(s("error"))}` : s("status"),
    tone: tone(e.kind, e.data),
  };
}

function shimDescriptor({ e, s }: EventFields): Descriptor {
  return {
    icon: "·",
    label: "shim",
    preview: truncate(s("line") || s("error")),
    tone: tone(e.kind, e.data),
  };
}

const EVENT_DESCRIPTORS: Record<string, (f: EventFields) => Descriptor> = {
  harness_output: harnessOutputDescriptor,
  iteration_started: ({ s }) => ({
    icon: "▶",
    label: "iteration",
    preview: s("trigger"),
    tone: "neutral",
  }),
  iteration_finished: iterationFinishedDescriptor,
  iteration_done: iterationFinishedDescriptor,
  launching_harness: ({ d, s }) => ({
    icon: "🚀",
    label: "harness",
    preview: `${s("harness")} (${d.interactive ? "interactive" : "batch"})`,
    tone: "neutral",
  }),
  harness_spawned: () => ({ icon: "🚀", label: "harness", preview: "spawned", tone: "neutral" }),
  status: ({ s }) => ({ icon: "📍", label: "status", preview: s("message"), tone: "neutral" }),
  shim: shimDescriptor,
  shim_error: shimDescriptor,
};

// descriptor maps one audit event to its chat-line display form. Never throws.
export function descriptor(e: AuditLogEvent): Descriptor {
  const d = parseData(e.data);
  const s = (k: string) => (typeof d[k] === "string" ? (d[k] as string) : "");
  const handler = lookup(EVENT_DESCRIPTORS, e.kind);
  if (handler) return handler({ e, d, s });
  return {
    icon: "·",
    label: e.kind,
    preview: truncate(s("reason") || s("message") || JSON.stringify(d)),
    tone: tone(e.kind, e.data),
  };
}

export type DisplayRow =
  | { kind: "event"; key: number; event: AuditLogEvent }
  | { kind: "thinking"; key: number; events: AuditLogEvent[]; tokens: number }
  | { kind: "proxycall"; key: number; call: Call; mode: "enrich" | "full" };

// isThinking marks the low-signal reasoning events collapseThinking folds:
// thinking_tokens counters and the redacted (empty-text) assistant thinking
// blocks this harness emits.
function isThinking(e: AuditLogEvent): boolean {
  if (e.kind !== "harness_output") return false;
  const line = parseData(e.data).line;
  if (typeof line !== "string") return false;
  try {
    const inner = JSON.parse(line) as Record<string, unknown>;
    if (inner.type === "system" && inner.subtype === "thinking_tokens") return true;
    if (inner.type === "assistant") return firstBlock(inner)?.type === "thinking";
  } catch {
    /* not thinking */
  }
  return false;
}

function thinkingTokens(e: AuditLogEvent): number {
  try {
    const inner = JSON.parse(String(parseData(e.data).line)) as Record<string, unknown>;
    const n = inner.estimated_tokens;
    return typeof n === "number" && Number.isFinite(n) ? n : 0;
  } catch {
    return 0;
  }
}

// collapseThinking folds each maximal run of consecutive thinking events into a
// single marker row (keyed by the run's first seq, tokens summed). Order is
// preserved; non-thinking events pass through as event rows.
export function collapseThinking(events: AuditLogEvent[]): DisplayRow[] {
  const out: DisplayRow[] = [];
  let run: AuditLogEvent[] = [];
  const flush = () => {
    if (run.length) {
      out.push({
        kind: "thinking",
        key: run[0].seq,
        events: run,
        tokens: run.reduce((a, e) => a + thinkingTokens(e), 0),
      });
      run = [];
    }
  };
  for (const e of events) {
    if (isThinking(e)) run.push(e);
    else {
      flush();
      out.push({ kind: "event", key: e.seq, event: e });
    }
  }
  flush();
  return out;
}

// fmtTokens renders a token count compactly: 2100 → "2.1k", 50 → "50".
export function fmtTokens(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);
}

// hasHarnessOutput reports whether the iteration produced harness stream-json
// (batch agents). Absent for interactive agents, which decides render mode.
export function hasHarnessOutput(events: AuditLogEvent[]): boolean {
  return events.some((e) => e.kind === "harness_output");
}

// rowTime returns the sort timestamp of a display row.
function rowTime(r: DisplayRow): string {
  if (r.kind === "event") return r.event.at;
  if (r.kind === "thinking") return r.events[0]?.at ?? "";
  return r.call.ts;
}

// mergeProxyCalls interleaves one proxycall row per call into the existing rows,
// ordered by timestamp (stable). Proxycall keys are negative to never collide
// with event seq keys.
export function mergeProxyCalls(
  rows: DisplayRow[],
  calls: Call[],
  mode: "enrich" | "full",
): DisplayRow[] {
  const proxyRows: DisplayRow[] = calls.map((call) => ({
    kind: "proxycall",
    key: -(call.seq + 1),
    call,
    mode,
  }));
  const all = [...rows, ...proxyRows];
  return all
    .map((r, i) => ({ r, i }))
    .sort((a, b) => {
      const ta = rowTime(a.r),
        tb = rowTime(b.r);
      const pa = Date.parse(ta),
        pb = Date.parse(tb);
      if (!Number.isNaN(pa) && !Number.isNaN(pb)) {
        if (pa < pb) return -1;
        if (pa > pb) return 1;
        return a.i - b.i; // stable: preserve original relative order on ties
      }
      if (ta < tb) return -1;
      if (ta > tb) return 1;
      return a.i - b.i; // stable: preserve original relative order on ties
    })
    .map(({ r }) => r);
}
