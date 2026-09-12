import { useEffect, useRef, useState } from "react";
import { EditablePresetCombobox, Label } from "tariboy-ui";

// Options are exactly what src/lib/runtimePresets.ts hands the create-agent
// form: runtimePresetOptions(harness, "models" | "efforts").
const EFFORTS = ["low", "medium", "high", "xhigh", "max", "ultracode"];
const CLAUDE_MODELS = [
  "claude-opus-4-8",
  "claude-sonnet-5",
  "claude-haiku-4-5",
  "claude-fable-5",
];

// The create-agent form wraps every combobox in a labelled field; on its own the
// h-8 input captures as a sliver.
const Field = ({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: React.ReactNode;
}) => (
  <div style={{ display: "grid", gap: 6, width: 400 }}>
    <Label htmlFor={id} style={{ fontSize: 13 }}>
      {label}
    </Label>
    {children}
    {hint ? (
      <p style={{ margin: 0, fontSize: 12, color: "var(--muted-foreground)" }}>{hint}</p>
    ) : null}
  </div>
);

// The preset list only exists while the input owns focus (onFocus opens it), so
// the open cells focus the real input and, where a filter is shown, drive a real
// input event through the component's own onChange. Nothing is stubbed.
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

export const EffortPreset = () => {
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
};

export const PresetsOpen = () => {
  const [model, setModel] = useState("claude-opus-4-8");
  const ref = useFocusedList();
  return (
    <div ref={ref}>
      <Field id="model-open" label="model">
        <EditablePresetCombobox
          id="model-open"
          ariaLabel="model"
          value={model}
          options={CLAUDE_MODELS}
          onChange={setModel}
          placeholder="image default"
        />
      </Field>
    </div>
  );
};

export const TypedFilter = () => {
  const [model, setModel] = useState("claude-opus-4-8");
  const ref = useFocusedList("claude-s");
  return (
    <div ref={ref}>
      <Field id="model-filter" label="model">
        <EditablePresetCombobox
          id="model-filter"
          ariaLabel="model"
          value={model}
          options={CLAUDE_MODELS}
          onChange={setModel}
          placeholder="image default"
        />
      </Field>
    </div>
  );
};

export const CustomValue = () => {
  const [model, setModel] = useState("claude-opus-5-20260501");
  return (
    <Field
      id="model-custom"
      label="model"
      hint="Free text is kept as typed and remembered as a preset for the next agent."
    >
      <EditablePresetCombobox
        id="model-custom"
        ariaLabel="model"
        value={model}
        options={CLAUDE_MODELS}
        onChange={setModel}
        placeholder="image default"
      />
    </Field>
  );
};

export const Disabled = () => (
  <Field id="model-locked" label="model" hint="worker:v1 pins the model — inherited from the image.">
    <EditablePresetCombobox
      id="model-locked"
      ariaLabel="model"
      value="gpt-5"
      options={["gpt-5"]}
      onChange={() => {}}
      disabled
    />
  </Field>
);
