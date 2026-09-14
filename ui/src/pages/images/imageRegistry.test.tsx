import { afterEach, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { setActiveDaemon } from "@/lib/api";
import BuiltImages from "./BuiltImages";

vi.mock("@/components/DaemonProvider", () => ({ useOptionalDaemons: () => null }));
afterEach(() => { vi.unstubAllGlobals(); localStorage.clear(); setActiveDaemon(null); });

it("groups names and distinguishes latest from newest by parsed build date with stable ties", async () => {
 const images = [
  { name: "reviewer", tag: "latest", image_version: "1.0.0", built_at: "2026-01-01T00:00:00Z" },
  { name: "reviewer", tag: "99.0.0", image_version: "99.0.0", built_at: "invalid" },
  { name: "reviewer", tag: "2.0.0", image_version: "2.0.0", built_at: "2026-01-02T00:00:00Z" },
  { name: "tie", tag: "latest", image_version: "1.2.3", built_at: "2026-01-02T00:00:00Z" },
  { name: "tie", tag: "zeta", built_at: "2026-01-02T00:00:00Z" },
  { name: "tie", tag: "1.2.3", image_version: "1.2.3", built_at: "2026-01-01T19:00:00-05:00" },
  { name: "missing", tag: "zeta", built_at: "invalid" },
  { name: "missing", tag: "alpha" },
  { name: "old", tag: "latest" },
 ];
 vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: { images } }) }));
 render(<MemoryRouter><BuiltImages hostId="" basePath="/servers/local/images" /></MemoryRouter>);
 const reviewer = (await screen.findByRole("link", { name: "reviewer" })).closest("tr")!;
 expect(within(reviewer).getByText("1.0.0")).toBeInTheDocument();
 expect(within(reviewer).getByText("2.0.0")).toBeInTheDocument();
 expect(screen.getAllByRole("row")).toHaveLength(5);
 expect(within(screen.getByRole("link", { name: "tie" }).closest("tr")!).getByRole("link", { name: "1.2.3" })).toBeInTheDocument();
 const missing = screen.getByRole("link", { name: "missing" }).closest("tr")!;
 expect(within(missing).getByText("No latest")).toBeInTheDocument();
 expect(within(missing).getByRole("link", { name: "alpha" })).toBeInTheDocument();
 expect(within(screen.getByRole("link", { name: "old" }).closest("tr")!).getByText("Version not specified")).toBeInTheDocument();
 expect(screen.getByRole("link", { name: "reviewer" })).toHaveAttribute("href", "/servers/local/images/reviewer");
 expect(screen.queryByRole("button", { name: /^Export / })).toBeNull();
});

it("opens a name's encoded tags on the explicit host and keeps actions at tag level", async () => {
 const { addDaemon } = await import("@/lib/daemons");
 const source = await addDaemon({ label: "Route", baseURL: "https://route", token: "token" });
 setActiveDaemon({ id: "other", label: "Other", baseURL: "https://other", token: "" });
 const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
  if (String(input) !== "https://route/api/images") throw new Error("wrong host");
  return new Response(JSON.stringify({ ok: true, result: { images: [
   { name: "reviewer", tag: "1.2.3+build.7", image_version: "1.2.3", digest: "full-digest", exportable: true, built_at: "2026-01-01T00:00:00Z", current_agents: ["worker"] },
   { name: "reviewer", tag: "latest", image_version: "1.2.3", digest: "latest-digest", built_at: "2026-01-01T00:00:00Z" },
   { name: "other", tag: "latest" },
  ] } }));
 });
 vi.stubGlobal("fetch", fetchMock);
 const { Routes, Route } = await import("react-router-dom");
 const { default: ImageTags } = await import("./ImageTags");
 render(<MemoryRouter initialEntries={["/servers/route/images/reviewer"]}><Routes>
  <Route path="/servers/:hostId/images/:name" element={<ImageTags hostId={source.id} basePath="/servers/route/images" />} />
 </Routes></MemoryRouter>);
 expect(await screen.findByRole("link", { name: "1.2.3+build.7" })).toHaveAttribute("href", "/servers/route/images/reviewer/1.2.3%2Bbuild.7");
 expect(screen.getByRole("link", { name: "Images" })).toHaveAttribute("href", "/servers/route/images");
 expect(screen.getByText("full-digest")).toBeInTheDocument();
 expect(screen.getByText("Current: worker")).toBeInTheDocument();
 expect(screen.getByRole("button", { name: "Remove reviewer:1.2.3+build.7" })).toBeDisabled();
 expect(screen.getByText("Latest", { exact: true })).toBeInTheDocument();
 expect(screen.getByText("Newest", { exact: true })).toBeInTheDocument();
 expect(screen.queryByLabelText("Import image archive")).toBeNull();
 expect(screen.queryByText("other")).toBeNull();
});

it("shows an empty tag list without import controls", async () => {
 vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ ok: true, result: { images: [] } }))));
 render(<MemoryRouter><BuiltImages hostId="" imageName="missing" /></MemoryRouter>);
 expect(await screen.findByText("No tags for this image on this host.")).toBeInTheDocument();
 expect(screen.queryByLabelText("Import image archive")).toBeNull();
});

it("reports an unavailable explicit host without reading the active host", async () => {
 setActiveDaemon({ id: "other", label: "Other", baseURL: "https://other", token: "" });
 const fetchMock = vi.fn();
 vi.stubGlobal("fetch", fetchMock);
 render(<MemoryRouter><BuiltImages hostId="missing-host" /></MemoryRouter>);
 expect(await screen.findByText("host missing-host is not available")).toBeInTheDocument();
 expect(fetchMock).not.toHaveBeenCalled();
});
