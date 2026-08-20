import { Link } from "react-router-dom";
import { useState } from "react";
import { usePolling } from "@/hooks/usePolling";
import { listAgents, getDaemonStatus } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export default function Home() {
  const { data: agentsData } = usePolling(listAgents, 2000);
  const { data: status } = usePolling(getDaemonStatus, 5000);
  const [q, setQ] = useState("");
  const agents = (agentsData?.agents ?? []).filter((a) => a.name.toLowerCase().includes(q.toLowerCase()));

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Agents</h1>
        <div className="flex items-center gap-3">
          {status && <span className="text-xs text-muted-foreground">v{status.version} · pid {status.pid}</span>}
          <Button asChild size="sm">
            <Link to="/agents/new">New agent</Link>
          </Button>
        </div>
      </div>
      <Input placeholder="Search agents…" value={q} onChange={(e) => setQ(e.target.value)} className="max-w-xs" />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {agents.map((a) => (
          <Link key={a.name} to={`/agent/${encodeURIComponent(a.name)}`}>
            <Card className="transition-colors hover:border-primary">
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-base">{a.name}</CardTitle>
                <Badge variant={a.state === "running" ? "default" : "secondary"}>{a.state}</Badge>
              </CardHeader>
              <CardContent className="text-sm text-muted-foreground">
                <div className="truncate">{a.image}</div>
                <div className="mt-1 flex gap-2 text-xs">
                  <span>{a.harness}</span>
                  {a.loop_enabled && <span>loop</span>}
                  {a.group && <span>· {a.group}</span>}
                </div>
              </CardContent>
            </Card>
          </Link>
        ))}
        {agents.length === 0 && <p className="text-sm text-muted-foreground">No agents.</p>}
      </div>
    </div>
  );
}
