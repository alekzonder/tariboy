import { useEffect, useRef, type ReactNode, type RefObject } from "react";
import { DaemonProvider, ImagesPage, MemoryRouter, Route, Routes } from "tariboy-ui";

// ImagesPage is the per-server image surface: the page header, the "Build from
// directory" panel (path · name · tag · Validate · Build) and BuiltImages — the
// archive-import card plus the built-image table whose refs link into the image
// detail routes.
//
// It needs DaemonProvider (it falls back to the active daemon when `hostId` is
// omitted) and a router, because every built image ref is a <Link> built from
// `basePath`. The real mount is App.tsx's /servers/:hostId/images.
//
// There is no daemon behind the capture server, so listImages fails and
// BuiltImages returns its error line in place of the table — the header and the
// build panel above it are the real chrome.
const Shell = ({ children, frameRef }: {
  children: ReactNode;
  frameRef?: RefObject<HTMLDivElement | null>;
}) => (
  <div
    ref={frameRef}
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

const BASE = "/servers/local/images";

const Screen = ({ frameRef }: { frameRef?: RefObject<HTMLDivElement | null> }) => (
  <MemoryRouter initialEntries={[BASE]}>
    <DaemonProvider>
      <Shell frameRef={frameRef}>
        <Routes>
          <Route path="/servers/:hostId/images" element={<ImagesPage hostId="" basePath={BASE} />} />
        </Routes>
      </Shell>
    </DaemonProvider>
  </MemoryRouter>
);

// React owns the value of a controlled <input>, so a preview that wants the
// build form filled has to go through the native setter and dispatch the real
// input event — assigning .value alone never reaches the component's state.
function useFillInputs(
  frameRef: RefObject<HTMLDivElement | null>,
  values: { placeholder: string; value: string }[],
) {
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const setter = Object.getOwnPropertyDescriptor(
        window.HTMLInputElement.prototype,
        "value",
      )?.set;
      for (const { placeholder, value } of values) {
        const input = frameRef.current?.querySelector<HTMLInputElement>(
          `input[placeholder="${placeholder}"]`,
        );
        if (!input || !setter) continue;
        setter.call(input, value);
        input.dispatchEvent(new Event("input", { bubbles: true }));
      }
    }, 0);
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [frameRef]);
}

export const Images = () => <Screen />;

export const BuildFromDirectory = () => {
  const frameRef = useRef<HTMLDivElement | null>(null);
  useFillInputs(frameRef, [
    { placeholder: "/absolute/path/to/image", value: "/home/agent/github/tariboy" },
    { placeholder: "name (required)", value: "worker" },
  ]);
  return <Screen frameRef={frameRef} />;
};
