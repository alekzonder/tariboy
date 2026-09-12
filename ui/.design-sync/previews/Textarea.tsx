import { Label, Textarea } from "tariboy-ui";

const field: React.CSSProperties = { display: "grid", gap: 6, width: 420 };
const help: React.CSSProperties = { fontSize: 12, color: "var(--muted-foreground)" };
const mono: React.CSSProperties = {
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
  fontSize: 12,
};

export const NotesField = () => (
  <div style={field}>
    <Label htmlFor="preview-notes">notes</Label>
    <Textarea
      id="preview-notes"
      defaultValue={
        "Owns the release train for TB-142. Restart after every worker:v2 rebuild."
      }
    />
    <span style={help}>Shown on the agent card and in the sidebar tooltip.</span>
  </div>
);

export const EnvironmentJson = () => (
  <div style={field}>
    <Label htmlFor="preview-env">environment JSON</Label>
    <Textarea
      id="preview-env"
      style={{ ...mono, minHeight: 112 }}
      defaultValue={
        '{\n  "TARIBOY_PROFILE": "team",\n  "GIT_AUTHOR_NAME": "builder",\n  "HARNESS_EFFORT": "high"\n}'
      }
    />
  </div>
);

export const Placeholder = () => (
  <div style={field}>
    <Label htmlFor="preview-oneshot">one-shot prompt</Label>
    <Textarea
      id="preview-oneshot"
      placeholder="one-shot exec prompt (optional)"
    />
  </div>
);

export const States = () => (
  <div style={{ display: "grid", gap: 12, width: 420 }}>
    <div style={{ display: "grid", gap: 6 }}>
      <Label htmlFor="preview-ta-disabled">standing user prompt</Label>
      <Textarea
        id="preview-ta-disabled"
        disabled
        defaultValue="Keep working through the review queue until it is empty."
      />
      <span style={help}>Editable once the agent is stopped.</span>
    </div>
    <div style={{ display: "grid", gap: 6 }}>
      <Label htmlFor="preview-ta-invalid">Import compose YAML</Label>
      <Textarea
        id="preview-ta-invalid"
        aria-invalid
        style={mono}
        defaultValue={"agents:\n  builder:\n    image worker:v2"}
      />
      <span style={{ ...help, color: "var(--destructive)" }}>
        Line 3: mapping value expected after “image”.
      </span>
    </div>
  </div>
);
