import { afterEach, describe, expect, it } from "vitest";
import { act, render, screen } from "@testing-library/react";
import { ThemeProvider } from "./theme-provider";
import { useTheme } from "./themeContext";

function Probe() {
  const { theme, setTheme } = useTheme();
  return (
    <button type="button" onClick={() => setTheme("dark")}>
      {theme}
    </button>
  );
}

describe("ThemeProvider", () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage")!;
  afterEach(() => {
    Object.defineProperty(globalThis, "localStorage", original);
    localStorage.clear();
  });

  it("restores and saves the chosen theme", () => {
    localStorage.setItem("tariboy-theme", "light");
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    act(() => screen.getByRole("button", { name: "light" }).click());
    expect(screen.getByRole("button", { name: "dark" })).toBeInTheDocument();
    expect(localStorage.getItem("tariboy-theme")).toBe("dark");
  });

  it("uses the default theme and still switches when storage is blocked", () => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      get() {
        throw new Error("blocked");
      },
    });
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    act(() => screen.getByRole("button", { name: "system" }).click());
    expect(screen.getByRole("button", { name: "dark" })).toBeInTheDocument();
  });
});
