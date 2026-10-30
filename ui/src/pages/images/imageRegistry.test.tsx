import { afterEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { createApi, setActiveDaemon } from "@/lib/api";
import { resolveDaemon, unresolvedDaemon, type Daemon } from "@/lib/daemons";
import { apiWrapper } from "@/test/api";
import BuiltImages from "./BuiltImages";

// BuiltImages renders under the route host's ApiProvider.
const on = (target: Daemon | null) => ({ wrapper: apiWrapper(createApi(target)) });

vi.mock("@/components/daemonContext", () => ({ useOptionalDaemons: () => null }));
afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  setActiveDaemon(null);
});

it("opens a name's encoded tags on the explicit host and keeps actions at tag level", async () => {
  const { addDaemon } = await import("@/lib/daemons");
  const source = await addDaemon({ label: "Route", baseURL: "https://route", token: "token" });
  setActiveDaemon({ id: "other", label: "Other", baseURL: "https://other", token: "" });
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) !== "https://route/api/images") throw new Error("wrong host");
    return new Response(
      JSON.stringify({
        ok: true,
        result: {
          images: [
            {
              name: "reviewer",
              tag: "1.2.3+build.7",
              image_version: "1.2.3",
              digest: "full-digest",
              exportable: true,
              built_at: "2026-01-01T00:00:00Z",
              current_agents: ["worker"],
            },
            {
              name: "reviewer",
              tag: "latest",
              image_version: "1.2.3",
              digest: "latest-digest",
              built_at: "2026-01-01T00:00:00Z",
            },
            { name: "other", tag: "latest" },
          ],
        },
      }),
    );
  });
  vi.stubGlobal("fetch", fetchMock);
  render(
    <MemoryRouter>
      <BuiltImages hostId={source.id} imageName="reviewer" />
    </MemoryRouter>,
    on(await resolveDaemon(source.id)),
  );
  expect(await screen.findByRole("link", { name: "1.2.3+build.7" })).toHaveAttribute(
    "href",
    "/library/images/reviewer/1.2.3%2Bbuild.7",
  );
  expect(screen.getByText("full-digest")).toBeInTheDocument();
  expect(screen.getByText("Current: worker")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Remove reviewer:1.2.3+build.7" })).toBeDisabled();
  expect(screen.getByText("Latest", { exact: true })).toBeInTheDocument();
  expect(screen.getByText("Newest", { exact: true })).toBeInTheDocument();
  expect(screen.queryByLabelText("Import image archive")).toBeNull();
  expect(screen.queryByText("other")).toBeNull();
});

it("shows an empty tag list without import controls", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ ok: true, result: { images: [] } }))),
  );
  render(
    <MemoryRouter>
      <BuiltImages hostId="" imageName="missing" />
    </MemoryRouter>,
    on(null),
  );
  expect(await screen.findByText("No tags for this image on this host.")).toBeInTheDocument();
  expect(screen.queryByLabelText("Import image archive")).toBeNull();
});

it("reports an unavailable explicit host without reading the active host", async () => {
  setActiveDaemon({ id: "other", label: "Other", baseURL: "https://other", token: "" });
  const fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  render(
    <MemoryRouter>
      <BuiltImages hostId="missing-host" imageName="reviewer" />
    </MemoryRouter>,
    on(unresolvedDaemon("missing-host")),
  );
  expect(await screen.findByText("host missing-host is not ready")).toBeInTheDocument();
  expect(fetchMock).not.toHaveBeenCalled();
});
