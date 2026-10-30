import type { ReactNode } from "react";
import { DaemonProvider, LibraryImagesPage, MemoryRouter, Route, Routes } from "tariboy-ui";

// LibraryImagesPage is the Library's Images tab: the toolbar (search · Show
// unused · Add image) over one row per image name, each name linking into the
// image detail routes.
//
// It needs DaemonProvider (it falls back to the active daemon) and a router,
// because every image name is a <Link>. The real mount is App.tsx's
// /library/images.
//
// There is no daemon behind the capture server, so listImages fails and the
// page shows its error line in place of the table — the toolbar above it is the
// real chrome.
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

export const Images = () => (
  <MemoryRouter initialEntries={["/library/images"]}>
    <DaemonProvider>
      <Shell>
        <Routes>
          <Route path="/library/images" element={<LibraryImagesPage hostId="" />} />
        </Routes>
      </Shell>
    </DaemonProvider>
  </MemoryRouter>
);
