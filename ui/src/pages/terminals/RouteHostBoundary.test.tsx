import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { DaemonProvider, useDaemons } from "@/components/DaemonProvider";
import { addDaemon, updateDaemon } from "@/lib/daemons";
import { RouteHostBoundary } from "./RouteHostBoundary";

function ActiveHostProbe() {
  const { activeId } = useDaemons();
  return <output data-testid="active-host">{activeId || "local"}</output>;
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("RouteHostBoundary", () => {
  it("selects the explicit route host before mounting its children", async () => {
    const remote = await addDaemon({
      label: "Production",
      baseURL: "http://127.0.0.1:19992",
      token: "test-token",
    });

    render(
      <DaemonProvider>
        <RouteHostBoundary hostId={remote.id}>
          <ActiveHostProbe />
        </RouteHostBoundary>
      </DaemonProvider>,
    );

    expect(await screen.findByTestId("active-host")).toHaveTextContent(remote.id);
  });

  it("fails closed when the explicit route host is unavailable", async () => {
    render(
      <DaemonProvider>
        <RouteHostBoundary hostId="missing-host">
          <ActiveHostProbe />
        </RouteHostBoundary>
      </DaemonProvider>,
    );

    expect(await screen.findByText("Host unavailable.")).toBeInTheDocument();
    expect(screen.queryByTestId("active-host")).toBeNull();
  });

  it("keeps a known route mounted but read-only while its endpoint is unavailable", async () => {
    const remote = await addDaemon({
      label: "Production",
      baseURL: "http://127.0.0.1:19992",
      token: "test-token",
    });
    await updateDaemon(remote.id, { label: remote.label, baseURL: "" });

    render(
      <DaemonProvider>
        <RouteHostBoundary hostId={remote.id}>
          <div data-testid="host-content">cached server content</div>
        </RouteHostBoundary>
      </DaemonProvider>,
    );

    expect(await screen.findByRole("status")).toHaveTextContent(/actions are temporarily unavailable/i);
    expect(screen.queryByText(/Host registry:/)).toBeNull();
    expect(screen.getByTestId("host-content").parentElement).toHaveAttribute("inert", "");
  });
});
