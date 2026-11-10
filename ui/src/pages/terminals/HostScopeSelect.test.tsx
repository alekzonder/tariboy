import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter, useLocation } from "react-router-dom";
import type { HostAgents } from "@/lib/aggregate";
import { HostScopeSelect } from "./HostScopeSelect";

const hosts: HostAgents[] = [
  { host: { id: "", label: "Local" }, agents: [] },
  { host: { id: "remote", label: "Remote" }, agents: [] },
];

function Location() {
  const location = useLocation();
  return <output aria-label="Location">{location.pathname + location.search}</output>;
}

afterEach(cleanup);

describe("HostScopeSelect with All hosts", () => {
  it("shows All hosts while no host is named", () => {
    render(
      <MemoryRouter initialEntries={["/tasks"]}>
        <HostScopeSelect hosts={hosts} hostId={undefined} allHosts />
      </MemoryRouter>,
    );
    expect(screen.getByRole("combobox", { name: "Host" })).toHaveTextContent("All hosts");
  });

  it("drops the host and its task on a pick of All hosts", async () => {
    render(
      <MemoryRouter initialEntries={["/tasks?host=remote&task=OPS-1"]}>
        <HostScopeSelect hosts={hosts} hostId="remote" allHosts />
        <Location />
      </MemoryRouter>,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: "Host" }));
    await user.click(screen.getByRole("option", { name: "All hosts" }));
    expect(screen.getByLabelText("Location")).toHaveTextContent(/^\/tasks$/);
  });

  it("names a host on a pick from All hosts", async () => {
    render(
      <MemoryRouter initialEntries={["/tasks"]}>
        <HostScopeSelect hosts={hosts} hostId={undefined} allHosts />
        <Location />
      </MemoryRouter>,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: "Host" }));
    await user.click(screen.getByRole("option", { name: "Remote" }));
    expect(screen.getByLabelText("Location")).toHaveTextContent("/tasks?host=remote");
  });

  it("offers no All hosts on other pages", async () => {
    render(
      <MemoryRouter initialEntries={["/inbox"]}>
        <HostScopeSelect hosts={hosts} hostId="" />
      </MemoryRouter>,
    );
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Host" }));
    expect(screen.queryByRole("option", { name: "All hosts" })).toBeNull();
  });
});
