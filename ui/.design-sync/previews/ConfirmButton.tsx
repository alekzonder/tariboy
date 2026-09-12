import { useEffect, useRef } from "react";
import { ConfirmButton, Button, Input } from "tariboy-ui";

// The agent Overview control row ConfirmButton ships in: loop toggle, guarded
// actions, one-shot exec prompt.
const ControlRow = ({ children }: { children: React.ReactNode }) => (
  <div style={{ width: 680, display: "grid", gap: 12 }}>
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "10px 12px",
        borderRadius: 10,
        border: "1px solid var(--border)",
        background: "var(--card)",
        fontSize: 12,
        fontFamily: "var(--font-mono)",
      }}
    >
      <span>cwd: /work/tariboy</span>
      <span style={{ marginLeft: "auto", color: "var(--muted-foreground)" }}>
        12:04 — waiting on builder to push a fix
      </span>
    </div>
    <div style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-end", gap: 8 }}>
      {children}
    </div>
  </div>
);

const ExecPrompt = () => (
  <Input placeholder="one-shot exec prompt (optional)" className="h-8" style={{ flex: 1, minWidth: 0 }} />
);

// ConfirmButton owns its AlertDialog state and exposes no `open` prop, so the
// open cell presses the real trigger on mount.
function usePressTrigger(label: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const buttons = Array.from(ref.current?.querySelectorAll("button") ?? []);
    buttons.find((button) => button.textContent?.trim() === label)?.click();
  }, [label]);
  return ref;
}

export const RestartAgent = () => (
  <ControlRow>
    <Button size="sm">Start</Button>
    <ConfirmButton
      label="Restart"
      description="The loop session is recreated with the current model and prompt. The harness context is lost."
      onConfirm={async () => {}}
    />
    <ExecPrompt />
    <Button size="sm" variant="secondary">
      Exec
    </Button>
  </ControlRow>
);

export const DestructiveKill = () => (
  <ControlRow>
    <Button size="sm">Start</Button>
    <ConfirmButton
      label="Restart"
      description="The loop session is recreated with the current model and prompt. The harness context is lost."
      onConfirm={async () => {}}
    />
    <ConfirmButton
      label="Kill"
      variant="destructive"
      description="The current iteration is killed via its shim. The running harness context is lost."
      onConfirm={async () => {}}
    />
    <ExecPrompt />
  </ControlRow>
);

export const ConfirmOpen = () => {
  const ref = usePressTrigger("Kill");
  return (
    <div ref={ref}>
      <ControlRow>
        <Button size="sm">Start</Button>
        <ConfirmButton
          label="Restart"
          description="The loop session is recreated with the current model and prompt. The harness context is lost."
          onConfirm={async () => {}}
        />
        <ConfirmButton
          label="Kill"
          variant="destructive"
          description="The current iteration is killed via its shim. The running harness context is lost."
          onConfirm={async () => {}}
        />
        <ExecPrompt />
      </ControlRow>
    </div>
  );
};
