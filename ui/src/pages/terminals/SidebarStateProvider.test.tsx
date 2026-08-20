import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
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
      <SidebarStateProvider>
        <Toggle />
        <Observer />
      </SidebarStateProvider>,
    );

    expect(screen.getByText("256:false")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "hide" }));

    expect(screen.getByText("256:true")).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("terminals:workspace:v1")!))
      .toMatchObject({ sidebar: { width: 256, hidden: true } });
  });
});
