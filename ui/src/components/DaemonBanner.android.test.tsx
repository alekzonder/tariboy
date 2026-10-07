import { describe, it, expect, vi, afterEach } from "vitest";
import { render } from "@testing-library/react";
import { DaemonBanner } from "./DaemonBanner";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("DaemonBanner in the Android app", () => {
  it("never polls or reports a same-origin daemon, which the app does not have", () => {
    vi.stubEnv("VITE_TARIBOY_SHELL", "android");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { container } = render(<DaemonBanner />);
    expect(container).toBeEmptyDOMElement();
    expect(fetch).not.toHaveBeenCalled();
  });
});
