import { lazy } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { LazyBoundary } from "./LazyBoundary";

function deferredPage() {
  let resolve!: (page: { default: () => React.JSX.Element }) => void;
  let reject!: (error: Error) => void;
  const Page = lazy(
    () =>
      new Promise<{ default: () => React.JSX.Element }>((res, rej) => {
        resolve = res;
        reject = rej;
      }),
  );
  return {
    Page,
    resolve: (text: string) => resolve({ default: () => <h1>{text}</h1> }),
    reject: (e: Error) => reject(e),
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("LazyBoundary", () => {
  it("shows an accessible loading status until the module loads", async () => {
    const page = deferredPage();
    render(
      <>
        <header>Titlebar</header>
        <LazyBoundary resetKey="/a">
          <page.Page />
        </LazyBoundary>
      </>,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Loading…");
    expect(screen.getByText("Titlebar")).toBeInTheDocument();

    await act(async () => page.resolve("Usage"));
    expect(screen.getByRole("heading", { name: "Usage" })).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("reports a failed module load and reloads only on request", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const reload = vi.fn();
    const page = deferredPage();
    render(
      <LazyBoundary resetKey="/a" reload={reload}>
        <page.Page />
      </LazyBoundary>,
    );

    await act(async () => page.reject(new Error("Failed to fetch dynamically imported module")));

    expect(screen.getByRole("alert")).toHaveTextContent("This screen failed to load.");
    expect(screen.getByRole("alert")).toHaveTextContent("Reloading discards unsaved input.");
    expect(reload).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Reload" }));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("clears the error when the reset key changes", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const broken = deferredPage();
    const { rerender } = render(
      <LazyBoundary resetKey="/a">
        <broken.Page />
      </LazyBoundary>,
    );
    await act(async () => broken.reject(new Error("offline")));
    expect(screen.getByRole("alert")).toBeInTheDocument();

    rerender(
      <LazyBoundary resetKey="/b">
        <h1>Other screen</h1>
      </LazyBoundary>,
    );

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Other screen" })).toBeInTheDocument();
  });
});
