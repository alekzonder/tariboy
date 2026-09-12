import { useEffect, useRef, type ReactNode } from "react";
import { AgentSubscriptions } from "tariboy-ui";

// AgentSubscriptions self-fetches two endpoints on mount: the agent's own
// subscription rows and the channel catalogue. Both reject in a preview, the
// component catches and falls back to empty lists, and what renders is the real
// pane an operator sees for an agent with nothing subscribed: the SUBSCRIPTIONS
// rail header, the "No subscriptions." empty state, the editable channel
// combobox and the (correctly disabled) Subscribe button.

// Type into the component's own channel input the way a user would — a native
// value set plus a bubbling input event, so React/cmdk see a genuine change.
// Nothing about the component is stubbed.
function useTypeChannel(value: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const input = ref.current?.querySelector<HTMLInputElement>('input[aria-label="channel"]');
      if (!input) return;
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
      setter?.call(input, value);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    }, 150);
    return () => window.clearTimeout(timer);
  }, [value]);
  return ref;
}

const Rail = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      display: "flex",
      width: 620,
      height: 300,
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
      background: "var(--background)",
      overflow: "hidden",
    }}
  >
    {children}
  </div>
);

export const NoSubscriptions = () => (
  <Rail>
    <AgentSubscriptions name="builder" />
  </Rail>
);

// The client-side channel-name guard (mirrors the daemon's bus.ValidChannel):
// free text that is not `prefix:segment` surfaces the inline error and keeps
// Subscribe disabled before anything is POSTed.
export const InvalidChannelName = () => {
  const ref = useTypeChannel("dev team");
  return (
    <div ref={ref}>
      <Rail>
        <AgentSubscriptions name="reviewer" />
      </Rail>
    </div>
  );
};

// A valid free-text channel: the guard clears and Subscribe enables.
export const ValidChannelTyped = () => {
  const ref = useTypeChannel("chat:dev-team");
  return (
    <div ref={ref}>
      <Rail>
        <AgentSubscriptions name="packager" />
      </Rail>
    </div>
  );
};
