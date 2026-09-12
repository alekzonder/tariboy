import { useEffect, useRef } from "react";
import { SecretsPanel } from "tariboy-ui";

// SecretsPanel is write-only by design: it lists secret KEY names fetched from
// the daemon and never renders a value back. With no daemon the list resolves
// empty, which is the panel's real first-run state — the empty list carries its
// own invitation copy instead of a bare "none", and the store form below is the
// whole point of the card.

// Type through the native setter so React's controlled inputs see a real input
// event; the text rendered is the component's own state.
function typeInto(el: HTMLInputElement | null, text: string) {
  if (!el) return;
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(el, text);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

const Frame = ({ children }: { children: React.ReactNode }) => (
  <div style={{ maxWidth: 660 }}>{children}</div>
);

export const NoSecretsYet = () => (
  <Frame>
    <SecretsPanel name="builder" />
  </Frame>
);

// Mid-entry: a key typed, the value masked by the component's own password
// input. This is what the operator looks at right before "Store secret".
export const EnteringASecret = () => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const id = window.setInterval(() => {
      const root = ref.current;
      const key = root?.querySelector("#secret-key") as HTMLInputElement | null;
      const value = root?.querySelector("#secret-value") as HTMLInputElement | null;
      if (!key || !value) return;
      window.clearInterval(id);
      typeInto(key, "ANTHROPIC_API_KEY");
      typeInto(value, "sk-ant-builder-key");
    }, 25);
    return () => window.clearInterval(id);
  }, []);
  return (
    <Frame>
      <div ref={ref}>
        <SecretsPanel name="builder" />
      </div>
    </Frame>
  );
};
