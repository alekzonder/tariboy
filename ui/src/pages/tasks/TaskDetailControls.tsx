import type { ComponentProps, ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  COUNT,
  DANGER_FILL,
  FIELD_MONO,
  LABEL,
  MONO,
  PRIMARY_FILL,
  SelectShell,
} from "./panelStyles";

/** One pair of the overview grid. An empty value is an em dash, not a hole. */
export function Meta({
  label,
  value,
  mono = true,
}: {
  label: string;
  value: ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="flex min-w-0 items-baseline gap-2.5">
      <dt className={cn(LABEL, "w-19 shrink-0 font-normal")}>{label}</dt>
      <dd className={cn("min-w-0 truncate", mono ? MONO : "text-body-sm")}>{value || "—"}</dd>
    </div>
  );
}

export function SectionHeading({
  label,
  count,
  action,
}: {
  label: string;
  count: number;
  action?: ReactNode;
}) {
  return (
    <div className="flex items-center gap-1.5">
      <span className={LABEL}>{label}</span>
      <span className={COUNT}>{count}</span>
      {action && <span className="ml-auto">{action}</span>}
    </div>
  );
}

/** The banner shape: one fill, one icon, one line, optional actions. */
export function Banner({
  tone,
  icon,
  text,
  actions,
  role,
}: {
  tone: "danger" | "primary";
  icon: ReactNode;
  text: string;
  actions?: ReactNode;
  role?: string;
}) {
  return (
    <div
      role={role}
      className={cn(
        "flex items-start gap-2.25 rounded-md px-2.5 py-2",
        tone === "danger" ? DANGER_FILL : PRIMARY_FILL,
      )}
    >
      <span className="shrink-0">{icon}</span>
      <span className="min-w-0 flex-1 self-center text-body-sm leading-[1.45] font-medium text-pretty">
        {text}
      </span>
      {actions}
    </div>
  );
}

/** The action a banner carries. `quiet` is the one that changes nothing. */
export function BannerButton({
  tone,
  children,
  className,
  ...props
}: {
  tone: "danger" | "quiet";
} & ComponentProps<typeof Button>) {
  return (
    <Button
      type="button"
      variant="ghost"
      {...props}
      className={cn(
        "h-6 shrink-0 rounded-control px-2.5 text-ui",
        tone === "danger"
          ? "bg-[color-mix(in_oklab,var(--status-failed)_14%,transparent)] text-status-failed hover:bg-[color-mix(in_oklab,var(--status-failed)_20%,transparent)] hover:text-status-failed"
          : "text-status-failed hover:bg-[color-mix(in_oklab,var(--status-failed)_10%,transparent)] hover:text-status-failed",
        className,
      )}
    >
      {children}
    </Button>
  );
}

/** A parameter of the task in the header row: a quiet label and a mono value
 *  in one 24px fill. */
export const CHIP =
  "relative inline-flex h-6 min-w-0 max-w-full shrink-0 items-center gap-1.5 rounded-control bg-muted px-2 text-ui text-muted-foreground focus-within:ring-2 focus-within:ring-ring";
/** The value inside a chip. */
export const CHIP_VALUE = "min-w-0 truncate font-mono text-meta tabular-nums text-foreground";

/**
 * A chip that changes its value from a closed list. It is a native select
 * sized to its value, so the label names it for a screen reader and the list
 * is the platform's own.
 */
export function ChipSelect({
  label,
  bare = false,
  value,
  onChange,
  disabled,
  className,
  children,
}: {
  label: string;
  /** The value explains itself (`P1 High`): the label is only read aloud. */
  bare?: boolean;
  /** A fill and a text color that say more than the neutral chip, for a status. */
  className?: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <label
      className={cn(CHIP, !className && "hover:bg-accent", disabled && "opacity-60", className)}
    >
      <span className={bare ? "sr-only" : "lowercase"}>{label}</span>
      <select
        className={cn(
          CHIP_VALUE,
          className && "text-inherit",
          "field-sizing-content max-w-[22ch] cursor-pointer appearance-none bg-transparent pr-3.5 outline-none",
        )}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      >
        {children}
      </select>
      <ChevronDown
        aria-hidden="true"
        className="pointer-events-none absolute right-2 size-2.5 opacity-45"
      />
    </label>
  );
}

/** A select is the same fill as an input, with the chevron drawn over it. */
export function SelectField({
  label,
  value,
  onChange,
  mono = false,
  children,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  mono?: boolean;
  children: ReactNode;
}) {
  return (
    <label className="flex min-w-0 flex-col gap-1.25">
      <span className={LABEL}>{label}</span>
      <SelectShell
        className={cn("flex", mono && FIELD_MONO)}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {children}
      </SelectShell>
    </label>
  );
}
