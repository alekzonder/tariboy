import { cn } from "@/lib/utils"

/**
 * The style layer's segmented control: a 9px-radius `--muted` trough with 24px
 * buttons in it, the selected one lifted onto `--card` by `--raise` — the same
 * two-level elevation as a selected sidebar row. Used for the task status
 * filter, and for any other small either/or switch in the console chrome.
 */
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
  className,
}: {
  options: ReadonlyArray<{ value: T; label: string }>
  value: T
  onChange: (value: T) => void
  /** Accessible name of the group, e.g. "Task status". */
  label: string
  className?: string
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      data-slot="segmented"
      className={cn("flex shrink-0 gap-0.5 rounded-[9px] bg-muted p-0.5", className)}
    >
      {options.map((option, index) => {
        const selected = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={selected}
            onClick={() => onChange(option.value)}
            onKeyDown={(event) => {
              // A radio group moves with the arrows, wrapping at either end.
              const step = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0
              if (!step) return
              event.preventDefault()
              const next = (index + step + options.length) % options.length
              ;(event.currentTarget.parentElement?.children[next] as HTMLElement | undefined)?.focus()
              onChange(options[next].value)
            }}
            className={cn(
              "h-6 rounded-[7px] px-2.5 text-[12px]",
              selected
                ? "bg-card font-medium text-foreground shadow-[var(--raise)]"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
