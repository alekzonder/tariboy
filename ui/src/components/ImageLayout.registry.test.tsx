import { afterEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { setActiveDaemon } from "@/lib/api";
import { addDaemon } from "@/lib/daemons";
import { ImageLayout } from "./ImageLayout";
import ImageTemplate from "@/pages/images/ImageTemplate";
import ImageFiles from "@/pages/ImageFiles";

vi.mock("@/components/DaemonProvider", () => ({ useOptionalDaemons: () => null }));
afterEach(() => { vi.unstubAllGlobals(); localStorage.clear(); setActiveDaemon(null); });
const result = (value: unknown) => new Response(JSON.stringify({ ok: true, result: value }));
const changed = () => new Response(JSON.stringify({ ok: false, error: { code: "image_changed", message: "Image changed" } }), { status: 409 });
const manifest = (digest: string) => ({ schema_version: 2, name: "reviewer", tag: "latest", digest, image_version: digest === "old" ? "1.0.0" : "2.0.0", built_at: "", parents: [], plugins: [], skills: [], requires_secrets: [], env: null, layers: [] });

it("reloads a changed template once with the new manifest and explicit route target", async () => {
 const host = await addDaemon({ label: "Route", baseURL: "https://route", token: "token" });
 setActiveDaemon({ id: "other", label: "Other", baseURL: "https://other", token: "" });
 let digest = "old";
 const requests: string[] = [];
 vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
  const url = String(input); requests.push(url);
  if (url === "https://route/api/images/reviewer%3Alatest") return result(manifest(digest));
  if (url === "https://route/api/images") return result({ images: [] });
  if (url.includes("https://route/api/images/reviewer%3Alatest/provenance")) return result({ ref: "reviewer:latest", digest, source_cwd: null, source_available: false });
  if (url === "https://route/api/images/reviewer%3Alatest/template?expected_digest=old") { digest = "new"; return changed(); }
  if (url === "https://route/api/images/reviewer%3Alatest/template?expected_digest=new") return result({ schema_version: 2, sha256: "new-template", entries: [] });
  throw new Error("unexpected request " + url);
 }));
 render(<MemoryRouter initialEntries={["/servers/route/images/reviewer/latest/template"]}><Routes>
  <Route path="/servers/:hostId/images/:name/:tag" element={<ImageLayout hostId={host.id} basePath="/servers/route/images" />}>
   <Route path="template" element={<ImageTemplate />} />
  </Route>
 </Routes></MemoryRouter>);
 expect(await screen.findByText("template sha256 new-template")).toBeInTheDocument();
 expect(screen.getByText("2.0.0", { exact: true })).toBeInTheDocument();
 expect(screen.queryByText("1.0.0", { exact: true })).toBeNull();
 expect(screen.getByRole("status")).toHaveTextContent(/image changed/i);
 expect(screen.getByRole("link", { name: "Images" })).toHaveAttribute("href", "/servers/route/images");
 expect(screen.getByRole("link", { name: "reviewer" })).toHaveAttribute("href", "/servers/route/images/reviewer");
 expect(requests.filter((url) => url === "https://route/api/images/reviewer%3Alatest")).toHaveLength(2);
});

it("stops automatic refresh after a second rebuild and offers a retry", async () => {
 let reads = 0;
 vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
  const url = String(input);
  if (url === "/api/images/reviewer%3Alatest") { reads++; return result(manifest("old")); }
  if (url === "/api/images") return result({ images: [] });
  if (url.includes("/provenance")) return result({ ref: "reviewer:latest", source_cwd: null, source_available: false });
  if (url.includes("/template")) return changed();
  throw new Error("unexpected request " + url);
 }));
 render(<MemoryRouter initialEntries={["/images/reviewer/latest/template"]}><Routes>
  <Route path="/images/:name/:tag" element={<ImageLayout hostId="" />}><Route path="template" element={<ImageTemplate />} /></Route>
 </Routes></MemoryRouter>);
 expect(await screen.findByRole("button", { name: "Refresh image" })).toBeInTheDocument();
 expect(reads).toBe(2);
 fireEvent.click(screen.getByRole("button", { name: "Refresh image" }));
 await waitFor(() => expect(reads).toBe(4));
});

it("discards the old file listing and ignores late file errors after a rebuild", async () => {
 let digest = "old";
 let finishOld!: (response: Response) => void;
 const oldRequest = new Promise<Response>((resolve) => { finishOld = resolve; });
 vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
  const url = String(input);
  if (url === "/api/images/reviewer%3Alatest") return result(manifest(digest));
  if (url === "/api/images") return result({ images: [] });
  if (url.includes("/provenance")) return result({ ref: "reviewer:latest", digest, source_cwd: null, source_available: false });
  if (url === "/api/images/reviewer%3Alatest/files?expected_digest=old") return result({ files: [{ path: "old.txt", is_dir: false, size: 3 }, { path: "trigger.txt", is_dir: false, size: 3 }], count: 2 });
  if (url === "/api/images/reviewer%3Alatest/files/old.txt?expected_digest=old") return oldRequest;
  if (url === "/api/images/reviewer%3Alatest/files/trigger.txt?expected_digest=old") { digest = "new"; return changed(); }
  if (url === "/api/images/reviewer%3Alatest/files?expected_digest=new") return result({ files: [{ path: "new.txt", is_dir: false, size: 3 }], count: 1 });
  if (url === "/api/images/reviewer%3Alatest/files/new.txt?expected_digest=new") return result({ path: "new.txt", content: "NEW CONTENT" });
  throw new Error("unexpected request " + url);
 }));
 render(<MemoryRouter initialEntries={["/images/reviewer/latest/files"]}><Routes>
  <Route path="/images/:name/:tag" element={<ImageLayout hostId="" />}><Route path="files" element={<ImageFiles />} /></Route>
 </Routes></MemoryRouter>);
 fireEvent.click(await screen.findByText("old.txt", { exact: true }));
 fireEvent.click(screen.getByText("trigger.txt", { exact: true }));
 fireEvent.click(await screen.findByText("new.txt", { exact: true }));
 expect(await screen.findByText("NEW CONTENT")).toBeInTheDocument();
 await act(async () => finishOld(changed()));
 expect(screen.getByText("NEW CONTENT")).toBeInTheDocument();
 expect(screen.queryByText("old.txt", { exact: true })).toBeNull();
 expect(screen.getByText("2.0.0", { exact: true })).toBeInTheDocument();
});
