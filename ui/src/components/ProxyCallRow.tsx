import { useState } from "react";
import { fmtTokens } from "@/lib/audit";
import type { Call, Block, Message } from "@/lib/api";
import { useApi } from "@/components/apiContext";

function toolInput(input: unknown): Record<string, unknown> {
  if (input && typeof input === "object") return input as Record<string, unknown>;
  if (typeof input === "string") {
    try {
      const parsed = JSON.parse(input);
      if (parsed && typeof parsed === "object") return parsed as Record<string, unknown>;
    } catch {
      /* retain the raw value below */
    }
    return { input };
  }
  return {};
}

function firstString(input: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    if (typeof input[key] === "string" && input[key] !== "") return input[key] as string;
    if (
      Array.isArray(input[key]) &&
      (input[key] as unknown[]).every((part) => typeof part === "string")
    )
      return (input[key] as string[]).join(" ");
  }
  for (const value of Object.values(input))
    if (typeof value === "string" && value !== "") return value;
  return Object.keys(input).length ? JSON.stringify(input) : "";
}

interface ToolBlockKind {
  matches: (lowerName: string) => boolean;
  label: string;
  keys: string[];
  as: "code" | "span";
}

const COMMAND_TOOLS = ["exec_command", "command_execution", "bash", "shell", "local_shell"];

// TOOL_BLOCK_KINDS maps a lower-cased tool name to its activity label, the
// input keys holding its salient text and the element that shows it. Tools no
// entry matches render as a generic "Tool" row.
const TOOL_BLOCK_KINDS: ToolBlockKind[] = [
  {
    matches: (lower) => COMMAND_TOOLS.includes(lower),
    label: "Command",
    keys: ["cmd", "command"],
    as: "code",
  },
  {
    matches: (lower) => lower === "skill" || lower.endsWith("__skill") || lower.endsWith(".skill"),
    label: "Skill",
    keys: ["skill", "name"],
    as: "span",
  },
];

const OTHER_TOOL_KEYS = ["query", "path", "prompt"];

function toolUseBlock(block: Block, key: string) {
  const name = block.tool_name ?? "tool";
  const lower = name.toLowerCase();
  const input = toolInput(block.input);
  const kind = TOOL_BLOCK_KINDS.find((k) => k.matches(lower));
  if (kind) {
    const Body = kind.as;
    return (
      <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1">
        <span className="font-semibold">{kind.label}</span>
        <Body className="whitespace-pre-wrap break-words text-muted-foreground">
          {firstString(input, kind.keys)}
        </Body>
      </div>
    );
  }
  return (
    <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1">
      <span className="font-semibold">Tool</span>
      <span className="min-w-0">
        <span>{name}</span>
        {Object.keys(input).length > 0 && (
          <code className="ml-2 whitespace-pre-wrap break-words text-muted-foreground">
            {firstString(input, OTHER_TOOL_KEYS)}
          </code>
        )}
      </span>
    </div>
  );
}

function activityBlock(block: Block, key: string) {
  if (block.type === "text") {
    return (
      <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1">
        <span className="font-semibold">Message</span>
        <span className="whitespace-pre-wrap break-words">{block.text}</span>
      </div>
    );
  }
  if (block.type === "tool_result") {
    return (
      <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1">
        <span className="font-semibold">Result</span>
        <span className="whitespace-pre-wrap break-words text-muted-foreground">
          {block.text || "Completed without output"}
        </span>
      </div>
    );
  }
  if (block.type !== "tool_use") return null;
  return toolUseBlock(block, key);
}

function InstructionsDisclosure({ call }: { call: Call }) {
  const [showInstr, setShowInstr] = useState(false);
  if (typeof call.instructions !== "string" || call.instructions.length === 0) return null;
  return (
    <div>
      <button className="text-left" onClick={() => setShowInstr((v) => !v)}>
        📜 instructions
        {call.instructions_changed && call.seq > 0 && (
          <span className="text-amber-600"> ⚠ changed</span>
        )}
      </button>
      {showInstr && <pre className="whitespace-pre-wrap opacity-80">{call.instructions}</pre>}
    </div>
  );
}

function ThinkingDisclosure({ thinking }: { thinking: Block[] }) {
  const [showThink, setShowThink] = useState(false);
  if (thinking.length === 0) return null;
  return (
    <div>
      <button className="text-left text-muted-foreground" onClick={() => setShowThink((v) => !v)}>
        🧠 thinking
      </button>
      {showThink && (
        <pre className="whitespace-pre-wrap opacity-70">
          {thinking.map((b) => b.text).join("\n")}
        </pre>
      )}
    </div>
  );
}

// CallSummary is the collapsed "AI call" line — model, tokens, price and the
// truncated flag — with the button that opens the raw view.
function CallSummary({ call, onViewRaw }: { call: Call; onViewRaw: () => void }) {
  const tok = call.usage
    ? `${fmtTokens(call.usage.input)}→${fmtTokens(call.usage.output)} tok`
    : "";
  const cost = call.cost_usd != null ? ` · $${call.cost_usd.toFixed(4)}` : "";
  return (
    <details className="mt-1 text-muted-foreground">
      <summary className="cursor-pointer">
        AI call · {call.model} · {tok}
        {cost}
        {call.truncated ? " · response truncated" : ""}
      </summary>
      <button className="mt-1 underline-offset-2 hover:underline" onClick={onViewRaw}>
        view raw ▾
      </button>
    </details>
  );
}

function RawView({ raw }: { raw: { request: string; response: string } }) {
  return (
    <pre className="whitespace-pre-wrap opacity-70 max-h-64 overflow-auto">
      {"REQUEST\n" + raw.request + "\n\nRESPONSE\n" + raw.response}
    </pre>
  );
}

export function ProxyCallRow({
  call,
  mode,
  name,
  iteration,
}: {
  call: Call;
  mode: "enrich" | "full";
  name: string;
  iteration: string;
}) {
  const api = useApi();
  const [raw, setRaw] = useState<{ request: string; response: string } | null>(null);
  const [rawOpen, setRawOpen] = useState(false);

  // A failed/empty proxy call serializes nil slices as JSON `null` (the types
  // claim Block[]/Message[], but the runtime value can be null) — guard so one
  // bad call can't throw and unmount the whole app.
  const blocks = call.response.blocks ?? [];
  const delta = call.delta ?? [];
  const thinking = blocks.filter((b) => b.type === "thinking");
  const assistant = blocks.filter((b) => b.type !== "thinking");

  const openRaw = async () => {
    setRawOpen((v) => !v);
    if (!raw) {
      const calls = await api.audit.fetchTranscriptRaw(name, iteration);
      const mine = calls.find((c) => c.seq === call.seq);
      if (mine) setRaw({ request: mine.request, response: mine.response });
    }
  };

  return (
    <div className="border-l-2 border-blue-500/40 pl-2 my-1 font-mono text-xs">
      {call.parse_error && <div className="text-amber-600">parse error — raw only</div>}

      <InstructionsDisclosure call={call} />

      <ThinkingDisclosure thinking={thinking} />

      {mode === "full" &&
        delta.flatMap((m: Message, i) =>
          (m.blocks ?? []).map((b, j) => activityBlock(b, `d${i}-${j}`)),
        )}
      {mode === "full" && assistant.map((b, i) => activityBlock(b, `a${i}`))}

      <CallSummary call={call} onViewRaw={openRaw} />
      {rawOpen && raw && <RawView raw={raw} />}
    </div>
  );
}
