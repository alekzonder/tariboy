import { describe, expect, it, vi } from "vitest";
import { notifyImagesChanged, subscribeImagesChanged } from "./imageEvents";

describe("image change notifications", () => {
  it("calls each subscriber until it unsubscribes", () => {
    const first = vi.fn();
    const second = vi.fn();
    const stopFirst = subscribeImagesChanged(first);
    const stopSecond = subscribeImagesChanged(second);

    notifyImagesChanged();
    stopFirst();
    notifyImagesChanged();
    stopSecond();
    notifyImagesChanged();

    expect(first).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledTimes(2);
  });

  it("keeps notifying the others when one subscriber throws", () => {
    const report = vi.spyOn(console, "error").mockImplementation(() => {});
    const after = vi.fn();
    const stopBroken = subscribeImagesChanged(() => {
      throw new Error("broken");
    });
    const stopAfter = subscribeImagesChanged(after);

    notifyImagesChanged();

    expect(after).toHaveBeenCalledTimes(1);
    stopBroken();
    stopAfter();
    report.mockRestore();
  });

  it("does not use window events", () => {
    const dispatch = vi.spyOn(window, "dispatchEvent");
    notifyImagesChanged();
    expect(dispatch).not.toHaveBeenCalled();
    dispatch.mockRestore();
  });
});
