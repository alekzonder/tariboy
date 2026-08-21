import { StrictMode, useRef } from "react";
import { createRoot } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import type { HostAgents } from "@/lib/aggregate";
import {
  TerminalWorkspace,
  type TerminalWorkspaceHandle,
} from "@/pages/terminals/TerminalWorkspace";
import { TerminalsSidebar } from "@/pages/terminals/TerminalsSidebar";
import "@/index.css";

const hosts: HostAgents[] = [{
  host: { id: "", label: "Local" },
  agents: [
    {
      name: "alpha",
      image: "bare:latest",
      state: "running",
      harness: "codex",
      loop_enabled: false,
      group: null,
      interactive: true,
    },
    {
      name: "beta",
      image: "bare:latest",
      state: "running",
      harness: "claude",
      loop_enabled: false,
      group: null,
      interactive: true,
    },
    {
      name: "gamma",
      image: "bare:latest",
      state: "running",
      harness: "opencode",
      loop_enabled: false,
      group: null,
      interactive: true,
    },
  ],
}];

const noQuestionAttention: ReadonlySet<string> = new Set();

export function Fixture() {
  const workspaceRef = useRef<TerminalWorkspaceHandle | null>(null);

  return (
    <MemoryRouter>
      <main className="flex h-screen bg-background text-foreground">
        <TerminalsSidebar
          hosts={hosts}
          onSelect={() => {}}
          workspaceMode
          onBeginWorkspaceDrag={(identity, event) => {
            workspaceRef.current?.beginExternalPointerDrag(identity, event.nativeEvent);
          }}
          onClone={() => {}}
          onCreate={() => {}}
          onAddServer={() => {}}
          onEditServer={() => {}}
          onRemoveServer={() => {}}
          daemonViews={[]}
          appVersion="test"
          onConnectHost={() => {}}
          attention={noQuestionAttention}
          width={240}
          onResize={() => {}}
        />
        <div className="min-h-0 min-w-0 flex-1 p-3">
          <TerminalWorkspace
            ref={workspaceRef}
            hosts={hosts}
            refresh={() => {}}
            onOpenConfiguration={() => {}}
          />
        </div>
      </main>
    </MemoryRouter>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode><Fixture /></StrictMode>,
);
