import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Label } from "@/components/ui/label";
import { EditablePresetCombobox } from "./EditablePresetCombobox";

const meta = { component: EditablePresetCombobox } satisfies Meta<typeof EditablePresetCombobox>;
export default meta;

type Story = StoryObj;

// Options are what src/lib/runtimePresets.ts hands the create-agent form.
const EFFORTS = ["low", "medium", "high", "xhigh", "max", "ultracode"];
const CLAUDE_MODELS = ["claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5", "claude-fable-5"];

// The create-agent form wraps every combobox in a labelled field.
const Field = ({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: ReactNode;
}) => (
  <div className="grid w-100 gap-1.5">
    <Label htmlFor={id} className="text-body">
      {label}
    </Label>
    {children}
    {hint ? <p className="m-0 text-ui text-muted-foreground">{hint}</p> : null}
  </div>
);

// The preset list only exists while the input owns focus, so the open cells
// focus the real input and, where a filter is shown, drive a real input event
// through the component's own onChange.
function useFocusedList(typed?: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const input = ref.current?.querySelector("input");
    if (!input) return;
    input.focus();
    if (typed === undefined) return;
    const setValue = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      "value",
    )?.set;
    setValue?.call(input, typed);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  }, [typed]);
  return ref;
}

function ModelDemo({
  id,
  initial,
  typed,
  focus,
  hint,
}: {
  id: string;
  initial: string;
  typed?: string;
  focus?: boolean;
  hint?: string;
}) {
  const [model, setModel] = useState(initial);
  const ref = useFocusedList(typed);
  const field = (
    <Field id={id} label="model" hint={hint}>
      <EditablePresetCombobox
        id={id}
        ariaLabel="model"
        value={model}
        options={CLAUDE_MODELS}
        onChange={setModel}
        placeholder="image default"
      />
    </Field>
  );
  return focus ? <div ref={ref}>{field}</div> : field;
}

function EffortDemo() {
  const [effort, setEffort] = useState("high");
  return (
    <Field id="effort" label="effort" hint="Effort preset passed to the claude harness.">
      <EditablePresetCombobox
        id="effort"
        ariaLabel="effort"
        value={effort}
        options={EFFORTS}
        onChange={setEffort}
        placeholder="image default"
      />
    </Field>
  );
}

export const EffortPreset: Story = { render: () => <EffortDemo /> };

export const PresetsOpen: Story = {
  render: () => <ModelDemo focus id="model-open" initial="claude-opus-4-8" />,
};

export const TypedFilter: Story = {
  render: () => <ModelDemo focus id="model-filter" initial="claude-opus-4-8" typed="claude-s" />,
};

export const CustomValue: Story = {
  render: () => (
    <ModelDemo
      id="model-custom"
      initial="claude-opus-5-20260501"
      hint="Free text is kept as typed and remembered as a preset for the next agent."
    />
  ),
};

export const Disabled: Story = {
  render: () => (
    <Field
      id="model-locked"
      label="model"
      hint="worker:v1 pins the model — inherited from the image."
    >
      <EditablePresetCombobox
        id="model-locked"
        ariaLabel="model"
        value="gpt-5"
        options={["gpt-5"]}
        onChange={() => {}}
        disabled
      />
    </Field>
  ),
};
