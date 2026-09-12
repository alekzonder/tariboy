import { useEffect, useRef } from "react";
import { AgentNameContext, NotesEditor } from "tariboy-ui";

// NotesEditor reads the agent name from context and self-fetches the notes for
// it, so a preview supplies the context (the bundle's own instance) and lets
// the fetch fail — the card, label and empty textarea are what the operator
// sees before the first byte arrives, and what stays when the agent has no
// notes yet.

// Drive the real textarea the way an operator does: set the value through the
// native setter and dispatch the input event React listens for. Nothing is
// stubbed — the text below is the component's own state, rendered by the
// component's own mono textarea.
function typeInto(el: HTMLTextAreaElement | null, text: string) {
  if (!el) return;
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
  setter?.call(el, text);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

const NOTES = `TB-142 — desktop updater contract.
Reviewer wants the shim to resolve after acknowledgement, not before.
Retry the packager run once build-01 is back; worker:v2 only.`;

const Frame = ({ children }: { children: React.ReactNode }) => (
  <AgentNameContext.Provider value="builder">
    <div style={{ maxWidth: 560 }}>{children}</div>
  </AgentNameContext.Provider>
);

export const Empty = () => (
  <Frame>
    <NotesEditor />
  </Frame>
);

export const WithNotes = () => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const id = window.setInterval(() => {
      const el = ref.current?.querySelector("textarea") as HTMLTextAreaElement | null;
      if (!el) return;
      window.clearInterval(id);
      typeInto(el, NOTES);
    }, 25);
    return () => window.clearInterval(id);
  }, []);
  return (
    <Frame>
      <div ref={ref}>
        <NotesEditor />
      </div>
    </Frame>
  );
};
