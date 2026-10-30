import { describe, expect, it } from "vitest";
import { hostScopeTarget } from "./navRoutes";

describe("hostScopeTarget", () => {
  it.each([
    ["/tasks", "", "d1", "/tasks?host=d1"],
    ["/tasks", "?host=d1&task=DEV-1", "", "/tasks?host=local&task=DEV-1"],
    ["/inbox", "?letter=m-1", "d1", "/inbox?letter=m-1&host=d1"],
    ["/library/sources", "", "d1", "/library/sources?host=d1"],
    ["/library/images/x/1", "?host=d1", "", "/library/images/x/1?host=local"],
    [
      "/servers/d1/settings/advanced/usage",
      "?x=1",
      "",
      "/servers/local/settings/advanced/usage?x=1",
    ],
  ])("moves %s%s to host %s", (pathname, search, hostId, target) => {
    expect(hostScopeTarget(pathname, search, hostId)).toBe(target);
  });
});
