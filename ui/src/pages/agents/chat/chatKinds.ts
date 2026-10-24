export const KINDS = [
  ["dm", "Chat"],
  ["tasks", "Tasks"],
  ["service", "Service"],
] as const;
type Kind = (typeof KINDS)[number][0];

export function isKind(value: string): value is Kind {
  return KINDS.some(([key]) => key === value);
}
