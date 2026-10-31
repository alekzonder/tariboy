import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter, useLocation } from "react-router-dom";
import type { HostAgents } from "@/lib/aggregate";
import { HostScopeSelect } from "./HostScopeSelect";
import { LibraryHeader } from "./LibraryHeader";

const hosts: HostAgents[] = [
  { host: { id: "", label: "Local" }, agents: [] },
  { host: { id: "remote", label: "Remote server with a long name" }, agents: [] },
];

function Location() {
  const location = useLocation();
  return <output aria-label="Location">{location.pathname + location.search}</output>;
}

afterEach(cleanup);

describe("Library host selection", () => {
  it.each(["images", "workflows", "sources"])(
    "puts the host before the %s navigation in keyboard order",
    (tab) => {
      render(
        <MemoryRouter initialEntries={[`/library/${tab}`]}>
          <LibraryHeader hostId="" hostControl={<HostScopeSelect hosts={hosts} hostId="" />} />
        </MemoryRouter>,
      );
      const host = screen.getByRole("combobox", { name: "Host" });
      const images = screen.getByRole("link", { name: "Images" });
      expect(host.compareDocumentPosition(images) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(screen.getByRole("link", { current: "page" })).toHaveAttribute(
        "href",
        `/library/${tab}`,
      );
    },
  );

  it.each(["/tasks", "/library/images", "/library/workflows", "/library/sources"])(
    "keeps the section and its query when switching hosts on %s",
    async (path) => {
      render(
        <MemoryRouter initialEntries={[`${path}?host=local&view=selected`]}>
          <HostScopeSelect hosts={hosts} hostId="" />
          <Location />
        </MemoryRouter>,
      );
      const user = userEvent.setup();
      await user.click(screen.getByRole("combobox", { name: "Host" }));
      await user.click(screen.getByRole("option", { name: "Remote server with a long name" }));
      expect(screen.getByLabelText("Location")).toHaveTextContent(
        `${path}?host=remote&view=selected`,
      );
    },
  );

  it("hides the host control with only local", () => {
    render(
      <MemoryRouter initialEntries={["/library/images"]}>
        <LibraryHeader
          hostId=""
          hostControl={<HostScopeSelect hosts={hosts.slice(0, 1)} hostId="" />}
        />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("combobox", { name: "Host" })).not.toBeInTheDocument();
  });
});
