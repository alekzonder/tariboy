import type { ConfigField } from "./registry";
import type { ConfigDraft } from "./useConfigDraft";

export const unsaved = (count: number) => `${count} unsaved ${count === 1 ? "change" : "changes"}`;

// describedBy links a control to its helper and its error, when shown.
export const describedBy = (field: ConfigField, draft: ConfigDraft) =>
  [field.helper && `${field.key}-help`, draft.fieldErrors[field.key] && `${field.key}-error`]
    .filter(Boolean)
    .join(" ") || undefined;

const INPUT_TYPES: Partial<Record<ConfigField["kind"], string>> = {
  number: "number",
  "secret-value": "password",
};

// inputAttributes picks the HTML input attributes for a plain input setting.
export function inputAttributes(field: ConfigField) {
  const number = field.kind === "number";
  return {
    type: INPUT_TYPES[field.kind],
    autoComplete: field.kind === "secret-value" ? "new-password" : undefined,
    min: number ? (field.minimum ?? 0) : undefined,
    step: field.key.startsWith("budget-") ? "0.01" : undefined,
  };
}
