import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { SidebarStateProvider } from "./SidebarStateProvider";
import { useSharedSidebarState } from "./sidebarStateContext";

function Toggle() {
  const sidebar = useSharedSidebarState();
  return (
    <button type="button" onClick={() => sidebar.setHidden(!sidebar.hidden)}>
      {sidebar.hidden ? "show" : "hide"}
    </button>
  );
}

function Observer() {
  const sidebar = useSharedSidebarState();
  return <output>{`${sidebar.width}:${sidebar.hidden}`}</output>;
}

beforeEach(() => localStorage.clear());

describe("SidebarStateProvider", () => {
  it("shares one persisted sidebar state between independent consumers", () => {
    render(
      <MemoryRouter><SidebarStateProvider>
        <Toggle />
        <Observer />
      </SidebarStateProvider></MemoryRouter>,
    );

    expect(screen.getByText("268:false")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "hide" }));

    expect(screen.getByText("268:true")).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!))
      .toMatchObject({ width: 268, hidden: true });
  });
});
