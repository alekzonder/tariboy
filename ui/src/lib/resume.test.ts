import { afterEach, expect, it, vi } from "vitest";
import { subscribeResume } from "./resume";

function setVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value: state });
}

afterEach(() => setVisibility("visible"));

it("calls the listener when the page becomes visible or the browser comes online", () => {
  const listener = vi.fn();
  const stop = subscribeResume(listener);

  setVisibility("hidden");
  document.dispatchEvent(new Event("visibilitychange"));
  expect(listener).not.toHaveBeenCalled();

  setVisibility("visible");
  document.dispatchEvent(new Event("visibilitychange"));
  expect(listener).toHaveBeenCalledTimes(1);

  window.dispatchEvent(new Event("online"));
  expect(listener).toHaveBeenCalledTimes(2);

  stop();
  document.dispatchEvent(new Event("visibilitychange"));
  window.dispatchEvent(new Event("online"));
  expect(listener).toHaveBeenCalledTimes(2);
});
