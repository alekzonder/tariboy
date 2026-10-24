import { describe, expect, it } from "vitest";
import { appChromeState, withView } from "./appChrome";

describe("appChromeState", () => {
  it.each([
    ["/", null, { sidebarRoute: true, viewSwitch: true, allTasks: false }],
    ["/", "all", { sidebarRoute: true, viewSwitch: true, allTasks: true }],
    ["/agents/local/a/console", "x", { sidebarRoute: true, viewSwitch: true, allTasks: false }],
    ["/servers/local/tasks", "all", { sidebarRoute: true, viewSwitch: false, allTasks: true }],
    ["/app-settings", null, { sidebarRoute: false, viewSwitch: false, allTasks: false }],
    ["/agents", null, { sidebarRoute: false, viewSwitch: false, allTasks: false }],
  ])("reads %s with view %s", (pathname, view, expected) => {
    expect(appChromeState({ pathname, view })).toEqual(expected);
  });
});

describe("withView", () => {
  it("sets view=all and keeps other params", () => {
    expect(withView(new URLSearchParams("a=1"), "all").toString()).toBe("a=1&view=all");
  });

  it("drops view for agents", () => {
    expect(withView(new URLSearchParams("view=all&a=1"), "agents").toString()).toBe("a=1");
  });

  it("does not mutate the input", () => {
    const current = new URLSearchParams("view=all");
    withView(current, "agents");
    expect(current.get("view")).toBe("all");
  });
});
