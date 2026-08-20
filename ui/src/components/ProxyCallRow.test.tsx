import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ProxyCallRow } from "@/components/ProxyCallRow";
import type { Call } from "@/lib/transcript";

afterEach(() => vi.restoreAllMocks());

const base: Call = {
  seq: 0, ts: "t", provider: "anthropic", model: "claude-opus-4",
  usage: { input: 1200, output: 800, cache_read: 0, cache_write: 0 },
  cost_usd: 0.01, instructions: "Follow the rules.", instructions_changed: true,
  delta: [{ role: "user", blocks: [{ type: "text", text: "run tests" }] }],
  response: { blocks: [{ type: "thinking", text: "check config first" }, { type: "text", text: "running" }], stop_reason: "end_turn" },
};

describe("ProxyCallRow", () => {
  it("enrich mode shows thinking + instructions, hides assistant/delta text", () => {
    render(<ProxyCallRow call={base} mode="enrich" name="a1" iteration="it" />);
    fireEvent.click(screen.getByText(/thinking/i));
    expect(screen.getByText("check config first")).toBeInTheDocument();
    fireEvent.click(screen.getByText(/instructions/i));
    expect(screen.getByText("Follow the rules.")).toBeInTheDocument();
    // assistant text and the user delta are NOT rendered in enrich mode
    expect(screen.queryByText("running")).not.toBeInTheDocument();
    expect(screen.queryByText("run tests")).not.toBeInTheDocument();
  });

  it("full mode renders assistant text and the delta", () => {
    render(<ProxyCallRow call={base} mode="full" name="a1" iteration="it" />);
    expect(screen.getByText("running")).toBeInTheDocument();
    expect(screen.getByText("run tests")).toBeInTheDocument();
  });

  it("full mode renders Codex commands, skills, results, and messages as readable activity", () => {
    const codex: Call = {
      ...base,
      provider: "openai",
      model: "gpt-5.6-sol",
      delta: [{ role: "tool", blocks: [{ type: "tool_result", tool_use_id: "call-1", text: "3 files matched" }] }],
      response: {
        blocks: [
          { type: "tool_use", tool_name: "exec_command", tool_use_id: "call-2", input: { cmd: "rg -n audit ui/src" } },
          { type: "tool_use", tool_name: "Skill", tool_use_id: "call-3", input: { skill: "brainstorming" } },
          { type: "text", text: "I found the audit renderer." },
        ],
        stop_reason: "completed",
      },
    };
    render(<ProxyCallRow call={codex} mode="full" name="a1" iteration="it" />);

    expect(screen.getByText("Command")).toBeInTheDocument();
    expect(screen.getByText("rg -n audit ui/src")).toBeInTheDocument();
    expect(screen.getByText("Skill")).toBeInTheDocument();
    expect(screen.getByText("brainstorming")).toBeInTheDocument();
    expect(screen.getByText("Result")).toBeInTheDocument();
    expect(screen.getByText("3 files matched")).toBeInTheDocument();
    expect(screen.getByText("Message")).toBeInTheDocument();
    expect(screen.getByText("I found the audit renderer.")).toBeInTheDocument();
    expect(screen.getByText(/AI call · gpt-5.6-sol/)).toBeInTheDocument();
  });

  it("renders a Responses local_shell command array as a shell command", () => {
    const codex: Call = {
      ...base,
      provider: "openai",
      response: { blocks: [{ type: "tool_use", tool_name: "local_shell", input: { type: "exec", command: ["bash", "-lc", "make check"] } }] },
    };
    render(<ProxyCallRow call={codex} mode="full" name="a1" iteration="it" />);
    expect(screen.getByText("Command")).toBeInTheDocument();
    expect(screen.getByText("bash -lc make check")).toBeInTheDocument();
  });

  it("seq 0 with instructions_changed true: instructions viewable, no 'changed' chip", () => {
    // seq 0's instructions_changed is true only because it's the first call, not
    // because anything actually changed — the chip must not appear here.
    render(<ProxyCallRow call={base} mode="enrich" name="a1" iteration="it" />);
    fireEvent.click(screen.getByText(/instructions/i));
    expect(screen.getByText("Follow the rules.")).toBeInTheDocument();
    expect(screen.queryByText(/changed/i)).not.toBeInTheDocument();
  });

  it("does not crash when blocks are null (failed/empty proxy response)", () => {
    // The backend serializes a nil []Block as JSON `null`; the type claims
    // Block[] but the runtime value can be null. Rendering must survive it —
    // a throw here unmounts the whole app (there is no error boundary).
    const broken = {
      ...base,
      parse_error: "response: unexpected end of JSON",
      delta: [{ role: "user", blocks: null }],
      response: { blocks: null, stop_reason: "" },
    } as unknown as Call;
    expect(() =>
      render(<ProxyCallRow call={broken} mode="full" name="a1" iteration="it" />),
    ).not.toThrow();
    expect(screen.getByText(/parse error/i)).toBeInTheDocument();
  });

  it("does not crash when delta itself is null (failed/empty proxy call)", () => {
    // The backend serializes a nil []Message as JSON `null`; the type claims
    // Message[] but the runtime value can be null. Full mode maps over delta,
    // so an unguarded map here throws and unmounts the whole app.
    const broken = { ...base, delta: null } as unknown as Call;
    expect(() =>
      render(<ProxyCallRow call={broken} mode="full" name="a1" iteration="it" />),
    ).not.toThrow();
  });

  it("unchanged later call still exposes viewable instructions", () => {
    const later: Call = { ...base, seq: 3, instructions_changed: false, instructions: "Still follow the rules." };
    render(<ProxyCallRow call={later} mode="enrich" name="a1" iteration="it" />);
    fireEvent.click(screen.getByText(/instructions/i));
    expect(screen.getByText("Still follow the rules.")).toBeInTheDocument();
    expect(screen.queryByText(/changed/i)).not.toBeInTheDocument();
  });
});
