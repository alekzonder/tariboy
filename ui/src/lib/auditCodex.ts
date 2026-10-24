import { parseData, salientArg, type AuditEvent } from "@/lib/audit";

// CodexHarnessPresentation is the small, user-facing subset of a Codex JSONL
// item. The original event stays available through pretty(); this only avoids
// making routine commands and messages look like an escaped protocol envelope.
export interface CodexHarnessPresentation {
  type: "command" | "message" | "tool" | "skill" | "thinking" | "file";
  label: string;
  icon: string;
  text: string;
  status?: string;
  output?: string;
}

type CodexItem = Record<string, unknown>;
type CodexItemHandler = (
  record: CodexItem,
  outerType: unknown,
) => CodexHarnessPresentation | undefined;

function nonEmptyString(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

// commandStatus prefers the reported status, else derives one from the event
// phase and the exit code.
function commandStatus(record: CodexItem, outerType: unknown): string {
  const reported = nonEmptyString(record.status);
  if (reported) return reported;
  if (outerType === "item.started") return "running";
  const exitCode = record.exit_code;
  return typeof exitCode === "number" && exitCode !== 0 ? "failed" : "completed";
}

function commandItem(record: CodexItem, outerType: unknown) {
  const text = nonEmptyString(record.command) ?? nonEmptyString(record.cmd);
  if (!text) return undefined;
  const output =
    nonEmptyString(record.aggregated_output) ??
    nonEmptyString(record.output) ??
    nonEmptyString(record.result);
  return {
    type: "command",
    label: "command",
    icon: "⌨",
    text,
    status: commandStatus(record, outerType),
    output,
  } satisfies CodexHarnessPresentation;
}

function messageItem(record: CodexItem) {
  const text = nonEmptyString(record.text) ?? nonEmptyString(record.message);
  return text
    ? ({
        type: "message",
        label: "message",
        icon: "💬",
        text,
        status: nonEmptyString(record.status),
      } satisfies CodexHarnessPresentation)
    : undefined;
}

function toolItem(record: CodexItem): CodexHarnessPresentation {
  const server = nonEmptyString(record.server) ?? nonEmptyString(record.server_name) ?? "MCP";
  const tool = nonEmptyString(record.tool) ?? nonEmptyString(record.name) ?? "tool";
  const args =
    record.arguments && typeof record.arguments === "object"
      ? (record.arguments as Record<string, unknown>)
      : {};
  const detail = salientArg(tool, args);
  return {
    type: "tool",
    label: "Tool",
    icon: "🔧",
    text: `${server} · ${tool}${detail && detail !== "{}" ? ` — ${detail}` : ""}`,
    status: nonEmptyString(record.status),
    output: nonEmptyString(record.result) ?? nonEmptyString(record.error),
  };
}

function skillItem(record: CodexItem) {
  const text = nonEmptyString(record.name) ?? nonEmptyString(record.skill);
  return text
    ? ({
        type: "skill",
        label: "Skill",
        icon: "✦",
        text,
        status: nonEmptyString(record.status),
      } satisfies CodexHarnessPresentation)
    : undefined;
}

function reasoningItem(record: CodexItem) {
  const text = nonEmptyString(record.text) ?? nonEmptyString(record.summary);
  return text
    ? ({ type: "thinking", label: "Thinking", icon: "🧠", text } satisfies CodexHarnessPresentation)
    : undefined;
}

// changedFile renders one file_change entry as "path (kind)", or "" when it
// names no path.
function changedFile(change: unknown): string {
  if (!change || typeof change !== "object") return "";
  const item = change as Record<string, unknown>;
  const path = nonEmptyString(item.path) ?? nonEmptyString(item.file_path) ?? "";
  const kind = nonEmptyString(item.kind) ?? nonEmptyString(item.type) ?? "";
  return path ? `${path}${kind ? ` (${kind})` : ""}` : "";
}

function changedFiles(record: CodexItem): string {
  const changes = Array.isArray(record.changes) ? record.changes : [];
  return changes.map(changedFile).filter(Boolean).join(", ");
}

function fileChangeItem(record: CodexItem) {
  const text = changedFiles(record);
  return text
    ? ({
        type: "file",
        label: "File change",
        icon: "📝",
        text,
        status: nonEmptyString(record.status),
      } satisfies CodexHarnessPresentation)
    : undefined;
}

const CODEX_ITEM_HANDLERS: Record<string, CodexItemHandler> = {
  command_execution: commandItem,
  agent_message: messageItem,
  mcp_tool_call: toolItem,
  skill: skillItem,
  reasoning: reasoningItem,
  file_change: fileChangeItem,
};

// codexHarnessPresentation recognizes the item.started/item.completed events
// emitted by the Codex harness. It deliberately returns undefined for every
// other line so generic harness and audit rendering remains unchanged.
export function codexHarnessPresentation(e: AuditEvent): CodexHarnessPresentation | undefined {
  if (e.kind !== "harness_output") return undefined;
  const line = parseData(e.data).line;
  if (typeof line !== "string") return undefined;
  try {
    const outer = JSON.parse(line) as Record<string, unknown>;
    if (outer.type !== "item.started" && outer.type !== "item.completed") return undefined;
    const item = outer.item;
    if (!item || typeof item !== "object") return undefined;
    const record = item as CodexItem;
    const handler =
      typeof record.type === "string" && Object.hasOwn(CODEX_ITEM_HANDLERS, record.type)
        ? CODEX_ITEM_HANDLERS[record.type]
        : undefined;
    return handler?.(record, outer.type);
  } catch {
    // Malformed protocol lines retain the existing generic rendering.
  }
  return undefined;
}
