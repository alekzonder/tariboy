import { Button } from "@/components/ui/button";
import type { DestinationTarget } from "./useImageTransfer";

export function TransferTargetList({
  targets,
  selected,
  allSelected,
  disabled,
  onToggleAll,
  onToggle,
}: {
  targets: DestinationTarget[];
  selected: Set<string>;
  allSelected: boolean;
  disabled: boolean;
  onToggleAll: () => void;
  onToggle: (id: string) => void;
}) {
  return (
    <>
      <Button variant="outline" size="sm" onClick={onToggleAll} disabled={disabled}>
        {allSelected ? "Clear all servers" : "All servers"}
      </Button>
      <ul className="space-y-2" aria-label="Eligible transfer targets">
        {targets.map((target) => (
          <li key={target.id}>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                aria-label={`Transfer to ${target.label}`}
                checked={selected.has(target.id)}
                onChange={() => onToggle(target.id)}
                disabled={disabled}
              />
              {target.label}
            </label>
          </li>
        ))}
      </ul>
    </>
  );
}
