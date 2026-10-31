import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import AgentPrompt from "@/pages/AgentPrompt";
import AgentContext from "@/pages/AgentContext";
import AgentFiles from "@/pages/AgentFiles";
import AgentScripts from "@/pages/AgentScripts";
import AuditLogPage from "@/pages/AuditLogPage";
import AgentUsagePage from "@/pages/AgentUsagePage";
import { Button } from "@/components/ui/button";
import { useApi } from "@/components/apiContext";
import { useAgentName } from "@/lib/agent";
import TabPage from "@/components/TabPage";

const VIEWS = [
  ["prompt", "Prompt"],
  ["context", "Context"],
  ["files", "Files"],
  ["scripts", "Scripts"],
  ["audit", "Full audit"],
  ["usage", "Raw usage"],
  ["raw", "Raw settings"],
] as const;
type View = (typeof VIEWS)[number][0];

function isView(value: string | null): value is View {
  return VIEWS.some(([key]) => key === value);
}

function RawSnapshot() {
  const name = useAgentName();
  const api = useApi();
  const [snapshot, setSnapshot] = useState<unknown>(null);
  useEffect(() => {
    void Promise.all([api.agents.getAgent(name), api.agents.getAgentStatus(name)])
      .then(([configuration, status]) => setSnapshot({ configuration, status }))
      .catch((error) => setSnapshot({ error: String(error) }));
  }, [api, name]);
  return (
    <pre className="overflow-auto rounded-md border bg-muted/30 p-4 text-xs">
      {JSON.stringify(snapshot, null, 2)}
    </pre>
  );
}

export default function AgentAdvancedTab({ base }: { base: string }) {
  const [params, setParams] = useSearchParams();
  const requested = params.get("view");
  const view: View = isView(requested) ? requested : "prompt";
  const content =
    view === "prompt" ? (
      <AgentPrompt />
    ) : view === "context" ? (
      <AgentContext />
    ) : view === "files" ? (
      <AgentFiles />
    ) : view === "scripts" ? (
      <AgentScripts />
    ) : view === "audit" ? (
      <AuditLogPage />
    ) : view === "usage" ? (
      <AgentUsagePage />
    ) : (
      <RawSnapshot />
    );
  return (
    <TabPage>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">Diagnostics</h2>
        <Button asChild size="sm" variant="outline">
          <Link to={`${base}/work`}>Back to Work</Link>
        </Button>
      </div>
      <div className="flex flex-wrap gap-2">
        {VIEWS.map(([key, label]) => (
          <Button
            key={key}
            size="sm"
            variant={view === key ? "default" : "outline"}
            onClick={() => setParams({ view: key })}
          >
            {label}
          </Button>
        ))}
        <Button asChild size="sm" variant="outline">
          <Link to={`${base}/mailbox`}>Mailbox</Link>
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">{content}</div>
    </TabPage>
  );
}
