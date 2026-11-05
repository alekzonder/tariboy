import { Link } from "react-router-dom";
import type { MetricsSlice } from "@/types";
import { SliceTable, SortBySelect } from "@/components/metrics/SliceTable";
import { SLICE_COLUMNS, TASKS_DONE } from "@/components/metrics/sliceColumns";
import { useSort, type Column } from "@/components/metrics/sliceSort";

const COLUMNS = [TASKS_DONE, ...SLICE_COLUMNS];

/** The Agents view: one row per agent. A name opens the agent's Metrics view;
 *  an agent that is gone from the host keeps its row without a link. */
export function AgentsTable({
  slices,
  agents,
  agentHref,
}: {
  slices: MetricsSlice[];
  /** The agents on the host now. */
  agents: string[];
  agentHref: (agent: string) => string;
}) {
  const { sort, setSort, toggle } = useSort({ id: "cost_period", dir: "desc" });
  const head: Column<MetricsSlice> = {
    id: "agent",
    label: "Agent",
    value: (s) => s.name ?? "",
    cell: (s) => {
      const name = s.name ?? "";
      return agents.includes(name) ? (
        <Link to={agentHref(name)} className="text-primary hover:underline">
          {name}
        </Link>
      ) : (
        <span title="This agent is not on the host any more." className="text-muted-foreground">
          {name}
        </span>
      );
    },
  };
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        A task with several agents counts for each of them. Total counts it once.
      </p>
      <SortBySelect columns={[head, ...COLUMNS]} sort={sort} onChange={setSort} />
      <SliceTable
        label="Agents"
        rows={slices}
        rowKey={(s) => s.name ?? ""}
        head={head}
        columns={COLUMNS}
        sort={sort}
        onSort={toggle}
        empty="No agents in this period."
      />
    </div>
  );
}
