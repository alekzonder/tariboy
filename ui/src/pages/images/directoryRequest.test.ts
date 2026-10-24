import { describe, expect, it } from "vitest";
import { directoryRequest } from "./directoryRequest";

describe("directoryRequest", () => {
  it("trims the path and name and omits the default tag", () => {
    expect(directoryRequest(" /src/demo ", " demo ", "latest")).toEqual({
      path: "/src/demo",
      name: "demo",
    });
    expect(directoryRequest("/src/demo", "demo", " latest ")).toEqual({
      path: "/src/demo",
      name: "demo",
    });
  });

  it("omits an empty tag", () => {
    expect(directoryRequest("/src/demo", "demo", "  ")).toEqual({
      path: "/src/demo",
      name: "demo",
    });
  });

  it("sends any other tag trimmed", () => {
    expect(directoryRequest("/src/demo", "demo", " v2 ")).toEqual({
      path: "/src/demo",
      name: "demo",
      tag: "v2",
    });
  });
});
