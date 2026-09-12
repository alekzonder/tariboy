import type { ReactNode } from "react";
import { DaemonProvider, ImageLayout, MemoryRouter, Route, Routes } from "tariboy-ui";

// ImageLayout is the shell for one image detail route: a header carrying the
// image ref, its terminal-only badge, digest and build time, the Run Agent /
// provenance / Remove actions, the four-tab strip, and an <Outlet/> for the
// active tab. It resolves `name:tag` from useParams() and then fetches the
// manifest, the image listing and the provenance record.
//
// With no daemon those fetches reject and the component shows its real error
// branch in the body, with the header, actions and tabs still in place —
// including "source provenance unavailable", the honest state when provenance
// is missing.
//
// It resolves name:tag from useParams(), so it must be mounted inside a MATCHED
// route — MemoryRouter alone matches no pattern and the ref would render as a
// bare ":" separator.

const Frame = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      height: 330,
      width: 840,
      overflow: "hidden",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius)",
      background: "var(--background)",
    }}
  >
    {children}
  </div>
);

export const ImageShell = () => (
  <MemoryRouter initialEntries={["/servers/build-01/images/worker/v1"]}>
    <DaemonProvider>
      <Frame>
        <Routes>
          <Route
            path="/servers/:hostId/images/:name/:tag/*"
            element={<ImageLayout hostId="build-01" basePath="/servers/build-01/images" />}
          />
        </Routes>
      </Frame>
    </DaemonProvider>
  </MemoryRouter>
);
