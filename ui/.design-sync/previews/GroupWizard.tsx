import { useEffect, useRef, type ReactNode } from "react";
import { GroupWizard } from "tariboy-ui";

// GroupWizard is the multi-section group builder: section 1 is the group (name +
// base cwd + which row leads), section 2 a repeatable list of agent drafts. It
// only fetches the image catalogue (to fill the per-row ImageCombobox), so with
// no daemon behind the preview the whole form still renders exactly as shipped —
// the combobox is simply offered no options.
//
// Every state below is reached by driving the component's own controls (native
// value sets + bubbling events, real clicks); nothing is stubbed or redrawn.

type Step = { selector: string; value?: string; click?: boolean; text?: string };

function useDrive(steps: Step[]) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const root = ref.current;
      if (!root) return;
      for (const step of steps) {
        const all = [...root.querySelectorAll<HTMLElement>(step.selector)];
        const el = step.text
          ? all.find((node) => node.textContent?.includes(step.text!))
          : all[0];
        if (!el) continue;
        if (step.click) {
          el.click();
        } else if (step.value !== undefined) {
          const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
          setter?.call(el, step.value);
          el.dispatchEvent(new Event("input", { bubbles: true }));
        }
      }
    }, 150);
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return ref;
}

const Frame = ({ children }: { children: ReactNode }) => (
  <div style={{ width: 720 }}>{children}</div>
);

// First open: one empty draft row, Create group disabled, and the leader-name
// gate spelled out in destructive text (a blank leader would create the group
// leaderless).
export const EmptyDraft = () => (
  <Frame>
    <GroupWizard />
  </Frame>
);

// Group named and the lead row named: the `lead:` line resolves and the
// leader-name warning clears. Create stays disabled until the row has an image.
export const NamedGroup = () => {
  const ref = useDrive([
    { selector: "#grp-name", value: "dev-team" },
    { selector: "#name-0", value: "builder" },
  ]);
  return (
    <div ref={ref}>
      <Frame>
        <GroupWizard />
      </Frame>
    </div>
  );
};

// The per-row Advanced panel opened through its own Collapsible trigger:
// harness / model / timeout / effort, the interactive + Autopilot switches, and
// the env (K=V) editor.
export const AdvancedOpen = () => {
  const ref = useDrive([
    { selector: "#grp-name", value: "dev-team" },
    { selector: "#name-0", value: "builder" },
    { selector: '[data-slot="agent-row"] button', text: "Advanced", click: true },
  ]);
  return (
    <div ref={ref}>
      <Frame>
        <GroupWizard />
      </Frame>
    </div>
  );
};
