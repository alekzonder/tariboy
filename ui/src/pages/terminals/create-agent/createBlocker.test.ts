import { describe, expect, it } from "vitest";
import { createBlocker } from "./createBlocker";

const ready = {
  sourceState: "ready" as const,
  hostLabel: "prod",
  targetIsCurrent: true,
  imagesError: "",
  image: "basic:latest",
  imageAvailable: true,
  manifestError: "",
  manifestIsCurrent: true,
  imageWaiting: null,
  folderMissing: false,
  missingTool: null,
};

describe("createBlocker", () => {
  it("lets a complete form start", () => {
    expect(createBlocker(ready)).toBeNull();
  });

  it("waits for the starter image before it reports the image as not built", () => {
    expect(
      createBlocker({
        ...ready,
        imageAvailable: false,
        imageWaiting: "Waiting for the starter image…",
      }),
    ).toEqual({ message: "Waiting for the starter image…", error: false });
  });

  it("asks for a folder, then for the coding tool", () => {
    expect(createBlocker({ ...ready, folderMissing: true, missingTool: "Codex" })).toEqual({
      message: "Choose a folder",
      error: false,
    });
    expect(createBlocker({ ...ready, missingTool: "Codex" })).toEqual({
      message: "Codex is not installed on prod",
      error: true,
    });
  });
});
