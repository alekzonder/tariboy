import type { ReactNode } from "react";
import { DaemonProvider, MemoryRouter, Route, Routes, StoresPage } from "tariboy-ui";

// StoresPage is the per-server image-source surface, and it is two screens in
// one component, switched by the `name` prop:
//
//   name absent  — the Stores list: header, the "Add Store" form (name · Git URL
//                  or absolute directory · Add store) and the Store/Source table.
//   name present — one Store: back link, source and clone path, Refresh and
//                  Remove store, and the per-image build table.
//
// `target` is an explicit Daemon (or null for the same-origin local daemon), NOT
// the active-daemon fallback, so it is passed directly. A router is required —
// the list links each row to `${basePath}/${name}` and Remove navigates back to
// `basePath` — and the mounted paths are App.tsx's /servers/:hostId/stores and
// /servers/:hostId/stores/:name.
//
// There is no daemon behind the capture server, so listStores/getStore fail: the
// list shows its empty table ("No Stores registered on this server.") with the
// error line beneath, and the detail shows its header and actions over the same
// honest failure.
const Shell = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      height: 620,
      width: 1180,
      overflow: "hidden",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
      background: "var(--background)",
    }}
  >
    {children}
  </div>
);

const BASE = "/servers/local/stores";

export const StoreList = () => (
  <MemoryRouter initialEntries={[BASE]}>
    <DaemonProvider>
      <Shell>
        <Routes>
          <Route
            path="/servers/:hostId/stores"
            element={<StoresPage target={null} basePath={BASE} />}
          />
        </Routes>
      </Shell>
    </DaemonProvider>
  </MemoryRouter>
);

export const StoreDetail = () => (
  <MemoryRouter initialEntries={[`${BASE}/tariboy-images`]}>
    <DaemonProvider>
      <Shell>
        <Routes>
          <Route
            path="/servers/:hostId/stores/:name"
            element={<StoresPage target={null} name="tariboy-images" basePath={BASE} />}
          />
        </Routes>
      </Shell>
    </DaemonProvider>
  </MemoryRouter>
);
