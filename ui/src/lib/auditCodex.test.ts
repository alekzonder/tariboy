import { describe, it, expect } from "vitest";
import { codexHarnessPresentation } from "./auditCodex";
import type { AuditLogEvent } from "./audit";

const codexEvent = (outer: unknown, kind = "harness_output"): AuditLogEvent => ({
  seq: 1,
  kind,
  source: "harness",
  at: "t",
  data: JSON.stringify({ line: typeof outer === "string" ? outer : JSON.stringify(outer) }),
  iteration_id: "it-1",
});
const completed = (item: Record<string, unknown>) =>
  codexHarnessPresentation(codexEvent({ type: "item.completed", item }));

describe("codexHarnessPresentation handlers", () => {
  it("command_execution → command with status and output", () => {
    expect(
      completed({
        type: "command_execution",
        command: "npm test",
        status: "completed",
        aggregated_output: "ok",
      }),
    ).toEqual({
      type: "command",
      label: "command",
      icon: "⌨",
      text: "npm test",
      status: "completed",
      output: "ok",
    });
    expect(
      codexHarnessPresentation(
        codexEvent({ type: "item.started", item: { type: "command_execution", cmd: "ls" } }),
      )?.status,
    ).toBe("running");
    expect(completed({ type: "command_execution", command: "x", exit_code: 2 })?.status).toBe(
      "failed",
    );
    expect(completed({ type: "command_execution", command: "x", exit_code: 0 })?.status).toBe(
      "completed",
    );
    expect(completed({ type: "command_execution" })).toBeUndefined();
  });

  it("agent_message → message", () => {
    expect(completed({ type: "agent_message", message: "hi", status: "done" })).toEqual({
      type: "message",
      label: "message",
      icon: "💬",
      text: "hi",
      status: "done",
    });
    expect(completed({ type: "agent_message" })).toBeUndefined();
  });

  it("mcp_tool_call → tool with server, tool and salient argument", () => {
    expect(
      completed({
        type: "mcp_tool_call",
        server: "apps",
        tool: "search",
        arguments: { query: "audit" },
        status: "completed",
        error: "boom",
      }),
    ).toEqual({
      type: "tool",
      label: "Tool",
      icon: "🔧",
      text: "apps · search — audit",
      status: "completed",
      output: "boom",
    });
    expect(completed({ type: "mcp_tool_call" })?.text).toBe("MCP · tool");
  });

  it("skill → skill", () => {
    expect(completed({ type: "skill", skill: "tdd" })).toEqual({
      type: "skill",
      label: "Skill",
      icon: "✦",
      text: "tdd",
      status: undefined,
    });
    expect(completed({ type: "skill" })).toBeUndefined();
  });

  it("reasoning → thinking", () => {
    expect(completed({ type: "reasoning", summary: "plan" })).toEqual({
      type: "thinking",
      label: "Thinking",
      icon: "🧠",
      text: "plan",
    });
    expect(completed({ type: "reasoning" })).toBeUndefined();
  });

  it("file_change → file with the changed-files list", () => {
    expect(
      completed({
        type: "file_change",
        changes: [{ path: "a.ts", kind: "update" }, { file_path: "b.ts" }, null, { kind: "x" }],
        status: "completed",
      }),
    ).toEqual({
      type: "file",
      label: "File change",
      icon: "📝",
      text: "a.ts (update), b.ts",
      status: "completed",
    });
    expect(completed({ type: "file_change", path: "README.md" })).toBeUndefined();
  });
});

describe("codexHarnessPresentation fallbacks", () => {
  it("no handler for the item type → undefined", () => {
    expect(completed({ type: "web_search" })).toBeUndefined();
    expect(
      codexHarnessPresentation(codexEvent({ type: "turn.completed", item: {} })),
    ).toBeUndefined();
    expect(codexHarnessPresentation(codexEvent({ type: "item.completed" }))).toBeUndefined();
    expect(
      codexHarnessPresentation(codexEvent({ type: "item.completed", item: {} }, "status")),
    ).toBeUndefined();
  });

  it("broken line → undefined without throwing", () => {
    expect(codexHarnessPresentation(codexEvent("{not json"))).toBeUndefined();
    expect(
      codexHarnessPresentation({ ...codexEvent({}), data: JSON.stringify({ line: 3 }) }),
    ).toBeUndefined();
  });
});
