import { useState } from "react"
import { toast } from "sonner"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ApiError, setColor } from "@/lib/api"
import { setCachedColor } from "@/lib/colorCache"
import { isValidHex, randomHex, swatchStyle } from "@/lib/utils"

const FALLBACK = "#888888"

const err = (e: unknown) => toast.error(e instanceof ApiError ? e.message : String(e))

/** The per-agent color affordance, sitting in the agent header. A round swatch
 * shows the agent's current color (the raw `#rrggbb` hex). Clicking it opens a
 * modal with a hex text input that reveals a color WHEEL (the native color
 * picker) on click, a Random button, and a Save button that POSTs
 * `/api/agents/<name>/color`.
 *
 * The header's color comes from the polled `/api/agents` snapshot, so on its own
 * a freshly-saved color wouldn't show until the next poll. `onSaved` lets the
 * host propagate the chosen hex immediately (optimistic update) so the swatch
 * and header tint update promptly on Save. */
export function AgentColorSwatch({
  name,
  color,
  onSaved,
}: {
  name: string
  color?: string
  onSaved?: (hex: string) => void | Promise<void>
}) {
  const [open, setOpen] = useState(false)
  // The text input value (may be mid-edit / invalid).
  const [text, setText] = useState(FALLBACK)
  // The native color wheel stays hidden until the input is clicked/focused.
  const [wheelOpen, setWheelOpen] = useState(false)
  const [saving, setSaving] = useState(false)

  const current = color && isValidHex(color) ? color : FALLBACK
  const valid = isValidHex(text)

  // Seed the draft from the live color whenever the dialog opens, and collapse
  // the wheel back to its hidden state.
  const onOpenChange = (next: boolean) => {
    if (next) {
      setText(current)
      setWheelOpen(false)
    }
    setOpen(next)
  }

  const randomize = () => setText(randomHex())

  const save = async () => {
    if (!valid) return
    const hex = text.trim().toLowerCase()
    setSaving(true)
    try {
      await setColor(name, hex)
      // Reset the cache immediately so a reload paints the new color (and the
      // cache agrees with the optimistic value) without waiting on a poll.
      setCachedColor(name, hex)
      toast.success(`color = ${hex}`)
      // Propagate the new color to the header/selector now (optimistic update)
      // so it doesn't wait for the next background poll.
      await onSaved?.(hex)
      setOpen(false)
    } catch (e) {
      err(e)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger asChild>
        <button
          type="button"
          data-testid="agent-color-swatch"
          aria-label="edit agent color"
          title="Edit agent color"
          className="agent-swatch inline-block size-5 shrink-0 cursor-pointer rounded-full align-middle ring-offset-background transition hover:opacity-80 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          style={swatchStyle(current)}
        />
      </DialogTrigger>
      {/* Don't autofocus the hex input on open — the wheel stays hidden until the
          user clicks the input (per the design). */}
      <DialogContent className="sm:max-w-xs" onOpenAutoFocus={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>Agent color</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col items-center gap-4">
          <div className="flex w-full items-center gap-2">
            <span
              data-testid="color-preview"
              className="agent-swatch size-6 shrink-0 rounded-full"
              style={swatchStyle(valid ? text : undefined)}
            />
            <Input
              aria-label="color hex"
              value={text}
              spellCheck={false}
              autoComplete="off"
              placeholder="#rrggbb"
              onClick={() => setWheelOpen(true)}
              onFocus={() => setWheelOpen(true)}
              onChange={(e) => setText(e.target.value)}
              className="font-mono"
              aria-invalid={!valid}
            />
          </div>
          {wheelOpen && (
            <input
              type="color"
              data-testid="color-wheel"
              aria-label="color wheel"
              value={valid ? text : FALLBACK}
              onChange={(e) => setText(e.target.value)}
              className="h-24 w-24 cursor-pointer rounded-md border bg-transparent p-1"
            />
          )}
        </div>
        <DialogFooter className="sm:justify-between">
          <Button type="button" variant="outline" onClick={randomize}>
            Random
          </Button>
          <Button type="button" onClick={() => void save()} disabled={!valid || saving}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
