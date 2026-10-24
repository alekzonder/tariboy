import { describe, it, expect } from "vitest";
import { render } from "@/test/queryRender";
import { ProxyCallRow } from "@/components/ProxyCallRow";
import type { Call } from "@/lib/api";

const callWithTool = (toolName: string | undefined, input: unknown): Call => ({
  seq: 1,
  ts: "t",
  provider: "openai",
  model: "gpt",
  instructions: "",
  instructions_changed: false,
  delta: [],
  response: { blocks: [{ type: "tool_use", tool_name: toolName, input }] },
});

// activityRow returns the label and body text of the single rendered activity row.
function activityRow(toolName: string | undefined, input: unknown) {
  const { container } = render(
    <ProxyCallRow call={callWithTool(toolName, input)} mode="full" name="a" iteration="i" />,
  );
  const row = container.querySelector(".grid");
  const [label, body] = Array.from(row?.children ?? []);
  return { label: label?.textContent, body, text: body?.textContent };
}

describe("ProxyCallRow tool block table", () => {
  it.each(["exec_command", "command_execution", "bash", "Bash", "shell", "local_shell"])(
    "%s → Command from cmd, then command",
    (tool) => {
      const row = activityRow(tool, { command: "ls", cmd: "pwd" });
      expect(row.label).toBe("Command");
      expect(row.body?.tagName).toBe("CODE");
      expect(row.text).toBe("pwd");
      expect(activityRow(tool, { command: "ls" }).text).toBe("ls");
    },
  );

  it.each(["skill", "Skill", "mcp__skill", "codex.skill"])(
    "%s → Skill from skill, then name",
    (tool) => {
      const row = activityRow(tool, { name: "n", skill: "tdd" });
      expect(row.label).toBe("Skill");
      expect(row.body?.tagName).toBe("SPAN");
      expect(row.text).toBe("tdd");
      expect(activityRow(tool, { name: "n" }).text).toBe("n");
    },
  );

  it.each([
    ["search", { path: "p", query: "q" }, "searchq"],
    ["read", { prompt: "x", path: "p" }, "readp"],
    ["ask", { prompt: "x" }, "askx"],
    ["other", { a: 1 }, 'other{"a":1}'],
    ["other", '{"query":"from json"}', "otherfrom json"],
    ["other", "plain text", "otherplain text"],
    ["none", {}, "none"],
    [undefined, {}, "tool"],
  ])("%s with %j → Tool %s", (tool, input, text) => {
    const row = activityRow(tool, input);
    expect(row.label).toBe("Tool");
    expect(row.text).toBe(text);
  });
});
