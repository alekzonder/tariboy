import { useState } from "react";
import { fmtTokens } from "@/lib/audit";
import { fetchTranscriptRaw, type Call, type Block, type Message } from "@/lib/transcript";

function toolInput(input: unknown): Record<string, unknown> {
  if (input && typeof input === "object") return input as Record<string, unknown>;
  if (typeof input === "string") {
    try {
      const parsed = JSON.parse(input);
      if (parsed && typeof parsed === "object") return parsed as Record<string, unknown>;
    } catch { /* retain the raw value below */ }
    return { input };
  }
  return {};
}

function firstString(input: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    if (typeof input[key] === "string" && input[key] !== "") return input[key] as string;
    if (Array.isArray(input[key]) && (input[key] as unknown[]).every((part) => typeof part === "string")) return (input[key] as string[]).join(" ");
  }
  for (const value of Object.values(input)) if (typeof value === "string" && value !== "") return value;
  return Object.keys(input).length ? JSON.stringify(input) : "";
}

function activityBlock(block: Block, key: string) {
  if (block.type === "text") {
    return <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1"><span className="font-semibold">Message</span><span className="whitespace-pre-wrap break-words">{block.text}</span></div>;
  }
  if (block.type === "tool_result") {
    return <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1"><span className="font-semibold">Result</span><span className="whitespace-pre-wrap break-words text-muted-foreground">{block.text || "Completed without output"}</span></div>;
  }
  if (block.type !== "tool_use") return null;
  const name = block.tool_name ?? "tool";
  const lower = name.toLowerCase();
  const input = toolInput(block.input);
  if (["exec_command", "command_execution", "bash", "shell", "local_shell"].includes(lower)) {
    return <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1"><span className="font-semibold">Command</span><code className="whitespace-pre-wrap break-words text-muted-foreground">{firstString(input, ["cmd", "command"])}</code></div>;
  }
  if (lower === "skill" || lower.endsWith("__skill") || lower.endsWith(".skill")) {
    return <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1"><span className="font-semibold">Skill</span><span className="whitespace-pre-wrap break-words text-muted-foreground">{firstString(input, ["skill", "name"])}</span></div>;
  }
  return <div key={key} className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2 py-1"><span className="font-semibold">Tool</span><span className="min-w-0"><span>{name}</span>{Object.keys(input).length > 0 && <code className="ml-2 whitespace-pre-wrap break-words text-muted-foreground">{firstString(input, ["query", "path", "prompt"])}</code>}</span></div>;
}

export function ProxyCallRow({
  call, mode, name, iteration,
}: { call: Call; mode: "enrich" | "full"; name: string; iteration: string }) {
  const [showInstr, setShowInstr] = useState(false);
  const [showThink, setShowThink] = useState(false);
  const [raw, setRaw] = useState<{ request: string; response: string } | null>(null);
  const [rawOpen, setRawOpen] = useState(false);

  // A failed/empty proxy call serializes nil slices as JSON `null` (the types
  // claim Block[]/Message[], but the runtime value can be null) — guard so one
  // bad call can't throw and unmount the whole app.
  const blocks = call.response.blocks ?? [];
  const delta = call.delta ?? [];
  const thinking = blocks.filter((b) => b.type === "thinking");
  const assistant = blocks.filter((b) => b.type !== "thinking");
  const tok = call.usage ? `${fmtTokens(call.usage.input)}→${fmtTokens(call.usage.output)} tok` : "";
  const cost = call.cost_usd != null ? ` · $${call.cost_usd.toFixed(4)}` : "";

  const openRaw = async () => {
    setRawOpen((v) => !v);
    if (!raw) {
      const calls = await fetchTranscriptRaw(name, iteration);
      const mine = calls.find((c) => c.seq === call.seq);
      if (mine) setRaw({ request: mine.request, response: mine.response });
    }
  };

  return (
    <div className="border-l-2 border-blue-500/40 pl-2 my-1 font-mono text-xs">
      {call.parse_error && <div className="text-amber-600">parse error — raw only</div>}

      {typeof call.instructions === "string" && call.instructions.length > 0 && (
        <div>
          <button className="text-left" onClick={() => setShowInstr((v) => !v)}>
            📜 instructions
            {call.instructions_changed && call.seq > 0 && <span className="text-amber-600"> ⚠ changed</span>}
          </button>
          {showInstr && <pre className="whitespace-pre-wrap opacity-80">{call.instructions}</pre>}
        </div>
      )}

      {thinking.length > 0 && (
        <div>
          <button className="text-left text-muted-foreground" onClick={() => setShowThink((v) => !v)}>🧠 thinking</button>
          {showThink && <pre className="whitespace-pre-wrap opacity-70">{thinking.map((b) => b.text).join("\n")}</pre>}
        </div>
      )}

      {mode === "full" && delta.flatMap((m: Message, i) => (m.blocks ?? []).map((b, j) => activityBlock(b, `d${i}-${j}`)))}
      {mode === "full" && assistant.map((b, i) => activityBlock(b, `a${i}`))}

      <details className="mt-1 text-muted-foreground">
        <summary className="cursor-pointer">AI call · {call.model} · {tok}{cost}{call.truncated ? " · response truncated" : ""}</summary>
        <button className="mt-1 underline-offset-2 hover:underline" onClick={openRaw}>view raw ▾</button>
      </details>
      {rawOpen && raw && (
        <pre className="whitespace-pre-wrap opacity-70 max-h-64 overflow-auto">{"REQUEST\n" + raw.request + "\n\nRESPONSE\n" + raw.response}</pre>
      )}
    </div>
  );
}
