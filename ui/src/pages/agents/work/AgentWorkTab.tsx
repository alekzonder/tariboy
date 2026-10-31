import { lazy, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { LazyBoundary } from "@/components/LazyBoundary";
import { Button } from "@/components/ui/button";
import { SearchInput } from "@/components/ui/search-input";
import type { AgentRow } from "@/lib/api";
import type { AgentNeeds } from "@/lib/agentState";
import { errorText } from "@/lib/errors";
import TaskDrawer from "@/pages/tasks/TaskDrawer";
import { TaskComposer } from "./TaskComposer";
import { useAgentWork } from "./useAgentWork";
import { workSections } from "./workModel";
import { WorkRow, WorkSection } from "./WorkSection";

// The card brings the Markdown renderer and the workflow graph.
const NeedsYouCard = lazy(() => import("@/pages/tasks/NeedsYouCard"));

/**
 * The agent's Work tab: the composer, then what waits for the user as cards,
 * then In progress, Queued and Done. `?answer=KEY` focuses that card,
 * `?give=1` the composer, and `?task=KEY` opens the task in the drawer.
 */
export default function AgentWorkTab({
  agent,
  base,
  needs,
  refresh,
  onNotificationsChanged,
}: {
  agent: AgentRow;
  base: string;
  needs: AgentNeeds;
  refresh: () => void;
  onNotificationsChanged?: () => void;
}) {
  const [params] = useSearchParams();
  const [openKey, setOpenKey] = useState(params.get("task") ?? "");
  const [showAll, setShowAll] = useState(false);
  const [search, setSearch] = useState("");
  const work = useAgentWork(agent.name, showAll);
  const items = needs.items ?? [];
  const sections = workSections({
    active: work.active,
    closed: work.closed,
    needs: new Set(items.map((item) => item.key)),
    now: work.now,
    showAll,
    search,
  });
  const changed = () => {
    work.reload();
    onNotificationsChanged?.();
  };
  const answer = params.get("answer") ?? "";
  const rowProps = (key: string) => ({
    cost: work.costs.get(key),
    onOpen: () => setOpenKey(key),
    onChanged: changed,
  });

  return (
    <div className="flex min-w-0 flex-col gap-4 px-4 py-3">
      <TaskComposer
        agent={agent}
        base={base}
        focus={params.get("give") === "1"}
        refresh={refresh}
        onOpenTask={setOpenKey}
      />
      {work.error && (
        <p role="alert" className="text-ui text-destructive">
          {errorText(work.error)}
        </p>
      )}
      {items.length > 0 && (
        <WorkSection title="Needs you" count={items.length}>
          <LazyBoundary resetKey="needs-you">
            <div className="flex min-w-0 flex-col gap-2">
              {items.map((item) => (
                <NeedsYouCard
                  key={item.key}
                  taskKey={item.key}
                  focus={item.key === answer}
                  onOpen={() => setOpenKey(item.key)}
                  onChanged={changed}
                />
              ))}
            </div>
          </LazyBoundary>
        </WorkSection>
      )}
      <WorkSection title="In progress" count={sections.inProgress.length}>
        <ul className="flex min-w-0 flex-col">
          {sections.inProgress.map((task) => (
            <WorkRow key={task.key} task={task} at={task.updated_at} {...rowProps(task.key)} />
          ))}
        </ul>
      </WorkSection>
      <WorkSection title="Queued" count={sections.queued.length}>
        <ul className="flex min-w-0 flex-col">
          {sections.queued.map((task) => (
            <WorkRow key={task.key} task={task} at={task.created_at} {...rowProps(task.key)} />
          ))}
        </ul>
      </WorkSection>
      <WorkSection
        title={showAll ? "Done" : "Done · last 7 days"}
        count={sections.done.length}
        aside={
          <>
            <SearchInput
              aria-label="Search done tasks"
              placeholder="Search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              className="h-6 w-40 max-w-full"
            />
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-ui"
              aria-pressed={showAll}
              onClick={() => setShowAll((all) => !all)}
            >
              {showAll ? "Last 7 days" : "Show all"}
            </Button>
          </>
        }
      >
        <ul className="flex min-w-0 flex-col">
          {sections.done.map((task) => (
            <WorkRow
              key={task.key}
              task={task}
              at={task.completed_at || task.updated_at}
              {...rowProps(task.key)}
            />
          ))}
        </ul>
      </WorkSection>
      {openKey && (
        <TaskDrawer
          key={openKey}
          taskKey={openKey}
          onClose={() => {
            setOpenKey("");
            changed();
          }}
        />
      )}
    </div>
  );
}
