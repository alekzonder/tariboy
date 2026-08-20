import { useEffect, useMemo } from "react"
import { isValidHex } from "@/lib/utils"

/** How long a cached color stays valid. A stale entry older than this is
 * ignored (and refreshed from the server's next snapshot). */
export const COLOR_CACHE_TTL_MS = 60 * 60 * 1000 // 1 hour

interface CacheEntry {
  color: string
  ts: number
}

/** localStorage key holding the cached color for one agent. Keyed per agent
 * name so each agent's color is remembered independently. */
export function colorCacheKey(name: string): string {
  return `agent:color:${name}`
}

/** Read an agent's cached color from `localStorage`. Returns the `#rrggbb` hex
 * when a valid, non-expired entry exists, otherwise `null` (missing, malformed,
 * invalid hex, or older than {@link COLOR_CACHE_TTL_MS}). `now` is injectable
 * for tests. */
export function getCachedColor(name: string, now: number = Date.now()): string | null {
  try {
    const raw = localStorage.getItem(colorCacheKey(name))
    if (raw === null) return null
    const parsed = JSON.parse(raw) as Partial<CacheEntry>
    if (
      !parsed ||
      typeof parsed.color !== "string" ||
      typeof parsed.ts !== "number" ||
      !isValidHex(parsed.color)
    ) {
      return null
    }
    if (now - parsed.ts >= COLOR_CACHE_TTL_MS) return null
    return parsed.color
  } catch {
    return null
  }
}

/** Write an agent's color to the cache, stamped with `now`, overwriting any
 * previous entry. Invalid hexes are ignored (never cached). `now` is injectable
 * for tests. */
export function setCachedColor(name: string, color: string, now: number = Date.now()): void {
  if (!isValidHex(color)) return
  try {
    const entry: CacheEntry = { color: color.trim().toLowerCase(), ts: now }
    localStorage.setItem(colorCacheKey(name), JSON.stringify(entry))
  } catch {
    /* localStorage unavailable / quota — caching is best-effort */
  }
}

/** Evict an agent's cached color from `localStorage`. Best-effort: a missing
 * key or an unavailable `localStorage` is a no-op. */
export function removeCachedColor(name: string): void {
  try {
    localStorage.removeItem(colorCacheKey(name))
  } catch {
    /* localStorage unavailable — eviction is best-effort */
  }
}

/** Resolve an agent's effective color, painting from the `localStorage` cache
 * immediately so the header/swatch render the right color on load without
 * waiting for (or flashing before) the first `/api/agents` poll. Whenever a
 * fresh, valid server color arrives it becomes the value AND refreshes the
 * cache; until then the last cached color is used. Re-reads the cache when the
 * agent `name` changes.
 *
 * A server value that is present but empty means the color was explicitly
 * CLEARED (via CLI/API), which is distinct from "not yet loaded" (`undefined`
 * /`null`, before the first poll resolves). A cleared color must paint nothing
 * and win over any stale cache — otherwise the old cached hex would keep
 * painting until the TTL expires — so it also evicts the cached entry. */
export function useCachedColor(name: string, serverColor?: string | null): string | undefined {
  const serverValid = typeof serverColor === "string" && isValidHex(serverColor)
  // Server responded, but with an empty/cleared value — NOT "not yet loaded".
  const serverCleared = typeof serverColor === "string" && serverColor.trim() === ""

  // The cached fallback, re-read whenever the agent changes (the shell isn't
  // remounted on navigation between agents).
  const cached = useMemo(() => getCachedColor(name) ?? undefined, [name])

  // Sync the cache to the server value — an external (localStorage) sync, not
  // React state, so no cascading renders. A valid color refreshes the entry; an
  // explicitly cleared color evicts it so it never repaints from stale cache.
  useEffect(() => {
    if (serverValid) setCachedColor(name, serverColor)
    else if (serverCleared) removeCachedColor(name)
  }, [name, serverColor, serverValid, serverCleared])

  // A valid server color wins; a cleared one paints nothing (no cache
  // fallback); only the genuinely not-yet-loaded case falls back to the cache.
  if (serverValid) return serverColor
  if (serverCleared) return undefined
  return cached
}
