import { GoalHelp } from "tariboy-ui";

// GoalHelp takes no props: it is the "?" affordance beside the Goal field in the
// agent workspace header, and its explanation lives behind a Dialog the viewer
// opens. Composed here in the header row it actually ships in.
export const InAgentHeader = () => (
  <span style={{ display: "flex", alignItems: "center", gap: 2, fontSize: 14 }}>
    <span style={{ color: "var(--muted-foreground)" }}>Goal:</span>
    <span style={{ font: "14px ui-monospace, SFMono-Regular, Menlo, monospace", color: "var(--primary)" }}>
      TB-142
    </span>
    <GoalHelp />
  </span>
);

export const NoCurrentGoal = () => (
  <span style={{ display: "flex", alignItems: "center", gap: 2, fontSize: 14 }}>
    <span style={{ color: "var(--muted-foreground)" }}>Goal:</span>
    <span style={{ color: "var(--muted-foreground)" }}>No current goal</span>
    <GoalHelp />
  </span>
);
