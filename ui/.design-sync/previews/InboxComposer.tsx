import { useEffect, useRef } from "react";
import { InboxComposer } from "tariboy-ui";

// The composer is the pinned footer of the agent Overview tab for
// non-interactive agents; it sizes itself to that column.
const Footer = ({ name, children }: { name: string; children: React.ReactNode }) => (
  <div style={{ width: 680, display: "grid", gap: 10 }}>
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 8,
        fontSize: 12,
        color: "var(--muted-foreground)",
      }}
    >
      <span style={{ fontFamily: "var(--font-mono)" }}>agent:{name}:inbox</span>
      <span style={{ marginLeft: "auto" }}>2 queued · picked up next iteration</span>
    </div>
    {children}
  </div>
);

// The draft lives in the component's own state, so the cells that show a typed
// message drive a real input event through its onChange — the same path a
// keystroke takes.
function useDraft(text: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const input = ref.current?.querySelector("input");
    if (!input) return;
    const setValue = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      "value",
    )?.set;
    setValue?.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  }, [text]);
  return ref;
}

export const Empty = () => (
  <Footer name="builder">
    <InboxComposer name="builder" />
  </Footer>
);

export const Drafted = () => {
  const ref = useDraft("TB-142 is blocked on the packager — re-run the release check first.");
  return (
    <Footer name="builder">
      <div ref={ref}>
        <InboxComposer name="builder" />
      </div>
    </Footer>
  );
};

export const WithUploadedPaths = () => {
  const ref = useDraft(
    "Use these for the release notes /work/files/worker-manifest.json /work/files/TB-142.patch",
  );
  return (
    <Footer name="packager">
      <div ref={ref}>
        <InboxComposer name="packager" />
      </div>
    </Footer>
  );
};

export const InOverviewFooter = () => (
  <div style={{ width: 680, display: "grid", gap: 12 }}>
    <div
      style={{
        height: 150,
        borderRadius: 10,
        border: "1px solid var(--border)",
        background: "var(--muted)",
        display: "flex",
        alignItems: "flex-end",
        padding: 10,
        fontSize: 12,
        color: "var(--muted-foreground)",
        fontFamily: "var(--font-mono)",
      }}
    >
      iteration 48 — reviewing TB-142
    </div>
    <InboxComposer name="reviewer" />
  </div>
);
