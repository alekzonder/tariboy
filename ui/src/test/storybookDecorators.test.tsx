import type { ReactNode } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import { useParams } from "react-router-dom";
import { afterEach, expect, it } from "vitest";
import type { Decorator } from "@storybook/react-vite";
import { useDaemons } from "@/components/daemonContext";
import { DaemonProvider } from "@/components/DaemonProvider";
import {
  STORYBOOK_DAEMON_URL,
  withDaemon,
  withQueryClient,
  withRoute,
  withTheme,
  type TariboyParameters,
} from "@/storybook/decorators";

type Context = Parameters<Decorator>[1];

function decorate(
  decorator: Decorator,
  Story: () => ReactNode,
  tariboy: TariboyParameters = {},
  globals: Record<string, string> = {},
) {
  const context = { parameters: { tariboy }, globals } as unknown as Context;
  const Component = Story as unknown as Parameters<Decorator>[0];
  return decorator(Component, context);
}

afterEach(() => {
  cleanup();
  localStorage.clear();
  document.documentElement.classList.remove("dark");
});

it("gives each story render its own query client", () => {
  const seen: QueryClient[] = [];
  const Probe = () => {
    seen.push(useQueryClient());
    return null;
  };
  render(decorate(withQueryClient, Probe, { query: true }));
  render(decorate(withQueryClient, Probe, { query: true }));
  expect(seen).toHaveLength(2);
  expect(seen[0]).not.toBe(seen[1]);
});

it("mounts a matched route for a useParams story", () => {
  const Probe = () => <p>{useParams().name}</p>;
  render(
    decorate(withRoute, Probe, {
      route: { path: "/agents/builder", pattern: "/agents/:name" },
    }),
  );
  expect(screen.getByText("builder")).toBeInTheDocument();
});

it("puts the dark class on the document for the dark theme", () => {
  const Probe = () => null;
  render(decorate(withTheme, Probe, {}, { theme: "dark" }));
  expect(document.documentElement.classList.contains("dark")).toBe(true);
  cleanup();
  render(decorate(withTheme, Probe, {}, { theme: "light" }));
  expect(document.documentElement.classList.contains("dark")).toBe(false);
});

it("points the daemon registry at an .invalid host", async () => {
  const Probe = () => {
    const { daemons, activeId } = useDaemons();
    const active = daemons.find((host) => host.id === activeId);
    return <p>{active?.baseURL ?? "none"}</p>;
  };
  render(<DaemonProvider>{decorate(withDaemon, Probe, { daemon: true })}</DaemonProvider>);
  await waitFor(() => expect(screen.getByText(STORYBOOK_DAEMON_URL)).toBeInTheDocument());
  expect(new URL(STORYBOOK_DAEMON_URL).hostname.endsWith(".invalid")).toBe(true);
});

it("restores both registry keys after each story", () => {
  localStorage.setItem("tariboy_daemons", '[{"id":"mine"}]');
  localStorage.removeItem("tariboy_active_daemon");
  const Probe = () => null;
  const view = render(decorate(withDaemon, Probe, { daemon: true }));
  expect(localStorage.getItem("tariboy_active_daemon")).toBe("storybook");
  view.unmount();
  expect(localStorage.getItem("tariboy_daemons")).toBe('[{"id":"mine"}]');
  expect(localStorage.getItem("tariboy_active_daemon")).toBeNull();
});
