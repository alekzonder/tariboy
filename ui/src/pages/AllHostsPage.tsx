import { usePolling } from "@/hooks/usePolling";
import { fetchAllAgents } from "@/lib/aggregate";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

// AllHostsPage shows agents across every registered daemon at once. Each host is
// a section; a dead/unauthorized host shows an error line without breaking the
// page (per-host degradation).
export default function AllHostsPage() {
  const { data } = usePolling(fetchAllAgents, 3000);
  const hosts = data ?? [];
  return (
    <div className="space-y-6 p-4">
      <h1 className="text-lg font-semibold">All hosts</h1>
      {hosts.map((h) => (
        <section key={h.host.id || "__local__"} className="space-y-2">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold">{h.host.label}</h2>
            {h.error && <span className="text-xs text-destructive">{h.error}</span>}
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {h.agents.map((a) => (
              <Card key={`${h.host.id}/${a.name}`}>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-base">{a.name}</CardTitle>
                  <Badge variant={a.state === "running" ? "default" : "secondary"}>{a.state}</Badge>
                </CardHeader>
                <CardContent className="text-sm text-muted-foreground">
                  <div className="truncate">{a.image}</div>
                  <div className="mt-1 flex gap-2 text-xs">
                    <span>{a.harness}</span>
                    {a.loop_enabled && <span>loop</span>}
                  </div>
                </CardContent>
              </Card>
            ))}
            {!h.error && h.agents.length === 0 && (
              <p className="text-sm text-muted-foreground">No agents.</p>
            )}
          </div>
        </section>
      ))}
    </div>
  );
}
