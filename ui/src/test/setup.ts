import "@testing-library/jest-dom"
import { TransformStream as NodeTransformStream } from "node:stream/web"

if (typeof globalThis.TransformStream === "undefined") {
  Object.defineProperty(globalThis, "TransformStream", {
    configurable: true,
    writable: true,
    value: NodeTransformStream,
  })
}

// Radix UI popovers/menus (DropdownMenu, Select) probe APIs that jsdom does not
// implement. Without these shims, opening a menu in a test throws.
if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
  Element.prototype.setPointerCapture = () => {}
  Element.prototype.releasePointerCapture = () => {}
}
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {}
}

// Radix ScrollArea observes its viewport with ResizeObserver, which jsdom lacks.
// A no-op class lets pages that wrap content in <ScrollArea> mount in tests.
if (typeof globalThis.ResizeObserver === "undefined") {
  class FakeResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver = FakeResizeObserver as unknown as typeof ResizeObserver
}

// jsdom here runs without a usable localStorage/sessionStorage: components read
// the theme on mount (useTheme / ThemeToggle) and the daemon registry keeps its
// write-only tokens in sessionStorage, so both need in-memory shims.
//
// The guard probes BEHAVIOUR rather than `typeof === "undefined"`. Node 22+ ships
// its own global localStorage, and under this jsdom environment that object
// survives but is not a working Storage (`clear` is missing), so a
// presence-based check silently skipped the shim and left every test that calls
// localStorage.clear() throwing. Installation goes through defineProperty
// because the platform global is not a plain writable property.
function installStorageShim(name: "localStorage" | "sessionStorage"): void {
  const existing = globalThis[name] as Storage | undefined
  try {
    if (existing && typeof existing.clear === "function") {
      existing.clear()
      return // a real, working Storage — leave it alone
    }
  } catch {
    // Throws on access/use: treat as unusable and replace below.
  }
  const store = new Map<string, string>()
  Object.defineProperty(globalThis, name, {
    configurable: true,
    writable: true,
    value: {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, String(v)),
      removeItem: (k: string) => void store.delete(k),
      clear: () => store.clear(),
      key: (i: number) => Array.from(store.keys())[i] ?? null,
      get length() {
        return store.size
      },
    } as Storage,
  })
}

installStorageShim("localStorage")
installStorageShim("sessionStorage")

if (typeof globalThis.matchMedia === "undefined") {
  globalThis.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList
}

// jsdom has no EventSource; pages that open an SSE stream on mount
// (subscribeAgentEvents) need a no-op so they can render in tests.
if (typeof globalThis.EventSource === "undefined") {
  class FakeEventSource {
    close() {}
    addEventListener() {}
    removeEventListener() {}
  }
  globalThis.EventSource = FakeEventSource as unknown as typeof EventSource
}
