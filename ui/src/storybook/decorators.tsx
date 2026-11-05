/* oxlint-disable react/only-export-components -- decorators are functions, not components; Storybook has no fast refresh boundary here */
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { Decorator } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui/tooltip";
import { DaemonHost, QueryHost, STORYBOOK_DAEMON_URL as DAEMON_URL, ThemeHost } from "./hosts";

// Decorators for Storybook. Storybook never reaches a daemon (spec 9.1): a
// story that needs the host registry gets one host on a name that cannot
// resolve, and the real registry keys come back after the story.

export type TariboyParameters = {
  query?: boolean;
  daemon?: boolean;
  route?: { path: string; pattern: string };
};

export const STORYBOOK_DAEMON_URL = DAEMON_URL;

const params = (context: Parameters<Decorator>[1]) =>
  (context.parameters.tariboy ?? {}) as TariboyParameters;

export const withTheme: Decorator = (Story, context) => (
  <ThemeHost dark={context.globals.theme === "dark"}>
    <Story />
  </ThemeHost>
);

export const withAppProviders: Decorator = (Story, context) => {
  const inner = (
    <TooltipProvider>
      <Story />
    </TooltipProvider>
  );
  return params(context).route ? inner : <MemoryRouter>{inner}</MemoryRouter>;
};

export const withQueryClient: Decorator = (Story, context) =>
  params(context).query === true ? (
    <QueryHost>
      <Story />
    </QueryHost>
  ) : (
    <Story />
  );

export const withDaemon: Decorator = (Story, context) =>
  params(context).daemon === true ? (
    <DaemonHost>
      <Story />
    </DaemonHost>
  ) : (
    <Story />
  );

export const withRoute: Decorator = (Story, context) => {
  const route = params(context).route;
  if (!route) return <Story />;
  return (
    <MemoryRouter initialEntries={[route.path]}>
      <Routes>
        <Route path={route.pattern} element={<Story />} />
      </Routes>
    </MemoryRouter>
  );
};

const SURFACE_WIDTH = { sidebar: 288, panel: 480, card: 360, row: 720 } as const;

export const withSurface =
  (kind: keyof typeof SURFACE_WIDTH, width?: number): Decorator =>
  (Story) => (
    <div
      data-surface={kind}
      className="bg-background text-foreground"
      style={{ width: width ?? SURFACE_WIDTH[kind] }}
    >
      <Story />
    </div>
  );

export const withWidth =
  (width: number): Decorator =>
  (Story) => (
    <div className="bg-background text-foreground" style={{ width, minHeight: "100vh" }}>
      <Story />
    </div>
  );
