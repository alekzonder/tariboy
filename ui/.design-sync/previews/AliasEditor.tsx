import { useEffect, useRef } from "react";
import { AgentNameContext, AliasEditor } from "tariboy-ui";

// AliasEditor self-fetches the alias for the agent named by AgentNameContext
// (GET /api/agents/:name/alias). With no daemon behind the preview that call
// rejects, so `alias` stays "" and the component renders its no-alias heading —
// the real state an operator sees for every agent that has never been aliased.
// The context MUST come from "tariboy-ui": importing it from @/lib/agent would
// be a second, unrelated context and useAgentName() would read "".

// Drive the component through its own UI (a real click on its own button, then
// a real input event on its own field) once the mount fetch has settled —
// nothing is stubbed or reimplemented.
function useEdit(draft: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    // Two ticks: the click flips the component into its editing branch, and the
    // input event lands on the field that branch renders.
    const open = window.setTimeout(() => {
      ref.current?.querySelector<HTMLElement>("button")?.click();
    }, 120);
    const type = window.setTimeout(() => {
      const input = ref.current?.querySelector<HTMLInputElement>('input[placeholder="alias"]');
      if (!input) return;
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
      setter?.call(input, draft);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    }, 260);
    return () => {
      window.clearTimeout(open);
      window.clearTimeout(type);
    };
  }, [draft]);
  return ref;
}

export const NoAlias = () => (
  <AgentNameContext.Provider value="builder">
    <AliasEditor />
  </AgentNameContext.Provider>
);

export const Editing = () => {
  const ref = useEdit("Reviewer (nightly)");
  return (
    <div ref={ref} style={{ width: 520 }}>
      <AgentNameContext.Provider value="reviewer">
        <AliasEditor />
      </AgentNameContext.Provider>
    </div>
  );
};

// The same heading in the header row it ships in (AgentLayout renders it beside
// the live loop state).
export const InAgentHeader = () => (
  <div
    style={{
      display: "flex",
      flexWrap: "wrap",
      alignItems: "center",
      gap: 12,
      borderBottom: "1px solid var(--border)",
      padding: "8px 16px",
      width: 620,
    }}
  >
    <AgentNameContext.Provider value="packager">
      <AliasEditor />
    </AgentNameContext.Provider>
    <span style={{ fontSize: 14, color: "var(--muted-foreground)" }}>
      status:{" "}
      <span style={{ font: "14px ui-monospace, SFMono-Regular, Menlo, monospace", color: "var(--foreground)" }}>
        running
      </span>
    </span>
  </div>
);
