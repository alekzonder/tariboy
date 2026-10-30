import { useContext, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useLocation, useNavigate, type NavigateFunction } from "react-router-dom";
import { useOptionalDaemons } from "@/components/daemonContext";
import { sidebarRoute } from "@/appChrome";
import { navPath } from "@/lib/navRoutes";
import { paramToHost } from "@/lib/terminalsHost";
import { firstAgentRow, type Rect, type TourStep } from "@/lib/onboarding/tour";
import { SidebarStateContext, type SidebarStateValue } from "@/pages/terminals/sidebarStateContext";

const POLL_MS = 100;
const WORKSPACE = /^\/(?:servers|agents)\/([^/]+)/;

interface StepContext {
  navigate: NavigateFunction;
  pathname: string;
  activeId: string;
  sidebar: SidebarStateValue | null;
  onSkip: () => void;
}

export interface TourView {
  /** The highlighted element; null while it is not (or not yet) on screen. */
  rect: Rect | null;
  width: number;
  height: number;
}

/** The server the tour shows: the one on screen, else the selected one. */
function hostOf(pathname: string, activeId: string): string {
  const match = WORKSPACE.exec(pathname);
  return match ? paramToHost(decodeURIComponent(match[1])) : activeId;
}

/** Opens the first agent of the sidebar; false when there is none. */
function openFirstAgent(pathname: string): boolean {
  if (pathname.startsWith("/agents/") && !pathname.includes("/teams/")) return true;
  const row = firstAgentRow();
  row?.click();
  return Boolean(row);
}

/** Puts the screen in the state the step talks about. */
function prepare(step: TourStep, ctx: StepContext) {
  if (step.home && ctx.pathname !== "/") ctx.navigate("/");
  const onWorkspace = sidebarRoute(ctx.pathname);
  // The sidebar exists only on the workspace screens.
  const item = step.route ?? (step.sidebar && !onWorkspace ? "tasks" : undefined);
  if (item) {
    const path = navPath(item, hostOf(ctx.pathname, ctx.activeId));
    if (ctx.pathname !== path && !ctx.pathname.startsWith(`${path}/`)) ctx.navigate(path);
  }
  const sidebar = ctx.sidebar;
  if (!sidebar) return;
  // The phone drawer is modal: it would take focus and clicks from the card, so
  // there the sidebar steps stand in the center instead.
  if (sidebar.compact) {
    if (sidebar.drawerOpen) sidebar.setDrawerOpen(false);
  } else if (step.sidebar && sidebar.hidden) {
    sidebar.setHidden(false);
  }
}

function sameView(a: TourView, b: TourView): boolean {
  const ra = a.rect;
  const rb = b.rect;
  const sameRect =
    ra === rb ||
    (ra !== null &&
      rb !== null &&
      ra.top === rb.top &&
      ra.left === rb.left &&
      ra.width === rb.width &&
      ra.height === rb.height);
  return sameRect && a.width === b.width && a.height === b.height;
}

/**
 * Prepares the step (server section, sidebar, first agent) and follows its
 * anchor. Polling, rather than observers, also follows an element that a route
 * change re-creates or that a lazy screen renders late; until the element is
 * there the card stands in the center.
 */
export function useTourTarget(step: TourStep, onSkip: () => void): TourView {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const activeId = useOptionalDaemons()?.activeId ?? "";
  const sidebar = useContext(SidebarStateContext);
  const ctx = useRef<StepContext>({ navigate, pathname, activeId, sidebar, onSkip });
  useLayoutEffect(() => {
    ctx.current = { navigate, pathname, activeId, sidebar, onSkip };
  });
  const [view, setView] = useState<TourView>({
    rect: null,
    width: window.innerWidth,
    height: window.innerHeight,
  });

  useEffect(() => {
    if (step.agent && !openFirstAgent(ctx.current.pathname)) {
      ctx.current.onSkip();
      return;
    }
    prepare(step, ctx.current);
    let scrolled = false;
    const poll = () => {
      const element = step.anchor ? document.querySelector(`[data-tour="${step.anchor}"]`) : null;
      if (element && !scrolled) {
        scrolled = true;
        element.scrollIntoView({ block: "nearest", inline: "nearest" });
      }
      const box = element?.getBoundingClientRect();
      const next: TourView = {
        // A hidden element has no box: treat it as missing.
        rect:
          box && box.width + box.height > 0
            ? {
                top: box.top,
                left: box.left,
                right: box.right,
                bottom: box.bottom,
                width: box.width,
                height: box.height,
              }
            : null,
        width: window.innerWidth,
        height: window.innerHeight,
      };
      setView((current) => (sameView(current, next) ? current : next));
    };
    poll();
    const timer = window.setInterval(poll, POLL_MS);
    return () => window.clearInterval(timer);
  }, [step]);

  return view;
}
