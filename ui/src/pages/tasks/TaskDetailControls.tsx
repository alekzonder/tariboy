import type { ComponentProps, ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { NativeSelect } from "@/components/ui/native-select";
import { COUNT, DANGER_FILL, FIELD_MONO, LABEL, MONO, PRIMARY_FILL } from "@/components/ui/recipes";

/** `label value` in the header strip: the word is quiet, the value is mono. */
export function MetaInline({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1.5 text-muted-foreground">
      {label}
      <span className="min-w-0 truncate font-mono text-caption tabular-nums text-foreground">
        {value}
      </span>
    </span>
  );
}

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
      <dd className={cn("min-w-0 truncate", mono ? MONO : "text-body")}>{value || "—"}</dd>
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
        "flex items-start gap-2.25 rounded-control px-2.5 py-2",
        tone === "danger" ? DANGER_FILL : PRIMARY_FILL,
      )}
    >
      <span className="shrink-0">{icon}</span>
      <span className="min-w-0 flex-1 self-center text-body leading-normal font-medium text-pretty">
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
          ? "bg-destructive-soft text-destructive hover:bg-destructive-mid hover:text-destructive"
          : "text-destructive hover:bg-destructive-soft hover:text-destructive",
        className,
      )}
    >
      {children}
    </Button>
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
      <NativeSelect
        className={cn("flex", mono && FIELD_MONO)}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {children}
      </NativeSelect>
    </label>
  );
}
