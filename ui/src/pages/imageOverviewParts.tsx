import { type ReactNode } from "react";

export function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex gap-2">
      <span className="w-32 shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 flex-1 break-words font-mono text-xs">{value}</span>
    </div>
  );
}

export function Chips({ items }: { items: string[] }) {
  return (
    <div className="flex flex-wrap gap-1">
      {items.map((s) => (
        <span key={s} className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
          {s}
        </span>
      ))}
    </div>
  );
}

export function ToolList({ label, items }: { label: string; items?: string[] }) {
  return items?.length ? (
    <div className="space-y-1">
      <div className="text-muted-foreground">{label}</div>
      <Chips items={items} />
    </div>
  ) : null;
}
