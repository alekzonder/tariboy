import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Actions,
  DockLocation,
  Layout,
  TabNode,
  TabSetNode,
  type Action,
  type ITabRenderValues,
} from "flexlayout-react";
import { X } from "lucide-react";
import type { HostAgents } from "@/lib/aggregate";
import type { AgentSummary } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { WorkspaceTerminalTile } from "./WorkspaceTerminalTile";
import {
  addTerminalToModel,
  createWorkspaceModel,
  findTerminalNode,
  moveTerminalInModel,
  removeTerminalFromModel,
  replaceTerminalInModel,
  terminalIdentities,
  workspaceJson,
} from "./workspaceModel";
import {
  readWorkspaceState,
  terminalKey,
  updateWorkspaceState,
  type TerminalIdentity,
} from "./workspaceState";
import {
  crossedDragThreshold,
  dropTargetForPoint,
  type WorkspaceDragSource,
  type WorkspaceDropTarget,
} from "./workspacePointerDrag";
import "./terminalWorkspace.css";

const PERSIST_DELAY_MS = 200;

interface IndexedAgent {
  hostLabel: string;
  agent: AgentSummary;
}

export interface TerminalWorkspaceHandle {
  addOrFocus: (identity: TerminalIdentity) => boolean;
  beginExternalPointerDrag: (identity: TerminalIdentity, event: PointerEvent) => void;
}

export interface TerminalWorkspaceProps {
  hosts: HostAgents[];
  refresh: () => void;
  onOpenConfiguration: (identity: TerminalIdentity) => void;
}

function identityFromTab(node: TabNode): TerminalIdentity | null {
  const config = node.getConfig() as Partial<TerminalIdentity> | undefined;
  if (
    !config
    || typeof config.hostId !== "string"
    || typeof config.agentName !== "string"
    || config.agentName.trim() === ""
  ) return null;
  return { hostId: config.hostId, agentName: config.agentName };
}

interface DragSession {
  source: WorkspaceDragSource;
  pointerId: number;
  captureTarget: Element | null;
  start: { x: number; y: number };
  active: boolean;
  target: WorkspaceDropTarget | null;
}

interface DragOverlay {
  source: WorkspaceDragSource;
  pointer: { x: number; y: number };
  target: WorkspaceDropTarget | null;
}

export const TerminalWorkspace = forwardRef<TerminalWorkspaceHandle, TerminalWorkspaceProps>(
  function TerminalWorkspace({ hosts, refresh, onOpenConfiguration }, ref) {
    const initial = useRef(readWorkspaceState()).current;
    // Replaced, not mutated, when a rejected move has to be rolled back: a
    // flexlayout Model has no in-place restore, so the rollback is a new model.
    const [model, setModel] = useState(() => createWorkspaceModel(initial.layout));
    const [, setRevision] = useState(0);
    const [activeTerminal, setActiveTerminal] = useState<string | null>(initial.activeTerminal);
    const activeRef = useRef(activeTerminal);
    const persistTimer = useRef<number | null>(null);
    const dirty = useRef(false);
    const [replaceNodeId, setReplaceNodeId] = useState<string | null>(null);
    const [notice, setNotice] = useState("");
    const workspaceRootRef = useRef<HTMLDivElement | null>(null);
    const dragSession = useRef<DragSession | null>(null);
    const [dragOverlay, setDragOverlay] = useState<DragOverlay | null>(null);

    const index = useMemo(() => {
      const next = new Map<string, IndexedAgent>();
      for (const host of hosts) {
        for (const agent of host.agents) {
          next.set(terminalKey({ hostId: host.host.id, agentName: agent.name }), {
            hostLabel: host.host.label,
            agent,
          });
        }
      }
      return next;
    }, [hosts]);

    const eligible = useMemo(() => {
      const result: Array<TerminalIdentity & { hostLabel: string }> = [];
      for (const host of hosts) {
        for (const agent of host.agents) {
          if (agent.interactive === false) continue;
          result.push({
            hostId: host.host.id,
            agentName: agent.name,
            hostLabel: host.host.label,
          });
        }
      }
      return result;
    }, [hosts]);

    const selectIdentity = useCallback((identity: TerminalIdentity | null) => {
      const key = identity ? terminalKey(identity) : null;
      activeRef.current = key;
      setActiveTerminal(key);
    }, []);

    const persistNow = useCallback(() => {
      if (!dirty.current) return;
      dirty.current = false;
      updateWorkspaceState((current) => ({
        ...current,
        layout: workspaceJson(model),
        activeTerminal: activeRef.current,
      }));
    }, [model]);

    const schedulePersistence = useCallback(() => {
      dirty.current = true;
      if (persistTimer.current !== null) window.clearTimeout(persistTimer.current);
      persistTimer.current = window.setTimeout(() => {
        persistTimer.current = null;
        persistNow();
      }, PERSIST_DELAY_MS);
    }, [persistNow]);

    useEffect(() => {
      const listener = (action: Action) => {
        if (action.type === Actions.SELECT_TAB) {
          const node = model.getNodeById(action.data.tabNode);
          if (node instanceof TabNode) selectIdentity(identityFromTab(node));
        }
        setRevision((value) => value + 1);
        schedulePersistence();
      };
      model.addChangeListener(listener);
      return () => {
        model.removeChangeListener(listener);
        if (persistTimer.current !== null) {
          window.clearTimeout(persistTimer.current);
          persistTimer.current = null;
        }
        persistNow();
      };
    }, [model, persistNow, schedulePersistence, selectIdentity]);

    useEffect(() => {
      model.setOnAllowDrop((_dragNode, dropInfo) => {
        if (dropInfo.location !== DockLocation.CENTER) return true;
        return dropInfo.node instanceof TabSetNode && dropInfo.node.getChildren().length === 0;
      });
    }, [model]);

    const addOrFocus = useCallback((identity: TerminalIdentity): boolean => {
      setNotice("");
      const current = index.get(terminalKey(identity));
      if (current?.agent.interactive === false) {
        setNotice(`${identity.agentName} does not have an interactive terminal.`);
        return false;
      }
      const activeNode = activeRef.current
        ? findTerminalNode(
            model,
            terminalIdentities(model).find(
              (candidate) => terminalKey(candidate) === activeRef.current,
            ) ?? identity,
          )
        : undefined;
      const result = addTerminalToModel(model, identity, {
        relativeTo: activeNode?.getId(),
        dock: "right",
      });
      selectIdentity(identity);
      if (!result.added) model.doAction(Actions.selectTab(result.nodeId));
      return true;
    }, [index, model, selectIdentity]);

    const closeNode = useCallback((nodeId: string) => {
      const closing = model.getNodeById(nodeId);
      const closingIdentity = closing instanceof TabNode ? identityFromTab(closing) : null;
      if (!removeTerminalFromModel(model, nodeId)) return;
      if (closingIdentity && terminalKey(closingIdentity) === activeRef.current) {
        selectIdentity(terminalIdentities(model)[0] ?? null);
      }
    }, [model, selectIdentity]);

    const replaceNode = useCallback((identity: TerminalIdentity) => {
      if (!replaceNodeId) return;
      if (replaceTerminalInModel(model, replaceNodeId, identity)) {
        selectIdentity(identity);
        setReplaceNodeId(null);
      }
    }, [model, replaceNodeId, selectIdentity]);

    const factory = useCallback((node: TabNode) => {
      const identity = identityFromTab(node);
      if (!identity) return <div className="p-4 text-sm text-destructive">Invalid terminal identity.</div>;
      const resolved = index.get(terminalKey(identity));
      return (
        <div
          className="h-full min-h-0"
          data-workspace-node-id={node.getId()}
        >
          <WorkspaceTerminalTile
            identity={identity}
            hostLabel={resolved?.hostLabel}
            agent={resolved?.agent}
            selected={activeTerminal === terminalKey(identity)}
            onFocus={() => {
              selectIdentity(identity);
              model.doAction(Actions.selectTab(node.getId()));
            }}
            onRetry={refresh}
            onReplace={() => setReplaceNodeId(node.getId())}
            onClose={() => closeNode(node.getId())}
            onOpenConfiguration={() => onOpenConfiguration(identity)}
            showHeader={false}
          />
        </div>
      );
    }, [
      activeTerminal,
      closeNode,
      index,
      model,
      onOpenConfiguration,
      refresh,
      selectIdentity,
    ]);

    const beginPointerDrag = useCallback((
      source: WorkspaceDragSource,
      event: PointerEvent,
    ) => {
      if (event.button !== 0) return;
      const captureTarget = event.target instanceof Element ? event.target : null;
      if (typeof captureTarget?.setPointerCapture === "function") {
        try {
          captureTarget.setPointerCapture(event.pointerId);
        } catch {
          // Global cancellation listeners remain the fallback when a WebView
          // rejects capture for a pointer that is no longer active.
        }
      }
      dragSession.current = {
        source,
        pointerId: event.pointerId,
        captureTarget,
        start: { x: event.clientX, y: event.clientY },
        active: false,
        target: null,
      };
    }, []);

    const beginExternalPointerDrag = useCallback((
      identity: TerminalIdentity,
      event: PointerEvent,
    ) => {
      const resolved = index.get(terminalKey(identity));
      if (!resolved || resolved.agent.interactive === false) return;
      beginPointerDrag({ kind: "external", identity }, event);
    }, [beginPointerDrag, index]);

    useImperativeHandle(
      ref,
      () => ({ addOrFocus, beginExternalPointerDrag }),
      [addOrFocus, beginExternalPointerDrag],
    );

    const resolveDropTarget = useCallback((
      clientX: number,
      clientY: number,
    ): WorkspaceDropTarget | null => {
      const root = workspaceRootRef.current;
      if (!root) return null;
      const rootRect = root.getBoundingClientRect();
      const pane = document
        .elementsFromPoint(clientX, clientY)
        .map((element) => {
          const direct = element.closest<HTMLElement>("[data-workspace-node-id]");
          if (direct) return direct;
          return element
            .closest<HTMLElement>(".flexlayout__tabset")
            ?.querySelector<HTMLElement>("[data-workspace-node-id]")
            ?? null;
        })
        .find((element): element is HTMLElement => Boolean(element));
      const nodeId = pane?.dataset.workspaceNodeId;
      if (pane && nodeId) {
        const rectOwner =
          pane.closest<HTMLElement>(".flexlayout__tabset")
          ?? pane.closest<HTMLElement>(".flexlayout__tab")
          ?? pane;
        const rect = rectOwner.getBoundingClientRect();
        return dropTargetForPoint(nodeId, {
          left: rect.left,
          top: rect.top,
          width: rect.width,
          height: rect.height,
        }, clientX, clientY);
      }

      if (
        clientX < rootRect.left
        || clientX > rootRect.right
        || clientY < rootRect.top
        || clientY > rootRect.bottom
      ) return null;

      if (terminalIdentities(model).length === 0) {
        return {
          nodeId: null,
          dock: "right",
          preview: {
            left: rootRect.left,
            top: rootRect.top,
            width: rootRect.width,
            height: rootRect.height,
          },
        };
      }
      return null;
    }, [model]);

    const clearPointerDrag = useCallback(() => {
      const session = dragSession.current;
      dragSession.current = null;
      if (
        session?.captureTarget
        && typeof session.captureTarget.hasPointerCapture === "function"
        && session.captureTarget.hasPointerCapture(session.pointerId)
      ) {
        try {
          session.captureTarget.releasePointerCapture(session.pointerId);
        } catch {
          // The browser can release capture before our pointerup cleanup.
        }
      }
      setDragOverlay(null);
      document.body.style.removeProperty("cursor");
      document.body.style.removeProperty("user-select");
    }, []);

    useEffect(() => {
      const onPointerMove = (event: PointerEvent) => {
        const session = dragSession.current;
        if (!session || event.pointerId !== session.pointerId) return;
        if (!session.active && !crossedDragThreshold(session.start, {
          x: event.clientX,
          y: event.clientY,
        })) return;
        event.preventDefault();
        session.active = true;
        session.target = resolveDropTarget(event.clientX, event.clientY);
        document.body.style.cursor = "grabbing";
        document.body.style.userSelect = "none";
        setDragOverlay({
          source: session.source,
          pointer: { x: event.clientX, y: event.clientY },
          target: session.target,
        });
      };

      const onPointerUp = (event: PointerEvent) => {
        const session = dragSession.current;
        if (!session || event.pointerId !== session.pointerId) return;
        const { source, target, active } = session;
        clearPointerDrag();
        if (!active || !target) return;

        if (source.kind === "external") {
          const result = addTerminalToModel(model, source.identity, target.nodeId
            ? { relativeTo: target.nodeId, dock: target.dock }
            : undefined);
          selectIdentity(source.identity);
          if (!result.added) model.doAction(Actions.selectTab(result.nodeId));
          return;
        }

        if (!target.nodeId) return;
        const move = moveTerminalInModel(model, source.nodeId, target.nodeId, target.dock);
        if (move.outcome === "moved") {
          selectIdentity(source.identity);
          model.doAction(Actions.selectTab(source.nodeId));
          return;
        }
        // "unchanged" means nothing happened, so there is nothing to say or undo.
        if (move.outcome !== "restored") return;
        // The rejected move mutated the model we are rendering, and the change
        // listener it woke has already queued that broken layout for persistence.
        // Drop the pending write before adopting the restored model, otherwise
        // the layout we just undid is what survives the next reload.
        dirty.current = false;
        if (persistTimer.current !== null) {
          window.clearTimeout(persistTimer.current);
          persistTimer.current = null;
        }
        setModel(move.model);
        // Dropping the pending write also drops whatever legitimate change was
        // still waiting behind it (a splitter resize moments earlier, say), so
        // write the restored layout out explicitly. It carries that change: the
        // rollback was built from the json captured before the rejected move.
        // Not via dirty.current = true — that would let the cleanup above
        // persist the broken model through its stale persistNow.
        updateWorkspaceState((current) => ({
          ...current,
          layout: workspaceJson(move.model),
          activeTerminal: activeRef.current,
        }));
        setNotice("That split would have broken the layout, so it was restored.");
      };

      const onPointerCancel = (event: PointerEvent) => {
        if (dragSession.current?.pointerId === event.pointerId) clearPointerDrag();
      };
      const onLostPointerCapture = (event: PointerEvent) => {
        if (dragSession.current?.pointerId === event.pointerId) clearPointerDrag();
      };
      const onKeyDown = (event: KeyboardEvent) => {
        if (event.key === "Escape" && dragSession.current) clearPointerDrag();
      };
      const onWindowBlur = () => {
        if (dragSession.current) clearPointerDrag();
      };
      const onVisibilityChange = () => {
        if (document.visibilityState === "hidden" && dragSession.current) {
          clearPointerDrag();
        }
      };

      window.addEventListener("pointermove", onPointerMove);
      window.addEventListener("pointerup", onPointerUp);
      window.addEventListener("pointercancel", onPointerCancel);
      window.addEventListener("lostpointercapture", onLostPointerCapture);
      window.addEventListener("keydown", onKeyDown);
      window.addEventListener("blur", onWindowBlur);
      document.addEventListener("visibilitychange", onVisibilityChange);
      return () => {
        window.removeEventListener("pointermove", onPointerMove);
        window.removeEventListener("pointerup", onPointerUp);
        window.removeEventListener("pointercancel", onPointerCancel);
        window.removeEventListener("lostpointercapture", onLostPointerCapture);
        window.removeEventListener("keydown", onKeyDown);
        window.removeEventListener("blur", onWindowBlur);
        document.removeEventListener("visibilitychange", onVisibilityChange);
        clearPointerDrag();
      };
    }, [clearPointerDrag, model, resolveDropTarget, selectIdentity]);

    const renderTab = useCallback((node: TabNode, values: ITabRenderValues) => {
      const identity = identityFromTab(node);
      if (!identity) return;
      const resolved = index.get(terminalKey(identity));
      const alive = resolved?.agent
        ? (resolved.agent.enabled ?? resolved.agent.state !== "stopped")
        : false;
      const state = !resolved
        ? "unavailable"
        : resolved.agent.interactive === false
          ? "non-interactive"
          : alive
            ? resolved.agent.state
            : "stopped";
      values.content = (
        <span
          className="flex min-w-0 flex-1 cursor-grab items-center gap-2 text-xs active:cursor-grabbing"
          data-workspace-node-id={node.getId()}
          data-testid={`workspace-drag-${identity.agentName}`}
          onPointerDownCapture={(event) => {
            beginPointerDrag(
              { kind: "internal", identity, nodeId: node.getId() },
              event.nativeEvent,
            );
          }}
        >
          <span className="truncate font-medium">{identity.agentName}</span>
          <span className="truncate text-muted-foreground">
            {resolved?.hostLabel || identity.hostId || "Local"}
          </span>
          <span className="shrink-0 text-muted-foreground">{state}</span>
        </span>
      );
      values.buttons.push(
        <button
          key="close"
          type="button"
          className="rounded p-0.5 hover:bg-accent"
          aria-label={`Close ${identity.agentName} terminal`}
          title="Detach terminal from workspace"
          onPointerDown={(event) => event.stopPropagation()}
          onClick={(event) => {
            event.stopPropagation();
            closeNode(node.getId());
          }}
        >
          <X className="size-3.5" />
        </button>,
      );
    }, [beginPointerDrag, closeNode, index]);

    const onAction = useCallback((action: Action): Action | undefined => {
      if (action.type === Actions.MOVE_NODE && action.data.location === "center") return undefined;
      if (action.type === Actions.ADD_TAB && action.data.location === "center") {
        if (terminalIdentities(model).length > 0) return undefined;
      }
      return action;
    }, [model]);

    const presentKeys = new Set(terminalIdentities(model).map(terminalKey));
    const replacementChoices = eligible.filter((identity) => !presentKeys.has(terminalKey(identity)));
    const empty = presentKeys.size === 0;

    return (
      <div
        ref={workspaceRootRef}
        data-testid="terminal-workspace"
        className="terminal-workspace relative h-full min-h-0 overflow-hidden"
      >
        <Layout
          model={model}
          factory={factory}
          onAction={onAction}
          onRenderTab={renderTab}
          realtimeResize
          supportsPopout={false}
          keyMap={{ closeTab: undefined, renameTab: undefined }}
        />
        {dragOverlay?.target && (
          <div
            data-testid="workspace-drop-preview"
            data-dock={dragOverlay.target.dock}
            className="pointer-events-none fixed z-[100] border-2 border-primary bg-primary/20"
            style={dragOverlay.target.preview}
          />
        )}
        {dragOverlay && (
          <div
            data-testid="workspace-drag-ghost"
            className="pointer-events-none fixed z-[101] max-w-64 truncate rounded border bg-background px-2 py-1 text-xs shadow-lg"
            style={{
              left: dragOverlay.pointer.x + 12,
              top: dragOverlay.pointer.y + 12,
            }}
          >
            {dragOverlay.source.identity.agentName}
            <span className="ml-2 text-muted-foreground">
              {index.get(terminalKey(dragOverlay.source.identity))?.hostLabel
                || dragOverlay.source.identity.hostId
                || "Local"}
            </span>
          </div>
        )}
        {empty && (
          <div className="pointer-events-none absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 text-center text-muted-foreground">
            <p className="text-base font-medium">Drag an interactive agent here</p>
            <p className="max-w-md text-sm">
              Drop it on an edge to split the workspace, or use Add to Workspace from the agent list.
            </p>
          </div>
        )}
        {notice && (
          <div role="status" className="absolute bottom-3 left-1/2 z-20 -translate-x-1/2 rounded-md border bg-background px-3 py-2 text-sm shadow">
            {notice}
          </div>
        )}
        <Dialog open={replaceNodeId !== null} onOpenChange={(open) => { if (!open) setReplaceNodeId(null); }}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Replace terminal</DialogTitle>
              <DialogDescription>
                Choose an interactive agent. The split position and size stay unchanged.
              </DialogDescription>
            </DialogHeader>
            <div className="max-h-80 space-y-2 overflow-auto">
              {replacementChoices.map((identity) => (
                <Button
                  key={terminalKey(identity)}
                  variant="outline"
                  className="w-full justify-between"
                  aria-label={`Use ${identity.agentName} on ${identity.hostLabel}`}
                  onClick={() => replaceNode(identity)}
                >
                  <span>{identity.agentName}</span>
                  <span className="text-muted-foreground">{identity.hostLabel}</span>
                </Button>
              ))}
              {replacementChoices.length === 0 && (
                <p className="text-sm text-muted-foreground">
                  No other interactive agents are available.
                </p>
              )}
            </div>
          </DialogContent>
        </Dialog>
      </div>
    );
  },
);
