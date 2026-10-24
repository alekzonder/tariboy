import { DraftField } from "./ConfigControls";
import { fieldOfKey, type ConfigField } from "./registry";
import type { BlockProps } from "./ConfigurationBlocks";

const field = (key: string) => fieldOfKey(key) as ConfigField;

const PERIODS = [
  ["hour", "Hour"],
  ["day", "Day"],
  ["week", "Week"],
  ["month", "Month"],
] as const;

export function BudgetsBlock({ draft, matched, sources }: BlockProps) {
  const budget = sources.view?.budget;
  if (!budget) return null;
  const exhausted = budget.exhausted ?? [];
  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {PERIODS.map(([period]) => {
          const key = `budget-${period}`;
          const spent = budget[`${period}_spent_usd`];
          const limit = Number(draft.values[key] ?? 0);
          const helper =
            limit === 0
              ? `Spent ${spent.toFixed(2)} · Unlimited`
              : `Spent ${spent.toFixed(2)} of ${limit.toFixed(2)}`;
          return (
            <DraftField
              key={key}
              field={{ ...field(key), helper }}
              draft={draft}
              matched={matched.has(key)}
            />
          );
        })}
      </div>
      {exhausted.length > 0 && (
        <p role="status" className="text-sm text-destructive">
          Out of budget: {exhausted.join(", ")}
        </p>
      )}
    </>
  );
}
