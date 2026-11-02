import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@/test/queryRender";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ImageLayout } from "@/components/ImageLayout";
import * as daemons from "@/lib/daemons";
import ImageOverview from "./ImageOverview";

vi.mock("@/lib/desktop", () => ({ openHostPathInVSCode: vi.fn() }));

const roles: Record<string, string | undefined> = {
  foo: "# Reviewer\n\nYou review pull requests.",
  blank: " \n\t\n",
  plain: undefined,
  bare: "Never shown.",
};

function manifest(name: string) {
  const role = roles[name];
  return {
    schema_version: 1,
    name,
    tag: "v1",
    digest: `sha256:${name}`,
    built_at: "2026-07-12T00:00:00Z",
    parents: [],
    plugins: [],
    requires_secrets: [],
    harness: { type: "claude", interactive: false },
    env: {},
    policy: {},
    layers: [],
    bare: name === "bare",
    ...(role === undefined ? {} : { role: { text: role } }),
  };
}

function stubFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string) => {
      const path = String(url).split("?")[0];
      let result: unknown = {};
      if (path.endsWith("/api/images")) {
        result = {
          images: Object.keys(roles).map((name) => ({
            name,
            tag: "v1",
            digest: `sha256:${name}`,
            built_at: "2026-07-12T00:00:00Z",
            bare: name === "bare",
          })),
          count: 4,
        };
      } else if (path.endsWith("/provenance")) {
        result = { ref: "x:v1", source_cwd: null, source_available: false };
      } else if (/\/api\/images\/[^/]+$/.test(path)) {
        result = manifest(decodeURIComponent(path.split("/").pop()!).split(":")[0]);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
}

beforeEach(() => {
  stubFetch();
  vi.spyOn(daemons, "resolveDaemon").mockResolvedValue(null);
});
afterEach(() => vi.restoreAllMocks());

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        {["/library/images/:name", "/library/images/:name/:tag"].map((route) => (
          <Route key={route} path={route} element={<ImageLayout />}>
            <Route index element={<ImageOverview />} />
            <Route path="template" element={<div>template body</div>} />
          </Route>
        ))}
      </Routes>
    </MemoryRouter>,
  );
}

async function expectRoleFirst() {
  const role = await screen.findByRole("region", { name: "Role" });
  expect(role).toHaveTextContent("You review pull requests.");
  const harness = screen.getByText("Harness");
  expect(role.compareDocumentPosition(harness) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(role.parentElement!.firstElementChild).toBe(role);
}

describe("ImageOverview Role card", () => {
  it("comes first on the Overview of a tag, before Harness", async () => {
    renderAt("/library/images/foo/v1");
    await expectRoleFirst();
  });

  it("comes first on the Overview of a name's newest tag", async () => {
    renderAt("/library/images/foo");
    await expectRoleFirst();
  });

  it("is absent on the other tabs", async () => {
    renderAt("/library/images/foo/v1/template");
    expect(await screen.findByText("template body")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Role" })).not.toBeInTheDocument();
  });

  it.each(["plain", "blank"])("shows the empty line for %s", async (name) => {
    renderAt(`/library/images/${name}/v1`);
    const role = await screen.findByRole("region", { name: "Role" });
    expect(role).toHaveTextContent("This image has no ROLE.md.");
  });

  it("is absent for bare, which keeps Terminal-only", async () => {
    renderAt("/library/images/bare/v1");
    expect(await screen.findByText(/cannot be rebuilt or automated/)).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Role" })).not.toBeInTheDocument();
    expect(screen.queryByText("Never shown.")).not.toBeInTheDocument();
  });
});
