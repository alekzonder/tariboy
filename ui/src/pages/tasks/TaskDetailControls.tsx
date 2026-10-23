import type { ComponentProps, ReactNode } from "react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { COUNT, DANGER_FILL, FIELD_MONO, LABEL, MONO, PRIMARY_FILL, SelectShell } from "./panelStyles"

/** `label value` in the header strip: the word is quiet, the value is mono. */
export function MetaInline({ label, value }: { label: string; value: string }) {
  return <span className="inline-flex min-w-0 items-center gap-1.5 text-muted-foreground">
    {label}<span className="min-w-0 truncate font-mono text-[11.5px] tabular-nums text-foreground">{value}</span>
  </span>
}

/** One pair of the overview grid. An empty value is an em dash, not a hole. */
export function Meta({ label, value, mono = true }: { label: string; value: string; mono?: boolean }) {
  return <div className="flex min-w-0 items-baseline gap-2.5">
    <dt className={cn(LABEL, "w-[76px] shrink-0 font-normal")}>{label}</dt>
    <dd className={cn("min-w-0 truncate", mono ? MONO : "text-[12.5px]")}>{value || "—"}</dd>
  </div>
}

export function SectionHeading({ label, count, action }: { label: string; count: number; action?: ReactNode }) {
  return <div className="flex items-center gap-1.5">
    <span className={LABEL}>{label}</span>
    <span className={COUNT}>{count}</span>
    {action && <span className="ml-auto">{action}</span>}
  </div>
}

/** The banner shape: one fill, one icon, one line, optional actions. */
export function Banner({ tone, icon, text, actions, role }: {
  tone: "danger" | "primary"
  icon: ReactNode
  text: string
  actions?: ReactNode
  role?: string
}) {
  return <div role={role}
    className={cn("flex items-start gap-[9px] rounded-[8px] px-2.5 py-2", tone === "danger" ? DANGER_FILL : PRIMARY_FILL)}>
    <span className="shrink-0">{icon}</span>
    <span className="min-w-0 flex-1 self-center text-[12.5px] leading-[1.45] font-medium text-pretty">{text}</span>
    {actions}
  </div>
}

/** The action a banner carries. `quiet` is the one that changes nothing. */
export function BannerButton({ tone, children, className, ...props }: {
  tone: "danger" | "quiet"
} & ComponentProps<typeof Button>) {
  return <Button type="button" variant="ghost" {...props}
    className={cn("h-6 shrink-0 rounded-[7px] px-2.5 text-[12px]",
      tone === "danger"
        ? "bg-[color-mix(in_oklab,var(--status-failed)_14%,transparent)] text-status-failed hover:bg-[color-mix(in_oklab,var(--status-failed)_20%,transparent)] hover:text-status-failed"
        : "text-status-failed hover:bg-[color-mix(in_oklab,var(--status-failed)_10%,transparent)] hover:text-status-failed",
      className)}>
    {children}
  </Button>
}

/** A select is the same fill as an input, with the chevron drawn over it. */
export function SelectField({ label, value, onChange, mono = false, children }: {
  label: string
  value: string
  onChange: (value: string) => void
  mono?: boolean
  children: ReactNode
}) {
  return <label className="flex min-w-0 flex-col gap-[5px]">
    <span className={LABEL}>{label}</span>
    <SelectShell className={cn("flex", mono && FIELD_MONO)} value={value} onChange={(event) => onChange(event.target.value)}>
      {children}
    </SelectShell>
  </label>
}
