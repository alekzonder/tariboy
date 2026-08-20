import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import { fsList, type FsEntry } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"
import { cn } from "@/lib/utils"

export interface PathAutocompleteProps {
  /** Full path currently in the input (controlled). Empty = managed workdir. */
  value: string
  onChange: (value: string) => void
  placeholder?: string
  id?: string
  className?: string
  "aria-label"?: string
  /** Debounce before a listing request; 0 in tests for determinism. */
  debounceMs?: number
  /** Target host for the listing (undefined = active daemon, null =
   * same-origin), for cross-host views like /terminals. */
  daemon?: Daemon | null
}

// splitPath breaks a full path into the parent listing *prefix* (everything up
// to and including the last "/", or "" when there is none) and the trailing
// segment being typed. The prefix is what we ask the backend to list; the tail
// is what we filter that listing by. Splitting on the last "/" preserves any
// leading slash so completing a segment keeps the path shape intact.
export function splitPath(full: string): { prefix: string; tail: string } {
  const idx = full.lastIndexOf("/")
  if (idx < 0) return { prefix: "", tail: full }
  return { prefix: full.slice(0, idx + 1), tail: full.slice(idx + 1) }
}

// PathAutocomplete is a shell-like directory typeahead over the daemon
// filesystem root ($HOME by default). One text input holds the full path; the
// dropdown shows the current parent's subfolders filtered by the segment being
// typed. ↑/↓ move the highlight, Tab drills into the highlighted folder, Enter
// accepts the typed path, Esc closes. Built on the shadcn Command primitives
// (cmdk) — cmdk owns arrow-key navigation while this component owns the
// path-segment logic, so Tab/Enter/Esc are intercepted before they reach cmdk.
export function PathAutocomplete({
  value,
  onChange,
  placeholder,
  id,
  className,
  debounceMs = 150,
  "aria-label": ariaLabel,
  daemon,
}: PathAutocompleteProps) {
  const { prefix, tail } = useMemo(() => splitPath(value), [value])
  const [entries, setEntries] = useState<FsEntry[]>([])
  const [open, setOpen] = useState(false)
  // cmdk's selected item value (we set each item's value to its index string).
  const [highlight, setHighlight] = useState("")
  const cache = useRef<Map<string, FsEntry[]>>(new Map())
  const reqSeq = useRef(0)
  // Cache keys are prefixed by daemon id so switching targets never serves a
  // stale listing from a different host under the same path.
  const daemonKey = daemon?.id ?? ""

  // Fetch the listing for the current prefix, debounced, with a per-prefix cache
  // so moving within one segment never refetches. A monotonically increasing
  // request id drops any response that arrives after the prefix moved on.
  useEffect(() => {
    // Bump the request id on EVERY prefix change, before the cache short-circuit,
    // so an in-flight fsList for a superseded prefix is always discarded — even
    // when the new prefix resolves from cache and never issues its own request.
    const seq = ++reqSeq.current
    const cacheKey = `${daemonKey}\0${prefix}`
    const cached = cache.current.get(cacheKey)
    if (cached) {
      setEntries(cached)
      return
    }
    const timer = setTimeout(() => {
      fsList(prefix, daemon)
        .then((r) => {
          if (seq !== reqSeq.current) return
          cache.current.set(cacheKey, r.entries)
          setEntries(r.entries)
        })
        .catch(() => {
          // Outside root / not a directory → nothing to offer for this prefix.
          if (seq !== reqSeq.current) return
          setEntries([])
        })
    }, debounceMs)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- daemon serialized as daemonKey to avoid identity churn
  }, [prefix, debounceMs, daemonKey])

  const filtered = useMemo(
    () =>
      entries.filter(
        (e) => e.dir && e.name.toLowerCase().includes(tail.toLowerCase()),
      ),
    [entries, tail],
  )

  // Drill into a folder: replace the tail with the folder name and a trailing
  // "/", which re-splits into a fresh prefix and lists that folder's children.
  const complete = useCallback(
    (entry: FsEntry) => {
      onChange(prefix + entry.name + "/")
      setOpen(true)
    },
    [onChange, prefix],
  )

  // cmdk tracks the selected item by its value (each item's value is its index
  // string). Fall back to the first match so Tab completes even before cmdk has
  // emitted an initial selection.
  const idx = Number(highlight)
  const highlighted = (filtered[Number.isInteger(idx) ? idx : 0] ??
    filtered[0]) as FsEntry | undefined

  const onKeyDown = (e: React.KeyboardEvent) => {
    // Let ↑/↓ bubble to cmdk (which owns navigation); intercept the rest so
    // cmdk's own Enter=select / Tab=focus-move handlers never fire.
    if (e.key === "Tab") {
      if (open && highlighted) {
        e.preventDefault()
        e.stopPropagation()
        complete(highlighted)
      }
      return
    }
    if (e.key === "Enter") {
      e.preventDefault()
      e.stopPropagation()
      setOpen(false)
      return
    }
    if (e.key === "Escape") {
      e.stopPropagation()
      setOpen(false)
    }
  }

  return (
    <Command
      shouldFilter={false}
      value={highlight}
      onValueChange={setHighlight}
      className={cn("relative overflow-visible bg-transparent", className)}
    >
      <CommandInput
        id={id}
        value={value}
        onValueChange={(v) => {
          onChange(v)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        aria-label={ariaLabel}
      />
      {open && (
        <CommandList
          data-slot="path-autocomplete-list"
          className="absolute top-full left-0 z-50 mt-1 w-full rounded-lg border bg-popover shadow-md"
        >
          <CommandEmpty>No matching folders</CommandEmpty>
          {filtered.map((e, i) => (
            <CommandItem
              key={e.name}
              value={String(i)}
              // Keep focus on the input so onBlur doesn't close the list before
              // the click-select fires.
              onMouseDown={(ev) => ev.preventDefault()}
              onSelect={() => complete(e)}
            >
              {e.name}
            </CommandItem>
          ))}
        </CommandList>
      )}
    </Command>
  )
}
