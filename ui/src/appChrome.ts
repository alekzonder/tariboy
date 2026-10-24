type AppView = "agents" | "all";

interface AppChromeState {
  // The agent sidebar and the console workspace switcher belong here.
  sidebarRoute: boolean;
  // Only where an agent is the subject: server pages are neither view.
  viewSwitch: boolean;
  allTasks: boolean;
}

export function appChromeState({
  pathname,
  view,
}: {
  pathname: string;
  view: string | null;
}): AppChromeState {
  const viewSwitch = pathname === "/" || pathname.startsWith("/agents/");
  return {
    sidebarRoute: viewSwitch || pathname.startsWith("/servers/"),
    viewSwitch,
    allTasks: view === "all",
  };
}

// The view lives in the query, not the path: switching keeps the selected
// agent's route, survives a reload and is one Back away.
export function withView(current: URLSearchParams, value: AppView): URLSearchParams {
  const next = new URLSearchParams(current);
  if (value === "all") next.set("view", "all");
  else next.delete("view");
  return next;
}
