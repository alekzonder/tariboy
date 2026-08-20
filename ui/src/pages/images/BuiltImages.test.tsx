import { afterEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import BuiltImages from "./BuiltImages";

afterEach(() => vi.restoreAllMocks());

it("exports runnable images and distinguishes original builds from imports", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok: true, status: 200,
    text: async () => JSON.stringify({ ok: true, result: { images: [
      { name: "built", tag: "v1", bare: false, exportable: true, source_cwd: "/srv/images/built" },
      { name: "imported", tag: "v1", bare: false, exportable: true, current_agents: ["reviewer"], pending_agents: ["worker"] },
      { name: "missing", tag: "v1", bare: false, exportable: true, source_cwd: "/srv/images/missing", source_available: false },
    ] } }),
  } as Response));
  render(
    <MemoryRouter>
      <BuiltImages hostId="" basePath="/servers/local/images" />
    </MemoryRouter>,
  );

  expect(await screen.findByRole("button", { name: "Export built:v1" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Export imported:v1" })).toBeEnabled();
  expect(screen.getByText("/srv/images/built")).toBeInTheDocument();
  expect(screen.getByText("Source CWD unavailable — imported artifact")).toBeInTheDocument();
  expect(screen.getByText("Source CWD unavailable — /srv/images/missing")).toBeInTheDocument();
  expect(screen.getByText("Current: reviewer")).toBeInTheDocument();
  expect(screen.getByText("Pending: worker")).toBeInTheDocument();
  expect(screen.getAllByTitle(/original sources are not included/i)).toHaveLength(3);
  expect(screen.getByLabelText("Import image archive")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "built:v1" }))
    .toHaveAttribute("href", "/servers/local/images/built/v1");
});

it("downloads the runnable bundle and confirms the saved portable filename", async () => {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const path = String(input);
    if (path === "/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [
        { name: "reviewer", tag: "v3", bare: false, exportable: true },
      ] } }), { status: 200 }));
    }
    if (path === "/api/images/reviewer%3Av3/export") {
      return Promise.resolve(new Response(new Blob(["archive"]), { status: 200 }));
    }
    return Promise.reject(new Error(`unexpected request ${path}`));
  });
  vi.stubGlobal("fetch", fetchMock);
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:archive");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
  const success = vi.spyOn(toast, "success");
  let downloaded = "";
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) { downloaded = this.download; });
  render(<MemoryRouter><BuiltImages hostId="" /></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "Export reviewer:v3" }));
  await waitFor(() => expect(downloaded).toBe("reviewer-v3.tariboy-image.tar.gz"));
  expect(success).toHaveBeenCalledWith(
    "image reviewer:v3 saved to file reviewer-v3.tariboy-image.tar.gz",
  );
});

it("lets the operator retag a conflicting image import", async () => {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path === "/api/images") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { images: [] } }), { status: 200 }));
    }
    if (path === "/api/image-imports" && init?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: { import_id: "import-1", ref: "reviewer:v1", digest: "abc" } }), { status: 200 }));
    }
    if (path === "/api/image-imports/import-1/apply" && init?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true, result: {} }), { status: 200 }));
    }
    return Promise.reject(new Error(`unexpected request ${init?.method} ${path}`));
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<MemoryRouter><BuiltImages hostId="" /></MemoryRouter>);

  fireEvent.change(screen.getByLabelText("Import image archive"), {
    target: { files: [new File(["archive"], "reviewer.tar.gz", { type: "application/gzip" })] },
  });
  fireEvent.change(await screen.findByLabelText("Import name"), { target: { value: "reviewer-copy" } });
  fireEvent.change(screen.getByLabelText("Import tag"), { target: { value: "v2" } });
  fireEvent.click(screen.getByRole("button", { name: "Import image" }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
    "/api/image-imports/import-1/apply",
    expect.objectContaining({ body: JSON.stringify({ ref: "reviewer-copy:v2" }) }),
  ));
});
