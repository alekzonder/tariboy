import type { AgentPool, WorkflowImageDefinition } from "@/types";
import { cn } from "@/lib/utils";
import { DANGER_FILL, MONO, PRIMARY_FILL } from "@/components/ui/recipes";

const CELL = "px-3 py-2 align-top";

/** The statuses of a workflow, who owns each one, and where its outcomes lead. */
export default function WorkflowStatusMap({
  definition,
  pools,
  onPool,
}: {
  definition: WorkflowImageDefinition;
  pools: AgentPool[];
  /** Opens the named pool for editing. */
  onPool: (name: string) => void;
}) {
  return (
    <div className="overflow-x-auto rounded-card border">
      <table aria-label="Statuses" className="w-full text-body">
        <thead className="bg-subtle text-left text-caption text-muted-foreground">
          <tr>
            {["Status", "Owner", "Outcomes"].map((label) => (
              <th key={label} className={cn(CELL, "font-medium")}>
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {definition.statuses.map((status) => {
            const pool = status.owner.kind === "pool" ? (status.owner.pool ?? "") : "";
            const agents = pools.find((item) => item.name === pool)?.agents.length ?? 0;
            return (
              <tr key={status.id} className="border-t">
                <td className={CELL}>
                  <span className={MONO}>{status.id}</span>
                  {[
                    status.id === definition.initial_status && "initial",
                    status.terminal && "terminal",
                  ]
                    .filter(Boolean)
                    .map((label) => (
                      <span
                        key={String(label)}
                        className="ml-1.5 rounded-chip bg-muted px-1 text-caption text-muted-foreground"
                      >
                        {label}
                      </span>
                    ))}
                </td>
                <td className={CELL}>
                  {pool ? (
                    <button
                      type="button"
                      data-empty={agents === 0}
                      aria-label={`Pool ${pool}, ${agents === 0 ? "no agents" : agents === 1 ? "1 agent" : `${agents} agents`}`}
                      className={cn(
                        "rounded-chip px-2 py-0.5 hover:underline",
                        MONO,
                        agents === 0 ? DANGER_FILL : PRIMARY_FILL,
                      )}
                      onClick={() => onPool(pool)}
                    >
                      {pool} · {agents === 0 ? "no agents" : agents}
                    </button>
                  ) : (
                    <span className="text-muted-foreground">
                      {status.terminal ? "—" : status.owner.kind}
                    </span>
                  )}
                </td>
                <td className={cn(CELL, MONO, "text-muted-foreground")}>
                  {(status.transitions ?? [])
                    .map((transition) => `${transition.on} → ${transition.to}`)
                    .join(" · ") || "—"}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
