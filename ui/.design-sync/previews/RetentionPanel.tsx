import { useEffect, useRef } from "react";
import { RetentionPanel } from "tariboy-ui";

// RetentionPanel self-fetches the policy for `name`; with no daemon the two
// number fields come up empty and the read-only "archive / max_bytes" line is
// omitted. Everything else on the card — the two timing captions, the
// non-destructive Save/Preview pair, and the bounded destructive group — is
// static, so this is the real card with the policy values typed in.

function typeInto(el: HTMLInputElement | null, text: string) {
  if (!el) return;
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(el, text);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

const Frame = ({ children }: { children: React.ReactNode }) => (
  <div style={{ maxWidth: 660 }}>{children}</div>
);

export const Policy = () => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const id = window.setInterval(() => {
      const root = ref.current;
      const keep = root?.querySelector("#keep-it") as HTMLInputElement | null;
      const days = root?.querySelector("#keep-days") as HTMLInputElement | null;
      if (!keep || !days) return;
      window.clearInterval(id);
      typeInto(keep, "50");
      typeInto(days, "14");
    }, 25);
    return () => window.clearInterval(id);
  }, []);
  return (
    <Frame>
      <div ref={ref}>
        <RetentionPanel name="builder" />
      </div>
    </Frame>
  );
};

// Prune is the one destructive action on the agent Settings tab, so it is
// confirmed first. The dialog is opened the way the operator opens it — by
// clicking the real "Prune now" button after mount — rather than by faking
// state the component does not expose.
export const ConfirmPrune = () => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let tries = 0;
    const id = window.setInterval(() => {
      const root = ref.current;
      if (!root || ++tries > 40) { window.clearInterval(id); return; }
      const btn = Array.from(root.querySelectorAll("button")).find(
        (b) => (b.textContent ?? "").trim() === "Prune now",
      );
      if (!btn) return;
      window.clearInterval(id);
      btn.click();
    }, 25);
    return () => window.clearInterval(id);
  }, []);
  return (
    <Frame>
      <div ref={ref}>
        <RetentionPanel name="builder" />
      </div>
    </Frame>
  );
};
