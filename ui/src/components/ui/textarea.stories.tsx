import type { Meta, StoryObj } from "@storybook/react-vite";
import { Label } from "./label";
import { Textarea } from "./textarea";

const meta = { component: Textarea } satisfies Meta<typeof Textarea>;
export default meta;

type Story = StoryObj;

const FIELD = "grid w-105 gap-1.5";
const HELP = "text-ui text-muted-foreground";
const MONO = "font-mono text-ui";

export const NotesField: Story = {
  render: () => (
    <div className={FIELD}>
      <Label htmlFor="preview-notes">notes</Label>
      <Textarea
        id="preview-notes"
        defaultValue="Owns the release train for TB-142. Restart after every worker:v2 rebuild."
      />
      <span className={HELP}>Shown on the agent card and in the sidebar tooltip.</span>
    </div>
  ),
};

export const EnvironmentJson: Story = {
  render: () => (
    <div className={FIELD}>
      <Label htmlFor="preview-env">environment JSON</Label>
      <Textarea
        id="preview-env"
        className={`${MONO} min-h-28`}
        defaultValue={
          '{\n  "TARIBOY_PROFILE": "team",\n  "GIT_AUTHOR_NAME": "builder",\n  "HARNESS_EFFORT": "high"\n}'
        }
      />
    </div>
  ),
};

export const Placeholder: Story = {
  render: () => (
    <div className={FIELD}>
      <Label htmlFor="preview-oneshot">one-shot prompt</Label>
      <Textarea id="preview-oneshot" placeholder="one-shot exec prompt (optional)" />
    </div>
  ),
};

export const States: Story = {
  render: () => (
    <div className="grid w-105 gap-3">
      <div className="grid gap-1.5">
        <Label htmlFor="preview-ta-disabled">standing user prompt</Label>
        <Textarea
          id="preview-ta-disabled"
          disabled
          defaultValue="Keep working through the review queue until it is empty."
        />
        <span className={HELP}>Editable once the agent is stopped.</span>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="preview-ta-invalid">Import compose YAML</Label>
        <Textarea
          id="preview-ta-invalid"
          aria-invalid
          className={MONO}
          defaultValue={"agents:\n  builder:\n    image worker:v2"}
        />
        <span className={`${HELP} text-destructive`}>
          Line 3: mapping value expected after “image”.
        </span>
      </div>
    </div>
  ),
};
