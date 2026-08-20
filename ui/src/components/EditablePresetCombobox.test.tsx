import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { EditablePresetCombobox } from "./EditablePresetCombobox";

function ControlledCombobox({
  initial = "",
  options = ["gpt-5", "private-model"],
}: {
  initial?: string;
  options?: string[];
}) {
  const [value, setValue] = useState(initial);
  return (
    <EditablePresetCombobox
      ariaLabel="model"
      value={value}
      options={options}
      onChange={setValue}
      placeholder="image default"
    />
  );
}

describe("EditablePresetCombobox", () => {
  it("accepts arbitrary values and filters presets case-insensitively", () => {
    const onChange = vi.fn();
    render(
      <EditablePresetCombobox
        ariaLabel="model"
        value=""
        options={["gpt-5", "Private-Model"]}
        onChange={onChange}
      />,
    );

    fireEvent.change(screen.getByRole("combobox", { name: "model" }), {
      target: { value: "private" },
    });

    expect(onChange).toHaveBeenCalledWith("private");
    expect(screen.getByRole("option", { name: "Private-Model" })).toBeVisible();
    expect(screen.queryByRole("option", { name: "gpt-5" })).not.toBeInTheDocument();
  });

  it("shows every preset on focus even when the field already has a value", () => {
    render(<ControlledCombobox initial="o3" />);

    fireEvent.focus(screen.getByRole("combobox", { name: "model" }));

    expect(screen.getByRole("option", { name: "gpt-5" })).toBeVisible();
    expect(screen.getByRole("option", { name: "private-model" })).toBeVisible();
    expect(screen.getByRole("combobox", { name: "model" })).toHaveValue("o3");
  });

  it("selects a preset before blur closes the list", () => {
    render(<ControlledCombobox />);
    const input = screen.getByRole("combobox", { name: "model" });
    fireEvent.focus(input);
    const option = screen.getByRole("option", { name: "gpt-5" });

    fireEvent.mouseDown(option);
    fireEvent.click(option);

    expect(input).toHaveValue("gpt-5");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("renders an empty state and respects disabled", () => {
    render(
      <EditablePresetCombobox
        ariaLabel="effort"
        value=""
        options={[]}
        onChange={vi.fn()}
        disabled
      />,
    );

    const input = screen.getByRole("combobox", { name: "effort" });
    expect(input).toBeDisabled();
    fireEvent.focus(input);
    expect(screen.queryByText("No presets")).not.toBeInTheDocument();
  });
});
